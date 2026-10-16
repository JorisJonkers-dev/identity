// Package catalog is the access context's view of the Clients the cluster declares: app.Clients
// over the sign-in context's index, translating at the boundary.
package catalog

import (
	"context"

	"github.com/JorisJonkers-dev/identity/internal/access/app"
	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// Index is what Catalog needs from the sign-in context.
type Index interface {
	All() []signindomain.Client
	Client(ctx context.Context, id string) (signindomain.Client, error)
}

// Catalog is the app.Clients.
type Catalog struct{ index Index }

// New returns a Catalog over index.
func New(index Index) *Catalog { return &Catalog{index: index} }

var _ app.Clients = (*Catalog)(nil)

func (c *Catalog) Clients() []domain.Client {
	all := c.index.All()
	out := make([]domain.Client, 0, len(all))
	for _, x := range all {
		out = append(out, client(x))
	}
	return out
}

func (c *Catalog) Client(ctx context.Context, id string) (domain.Client, bool) {
	x, err := c.index.Client(ctx, id)
	if err != nil {
		return domain.Client{}, false
	}
	return client(x), true
}

// client names a token Client, which has no label, by its id.
func client(x signindomain.Client) domain.Client {
	label := x.Label
	if label == "" {
		label = x.ID
	}
	return domain.Client{ID: x.ID, Label: label, Roles: x.Roles}
}
