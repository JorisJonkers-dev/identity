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
	signedOut *uuid.UUID
}

func (u users) Users(context.Context) ([]usersdomain.User, error) { return u.list, u.err }

func (u users) SignOutEverywhere(_ context.Context, id uuid.UUID) error {
	*u.signedOut = id
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

func TestSigningOutAPersonSignsTheUserOutEverywhere(t *testing.T) {
	var got uuid.UUID
	id := uuid.New()
	if err := people.New(users{signedOut: &got}).SignOut(context.Background(), id); err != nil || got != id {
		t.Fatalf("SignOut = %v, signed out %v", err, got)
	}
}
