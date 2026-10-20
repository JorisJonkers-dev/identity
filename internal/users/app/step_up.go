package app

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Step-up (docs/adr/0019): a signed-in User proves two-factor again, in the session they have,
// so an app that wants a fresh proof lets them in. The proof goes through a challenge bound to the
// session, with the same five codes in fifteen minutes a sign-in has.

// StartStepUp starts proving the session again, and returns the kinds of two-factor method the User
// can prove it with: ErrNoTwoFactor when they have none.
func (s *Service) StartStepUp(ctx context.Context, sessionToken string) ([]string, error) {
	hash := hashOf(sessionToken)
	sess, err := s.store.SessionByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	kinds, err := s.TwoFactorMethodKinds(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	if len(kinds) == 0 {
		return nil, ErrNoTwoFactor
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

// HasTwoFactor reports whether a User added an authenticator app or a passkey.
func (s *Service) HasTwoFactor(ctx context.Context, userID uuid.UUID) (bool, error) {
	return s.hasTwoFactor(ctx, userID)
}

// UsersWithTwoFactor is every User who added an authenticator app or a passkey.
func (s *Service) UsersWithTwoFactor(ctx context.Context) ([]uuid.UUID, error) {
	return s.store.UsersWithTwoFactor(ctx)
}

// NotifyTwoFactorRequired mails a User that a Group of theirs now requires two-factor, and holds
// back its roles until they set it up (identity#88).
func (s *Service) NotifyTwoFactorRequired(ctx context.Context, userID uuid.UUID, group string) error {
	u, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	return s.mailer.Send(ctx, Mail{
		To:      u.Email.String(),
		Subject: group + " now requires two-factor",
		Text: "The apps " + group + " gives you stay closed until you add a passkey or an authenticator app:\n\n" +
			s.publicURL + "/security/two-factor\n\nYour other apps keep working.\n",
	})
}
