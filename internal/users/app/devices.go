package app

import (
	"bytes"
	"context"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
)

// Device is one of a User's Single sign-on sessions, as the Devices list shows it (identity#78).
type Device struct {
	ID         uuid.UUID
	Label      string
	LastSeenAt time.Time
	// Current is the session the list was asked from.
	Current bool
}

// Devices lists a User's live sessions, the most recently seen first, marking the one sessionToken
// names.
func (s *Service) Devices(ctx context.Context, userID uuid.UUID, sessionToken string) ([]Device, error) {
	sessions, err := s.store.SessionsOf(ctx, userID, s.now())
	if err != nil {
		return nil, err
	}
	current := hashOf(sessionToken)
	out := make([]Device, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, Device{ID: sess.ID, Label: sess.Device, LastSeenAt: sess.LastSeenAt, Current: bytes.Equal(sess.TokenHash, current)})
	}
	return out, nil
}

// SignOutDevice ends one of the User's sessions, and the apps it signed in to are told
// (docs/adr/0007). ErrNotFound when the User has no such session.
func (s *Service) SignOutDevice(ctx context.Context, userID, deviceID uuid.UUID) error {
	ended, err := s.store.DeleteSession(ctx, userID, deviceID)
	if err != nil {
		return err
	}
	if !ended {
		return ErrNotFound
	}
	if s.signedOut != nil {
		s.signedOut(ctx, userID, []uuid.UUID{deviceID})
	}
	s.record(ctx, audit.Event{Action: audit.SignedOut, User: userID})
	return nil
}

// SignOutOtherDevices ends every session of the User but the one sessionToken names.
func (s *Service) SignOutOtherDevices(ctx context.Context, userID uuid.UUID, sessionToken string) error {
	ended, err := s.endSessions(ctx, userID, hashOf(sessionToken))
	if ended > 0 {
		s.record(ctx, audit.Event{Action: audit.SignedOut, User: userID})
	}
	return err
}
