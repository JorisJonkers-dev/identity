// Package contexts is the Gate's view of the sign-in and users contexts: gated Clients by host,
// App Roles, and Single sign-on sessions, translated at the boundary.
package contexts

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/gate/app"
	signinapp "github.com/JorisJonkers-dev/identity/internal/signin/app"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
	usersapp "github.com/JorisJonkers-dev/identity/internal/users/app"
	usersdomain "github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// SignIn is what the Gate needs from the sign-in context.
type SignIn interface {
	ClientForHost(ctx context.Context, host string) (signindomain.Client, error)
	Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error)
	ResolvePersonalToken(ctx context.Context, secret string) (tokenID, subject string, ok bool)
	CheckPersonalToken(ctx context.Context, tokenID, clientID string) (signinapp.PersonalToken, error)
}

// Users is what the Gate needs from the users context.
type Users interface {
	SignedIn(ctx context.Context, token string) (usersdomain.User, usersapp.Session, error)
	Vouch(ctx context.Context, sessionID uuid.UUID) (usersdomain.User, usersapp.Session, error)
	User(ctx context.Context, id uuid.UUID) (usersdomain.User, error)
}

// Contexts is the app.Clients, app.Roles, app.Accounts and app.Tokens.
type Contexts struct {
	signIn SignIn
	users  Users
}

// New returns Contexts over the two.
func New(signIn SignIn, users Users) *Contexts { return &Contexts{signIn: signIn, users: users} }

var (
	_ app.Clients  = (*Contexts)(nil)
	_ app.Roles    = (*Contexts)(nil)
	_ app.Accounts = (*Contexts)(nil)
	_ app.Tokens   = (*Contexts)(nil)
)

// BearerClient is the Client a host belongs to when it takes Personal Tokens: a gated or a token
// one. A host with none, or with a native Client, is ErrUnknownHost.
func (c *Contexts) BearerClient(ctx context.Context, host string) (app.Client, error) {
	cl, err := c.signIn.ClientForHost(ctx, host)
	if errors.Is(err, signinapp.ErrNotFound) || err == nil && cl.SignIn == signindomain.Native {
		return app.Client{}, app.ErrUnknownHost
	}
	if err != nil {
		return app.Client{}, err
	}
	return app.Client{ID: cl.ID, Label: cl.Label, Host: cl.Host}, nil
}

// Bearer checks a Personal Token for a Client through sign-in, which owns the check
// (docs/adr/0017): a token that names nothing, or is not active, is ErrNoSignIn.
func (c *Contexts) Bearer(ctx context.Context, secret, clientID string) (app.Person, []string, error) {
	tokenID, _, ok := c.signIn.ResolvePersonalToken(ctx, secret)
	if !ok {
		return app.Person{}, nil, app.ErrNoSignIn
	}
	t, err := c.signIn.CheckPersonalToken(ctx, tokenID, clientID)
	if errors.Is(err, signinapp.ErrInactive) {
		return app.Person{}, nil, app.ErrNoSignIn
	}
	if err != nil {
		return app.Person{}, nil, err
	}
	u, err := c.users.User(ctx, t.UserID)
	if err != nil {
		return app.Person{}, nil, err
	}
	return app.Person{UserID: u.ID, Email: u.Email.String(), Name: u.Name.String()}, t.Roles, nil
}

// GatedClient is the Client a host belongs to when the Gate signs in for it; a host with none, or
// one whose Client signs in another way, is ErrUnknownHost.
func (c *Contexts) GatedClient(ctx context.Context, host string) (app.Client, error) {
	cl, err := c.signIn.ClientForHost(ctx, host)
	if errors.Is(err, signinapp.ErrNotFound) || err == nil && cl.SignIn != signindomain.Gated {
		return app.Client{}, app.ErrUnknownHost
	}
	if err != nil {
		return app.Client{}, err
	}
	return app.Client{ID: cl.ID, Label: cl.Label, Host: cl.Host}, nil
}

func (c *Contexts) Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error) {
	return c.signIn.Roles(ctx, userID, clientID)
}

func (c *Contexts) SignedIn(ctx context.Context, token string) (app.Person, error) {
	return person(c.users.SignedIn(ctx, token))
}

func (c *Contexts) Vouch(ctx context.Context, sessionID uuid.UUID) (app.Person, error) {
	return person(c.users.Vouch(ctx, sessionID))
}

func person(u usersdomain.User, s usersapp.Session, err error) (app.Person, error) {
	if errors.Is(err, usersapp.ErrNoSession) {
		return app.Person{}, app.ErrNoSignIn
	}
	if err != nil {
		return app.Person{}, err
	}
	return app.Person{UserID: u.ID, Email: u.Email.String(), Name: u.Name.String(), SessionID: s.ID}, nil
}
