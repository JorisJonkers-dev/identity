// Package contexts is the tokens context's view of the sign-in and access contexts: a User's App
// Roles per Client, and the Clients the cluster declares, translated at the boundary.
package contexts

import (
	"context"

	"github.com/google/uuid"

	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
	"github.com/JorisJonkers-dev/identity/internal/tokens/app"
)

// SignIn is what the tokens context needs from sign-in: the App Roles a Client still declares.
type SignIn interface {
	Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error)
}

// Access is what the tokens context needs from access: every App Role a User holds, per Client.
type Access interface {
	RolesByClient(ctx context.Context, userID uuid.UUID) (map[string][]string, error)
}

// StepUps is each Client's step-up window in minutes, 0 when it is off.
type StepUps interface {
	StepUp(ctx context.Context, clientID string) (int, error)
}

// Index is the Clients the cluster declares.
type Index interface {
	Client(ctx context.Context, id string) (signindomain.Client, error)
}

// Contexts is the app.Roles and app.Clients.
type Contexts struct {
	signIn  SignIn
	access  Access
	index   Index
	stepUps StepUps
}

// New returns Contexts over the four.
func New(signIn SignIn, access Access, index Index, stepUps StepUps) *Contexts {
	return &Contexts{signIn: signIn, access: access, index: index, stepUps: stepUps}
}

func (c *Contexts) StepUp(ctx context.Context, clientID string) (bool, error) {
	minutes, err := c.stepUps.StepUp(ctx, clientID)
	return minutes > 0, err
}

var (
	_ app.Roles   = (*Contexts)(nil)
	_ app.Clients = (*Contexts)(nil)
)

func (c *Contexts) Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error) {
	return c.signIn.Roles(ctx, userID, clientID)
}

func (c *Contexts) ByClient(ctx context.Context, userID uuid.UUID) (map[string][]string, error) {
	return c.access.RolesByClient(ctx, userID)
}

func (c *Contexts) Exists(ctx context.Context, clientID string) bool {
	_, err := c.index.Client(ctx, clientID)
	return err == nil
}

// Label is a Client's label, its id when it has none, and its id when it is gone.
func (c *Contexts) Label(ctx context.Context, clientID string) string {
	cl, err := c.index.Client(ctx, clientID)
	if err != nil || cl.Label == "" {
		return clientID
	}
	return cl.Label
}
