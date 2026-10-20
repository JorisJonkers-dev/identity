package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

const (
	// resetLifetime is how long a reset link works.
	resetLifetime = 30 * time.Minute
	// Reset requests are counted per address and per client over resetWindow; past the limit they
	// still answer the same, and send nothing.
	resetWindow     = 15 * time.Minute
	resetsPerEmail  = 3
	resetsPerClient = 10
)

// PasswordReset is a mailed link to choose a new password (identity#81). ProvedAt is when the
// User proved two-factor for it; nil until then.
type PasswordReset struct {
	UserID    uuid.UUID
	TokenHash []byte
	ExpiresAt time.Time
	ProvedAt  *time.Time
}

// limiter counts recent hits per key, in memory: each replica counts its own.
type limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

// allow records a hit for key and reports whether it is within most over window.
func (l *limiter) allow(key string, most int, now time.Time, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[string][]time.Time{}
	}
	recent := l.hits[key][:0]
	for _, at := range l.hits[key] {
		if now.Sub(at) < window {
			recent = append(recent, at)
		}
	}
	recent = append(recent, now)
	l.hits[key] = recent
	return len(recent) <= most
}

type clientKey struct{}

// WithClient names the client a request came from, for limits counted per client.
func WithClient(ctx context.Context, address string) context.Context {
	return context.WithValue(ctx, clientKey{}, address)
}

// clientOf is the client ctx names; "" when it names none.
func clientOf(ctx context.Context) string {
	a, _ := ctx.Value(clientKey{}).(string)
	return a
}

// AskPasswordReset mails a reset link to the User with rawEmail, if there is one and neither the
// address nor the client asked too often. It answers the same either way.
func (s *Service) AskPasswordReset(ctx context.Context, rawEmail string) error {
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return err
	}
	now := s.now()
	byEmail := s.resets.allow("email:"+email.String(), resetsPerEmail, now, resetWindow)
	byClient := s.resets.allow("client:"+clientOf(ctx), resetsPerClient, now, resetWindow)
	if !byEmail || !byClient {
		return nil
	}
	u, err := s.store.UserByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	token, hash, err := s.token()
	if err != nil {
		return err
	}
	if err := s.store.SavePasswordReset(ctx, PasswordReset{UserID: u.ID, TokenHash: hash, ExpiresAt: now.Add(resetLifetime)}); err != nil {
		return err
	}
	return s.mailer.Send(ctx, Mail{
		To:      email.String(),
		Subject: "Reset your password",
		Text: "Choose a new password here:\n\n" + s.publicURL + "/reset?token=" + token +
			"\n\nThe link works once, for 30 minutes. If you did not ask for this, ignore this message; your password stays as it is.\n",
	})
}

// CheckPasswordReset says which kinds of two-factor method the reset must be proved with, none when the
// User has none or proved it already. ErrInvalidToken when the link is spent or expired.
func (s *Service) CheckPasswordReset(ctx context.Context, token string) ([]string, error) {
	r, err := s.liveReset(ctx, token)
	if err != nil {
		return nil, err
	}
	if r.ProvedAt != nil {
		return nil, nil
	}
	kinds, err := s.TwoFactorMethodKinds(ctx, r.UserID)
	if err != nil || len(kinds) == 0 {
		return nil, err
	}
	return kinds, s.saveChallenge(ctx, r.UserID, resetChallenge(token), nil)
}

// ResetWithCode proves the reset with a code from one of the User's authenticators.
func (s *Service) ResetWithCode(ctx context.Context, token, code string) error {
	r, err := s.liveReset(ctx, token)
	if err != nil {
		return err
	}
	if _, err := s.answerWithCode(ctx, resetChallenge(token), code); err != nil {
		return err
	}
	return s.store.ProvePasswordReset(ctx, r.UserID, s.now())
}

// ResetPasskeyOptions starts the passkey ceremony that proves the reset.
func (s *Service) ResetPasskeyOptions(ctx context.Context, token string) ([]byte, error) {
	if _, err := s.liveReset(ctx, token); err != nil {
		return nil, err
	}
	return s.passkeyOptions(ctx, resetChallenge(token))
}

// ResetWithPasskey proves the reset with the browser's passkey assertion.
func (s *Service) ResetWithPasskey(ctx context.Context, token string, response []byte) error {
	r, err := s.liveReset(ctx, token)
	if err != nil {
		return err
	}
	if _, err := s.answerWithPasskey(ctx, resetChallenge(token), response); err != nil {
		return err
	}
	return s.store.ProvePasswordReset(ctx, r.UserID, s.now())
}

// CompletePasswordReset sets the new password, ends every session and mails the User. A User with
// two-factor must have proved it for this reset: ErrFreshTwoFactorNeeded otherwise.
func (s *Service) CompletePasswordReset(ctx context.Context, token, rawPassword string) error {
	password, err := domain.NewPassword(rawPassword)
	if err != nil {
		return err
	}
	r, err := s.liveReset(ctx, token)
	if err != nil {
		return err
	}
	has, err := s.hasTwoFactor(ctx, r.UserID)
	if err != nil {
		return err
	}
	if has && r.ProvedAt == nil {
		return ErrFreshTwoFactorNeeded
	}
	u, err := s.store.UserByID(ctx, r.UserID)
	if err != nil {
		return err
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return err
	}
	taken, err := s.store.TakePasswordReset(ctx, r.TokenHash)
	if err != nil {
		return err
	}
	if !taken {
		return ErrInvalidToken
	}
	if err := s.store.UpdatePasswordHash(ctx, r.UserID, hash); err != nil {
		return err
	}
	if _, err := s.endSessions(ctx, r.UserID, nil); err != nil {
		return err
	}
	s.record(ctx, audit.Event{Action: audit.PasswordReset, User: r.UserID})
	return s.mailer.Send(ctx, Mail{
		To:      u.Email.String(),
		Subject: "Your password was reset",
		Text:    "Your password was just reset, and every device was signed out.\n\nIf you did not do this, contact an Admin at once.\n",
	})
}

// liveReset is the reset a link's token names, while it works; ErrInvalidToken otherwise.
func (s *Service) liveReset(ctx context.Context, token string) (PasswordReset, error) {
	r, err := s.store.PasswordResetByHash(ctx, hashOf(token))
	if errors.Is(err, ErrNotFound) || err == nil && !s.now().Before(r.ExpiresAt) {
		return PasswordReset{}, ErrInvalidToken
	}
	return r, err
}

// resetChallenge is the hash of the challenge proving a reset's two-factor, kept apart from the
// link's own so neither stands in for the other.
func resetChallenge(token string) []byte { return hashOf("reset:" + token) }
