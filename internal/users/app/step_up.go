package app

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Step-up (docs/adr/0019): a signed-in User proves a Second Factor again, in the session they have,
// so an app that wants a fresh proof lets them in. The proof goes through a challenge bound to the
// session, with the same five codes in fifteen minutes a sign-in has.

// StartStepUp starts proving the session again, and returns the kinds of Second Factor the User
// can prove it with: ErrNoSecondFactor when they have none.
func (s *Service) StartStepUp(ctx context.Context, sessionToken string) ([]string, error) {
	hash := hashOf(sessionToken)
	sess, err := s.store.SessionByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	kinds, err := s.SecondFactorKinds(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	if len(kinds) == 0 {
		return nil, ErrNoSecondFactor
	}
	if _, err := s.challenge(ctx, sess.UserID, hash); err != nil {
		return nil, err
	}
	return kinds, nil
}

// StepUpWithCode proves the session again with a code from one of the User's authenticators.
func (s *Service) StepUpWithCode(ctx context.Context, sessionToken, code string) error {
	hash, err := s.stepUpChallenge(ctx, sessionToken)
	if err != nil {
		return err
	}
	if _, err := s.answerWithCode(ctx, hash, code); err != nil {
		return err
	}
	return s.store.SetSessionMethods(ctx, hashOf(sessionToken), []string{methodPassword, methodOTP}, s.now())
}

// StepUpPasskeyOptions starts the passkey ceremony that proves the session again.
func (s *Service) StepUpPasskeyOptions(ctx context.Context, sessionToken string) ([]byte, error) {
	hash, err := s.stepUpChallenge(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	return s.passkeyOptions(ctx, hash)
}

// StepUpWithPasskey proves the session again with the browser's passkey assertion.
func (s *Service) StepUpWithPasskey(ctx context.Context, sessionToken string, response []byte) error {
	hash, err := s.stepUpChallenge(ctx, sessionToken)
	if err != nil {
		return err
	}
	if _, err := s.answerWithPasskey(ctx, hash, response); err != nil {
		return err
	}
	return s.store.SetSessionMethods(ctx, hashOf(sessionToken), []string{methodPassword, methodPasskey}, s.now())
}

// stepUpChallenge is the hash of the session's step-up challenge; ErrInvalidToken when StartStepUp
// made none, or it is spent.
func (s *Service) stepUpChallenge(ctx context.Context, sessionToken string) ([]byte, error) {
	hash, err := s.store.StepUpChallenge(ctx, hashOf(sessionToken))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidToken
	}
	return hash, err
}

// HasSecondFactor reports whether a User added an authenticator app or a passkey.
func (s *Service) HasSecondFactor(ctx context.Context, userID uuid.UUID) (bool, error) {
	return s.hasSecondFactor(ctx, userID)
}

// UsersWithTwoFactor is every User who added an authenticator app or a passkey.
func (s *Service) UsersWithTwoFactor(ctx context.Context) ([]uuid.UUID, error) {
	return s.store.UsersWithTwoFactor(ctx)
}
