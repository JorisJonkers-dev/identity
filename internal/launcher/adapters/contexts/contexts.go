// Package contexts is the Launcher's view of the sign-in and access contexts: the Clients a person
// can open, and the App Roles a User holds, translated at the boundary.
package contexts

import (
	"context"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/launcher/app"
	"github.com/JorisJonkers-dev/identity/internal/launcher/domain"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// Index is what the Launcher needs from the sign-in context.
type Index interface {
	All() []signindomain.Client
	Client(ctx context.Context, id string) (signindomain.Client, error)
}

// Access is what the Launcher needs from the access context.
type Access interface {
	RolesByClient(ctx context.Context, userID uuid.UUID) (map[string][]string, error)
	HeldBackClients(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// Contexts is the app.Clients and app.Roles.
type Contexts struct {
	index  Index
	access Access
}

// New returns Contexts over the two.
func New(index Index, access Access) *Contexts { return &Contexts{index: index, access: access} }

var (
	_ app.Clients = (*Contexts)(nil)
	_ app.Roles   = (*Contexts)(nil)
)

// Launchable is the native and gated Clients; a token Client has nothing to open.
func (c *Contexts) Launchable() []domain.Client {
	var out []domain.Client
	for _, x := range c.index.All() {
		if x.SignIn != signindomain.Token {
			out = append(out, client(x))
		}
	}
	return out
}

func (c *Contexts) Client(id string) (domain.Client, bool) {
	x, err := c.index.Client(context.Background(), id)
	if err != nil || x.SignIn == signindomain.Token {
		return domain.Client{}, false
	}
	return client(x), true
}

func (c *Contexts) RolesByClient(ctx context.Context, userID uuid.UUID) (map[string][]string, error) {
	return c.access.RolesByClient(ctx, userID)
}

func (c *Contexts) HeldBack(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return c.access.HeldBackClients(ctx, userID)
}

func client(x signindomain.Client) domain.Client {
	return domain.Client{
		ID: x.ID, Label: x.Label, Description: x.Description, Host: x.Host, LaunchURL: x.LaunchURL, IconURL: x.IconURL,
	}
}
