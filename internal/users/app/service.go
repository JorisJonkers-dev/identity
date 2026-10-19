// Package app is the users context's use cases: signing up, confirming an email, signing in and
// out, and changing one's own Profile. It reaches storage, hashing and mail only through ports.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// What a caller can be refused, beyond the domain's own rules.
var (
	ErrNotFound           = errors.New("not found")
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("the email or the password is wrong")
	ErrInvalidToken       = errors.New("this link has expired or was already used")
	ErrNoSession          = errors.New("not signed in")
	ErrBusy               = errors.New("too many password checks at once")
	// ErrSecondFactorNeeded means the password was right and a Second Factor is enrolled: SignIn's
	// token is then a sign-in challenge, which CompleteSignIn turns into a session.
	ErrSecondFactorNeeded = errors.New("the sign-in needs a Second Factor")
	ErrInvalidCode        = errors.New("the code is wrong or was already used")
	ErrUnavailable        = errors.New("second factors are not set up on this server")
	// ErrNoSecondFactor is a User asked to prove a Second Factor they never added.
	ErrNoSecondFactor = errors.New("the User has no Second Factor")
)

const (
	signupLifetime       = 24 * time.Hour
	sessionLifetime      = 30 * 24 * time.Hour
	adminSessionLifetime = 12 * time.Hour
	// A session's last use is written at most once a minute, not on every request (docs/adr/0009).
	touchInterval = time.Minute
	tokenBytes    = 32
)

// Signup is a sign-up waiting for its email to be confirmed, stored by its token's hash. It holds
// the credentials the User will have, so confirming creates exactly the account that was asked for
// and an unconfirmed account never exists to be taken over.
type Signup struct {
	TokenHash    []byte
	Email        domain.Email
	Name         domain.Name
	PasswordHash string
	ExpiresAt    time.Time
}

// Session is a Single sign-on session, stored by its token's hash.
type Session struct {
	TokenHash  []byte
	ID         uuid.UUID
	UserID     uuid.UUID
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	// Methods is how the session was proved, as RFC 8176 names them: "pwd", and "otp" once a
	// Second Factor was.
	Methods []string
	// TwoFactorAt is when the session last proved a Second Factor; nil when it never did.
	TwoFactorAt *time.Time
	// Device is the browser and operating system the session was started from.
	Device string
}

