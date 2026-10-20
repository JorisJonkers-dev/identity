// Package contexts is the Gate's view of the sign-in and users contexts: gated Clients by host,
// App Roles, and Single sign-on sessions, translated at the boundary.
package contexts

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	connectedapp "github.com/JorisJonkers-dev/identity/internal/connected/app"
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
	HasTwoFactor(ctx context.Context, userID uuid.UUID) (bool, error)
}

// StepUps is each Client's step-up window in minutes, 0 when it is off.
type StepUps interface {
	StepUp(ctx context.Context, clientID string) (int, error)
}

// MCPResources tells which Clients take Connected Apps (identity#23).
type MCPResources interface {
	IsMCPResource(ctx context.Context, clientID string) (bool, error)
}

// AppTokens checks a Connected App's access token for a Client; connectedapp.ErrInvalidToken for
// one that is not active.
type AppTokens interface {
	Check(ctx context.Context, token, clientID string) (connectedapp.AccessToken, []string, error)
}

// appTokenPrefix starts a Connected App's access token.
const appTokenPrefix = "idapp_"

// Contexts is the app.Clients, app.Roles, app.Accounts and app.Tokens.
type Contexts struct {
	signIn    SignIn
	users     Users
	stepUps   StepUps
	mcp       MCPResources
	appTokens AppTokens
}

// SetConnectedApps lets MCP resources' hosts take Connected Apps' tokens; until it is called, none
// does. It is set once, before the Gate serves.
func (c *Contexts) SetConnectedApps(mcp MCPResources, tokens AppTokens) {
	c.mcp, c.appTokens = mcp, tokens
}

// New returns Contexts over the three.
func New(signIn SignIn, users Users, stepUps StepUps) *Contexts {
	return &Contexts{signIn: signIn, users: users, stepUps: stepUps}
}

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
	mcp := false
	if c.mcp != nil {
		if mcp, err = c.mcp.IsMCPResource(ctx, cl.ID); err != nil {
			return app.Client{}, err
		}
	}
	return app.Client{ID: cl.ID, Label: cl.Label, Host: cl.Host, MCP: mcp}, nil
}

// Bearer checks a Personal Token for a Client through sign-in, which owns the check
// (docs/adr/0017): a token that names nothing, or is not active, is ErrNoSignIn.
func (c *Contexts) Bearer(ctx context.Context, secret, clientID string) (app.Person, []string, error) {
	if strings.HasPrefix(secret, appTokenPrefix) {
		return c.appBearer(ctx, secret, clientID)
	}
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
	return app.Person{UserID: u.ID, Email: u.Email.String(), Name: u.Name.String(), SessionID: t.ID}, t.Roles, nil
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
	minutes, err := c.stepUps.StepUp(ctx, cl.ID)
	if err != nil {
		return app.Client{}, err
	}
	return app.Client{ID: cl.ID, Label: cl.Label, Host: cl.Host, StepUp: time.Duration(minutes) * time.Minute}, nil
}

func (c *Contexts) Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error) {
	return c.signIn.Roles(ctx, userID, clientID)
}

// SignedIn is the User of a Single sign-on session, with when it last proved two-factor and
// whether they have it, for a Step-up Client.
func (c *Contexts) SignedIn(ctx context.Context, token string) (app.Person, error) {
	p, err := person(c.users.SignedIn(ctx, token))
	if err != nil {
		return app.Person{}, err
	}
	p.TwoFactor, err = c.users.HasTwoFactor(ctx, p.UserID)
	return p, err
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
	p := app.Person{UserID: u.ID, Email: u.Email.String(), Name: u.Name.String(), SessionID: s.ID}
	if s.TwoFactorAt != nil {
		p.TwoFactorAt = *s.TwoFactorAt
	}
	return p, nil
}

// appBearer checks a Connected App's token through the connected context; its cached checks are
// kept under the token's id, so revoking the Connected App can Forget them.
func (c *Contexts) appBearer(ctx context.Context, token, clientID string) (app.Person, []string, error) {
	if c.appTokens == nil {
		return app.Person{}, nil, app.ErrNoSignIn
	}
	t, roles, err := c.appTokens.Check(ctx, token, clientID)
	if errors.Is(err, connectedapp.ErrInvalidToken) {
		return app.Person{}, nil, app.ErrNoSignIn
	}
	if err != nil {
		return app.Person{}, nil, err
	}
	u, err := c.users.User(ctx, t.UserID)
	if err != nil {
		return app.Person{}, nil, err
	}
	return app.Person{UserID: u.ID, Email: u.Email.String(), Name: u.Name.String(), SessionID: t.ID}, roles, nil
}
