package contexts_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/launcher/adapters/contexts"
	"github.com/JorisJonkers-dev/identity/internal/launcher/domain"
	signinapp "github.com/JorisJonkers-dev/identity/internal/signin/app"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var errBoom = errors.New("boom")

var (
	grafana = signindomain.Client{
		ID: "grafana", SignIn: signindomain.Native, Host: "grafana.test", Label: "Grafana", Description: "Metrics",
		LaunchURL: "https://grafana.test/d", IconURL: "https://grafana.test/icon.png", Roles: []string{"viewer"},
	}
	stats  = signindomain.Client{ID: "stats", SignIn: signindomain.Gated, Host: "stats.test", Label: "Stats"}
	backup = signindomain.Client{ID: "backup", SignIn: signindomain.Token}
)

type index []signindomain.Client

func (x index) All() []signindomain.Client { return x }

func (x index) Client(_ context.Context, id string) (signindomain.Client, error) {
	if id == "broken" {
		return signindomain.Client{}, errBoom
	}
	for _, c := range x {
		if c.ID == id {
			return c, nil
		}
	}
	return signindomain.Client{}, signinapp.ErrNotFound
}

type access map[string][]string

func (a access) RolesByClient(context.Context, uuid.UUID) (map[string][]string, error) { return a, nil }

func newContexts() *contexts.Contexts {
	return contexts.New(index{grafana, stats, backup}, access{"grafana": {"viewer"}})
}

func TestNativeAndGatedClientsCanBeLaunched(t *testing.T) {
	var ids []string
	for _, c := range newContexts().Launchable() {
		ids = append(ids, c.ID)
	}
	if !slices.Equal(ids, []string{"grafana", "stats"}) {
		t.Fatalf("launchable = %v, want grafana and stats", ids)
	}
}

func TestALaunchableClientKeepsWhatTheLauncherShows(t *testing.T) {
	want := domain.Client{
		ID: "grafana", Label: "Grafana", Description: "Metrics", Host: "grafana.test",
		LaunchURL: "https://grafana.test/d", IconURL: "https://grafana.test/icon.png",
	}
	if got := newContexts().Launchable()[0]; got != want {
		t.Fatalf("client = %+v, want %+v", got, want)
	}
}

func TestAClientIsFoundByID(t *testing.T) {
	c, ok := newContexts().Client("stats")
	if !ok || c.Label != "Stats" {
		t.Fatalf("Client(stats) = %+v, %v", c, ok)
	}
}

func TestATokenClientIsNotFound(t *testing.T) {
	if _, ok := newContexts().Client("backup"); ok {
		t.Fatal("a token Client was found")
	}
}

func TestAnUnknownClientIsNotFound(t *testing.T) {
	if _, ok := newContexts().Client("nobody"); ok {
		t.Fatal("an unknown Client was found")
	}
	if _, ok := newContexts().Client("broken"); ok {
		t.Fatal("a failing lookup found a Client")
	}
}

func TestRolesComeFromTheAccessContext(t *testing.T) {
	held, err := newContexts().RolesByClient(context.Background(), uuid.New())
	if err != nil || !slices.Equal(held["grafana"], []string{"viewer"}) {
		t.Fatalf("RolesByClient = %v, %v", held, err)
	}
}
