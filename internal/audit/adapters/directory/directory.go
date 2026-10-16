// Package directory is the audit context's view of the users and sign-in contexts: who the Users
// are and what Clients are called, translated at the boundary.
package directory

import (
	"context"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/audit/app"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
	usersdomain "github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// Users is what the Directory needs from the users context.
type Users interface {
	Users(ctx context.Context) ([]usersdomain.User, error)
	ActiveSessions(ctx context.Context) (sessions, users int, err error)
}

// Access is what the Directory needs from the access context.
type Access interface {
	UnassignedUsers(ctx context.Context) ([]uuid.UUID, error)
}

// Index is what the Directory needs from the sign-in context.
type Index interface {
	Client(ctx context.Context, id string) (signindomain.Client, error)
}

// Directory is the app.Directory.
type Directory struct {
	users  Users
	access Access
	index  Index
}

// New returns a Directory over the three.
func New(users Users, access Access, index Index) *Directory {
	return &Directory{users: users, access: access, index: index}
}

var _ app.Directory = (*Directory)(nil)

func (d *Directory) People(ctx context.Context) ([]app.Person, error) {
	users, err := d.users.Users(ctx)
	if err != nil {
		return nil, err
	}
	people := make([]app.Person, 0, len(users))
	for _, u := range users {
		people = append(people, app.Person{ID: u.ID, Name: u.Name.String(), Email: u.Email.String(), CreatedAt: u.CreatedAt})
	}
	return people, nil
}

func (d *Directory) Unassigned(ctx context.Context) ([]uuid.UUID, error) {
	return d.access.UnassignedUsers(ctx)
}

func (d *Directory) ActiveSessions(ctx context.Context) (sessions, users int, err error) {
	return d.users.ActiveSessions(ctx)
}

// ClientLabel is a Client's label, its id when it has none, and its id when it is gone.
func (d *Directory) ClientLabel(ctx context.Context, id string) string {
	c, err := d.index.Client(ctx, id)
	if err != nil || c.Label == "" {
		return id
	}
	return c.Label
}
