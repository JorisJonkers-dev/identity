// Package clients is where the sign-in context finds Clients. Static is a fixed set; Clients from
// Kubernetes Client objects replace it (identity#13).
package clients

import (
	"context"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// Static is an app.Clients over a fixed set, keyed by id.
type Static map[string]domain.Client

var _ app.Clients = Static{}

func (s Static) Client(_ context.Context, id string) (domain.Client, error) {
	c, ok := s[id]
	if !ok {
		return domain.Client{}, app.ErrNotFound
	}
	return c, nil
}
