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
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// What a caller can be refused, beyond the domain's own rules.
var (
	ErrNotFound           = errors.New("not found")
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("the email or the password is wrong")
	ErrUnconfirmed        = errors.New("confirm your email first; a new link is on its way")
	ErrInvalidToken       = errors.New("this link has expired or was already used")
	ErrNoSession          = errors.New("not signed in")
)

const (
	confirmationLifetime = 24 * time.Hour
	sessionLifetime      = 30 * 24 * time.Hour
	// A session's last use is written at most once a minute, not on every request (docs/adr/0009).
	touchInterval = time.Minute
	tokenBytes    = 32
)

// Confirmation is an email confirmation waiting to be used, stored by its token's hash.
type Confirmation struct {
	TokenHash []byte
	UserID    uuid.UUID
	ExpiresAt time.Time
}

// Session is a Single sign-on session, stored by its token's hash.
type Session struct {
	TokenHash  []byte
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
	ConfirmUser(ctx context.Context, id uuid.UUID, at time.Time) error
	UpdateName(ctx context.Context, id uuid.UUID, n domain.Name) error
	UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error
	CreateConfirmation(ctx context.Context, c Confirmation) error
	TakeConfirmation(ctx context.Context, tokenHash []byte) (Confirmation, error)
	CreateSession(ctx context.Context, s Session) error
	SessionByHash(ctx context.Context, tokenHash []byte) (Session, error)
	TouchSession(ctx context.Context, tokenHash []byte, lastSeen, expires time.Time) error
	DeleteSession(ctx context.Context, tokenHash []byte) error
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

	decoyOnce sync.Once
	decoy     string
	decoyErr  error
}

// New returns a Service. publicURL is where links in mail point, with no trailing slash.
func New(store Store, hasher Hasher, mailer Mailer, now func() time.Time, random io.Reader, publicURL string) *Service {
	return &Service{store: store, hasher: hasher, mailer: mailer, now: now, random: random, publicURL: publicURL}
}

// SignUp registers an unconfirmed User and mails a confirmation link. It answers the same for an
// email that is already registered, and tells that address so by mail instead.
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

	existing, err := s.store.UserByEmail(ctx, email)
	switch {
	case err == nil:
		return s.answerExisting(ctx, existing)
	case !errors.Is(err, ErrNotFound):
		return err
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return err
	}
	u := domain.User{ID: uuid.New(), Email: email, Name: name, PasswordHash: hash, CreatedAt: s.now()}
	if err := s.store.CreateUser(ctx, u); err != nil {
		if !errors.Is(err, ErrEmailTaken) {
			return err
		}
		// Another sign-up for the same email committed first: answer as for an existing User.
		if existing, err = s.store.UserByEmail(ctx, email); err != nil {
			return err
		}
		return s.answerExisting(ctx, existing)
	}
	return s.mailConfirmation(ctx, u)
}

func (s *Service) answerExisting(ctx context.Context, u domain.User) error {
	if !u.Confirmed() {
		return s.mailConfirmation(ctx, u)
	}
	return s.mailer.Send(ctx, Mail{
		To:      u.Email.String(),
		Subject: "You already have an account",
		Text: "Someone, perhaps you, tried to sign up with this email address, which already has an account.\n\n" +
			"Sign in at " + s.publicURL + "/signin\n\nIf it was not you, you can ignore this message.\n",
	})
}

func (s *Service) mailConfirmation(ctx context.Context, u domain.User) error {
	token, hash, err := s.token()
	if err != nil {
		return err
	}
	if err := s.store.CreateConfirmation(ctx, Confirmation{TokenHash: hash, UserID: u.ID, ExpiresAt: s.now().Add(confirmationLifetime)}); err != nil {
		return err
	}
	return s.mailer.Send(ctx, Mail{
		To:      u.Email.String(),
		Subject: "Confirm your email",
		Text: "Hello " + u.Name.String() + ",\n\nConfirm your email address to finish signing up:\n\n" +
			s.publicURL + "/confirm?token=" + token + "\n\nThe link works once, for 24 hours.\n",
	})
}

// Confirm uses a confirmation token, once, before it expires.
func (s *Service) Confirm(ctx context.Context, token string) error {
	c, err := s.store.TakeConfirmation(ctx, hashOf(token))
	if errors.Is(err, ErrNotFound) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}
	now := s.now()
	if !now.Before(c.ExpiresAt) {
		return ErrInvalidToken
	}
	return s.store.ConfirmUser(ctx, c.UserID, now)
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
	if !u.Confirmed() {
		if err := s.mailConfirmation(ctx, u); err != nil {
			return "", err
		}
		return "", ErrUnconfirmed
	}

	token, hash, err := s.token()
	if err != nil {
		return "", err
	}
	now := s.now()
	if err := s.store.CreateSession(ctx, Session{TokenHash: hash, UserID: u.ID, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(sessionLifetime)}); err != nil {
		return "", err
	}
	return token, nil
}

// decoyCheck verifies password against a hash no User has, to spend what a real check spends.
func (s *Service) decoyCheck(password string) error {
	s.decoyOnce.Do(func() {
		p, _ := domain.NewPassword("a decoy no account has")
		s.decoy, s.decoyErr = s.hasher.Hash(p)
	})
	if s.decoyErr != nil {
		return s.decoyErr
	}
	_, err := s.hasher.Verify(s.decoy, password)
	return err
}

// Authenticate returns the User a session token belongs to, and extends the session from its last
// use, writing that at most once a minute.
func (s *Service) Authenticate(ctx context.Context, token string) (domain.User, error) {
	hash := hashOf(token)
	sess, err := s.store.SessionByHash(ctx, hash)
	if errors.Is(err, ErrNotFound) {
		return domain.User{}, ErrNoSession
	}
	if err != nil {
		return domain.User{}, err
	}
	now := s.now()
	if !now.Before(sess.ExpiresAt) {
		return domain.User{}, ErrNoSession
	}
	if now.Sub(sess.LastSeenAt) >= touchInterval {
		if err := s.store.TouchSession(ctx, hash, now, now.Add(sessionLifetime)); err != nil {
			return domain.User{}, err
		}
	}
	return s.store.UserByID(ctx, sess.UserID)
}

// SignOut ends the session a token belongs to; ending one that does not exist is not an error.
func (s *Service) SignOut(ctx context.Context, token string) error {
	return s.store.DeleteSession(ctx, hashOf(token))
}

// User returns a User by id.
func (s *Service) User(ctx context.Context, id uuid.UUID) (domain.User, error) {
	return s.store.UserByID(ctx, id)
}

// Rename changes a User's name.
func (s *Service) Rename(ctx context.Context, id uuid.UUID, rawName string) error {
	name, err := domain.NewName(rawName)
	if err != nil {
		return err
	}
	return s.store.UpdateName(ctx, id, name)
}

// ChangePassword replaces a User's password after checking the current one.
func (s *Service) ChangePassword(ctx context.Context, id uuid.UUID, current, next string) error {
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
	return s.store.UpdatePasswordHash(ctx, id, hash)
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
