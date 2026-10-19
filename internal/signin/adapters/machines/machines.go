// Package machines is the sign-in context's view of the access context's Machine Identities:
// app.Machines over the access use cases, translating at the boundary.
package machines

import (
	"context"
	"errors"

	"github.com/google/uuid"

	accessapp "github.com/JorisJonkers-dev/identity/internal/access/app"
	accessdomain "github.com/JorisJonkers-dev/identity/internal/access/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

// Access is what Machines needs from the access context.
type Access interface {
	Machine(ctx context.Context, id uuid.UUID) (accessdomain.Machine, error)
	MachineByBinding(ctx context.Context, namespace, serviceAccount string) (accessdomain.Machine, error)
	MachineRoles(ctx context.Context, id uuid.UUID, clientID string) ([]string, error)
}

// Machines is the app.Machines.
type Machines struct{ access Access }

// New returns Machines over access.
func New(access Access) *Machines { return &Machines{access: access} }

var _ app.Machines = (*Machines)(nil)

func (m *Machines) Machine(ctx context.Context, id uuid.UUID) (app.MachineIdentity, error) {
	return identity(m.access.Machine(ctx, id))
}

func (m *Machines) MachineByBinding(ctx context.Context, namespace, serviceAccount string) (app.MachineIdentity, error) {
	return identity(m.access.MachineByBinding(ctx, namespace, serviceAccount))
}

func (m *Machines) MachineRoles(ctx context.Context, id uuid.UUID, clientID string) ([]string, error) {
	return m.access.MachineRoles(ctx, id, clientID)
}

func identity(d accessdomain.Machine, err error) (app.MachineIdentity, error) {
	if errors.Is(err, accessapp.ErrNotFound) {
		return app.MachineIdentity{}, app.ErrNotFound
	}
	if err != nil {
		return app.MachineIdentity{}, err
	}
	return app.MachineIdentity{ID: d.ID, Name: d.Name, Namespace: d.Namespace, ServiceAccount: d.ServiceAccount}, nil
}
