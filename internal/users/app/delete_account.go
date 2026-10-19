package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
)

// freshTwoFactor is how recently a session must have proved two-factor to delete its account.
const freshTwoFactor = 5 * time.Minute

var (
	// ErrLastAdmin is the last Admin deleting their account, which would leave no one to manage
	// access.
	ErrLastAdmin = errors.New("the last Admin cannot delete their account")
	// ErrFreshTwoFactorNeeded is a User with two-factor whose session did not prove it just now.
	ErrFreshTwoFactorNeeded = errors.New("prove two-factor first")
)

// Deleted is what went with a deleted User: their sessions and Personal Tokens, by id.
type Deleted struct {
	Sessions []uuid.UUID
	Tokens   []uuid.UUID
}

// WithTokensDeleted is told the ids of the Personal Tokens a deleted User had, so whatever caches
// their checks drops them.
func WithTokensDeleted(f func(ctx context.Context, tokenIDs []uuid.UUID)) Option {
	return func(s *Service) { s.tokensDeleted = f }
}

// DeleteAccount deletes the signed-in User (identity#80). It needs their password and, when they
// have two-factor, a session that proved it within five minutes. Their sessions end with
// back-channel logout, their Personal Tokens stop working, and their memberships and direct
// Assignments go; Audit Events about them stay, naming no one. The last Admin is refused.
func (s *Service) DeleteAccount(ctx context.Context, userID uuid.UUID, sessionToken, password string) error {
	if err := s.checkPassword(ctx, userID, password); err != nil {
		return err
	}
	has, err := s.hasSecondFactor(ctx, userID)
	if err != nil {
		return err
	}
	if has {
		sess, err := s.store.SessionByHash(ctx, hashOf(sessionToken))
		if err != nil {
			return err
		}
		if sess.TwoFactorAt == nil || s.now().Sub(*sess.TwoFactorAt) > freshTwoFactor {
			return ErrFreshTwoFactorNeeded
		}
	}
	d, err := s.store.DeleteUser(ctx, userID)
	if err != nil {
		return err
	}
	if len(d.Sessions) > 0 && s.signedOut != nil {
		s.signedOut(ctx, userID, d.Sessions)
	}
	if len(d.Tokens) > 0 && s.tokensDeleted != nil {
		s.tokensDeleted(ctx, d.Tokens)
	}
	s.record(ctx, audit.Event{Action: audit.AccountDeleted, User: userID})
	return nil
}
