package directory_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/audit/adapters/directory"
	"github.com/JorisJonkers-dev/identity/internal/audit/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
	usersdomain "github.com/JorisJonkers-dev/identity/internal/users/domain"
)

type users struct {
	list []usersdomain.User
	err  error
}

func (u users) Users(context.Context) ([]usersdomain.User, error) { return u.list, u.err }

func (u users) ActiveSessions(context.Context) (sessions, users int, err error) { return 3, 2, u.err }

type access struct{ ids []uuid.UUID }

type tokens struct{}

func (tokens) ExpiringSoon(context.Context) (int, error) { return 4, nil }

func (a access) UnassignedUsers(context.Context) ([]uuid.UUID, error) { return a.ids, nil }

var joined = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func TestPeopleAreUsersByNameAndEmail(t *testing.T) {
	t.Parallel()
	e, _ := usersdomain.NewEmail("ada@example.org")
	n, _ := usersdomain.NewName("Ada")
	id := uuid.New()
	got, err := directory.New(users{list: []usersdomain.User{{ID: id, Email: e, Name: n, PasswordHash: "h", CreatedAt: joined}}}, access{}, tokens{}, clients.NewIndex()).People(context.Background())
	if err != nil || len(got) != 1 || got[0] != (app.Person{ID: id, Name: "Ada", Email: "ada@example.org", CreatedAt: joined}) {
		t.Fatalf("People = %+v, %v", got, err)
	}
}

func TestPeopleFailWhenTheUsersDo(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	if _, err := directory.New(users{err: boom}, access{}, tokens{}, clients.NewIndex()).People(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("People = %v", err)
	}
}

func TestAClientIsCalledByItsLabelOrElseItsID(t *testing.T) {
	t.Parallel()
	index := clients.NewIndex()
	index.Put(signindomain.Client{ID: "grafana", SignIn: signindomain.Native, Host: "grafana.test", Label: "Grafana", Roles: []string{"user"}})
	index.Put(signindomain.Client{ID: "backup", SignIn: signindomain.Token, Roles: []string{"user"}})
	d := directory.New(users{}, access{}, tokens{}, index)
	for id, want := range map[string]string{"grafana": "Grafana", "backup": "backup", "gone": "gone"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			if got := d.ClientLabel(context.Background(), id); got != want {
				t.Fatalf("label = %q, want %q", got, want)
			}
		})
	}
}

func TestTheUnassignedComeFromAccess(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	if got, err := directory.New(users{}, access{ids: []uuid.UUID{id}}, tokens{}, clients.NewIndex()).Unassigned(context.Background()); err != nil || !slices.Equal(got, []uuid.UUID{id}) {
		t.Fatalf("Unassigned = %v, %v", got, err)
	}
}

func TestActiveSessionsComeFromTheUsers(t *testing.T) {
	t.Parallel()
	if sessions, holders, err := directory.New(users{}, access{}, tokens{}, clients.NewIndex()).ActiveSessions(context.Background()); err != nil || sessions != 3 || holders != 2 {
		t.Fatalf("ActiveSessions = %d, %d, %v", sessions, holders, err)
	}
}

func TestTokensExpiringComeFromTheTokens(t *testing.T) {
	t.Parallel()
	if n, err := directory.New(users{}, access{}, tokens{}, clients.NewIndex()).TokensExpiring(context.Background()); err != nil || n != 4 {
		t.Fatalf("TokensExpiring = %d, %v", n, err)
	}
}