// Store is the users context's persistence.
type Store interface {
	CreateUser(ctx context.Context, u domain.User) error
	UserByEmail(ctx context.Context, e domain.Email) (domain.User, error)
	UserByID(ctx context.Context, id uuid.UUID) (domain.User, error)
	// Users is every User, by name.
	Users(ctx context.Context) ([]domain.User, error)
	UpdateName(ctx context.Context, id uuid.UUID, n domain.Name) error
	UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error
	CreateSignup(ctx context.Context, s Signup) error
	SignupByHash(ctx context.Context, tokenHash []byte) (Signup, error)
	DeleteSignup(ctx context.Context, tokenHash []byte) error
	CreateSession(ctx context.Context, s Session) error
	SessionByHash(ctx context.Context, tokenHash []byte) (Session, error)
	// CountActiveSessions is how many sessions are unexpired at now, and how many Users hold them.
	CountActiveSessions(ctx context.Context, now time.Time) (sessions, users int, err error)
	// SessionByID is ErrNotFound when there is none.
	SessionByID(ctx context.Context, id uuid.UUID) (Session, error)
	TouchSession(ctx context.Context, tokenHash []byte, lastSeen, expires time.Time) error
	// SessionsOf is a User's sessions live at now, the most recently seen first.
	SessionsOf(ctx context.Context, userID uuid.UUID, now time.Time) ([]Session, error)
	// SaveEmailChange keeps a User's one pending email change, replacing any before it.
	SaveEmailChange(ctx context.Context, c EmailChange) error
	// EmailChangeOf is ErrNotFound when the User has none pending.
	EmailChangeOf(ctx context.Context, userID uuid.UUID) (EmailChange, error)
	// TakeEmailChange removes and returns the change a token hash names; ErrNotFound when none.
	TakeEmailChange(ctx context.Context, tokenHash []byte) (EmailChange, error)
	DeleteEmailChange(ctx context.Context, userID uuid.UUID) error
	// UpdateEmail is ErrEmailTaken when another User has the address.
	UpdateEmail(ctx context.Context, userID uuid.UUID, email domain.Email) error
	// DeleteUser deletes a User with their sessions and Personal Tokens, returning their ids; it is
	// ErrLastAdmin for the last Admin, even when two deletions race.
	DeleteUser(ctx context.Context, userID uuid.UUID) (Deleted, error)
	// DeleteSession ends one of a User's sessions, and reports whether it was theirs.
	DeleteSession(ctx context.Context, userID, id uuid.UUID) (bool, error)
	// DeleteOtherSessions ends every session of a User but the one keepTokenHash names, or every
	// one when it is nil, and returns the ids of those it ended.
	DeleteOtherSessions(ctx context.Context, userID uuid.UUID, keepTokenHash []byte) ([]uuid.UUID, error)
	// SetSessionMethods records how a session was proved, and that it proved a Second Factor at.
	SetSessionMethods(ctx context.Context, tokenHash []byte, methods []string, at time.Time) error
	// SaveChallenge makes or replaces the User's one challenge. Its spent attempts carry over unless
	// the window that holds them began at or before windowSince.
	SaveChallenge(ctx context.Context, c Challenge, windowSince time.Time) error
	// SpendChallengeAttempt counts one code against an unexpired challenge with attempts left and
	// returns its User; ErrNotFound when there is none.
	SpendChallengeAttempt(ctx context.Context, tokenHash []byte, maxAttempts int, now time.Time) (uuid.UUID, error)
	DeleteChallenge(ctx context.Context, tokenHash []byte) error
	// StepUpChallenge is the token hash of the challenge that proves a session again; ErrNotFound
	// when it has none.
	StepUpChallenge(ctx context.Context, sessionHash []byte) ([]byte, error)
	TOTPFactors(ctx context.Context, userID uuid.UUID) ([]TOTPFactor, error)
	CreateTOTPFactor(ctx context.Context, f TOTPFactor) error
	// UseTOTPStep records a step newer than the factor's last, reporting whether it was newer.
	UseTOTPStep(ctx context.Context, id uuid.UUID, step int64, at time.Time) (bool, error)
	// DeleteTOTPFactor reports whether the User had the factor.
	DeleteTOTPFactor(ctx context.Context, id, userID uuid.UUID) (bool, error)
	SaveTOTPEnrolment(ctx context.Context, e TOTPEnrolment) error
	// TOTPEnrolment is ErrNotFound when the User has none.
	TOTPEnrolment(ctx context.Context, userID uuid.UUID) (TOTPEnrolment, error)
	DeleteTOTPEnrolment(ctx context.Context, userID uuid.UUID) error
	Passkeys(ctx context.Context, userID uuid.UUID) ([]Passkey, error)
	CreatePasskey(ctx context.Context, p Passkey) error
	// UsePasskey keeps the record an assertion moved on, by its credential id, only if the stored
	// record is still previous; it reports whether it was.
	UsePasskey(ctx context.Context, userID uuid.UUID, used NewPasskey, previous []byte, at time.Time) (bool, error)
	// DeletePasskey reports whether the User had the passkey.
	DeletePasskey(ctx context.Context, id, userID uuid.UUID) (bool, error)
	SavePasskeyEnrolment(ctx context.Context, e PasskeyEnrolment) error
	// PasskeyEnrolment is ErrNotFound when the User has none.
	PasskeyEnrolment(ctx context.Context, userID uuid.UUID) (PasskeyEnrolment, error)
	DeletePasskeyEnrolment(ctx context.Context, userID uuid.UUID) error
	// ChallengeCeremony is an unexpired challenge's User and passkey ceremony; ErrNotFound when
	// there is none.
	ChallengeCeremony(ctx context.Context, tokenHash []byte, now time.Time) (uuid.UUID, []byte, error)
	SetChallengeCeremony(ctx context.Context, tokenHash, ceremony []byte, now time.Time) error
	// TakeChallengeCeremony removes and returns an unexpired challenge's passkey ceremony, so only
	// one request can use it; ErrNotFound when there is none to take.
	TakeChallengeCeremony(ctx context.Context, tokenHash []byte, now time.Time) (uuid.UUID, []byte, error)
}

