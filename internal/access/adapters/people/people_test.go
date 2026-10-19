package people_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/adapters/people"
	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	usersdomain "github.com/JorisJonkers-dev/identity/internal/users/domain"
)

type users struct {
	list      []usersdomain.User
	err       error
	signedOut *[2]uuid.UUID
	twoFactor []uuid.UUID
	factorErr error
}

func (u users) Users(context.Context) ([]usersdomain.User, error) { return u.list, u.err }

func (u users) UsersWithTwoFactor(context.Context) ([]uuid.UUID, error) {
	return u.twoFactor, u.factorErr
}

func (u users) SignOutEverywhere(_ context.Context, actor, id uuid.UUID) error {
	*u.signedOut = [2]uuid.UUID{actor, id}
	return u.err
}

func TestPeopleAreUsersWithoutTheirSecrets(t *testing.T) {
	e, _ := usersdomain.NewEmail("ada@example.org")
	n, _ := usersdomain.NewName("Ada")
	id := uuid.New()
	got, err := people.New(users{list: []usersdomain.User{{ID: id, Email: e, Name: n, PasswordHash: "h"}}}).People(context.Background())
	if err != nil || len(got) != 1 || got[0] != (domain.Person{ID: id, Email: "ada@example.org", Name: "Ada"}) {
		t.Fatalf("People = %+v, %v", got, err)
	}
	boom := errors.New("boom")
	if _, err := people.New(users{err: boom}).People(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("a failing users context: %v", err)
	}
}

func TestAPersonSaysWhetherTheyHaveTwoFactor(t *testing.T) {
	e, _ := usersdomain.NewEmail("ada@example.org")
	n, _ := usersdomain.NewName("Ada")
	with, without := uuid.New(), uuid.New()
	got, err := people.New(users{list: []usersdomain.User{{ID: with, Email: e, Name: n}, {ID: without, Email: e, Name: n}}, twoFactor: []uuid.UUID{with}}).People(context.Background())
	if err != nil || !got[0].TwoFactor || got[1].TwoFactor {
		t.Fatalf("People = %+v, %v", got, err)
	}
}

func TestPeopleFailWhenWhoHasTwoFactorCannotBeRead(t *testing.T) {
	boom := errors.New("boom")
	if _, err := people.New(users{factorErr: boom}).People(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestSigningOutAPersonSignsTheUserOutEverywhereAtTheActorsAsking(t *testing.T) {
	var got [2]uuid.UUID
	actor, id := uuid.New(), uuid.New()
	if err := people.New(users{signedOut: &got}).SignOut(context.Background(), actor, id); err != nil || got != [2]uuid.UUID{actor, id} {
		t.Fatalf("SignOut = %v, signed out %v", err, got)
	}
}
