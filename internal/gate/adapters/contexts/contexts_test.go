package contexts_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/gate/adapters/contexts"
	"github.com/JorisJonkers-dev/identity/internal/gate/app"
	signinapp "github.com/JorisJonkers-dev/identity/internal/signin/app"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
	usersapp "github.com/JorisJonkers-dev/identity/internal/users/app"
	usersdomain "github.com/JorisJonkers-dev/identity/internal/users/domain"
)

var errBoom = errors.New("boom")

type signIn map[string]signindomain.Client

func (s signIn) ClientForHost(_ context.Context, host string) (signindomain.Client, error) {
	if host == "broken" {
		return signindomain.Client{}, errBoom
	}
	c, ok := s[host]
	if !ok {
		return signindomain.Client{}, signinapp.ErrNotFound
	}
	return c, nil
}

func (s signIn) Roles(context.Context, uuid.UUID, string) ([]string, error) {
	return []string{"user"}, nil
}

type users struct {
	user usersdomain.User
	sess usersapp.Session
	err  error
}

func (u users) SignedIn(context.Context, string) (usersdomain.User, usersapp.Session, error) {
	return u.user, u.sess, u.err
}

func (u users) Vouch(context.Context, uuid.UUID) (usersdomain.User, usersapp.Session, error) {
	return u.user, u.sess, u.err
}

func TestOnlyAGatedClientIsTheGates(t *testing.T) {
	c := contexts.New(signIn{
		"stats.jorisjonkers.dev":   {ID: "stats", Label: "Stats", Host: "stats.jorisjonkers.dev", SignIn: signindomain.Gated},
		"grafana.jorisjonkers.dev": {ID: "grafana", Host: "grafana.jorisjonkers.dev", SignIn: signindomain.Native},
	}, users{})
	ctx := context.Background()
	if got, err := c.GatedClient(ctx, "stats.jorisjonkers.dev"); err != nil || got != (app.Client{ID: "stats", Label: "Stats", Host: "stats.jorisjonkers.dev"}) {
		t.Fatalf("GatedClient = %+v, %v", got, err)
	}
	for _, host := range []string{"grafana.jorisjonkers.dev", "nobody.jorisjonkers.dev"} {
		if _, err := c.GatedClient(ctx, host); !errors.Is(err, app.ErrUnknownHost) {
			t.Errorf("GatedClient(%s) = %v", host, err)
		}
	}
	if _, err := c.GatedClient(ctx, "broken"); !errors.Is(err, errBoom) {
		t.Errorf("a failing lookup = %v", err)
	}
	if roles, _ := c.Roles(ctx, uuid.New(), "stats"); len(roles) != 1 {
		t.Errorf("Roles = %v", roles)
	}
}

func TestAccountsTranslateTheSession(t *testing.T) {
	e, _ := usersdomain.NewEmail("ada@example.org")
	n, _ := usersdomain.NewName("Ada")
	u := usersdomain.User{ID: uuid.New(), Email: e, Name: n}
	s := usersapp.Session{ID: uuid.New()}
	ctx := context.Background()
	c := contexts.New(signIn{}, users{user: u, sess: s})
	want := app.Person{UserID: u.ID, Email: "ada@example.org", Name: "Ada", SessionID: s.ID}
	if p, err := c.SignedIn(ctx, "sso"); err != nil || p != want {
		t.Fatalf("SignedIn = %+v, %v", p, err)
	}
	if p, err := c.Vouch(ctx, s.ID); err != nil || p != want {
		t.Fatalf("Vouch = %+v, %v", p, err)
	}
	if _, err := contexts.New(signIn{}, users{err: usersapp.ErrNoSession}).Vouch(ctx, s.ID); !errors.Is(err, app.ErrNoSignIn) {
		t.Fatalf("no session = %v", err)
	}
	if _, err := contexts.New(signIn{}, users{err: errBoom}).SignedIn(ctx, "sso"); !errors.Is(err, errBoom) {
		t.Fatalf("a failure = %v", err)
	}
}
