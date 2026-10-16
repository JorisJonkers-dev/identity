// Package people is the access context's view of Users: app.People over the users context,
// translating at the boundary.
package people

import (
	"context"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/app"
	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	usersdomain "github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// Users is what People needs from the users context.
type Users interface {
	Users(ctx context.Context) ([]usersdomain.User, error)
	SignOutEverywhere(ctx context.Context, actor, userID uuid.UUID) error
}

// People is the app.People.
type People struct{ users Users }

// New returns People over users.
func New(users Users) *People { return &People{users: users} }

var _ app.People = (*People)(nil)

func (p *People) People(ctx context.Context) ([]domain.Person, error) {
	users, err := p.users.Users(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Person, 0, len(users))
	for _, u := range users {
		out = append(out, domain.Person{ID: u.ID, Email: u.Email.String(), Name: u.Name.String()})
	}
	return out, nil
}

// SignOut signs the User out everywhere, at the actor's asking.
func (p *People) SignOut(ctx context.Context, actor, userID uuid.UUID) error {
	return p.users.SignOutEverywhere(ctx, actor, userID)
}
