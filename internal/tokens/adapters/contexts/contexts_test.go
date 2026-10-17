package contexts_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
	"github.com/JorisJonkers-dev/identity/internal/tokens/adapters/contexts"
)

var errBoom = errors.New("boom")

type signIn struct{ asked string }

func (s *signIn) Roles(_ context.Context, _ uuid.UUID, clientID string) ([]string, error) {
	s.asked = clientID
	return []string{"viewer"}, errBoom
}

type access struct{}

func (access) RolesByClient(context.Context, uuid.UUID) (map[string][]string, error) {
	return map[string][]string{"grafana": {"viewer"}}, errBoom
}

func index() *clients.Index {
	i := clients.NewIndex()
	i.Put(signindomain.Client{ID: "grafana", SignIn: signindomain.Native, Host: "grafana.test", Label: "Grafana", Roles: []string{"user"}})
	i.Put(signindomain.Client{ID: "backup", SignIn: signindomain.Token, Roles: []string{"user"}})
	return i
}

func TestRolesComeFromSignInWhichKnowsWhatTheClientStillDeclares(t *testing.T) {
	t.Parallel()
	s := &signIn{}
	roles, err := contexts.New(s, access{}, index()).Roles(context.Background(), uuid.New(), "grafana")
	if len(roles) != 1 || !errors.Is(err, errBoom) || s.asked != "grafana" {
		t.Fatalf("Roles = %v, %v", roles, err)
	}
}

func TestRolesPerClientComeFromAccess(t *testing.T) {
	t.Parallel()
	held, err := contexts.New(&signIn{}, access{}, index()).ByClient(context.Background(), uuid.New())
	if len(held["grafana"]) != 1 || !errors.Is(err, errBoom) {
		t.Fatalf("ByClient = %v, %v", held, err)
	}
}

func TestAClientExistsWhileTheClusterDeclaresIt(t *testing.T) {
	t.Parallel()
	c := contexts.New(&signIn{}, access{}, index())
	if !c.Exists(context.Background(), "grafana") || c.Exists(context.Background(), "gone") {
		t.Fatal("exists wrong")
	}
}

func TestAClientIsCalledByItsLabelOrElseItsID(t *testing.T) {
	t.Parallel()
	c := contexts.New(&signIn{}, access{}, index())
	for id, want := range map[string]string{"grafana": "Grafana", "backup": "backup", "gone": "gone"} {
		if got := c.Label(context.Background(), id); got != want {
			t.Fatalf("%s is %q, want %q", id, got, want)
		}
	}
}
