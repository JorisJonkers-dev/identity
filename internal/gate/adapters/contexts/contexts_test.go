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

// adaID holds the live Personal Token idpat_live; idpat_revoked is not active, and idpat_broken
// fails its check.
var adaID = uuid.MustParse("00000000-0000-4000-8000-0000000000a1")

func (signIn) ResolvePersonalToken(_ context.Context, secret string) (string, string, bool) {
	id, ok := map[string]string{"idpat_live": "pat-live", "idpat_revoked": "pat-revoked", "idpat_broken": "pat-broken"}[secret]
	return id, adaID.String(), ok
}

func (signIn) CheckPersonalToken(_ context.Context, tokenID, _ string) (signinapp.PersonalToken, error) {
	switch tokenID {
	case "pat-live":
		return signinapp.PersonalToken{UserID: adaID, Roles: []string{"viewer"}}, nil
	case "pat-revoked":
		return signinapp.PersonalToken{}, signinapp.ErrInactive
	}
	return signinapp.PersonalToken{}, errBoom
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

func (u users) User(context.Context, uuid.UUID) (usersdomain.User, error) { return u.user, u.err }

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

func TestAGatedOrTokenClientTakesPersonalTokens(t *testing.T) {
	c := contexts.New(signIn{
		"stats.jorisjonkers.dev":   {ID: "stats", Label: "Stats", Host: "stats.jorisjonkers.dev", SignIn: signindomain.Gated},
		"backup.jorisjonkers.dev":  {ID: "backup", Host: "backup.jorisjonkers.dev", SignIn: signindomain.Token},
		"grafana.jorisjonkers.dev": {ID: "grafana", Host: "grafana.jorisjonkers.dev", SignIn: signindomain.Native},
	}, users{})
	for host, want := range map[string]string{"stats.jorisjonkers.dev": "stats", "backup.jorisjonkers.dev": "backup"} {
		t.Run(host, func(t *testing.T) {
			if got, err := c.BearerClient(context.Background(), host); err != nil || got.ID != want {
				t.Fatalf("BearerClient = %+v, %v", got, err)
			}
		})
	}
	for host, want := range map[string]error{"grafana.jorisjonkers.dev": app.ErrUnknownHost, "nobody.jorisjonkers.dev": app.ErrUnknownHost, "broken": errBoom} {
		t.Run(host, func(t *testing.T) {
			if _, err := c.BearerClient(context.Background(), host); !errors.Is(err, want) {
				t.Fatalf("err = %v, want %v", err, want)
			}
		})
	}
}

func TestALivePersonalTokenAnswersItsUserAndRoles(t *testing.T) {
	email, _ := usersdomain.NewEmail("ada@example.org")
	name, _ := usersdomain.NewName("Ada")
	c := contexts.New(signIn{}, users{user: usersdomain.User{ID: adaID, Email: email, Name: name}})
	p, roles, err := c.Bearer(context.Background(), "idpat_live", "stats")
	if err != nil || p != (app.Person{UserID: adaID, Email: "ada@example.org", Name: "Ada"}) || len(roles) != 1 || roles[0] != "viewer" {
		t.Fatalf("Bearer = %+v, %v, %v", p, roles, err)
	}
}

func TestAPersonalTokenThatIsNotActiveIsNoSignIn(t *testing.T) {
	for _, secret := range []string{"idpat_unknown", "idpat_revoked"} {
		t.Run(secret, func(t *testing.T) {
			if _, _, err := contexts.New(signIn{}, users{}).Bearer(context.Background(), secret, "stats"); !errors.Is(err, app.ErrNoSignIn) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAPersonalTokenCheckFailsWithWhatItReads(t *testing.T) {
	for name, tc := range map[string]struct {
		secret string
		users  users
	}{
		"the token's check": {secret: "idpat_broken"},
		"its User":          {secret: "idpat_live", users: users{err: errBoom}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := contexts.New(signIn{}, tc.users).Bearer(context.Background(), tc.secret, "stats"); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
