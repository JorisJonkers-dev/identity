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
)

const (
	signupLifetime  = 24 * time.Hour
	sessionLifetime = 30 * 24 * time.Hour
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
	TouchSession(ctx context.Context, tokenHash []byte, lastSeen, expires time.Time) error
	DeleteSession(ctx context.Context, tokenHash []byte) error
	DeleteOtherSessions(ctx context.Context, userID uuid.UUID, keepTokenHash []byte) error
	DeleteSessionsOfUser(ctx context.Context, userID uuid.UUID) error
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

	decoy atomic.Pointer[string]
}

// New returns a Service. publicURL is where links in mail point, with no trailing slash.
func New(store Store, hasher Hasher, mailer Mailer, now func() time.Time, random io.Reader, publicURL string) *Service {
	return &Service{store: store, hasher: hasher, mailer: mailer, now: now, random: random, publicURL: publicURL}
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
	err = s.store.CreateUser(ctx, domain.User{ID: uuid.New(), Email: signup.Email, Name: signup.Name, PasswordHash: signup.PasswordHash, CreatedAt: now})
	if errors.Is(err, ErrEmailTaken) {
		return errors.Join(ErrInvalidToken, s.store.DeleteSignup(ctx, hash))
	}
	if err != nil {
		return err
	}
	return s.store.DeleteSignup(ctx, hash)
}

// SignIn checks an email and a password and starts a Single sign-on session, returning its token.
// Every refusal costs one hash check, so timing does not tell an unknown email from a wrong password.
func (s *Service) SignIn(ctx context.Context, rawEmail, password string) (string, error) {
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return "", ErrInvalidCredentials
	}
	u, err := s.store.UserByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		if err := s.decoyCheck(password); err != nil {
			return "", err
		}
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", err
	}
	ok, err := s.hasher.Verify(u.PasswordHash, password)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrInvalidCredentials
	}

	token, hash, err := s.token()
	if err != nil {
		return "", err
	}
	id, err := uuid.NewRandomFromReader(s.random)
	if err != nil {
		return "", err
	}
	now := s.now()
	sess := Session{TokenHash: hash, ID: id, UserID: u.ID, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(sessionLifetime)}
	if err := s.store.CreateSession(ctx, sess); err != nil {
		return "", err
	}
	return token, nil
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
	hash := hashOf(token)
	sess, err := s.store.SessionByHash(ctx, hash)
	if errors.Is(err, ErrNotFound) {
		return domain.User{}, Session{}, ErrNoSession
	}
	if err != nil {
		return domain.User{}, Session{}, err
	}
	now := s.now()
	if !now.Before(sess.ExpiresAt) {
		return domain.User{}, Session{}, ErrNoSession
	}
	if now.Sub(sess.LastSeenAt) >= touchInterval {
		if err := s.store.TouchSession(ctx, hash, now, now.Add(sessionLifetime)); err != nil {
			return domain.User{}, Session{}, err
		}
	}
	u, err := s.store.UserByID(ctx, sess.UserID)
	return u, sess, err
}

// SignOutEverywhere ends every session of a User (docs/adr/0007).
func (s *Service) SignOutEverywhere(ctx context.Context, userID uuid.UUID) error {
	return s.store.DeleteSessionsOfUser(ctx, userID)
}

// SignOut ends the session a token belongs to; ending one that does not exist is not an error.
func (s *Service) SignOut(ctx context.Context, token string) error {
	return s.store.DeleteSession(ctx, hashOf(token))
}

// User returns a User by id.
func (s *Service) User(ctx context.Context, id uuid.UUID) (domain.User, error) {
	return s.store.UserByID(ctx, id)
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
	return s.store.UpdateName(ctx, id, name)
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
	return s.store.DeleteOtherSessions(ctx, id, hashOf(sessionToken))
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