// Sealer keeps a secret as ciphertext only Vault can open (docs/adr/0013).
type Sealer interface {
	Seal(ctx context.Context, plaintext []byte) (string, error)
	Open(ctx context.Context, ciphertext string) ([]byte, error)
}

// Admins tells who is an Admin, whose session lasts 12 hours (docs/adr/0007).
type Admins interface {
	IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error)
}

// Hasher turns a password into a stored hash and checks a candidate against one.
type Hasher interface {
	Hash(p domain.Password) (string, error)
	Verify(hash, password string) (bool, error)
}

// Mail is one plain-text message to one address.
type Mail struct {
	To, Subject, Text string
}

// Mailer sends mail (docs/adr/0012).
type Mailer interface {
	Send(ctx context.Context, m Mail) error
}

// Service runs the use cases.
type Service struct {
	store     Store
	hasher    Hasher
	mailer    Mailer
	now       func() time.Time
	random    io.Reader
	publicURL string
	sealer    Sealer
	webauthn  WebAuthn
	admins    Admins
	signedOut SignedOut
	// tokensDeleted is told which Personal Tokens went with a deleted User.
	tokensDeleted func(ctx context.Context, tokenIDs []uuid.UUID)
	audit         Audit
	matchTOTP     TOTPMatch

	decoy atomic.Pointer[string]
}

// Option sets what a Service may run without.
type Option func(*Service)

// WithSealer lets Users enrol a TOTP Second Factor, its secret sealed by sealer.
func WithSealer(sealer Sealer) Option { return func(s *Service) { s.sealer = sealer } }

// SignedOut is told which sessions of a User ended, so what was signed in through them can end too:
// back-channel logout to native Clients, and the Gate's host sessions (docs/adr/0007).
type SignedOut func(ctx context.Context, userID uuid.UUID, sessionIDs []uuid.UUID)

// WithSignedOut tells f about every session that ends.
func WithSignedOut(f SignedOut) Option { return func(s *Service) { s.signedOut = f } }

// Audit records sign-ins, failed sign-ins, sign-outs and Profile changes as Audit Events.
type Audit interface {
	Record(ctx context.Context, e audit.Event)
}

// WithAudit records Audit Events through a.
func WithAudit(a Audit) Option { return func(s *Service) { s.audit = a } }

// record keeps e when the Service has an Audit.
func (s *Service) record(ctx context.Context, e audit.Event) {
	if s.audit != nil {
		s.audit.Record(ctx, e)
	}
}

// failed records a failed sign-in and returns err, the refusal.
func (s *Service) failed(ctx context.Context, e audit.Event, err error) error {
	e.Action = audit.SignInFailed
	s.record(ctx, e)
	return err
}

// TOTPMatch checks an authenticator app's code against its secret at now, accepting only a step
// after the one last used, and returns the step it matched.
type TOTPMatch func(secret, code string, now time.Time, after int64) (step int64, ok bool)

// WithTOTPMatch checks authenticator codes with m instead of RFC 6238 alone; only a local run's
// dev build passes one.
func WithTOTPMatch(m TOTPMatch) Option { return func(s *Service) { s.matchTOTP = m } }

// WithAdmins caps an Admin's session at 12 hours.
func WithAdmins(admins Admins) Option { return func(s *Service) { s.admins = admins } }

