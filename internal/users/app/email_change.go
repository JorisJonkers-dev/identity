package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// emailChangeLifetime is how long the link to a new address works.
const emailChangeLifetime = 24 * time.Hour

// EmailChange is a new sign-in email waiting for the link mailed to it (identity#79).
type EmailChange struct {
	UserID    uuid.UUID
	Email     domain.Email
	TokenHash []byte
	ExpiresAt time.Time
}

// AskEmailChange mails a link to rawEmail that makes it the User's sign-in email once followed.
// It needs the User's password. An address in use gets the same answer as a free one; its link
// then fails.
func (s *Service) AskEmailChange(ctx context.Context, userID uuid.UUID, rawEmail, password string) error {
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return err
	}
	if err := s.checkPassword(ctx, userID, password); err != nil {
		return err
	}
	return s.startEmailChange(ctx, userID, email)
}

// startEmailChange keeps a pending change to email and mails the address its link.
func (s *Service) startEmailChange(ctx context.Context, userID uuid.UUID, email domain.Email) error {
	token, hash, err := s.token()
	if err != nil {
		return err
	}
	c := EmailChange{UserID: userID, Email: email, TokenHash: hash, ExpiresAt: s.now().Add(emailChangeLifetime)}
	if err := s.store.SaveEmailChange(ctx, c); err != nil {
		return err
	}
	return s.mailer.Send(ctx, Mail{
		To:      email.String(),
		Subject: "Confirm your new email",
		Text: "Confirm this address as your new sign-in email:\n\n" + s.publicURL + "/account/email?token=" + token +
			"\n\nThe link works once, for 24 hours. Until then you sign in with your current email. If you did not ask for this, ignore this message.\n",
	})
}

// PendingEmail is the address a User asked to change to and has not confirmed; "" when none waits.
func (s *Service) PendingEmail(ctx context.Context, userID uuid.UUID) (string, error) {
	c, err := s.store.EmailChangeOf(ctx, userID)
	if errors.Is(err, ErrNotFound) || err == nil && !s.now().Before(c.ExpiresAt) {
		return "", nil
	}
	return c.Email.String(), err
}

// CancelEmailChange drops a pending change; its link stops working.
func (s *Service) CancelEmailChange(ctx context.Context, userID uuid.UUID) error {
	return s.store.DeleteEmailChange(ctx, userID)
}

// ConfirmEmailChange makes the address a link was mailed to the User's sign-in email, and tells
// the old address. A spent or expired link, or an address taken since, is ErrInvalidToken.
func (s *Service) ConfirmEmailChange(ctx context.Context, token string) error {
	c, err := s.store.TakeEmailChange(ctx, hashOf(token))
	if errors.Is(err, ErrNotFound) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}
	if !s.now().Before(c.ExpiresAt) {
		return ErrInvalidToken
	}
	u, err := s.store.UserByID(ctx, c.UserID)
	if err != nil {
		return err
	}
	err = s.store.UpdateEmail(ctx, c.UserID, c.Email)
	if errors.Is(err, ErrEmailTaken) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}
	s.record(ctx, audit.Event{Action: audit.EmailChanged, User: c.UserID, Email: u.Email.String()})
	return s.mailer.Send(ctx, Mail{
		To:      u.Email.String(),
		Subject: "Your sign-in email changed",
		Text: "You now sign in with " + c.Email.String() + " instead of this address.\n\n" +
			"If you did not change it, contact an Admin at once.\n",
	})
}
