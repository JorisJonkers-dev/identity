// Package contexts is the connected context's view of the sign-in and access contexts: the
// Clients the cluster declares with their step-up, and a User's App Roles, translated at the
// boundary.
package contexts

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/connected/app"
	signinapp "github.com/JorisJonkers-dev/identity/internal/signin/app"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// Index is the Clients the cluster declares, by id and by host.
type Index interface {
	Client(ctx context.Context, id string) (signindomain.Client, error)
	ClientForHost(ctx context.Context, host string) (signindomain.Client, error)
}

// StepUps is each Client's step-up window in minutes, 0 when it is off.
type StepUps interface {
	StepUp(ctx context.Context, clientID string) (int, error)
}

// Access is every App Role a User holds on a Client, directly and through Groups.
type Access interface {
	Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error)
}

// Contexts is the app.Clients and app.Roles.
type Contexts struct {
	index   Index
	stepUps StepUps
	access  Access
}

// New returns Contexts over the three.
func New(index Index, stepUps StepUps, access Access) *Contexts {
	return &Contexts{index: index, stepUps: stepUps, access: access}
}

var (
	_ app.Clients = (*Contexts)(nil)
	_ app.Roles   = (*Contexts)(nil)
)

func (c *Contexts) Client(ctx context.Context, id string) (app.Client, error) {
	cl, err := c.index.Client(ctx, id)
	return c.client(ctx, cl, err)
}

func (c *Contexts) ClientForHost(ctx context.Context, host string) (app.Client, error) {
	cl, err := c.index.ClientForHost(ctx, host)
	return c.client(ctx, cl, err)
}

func (c *Contexts) client(ctx context.Context, cl signindomain.Client, err error) (app.Client, error) {
	if errors.Is(err, signinapp.ErrNotFound) {
		return app.Client{}, app.ErrNotFound
	}
	if err != nil {
		return app.Client{}, err
	}
	minutes, err := c.stepUps.StepUp(ctx, cl.ID)
	if err != nil {
		return app.Client{}, err
	}
	return app.Client{ID: cl.ID, Label: cl.Label, Roles: cl.Roles, StepUp: minutes > 0}, nil
}

func (c *Contexts) Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error) {
	return c.access.Roles(ctx, userID, clientID)
}