// New returns a Service. publicURL is where links in mail point, with no trailing slash.
func New(store Store, hasher Hasher, mailer Mailer, now func() time.Time, random io.Reader, publicURL string, opts ...Option) *Service {
	s := &Service{store: store, hasher: hasher, mailer: mailer, now: now, random: random, publicURL: publicURL, matchTOTP: domain.MatchTOTP}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SignUp waits for an email confirmation and mails the link. It answers the same for an email
// that is already registered, and tells that address so by mail instead. No mail carries text the
// signer-up chose: the address may not be theirs.
func (s *Service) SignUp(ctx context.Context, rawEmail, rawName, rawPassword string) error {
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return err
	}
	name, err := domain.NewName(rawName)
	if err != nil {
		return err
	}
	password, err := domain.NewPassword(rawPassword)
	if err != nil {
		return err
	}

	// Hash first, whatever the lookup finds: hashing is the slow step, and skipping it for a
	// registered email would let the response time say who has an account.
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return err
	}
	switch _, err := s.store.UserByEmail(ctx, email); {
	case err == nil:
		return s.mailer.Send(ctx, Mail{
			To:      email.String(),
			Subject: "You already have an account",
			Text: "Someone, perhaps you, tried to sign up with this email address, which already has an account.\n\n" +
				"Sign in at " + s.publicURL + "/signin\n\nIf it was not you, you can ignore this message.\n",
		})
	case !errors.Is(err, ErrNotFound):
		return err
	}

	token, tokenHash, err := s.token()
	if err != nil {
		return err
	}
	signup := Signup{TokenHash: tokenHash, Email: email, Name: name, PasswordHash: hash, ExpiresAt: s.now().Add(signupLifetime)}
	if err := s.store.CreateSignup(ctx, signup); err != nil {
		return err
	}
	return s.mailer.Send(ctx, Mail{
		To:      email.String(),
		Subject: "Confirm your email",
		Text: "Confirm this email address to finish signing up:\n\n" + s.publicURL + "/confirm?token=" + token +
			"\n\nThe link works once, for 24 hours. If you did not sign up, ignore this message and no account is made.\n",
	})
}

// Confirm creates the User a sign-up describes, given its mailed token and the password chosen at
// sign-up. Both are needed: the mailbox's owner never knows a stranger's password, and the stranger
// never sees the link, so signing up with someone else's address first takes nothing over. A wrong
// password leaves the link usable; an expired link, or one for an email already confirmed, is spent.
func (s *Service) Confirm(ctx context.Context, token, password string) error {
	hash := hashOf(token)
	signup, err := s.store.SignupByHash(ctx, hash)
	if errors.Is(err, ErrNotFound) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}
	now := s.now()
	if !now.Before(signup.ExpiresAt) {
		return errors.Join(ErrInvalidToken, s.store.DeleteSignup(ctx, hash))
	}
	ok, err := s.hasher.Verify(signup.PasswordHash, password)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCredentials
	}
	id := uuid.New()
	err = s.store.CreateUser(ctx, domain.User{ID: id, Email: signup.Email, Name: signup.Name, PasswordHash: signup.PasswordHash, CreatedAt: now})
	if errors.Is(err, ErrEmailTaken) {
		return errors.Join(ErrInvalidToken, s.store.DeleteSignup(ctx, hash))
	}
	if err != nil {
		return err
	}
	s.record(ctx, audit.Event{Action: audit.UserCreated, User: id})
	return s.store.DeleteSignup(ctx, hash)
}

// SignIn checks an email and a password and starts a Single sign-on session, returning its token.
// Every refusal costs one hash check, so timing does not tell an unknown email from a wrong password.
func (s *Service) SignIn(ctx context.Context, rawEmail, password string) (string, error) {
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return "", s.failed(ctx, audit.Event{Reason: audit.ReasonPassword}, ErrInvalidCredentials)
	}
	refused := audit.Event{Email: email.String(), Reason: audit.ReasonPassword}
	u, err := s.store.UserByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		if err := s.decoyCheck(password); err != nil {
			return "", err
		}
		return "", s.failed(ctx, refused, ErrInvalidCredentials)
	}
	if err != nil {
		return "", err
	}
	ok, err := s.hasher.Verify(u.PasswordHash, password)
	if err != nil {
		return "", err
	}
	if !ok {
		refused.User = u.ID
		return "", s.failed(ctx, refused, ErrInvalidCredentials)
	}
	has, err := s.hasSecondFactor(ctx, u.ID)
	if err != nil {
		return "", err
	}
	if has {
		challenge, err := s.challenge(ctx, u.ID, nil)
		if err != nil {
			return "", err
		}
		return challenge, ErrSecondFactorNeeded
	}
	return s.startSession(ctx, u.ID, []string{methodPassword})
}

