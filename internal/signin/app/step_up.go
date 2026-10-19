package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var (
	// ErrStepUpNeeded is a sign-in to a Step-up Client whose session last proved two-factor too
	// long ago, or never (docs/adr/0019).
	ErrStepUpNeeded = errors.New("the Client wants a fresher two-factor proof")
	// ErrTwoFactorNeeded is a sign-in to a Step-up Client by a User with no two-factor to prove.
	ErrTwoFactorNeeded = errors.New("the Client wants two-factor, and the User has none")
	// ErrInvalidStepUp is a window other than off, 5, 15 or 60 minutes.
	ErrInvalidStepUp = errors.New("step-up is off, 5, 15 or 60 minutes")
)

// StepUpSettings keeps each Client's step-up window in minutes, by Client id, so a Removed Client
// comes back with it.
type StepUpSettings interface {
	// StepUp is the Client's window in minutes; 0 when it is off.
	StepUp(ctx context.Context, clientID string) (int, error)
	// SetStepUp sets the window; 0 turns it off.
	SetStepUp(ctx context.Context, clientID string, minutes int) error
}

// SetStepUps lets sign-ins ask how fresh each Client wants two-factor; until it is called, none
// does. It is set once, before the Provider serves.
func (s *Service) SetStepUps(settings StepUpSettings) { s.stepUps = settings }

// fresh is nil when the Client wants no step-up or the session proved two-factor within its window.
func (s *Service) fresh(ctx context.Context, signIn domain.SignIn, clientID string) error {
	if s.stepUps == nil {
		return nil
	}
	minutes, err := s.stepUps.StepUp(ctx, clientID)
	if err != nil || minutes == 0 {
		return err
	}
	// A session that never proved two-factor has a zero time, older than any window.
	if s.now().Sub(signIn.TwoFactorAt) <= time.Duration(minutes)*time.Minute {
		return nil
	}
	has, err := s.accounts.HasTwoFactor(ctx, signIn.UserID)
	if err != nil {
		return err
	}
	if !has {
		return ErrTwoFactorNeeded
	}
	return ErrStepUpNeeded
}

// ClientTokens is the Personal Tokens for a Client, which step-up ends: a bearer token proves
// nothing fresh.
type ClientTokens interface {
	LiveForClient(ctx context.Context, clientID string) (int, error)
	RevokeForClient(ctx context.Context, actor uuid.UUID, clientID string) error
}

// StepUps lets Admins set how fresh each Client wants two-factor.
type StepUps struct {
	settings StepUpSettings
	clients  Clients
	admins   Admins
	audit    Audit
	tokens   ClientTokens
}

// NewStepUps returns StepUps.
func NewStepUps(settings StepUpSettings, clients Clients, admins Admins, audit Audit, tokens ClientTokens) *StepUps {
	return &StepUps{settings: settings, clients: clients, admins: admins, audit: audit, tokens: tokens}
}

// StepUpState is a Client's window in minutes, 0 when it is off, and how many live Personal
// Tokens turning it on would revoke.
type StepUpState struct {
	Minutes int
	Tokens  int
}

// StepUp is a Client's step-up, as an Admin reads it.
func (s *StepUps) StepUp(ctx context.Context, actor Actor, clientID string) (StepUpState, error) {
	if err := admin(ctx, s.admins, actor); err != nil {
		return StepUpState{}, err
	}
	if _, err := s.clients.Client(ctx, clientID); err != nil {
		return StepUpState{}, err
	}
	minutes, err := s.settings.StepUp(ctx, clientID)
	if err != nil {
		return StepUpState{}, err
	}
	tokens, err := s.tokens.LiveForClient(ctx, clientID)
	return StepUpState{Minutes: minutes, Tokens: tokens}, err
}

// Set sets a Client's window: off (0), 5, 15 or 60 minutes.
func (s *StepUps) Set(ctx context.Context, actor Actor, clientID string, minutes int) error {
	if err := admin(ctx, s.admins, actor); err != nil {
		return err
	}
	switch minutes {
	case 0, 5, 15, 60:
	default:
		return ErrInvalidStepUp
	}
	if _, err := s.clients.Client(ctx, clientID); err != nil {
		return err
	}
	// The tokens go first: should setting fail after, none is left that step-up would refuse.
	if minutes > 0 {
		if err := s.tokens.RevokeForClient(ctx, actor.ID, clientID); err != nil {
			return err
		}
	}
	if err := s.settings.SetStepUp(ctx, clientID, minutes); err != nil {
		return err
	}
	s.audit.Record(ctx, audit.Event{Action: audit.StepUpChanged, Actor: actor.ID, Client: clientID, Minutes: minutes})
	return nil
}
