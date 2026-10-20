// Package connected is the sign-in context's view of the connected context: app.Consents over its
// use cases, translated at the boundary.
package connected

import (
	"context"
	"errors"

	"github.com/google/uuid"

	connectedapp "github.com/JorisJonkers-dev/identity/internal/connected/app"
	connecteddomain "github.com/JorisJonkers-dev/identity/internal/connected/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

// Apps is what sign-in needs from the connected context.
type Apps interface {
	Consented(ctx context.Context, userID uuid.UUID, registrationID, clientID string) (uuid.UUID, error)
	Consent(ctx context.Context, userID uuid.UUID, registrationID, clientID string) (uuid.UUID, error)
	Registration(ctx context.Context, clientID string) (connecteddomain.Registration, error)
}

// Consents is the app.Consents.
type Consents struct{ apps Apps }

// New returns Consents over apps.
func New(apps Apps) *Consents { return &Consents{apps: apps} }

var _ app.Consents = (*Consents)(nil)

func (c *Consents) Consented(ctx context.Context, userID uuid.UUID, registrationID, clientID string) error {
	_, err := c.apps.Consented(ctx, userID, registrationID, clientID)
	if errors.Is(err, connectedapp.ErrNotFound) {
		return app.ErrConsentNeeded
	}
	return refusal(err)
}

func (c *Consents) Consent(ctx context.Context, userID uuid.UUID, registrationID, clientID string) error {
	_, err := c.apps.Consent(ctx, userID, registrationID, clientID)
	if errors.Is(err, connectedapp.ErrNotFound) {
		return app.ErrNotFound
	}
	return refusal(err)
}

func (c *Consents) Name(ctx context.Context, registrationID string) (string, error) {
	r, err := c.apps.Registration(ctx, registrationID)
	if errors.Is(err, connectedapp.ErrNotFound) {
		return "", app.ErrNotFound
	}
	return r.Name, err
}

// refusal is a consent the connected context refuses, as sign-in names it.
func refusal(err error) error {
	switch {
	case errors.Is(err, connectedapp.ErrNotMCPResource), errors.Is(err, connectedapp.ErrStepUpClient):
		return app.ErrNotConnectable
	case errors.Is(err, connectedapp.ErrNoAccess):
		return app.ErrNoAccess
	}
	return err
}