// startSession makes a Single sign-on session proved by methods and returns its token. An Admin's
// session lasts 12 hours, everyone else's 30 days from its last use.
func (s *Service) startSession(ctx context.Context, userID uuid.UUID, methods []string) (string, error) {
	token, hash, err := s.token()
	if err != nil {
		return "", err
	}
	id, err := uuid.NewRandomFromReader(s.random)
	if err != nil {
		return "", err
	}
	now := s.now()
	end, err := s.end(ctx, userID, now)
	if err != nil {
		return "", err
	}
	sess := Session{
		TokenHash: hash, ID: id, UserID: userID, CreatedAt: now, LastSeenAt: now,
		ExpiresAt: earliest(now.Add(sessionLifetime), end), Methods: methods, Device: deviceOf(ctx),
	}
	if ProvedSecondFactor(methods) {
		sess.TwoFactorAt = &now
	}
	if err := s.store.CreateSession(ctx, sess); err != nil {
		return "", err
	}
	s.record(ctx, audit.Event{Action: audit.SignedIn, User: userID})
	return token, nil
}

// end is when a session made at created must end whatever its use: 12 hours on for an Admin, and
// never for anyone else.
func (s *Service) end(ctx context.Context, userID uuid.UUID, created time.Time) (time.Time, error) {
	if s.admins == nil {
		return time.Time{}, nil
	}
	admin, err := s.admins.IsAdmin(ctx, userID)
	if err != nil || !admin {
		return time.Time{}, err
	}
	return created.Add(adminSessionLifetime), nil
}

// earliest is the sooner of t and an end; a zero end is no end.
func earliest(t, end time.Time) time.Time {
	if !end.IsZero() && end.Before(t) {
		return end
	}
	return t
}

// decoyCheck verifies password against a hash no User has, to spend what a real check spends.
func (s *Service) decoyCheck(password string) error {
	decoy, err := s.decoyHash()
	if err != nil {
		return err
	}
	_, err = s.hasher.Verify(decoy, password)
	return err
}

// decoyHash keeps the first decoy made; a failure, such as ErrBusy, is not kept. No lock is held
// while hashing, so a sign-in waits only on the hasher's own bound, never on another sign-in.
func (s *Service) decoyHash() (string, error) {
	if d := s.decoy.Load(); d != nil {
		return *d, nil
	}
	p, _ := domain.NewPassword("a decoy no account has")
	h, err := s.hasher.Hash(p)
	if err != nil {
		return "", err
	}
	s.decoy.CompareAndSwap(nil, &h)
	return *s.decoy.Load(), nil
}

// Authenticate returns the User a session token belongs to, and extends the session from its last
// use, writing that at most once a minute.
func (s *Service) Authenticate(ctx context.Context, token string) (domain.User, error) {
	u, _, err := s.SignedIn(ctx, token)
	return u, err
}

// SignedIn is Authenticate that also returns the session, whose CreatedAt is when the password
// was checked.
func (s *Service) SignedIn(ctx context.Context, token string) (domain.User, Session, error) {
	return s.alive(ctx)(s.store.SessionByHash(ctx, hashOf(token)))
}

// Vouch is SignedIn by the session's public id, for the Gate, whose host cookie names the Single
// sign-on session it was made from (docs/adr/0003).
func (s *Service) Vouch(ctx context.Context, sessionID uuid.UUID) (domain.User, Session, error) {
	return s.alive(ctx)(s.store.SessionByID(ctx, sessionID))
}

// alive checks a session a lookup found: unexpired, an Admin's within 12 hours, and touched at most
// once a minute.
func (s *Service) alive(ctx context.Context) func(Session, error) (domain.User, Session, error) {
	return func(sess Session, err error) (domain.User, Session, error) {
		if errors.Is(err, ErrNotFound) {
			return domain.User{}, Session{}, ErrNoSession
		}
		if err != nil {
			return domain.User{}, Session{}, err
		}
		return s.check(ctx, sess)
	}
}

