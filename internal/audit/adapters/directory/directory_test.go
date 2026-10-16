package directory_test

import (
	"context"
	"errors"
	"testing"

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

func TestPeopleAreUsersByNameAndEmail(t *testing.T) {
	t.Parallel()
	e, _ := usersdomain.NewEmail("ada@example.org")
	n, _ := usersdomain.NewName("Ada")
	id := uuid.New()
	got, err := directory.New(users{list: []usersdomain.User{{ID: id, Email: e, Name: n, PasswordHash: "h"}}}, clients.NewIndex()).People(context.Background())
	if err != nil || len(got) != 1 || got[0] != (app.Person{ID: id, Name: "Ada", Email: "ada@example.org"}) {
		t.Fatalf("People = %+v, %v", got, err)
	}
}

func TestPeopleFailWhenTheUsersDo(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	if _, err := directory.New(users{err: boom}, clients.NewIndex()).People(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("People = %v", err)
	}
}

func TestAClientIsCalledByItsLabelOrElseItsID(t *testing.T) {
	t.Parallel()
	index := clients.NewIndex()
	index.Put(signindomain.Client{ID: "grafana", SignIn: signindomain.Native, Host: "grafana.test", Label: "Grafana", Roles: []string{"user"}})
	index.Put(signindomain.Client{ID: "backup", SignIn: signindomain.Token, Roles: []string{"user"}})
	d := directory.New(users{}, index)
	for id, want := range map[string]string{"grafana": "Grafana", "backup": "backup", "gone": "gone"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			if got := d.ClientLabel(context.Background(), id); got != want {
				t.Fatalf("label = %q, want %q", got, want)
			}
		})
	}
}
