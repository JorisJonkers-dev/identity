// Package accounts is the sign-in context's view of the users context: app.Accounts over the users
// use cases, translating at the boundary so neither context sees the other's types.
package accounts

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
	usersapp "github.com/JorisJonkers-dev/identity/internal/users/app"
	usersdomain "github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// Users is what Accounts needs from the users context.
type Users interface {
	SignedIn(ctx context.Context, token string) (usersdomain.User, usersapp.Session, error)
	User(ctx context.Context, id uuid.UUID) (usersdomain.User, error)
	SignOutEverywhere(ctx context.Context, actor, id uuid.UUID) error
}

// Accounts is the app.Accounts.
type Accounts struct{ users Users }

// New returns Accounts over users.
func New(users Users) *Accounts { return &Accounts{users: users} }

var _ app.Accounts = (*Accounts)(nil)

func (a *Accounts) SignedIn(ctx context.Context, token string) (domain.SignIn, error) {
	_, s, err := a.users.SignedIn(ctx, token)
	if errors.Is(err, usersapp.ErrNoSession) {
		return domain.SignIn{}, app.ErrNoSession
	}
	if err != nil {
		return domain.SignIn{}, err
	}
	return domain.SignIn{UserID: s.UserID, SessionID: s.ID, At: s.CreatedAt, Methods: s.Methods}, nil
}

func (a *Accounts) Claims(ctx context.Context, id uuid.UUID) (app.Claims, error) {
	u, err := a.users.User(ctx, id)
	if err != nil {
		return app.Claims{}, err
	}
	return app.Claims{Email: u.Email.String(), Name: u.Name.String()}, nil
}

func (a *Accounts) SignOutEverywhere(ctx context.Context, id uuid.UUID) error {
	return a.users.SignOutEverywhere(ctx, uuid.Nil, id)
}