func (s *Service) check(ctx context.Context, sess Session) (domain.User, Session, error) {
	hash := sess.TokenHash
	now := s.now()
	end, err := s.end(ctx, sess.UserID, sess.CreatedAt)
	if err != nil {
		return domain.User{}, Session{}, err
	}
	expires := earliest(sess.ExpiresAt, end)
	if !now.Before(expires) {
		return domain.User{}, Session{}, ErrNoSession
	}
	if now.Sub(sess.LastSeenAt) >= touchInterval {
		if err := s.store.TouchSession(ctx, hash, now, earliest(now.Add(sessionLifetime), end)); err != nil {
			return domain.User{}, Session{}, err
		}
	}
	u, err := s.store.UserByID(ctx, sess.UserID)
	return u, sess, err
}

// SignOutEverywhere ends every session of a User (docs/adr/0007). The actor is who asked: an
// Admin, or uuid.Nil for the User themself.
func (s *Service) SignOutEverywhere(ctx context.Context, actor, userID uuid.UUID) error {
	return s.signOut(ctx, actor, userID)
}

// signOut ends every session of a User and records it, when there was any to end.
func (s *Service) signOut(ctx context.Context, actor, userID uuid.UUID) error {
	ended, err := s.endSessions(ctx, userID, nil)
	if ended > 0 {
		s.record(ctx, audit.Event{Action: audit.SignedOut, Actor: actor, User: userID})
	}
	return err
}

// SignOut signs the User a token belongs to out everywhere (docs/adr/0007); signing out a session
// that does not exist is not an error.
func (s *Service) SignOut(ctx context.Context, token string) error {
	sess, err := s.store.SessionByHash(ctx, hashOf(token))
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.signOut(ctx, uuid.Nil, sess.UserID)
}

// endSessions ends every session of a User but the one keep names, says which ended, and returns
// how many.
func (s *Service) endSessions(ctx context.Context, userID uuid.UUID, keep []byte) (int, error) {
	ended, err := s.store.DeleteOtherSessions(ctx, userID, keep)
	if err != nil {
		return 0, err
	}
	if len(ended) > 0 && s.signedOut != nil {
		s.signedOut(ctx, userID, ended)
	}
	return len(ended), nil
}

// ActiveSessions is how many Single sign-on sessions are live, and how many Users hold them.
func (s *Service) ActiveSessions(ctx context.Context) (sessions, users int, err error) {
	return s.store.CountActiveSessions(ctx, s.now())
}

// User returns a User by id.
func (s *Service) User(ctx context.Context, id uuid.UUID) (domain.User, error) {
	return s.store.UserByID(ctx, id)
}

// UserByEmail returns the User with an email, written in any case.
func (s *Service) UserByEmail(ctx context.Context, rawEmail string) (domain.User, error) {
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return domain.User{}, err
	}
	return s.store.UserByEmail(ctx, email)
}

// Users returns every User, by name.
func (s *Service) Users(ctx context.Context) ([]domain.User, error) {
	return s.store.Users(ctx)
}

// Rename changes a User's name.
func (s *Service) Rename(ctx context.Context, id uuid.UUID, rawName string) error {
	name, err := domain.NewName(rawName)
	if err != nil {
		return err
	}
	if err := s.store.UpdateName(ctx, id, name); err != nil {
		return err
	}
	s.record(ctx, audit.Event{Action: audit.UserRenamed, User: id})
	return nil
}

// ChangePassword replaces a User's password after checking the current one, and ends every other
// session of theirs: whoever knew the old password is signed out everywhere but here.
func (s *Service) ChangePassword(ctx context.Context, id uuid.UUID, sessionToken, current, next string) error {
	password, err := domain.NewPassword(next)
	if err != nil {
		return err
	}
	u, err := s.store.UserByID(ctx, id)
	if err != nil {
		return err
	}
	ok, err := s.hasher.Verify(u.PasswordHash, current)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCredentials
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return err
	}
	if err := s.store.UpdatePasswordHash(ctx, id, hash); err != nil {
		return err
	}
	s.record(ctx, audit.Event{Action: audit.PasswordChanged, User: id})
	_, err = s.endSessions(ctx, id, hashOf(sessionToken))
	return err
}

// token returns a fresh random token and the hash it is stored under.
func (s *Service) token() (string, []byte, error) {
	b := make([]byte, tokenBytes)
	if _, err := io.ReadFull(s.random, b); err != nil {
		return "", nil, fmt.Errorf("read a token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, hashOf(token), nil
}

func hashOf(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
