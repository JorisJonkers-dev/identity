package app

import (
	"context"
	"errors"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

var (
	// ErrNotAdmin is someone who is not an Admin managing another person.
	ErrNotAdmin = errors.New("only Admins manage people")
	// ErrAdminTwoFactorProofNeeded is an Admin whose session proved no two-factor.
	ErrAdminTwoFactorProofNeeded = errors.New("people are managed only from a session that proved two-factor")
)

// Actor is an Admin managing a person, and whether their session proved two-factor.
type Actor struct {
	ID        uuid.UUID
	TwoFactor bool
}

// admin lets an Admin through, and only from a session that proved two-factor.
func (s *Service) admin(ctx context.Context, actor Actor) error {
	if s.admins == nil {
		return ErrNotAdmin
	}
	yes, err := s.admins.IsAdmin(ctx, actor.ID)
	if err != nil {
		return err
	}
	if !yes {
		return ErrNotAdmin
	}
	if !actor.TwoFactor {
		return ErrAdminTwoFactorProofNeeded
	}
	return nil
}

// RenameAs changes a person's name, as an Admin (identity#86).
func (s *Service) RenameAs(ctx context.Context, actor Actor, userID uuid.UUID, rawName string) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	name, err := domain.NewName(rawName)
	if err != nil {
		return err
	}
	if err := s.store.UpdateName(ctx, userID, name); err != nil {
		return err
	}
	s.record(ctx, audit.Event{Action: audit.UserRenamed, Actor: actor.ID, User: userID})
	return nil
}

// AskEmailChangeAs starts a person's email change, as an Admin: it waits on the link mailed to the
// new address, as the person's own does (identity#86).
func (s *Service) AskEmailChangeAs(ctx context.Context, actor Actor, userID uuid.UUID, rawEmail string) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return err
	}
	if _, err := s.store.UserByID(ctx, userID); err != nil {
		return err
	}
	return s.startEmailChange(ctx, userID, email)
}

// DeleteAs deletes a person, as an Admin, with the same effects as their own deletion; the last
// Admin is refused (identity#87).
func (s *Service) DeleteAs(ctx context.Context, actor Actor, userID uuid.UUID) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	return s.deleteUser(ctx, actor.ID, userID)
}
