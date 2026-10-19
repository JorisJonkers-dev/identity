package accounts_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/accounts"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	usersapp "github.com/JorisJonkers-dev/identity/internal/users/app"
	usersdomain "github.com/JorisJonkers-dev/identity/internal/users/domain"
)

type users struct {
	err       error
	session   usersapp.Session
	user      usersdomain.User
	signedOut uuid.UUID
	actor     uuid.UUID
	has       bool
}

func (u *users) SignedIn(context.Context, string) (usersdomain.User, usersapp.Session, error) {
	return u.user, u.session, u.err
}
func (u *users) User(context.Context, uuid.UUID) (usersdomain.User, error) { return u.user, u.err }
func (u *users) SignOutEverywhere(_ context.Context, actor, id uuid.UUID) error {
	u.signedOut, u.actor = id, actor
	return u.err
}

func (u *users) HasSecondFactor(context.Context, uuid.UUID) (bool, error) { return u.has, u.err }

func TestSignedInTranslatesTheSession(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	u := &users{session: usersapp.Session{ID: uuid.New(), UserID: uuid.New(), CreatedAt: at, Methods: []string{"pwd", "otp"}}}
	got, err := accounts.New(u).SignedIn(context.Background(), "tok")
	if err != nil || got.UserID != u.session.UserID || got.SessionID != u.session.ID || !got.At.Equal(at) ||
		len(got.Methods) != 2 || got.Methods[1] != "otp" {
		t.Fatalf("SignedIn = %+v, %v", got, err)
	}
	u.err = usersapp.ErrNoSession
	if _, err := accounts.New(u).SignedIn(context.Background(), "tok"); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("no session: %v", err)
	}
	boom := errors.New("boom")
	u.err = boom
	if _, err := accounts.New(u).SignedIn(context.Background(), "tok"); !errors.Is(err, boom) {
		t.Fatalf("a failure: %v", err)
	}
}

func TestClaimsAndSignOut(t *testing.T) {
	e, _ := usersdomain.NewEmail("ada@example.org")
	n, _ := usersdomain.NewName("Ada")
	u := &users{user: usersdomain.User{Email: e, Name: n}}
	a := accounts.New(u)
	if c, err := a.Claims(context.Background(), uuid.New()); err != nil || c != (app.Claims{Email: "ada@example.org", Name: "Ada"}) {
		t.Fatalf("Claims = %+v, %v", c, err)
	}
	id := uuid.New()
	u.actor = uuid.New()
	if err := a.SignOutEverywhere(context.Background(), id); err != nil || u.signedOut != id || u.actor != uuid.Nil {
		t.Fatalf("SignOutEverywhere = %v", err)
	}
	u.err = errors.New("boom")
	if _, err := a.Claims(context.Background(), id); err == nil {
		t.Fatal("a failing lookup gave claims")
	}
}

func TestSignedInSaysWhenTheSessionLastProvedTwoFactor(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	u := &users{session: usersapp.Session{TwoFactorAt: &at}}
	if got, err := accounts.New(u).SignedIn(context.Background(), "tok"); err != nil || !got.TwoFactorAt.Equal(at) {
		t.Fatalf("SignedIn = %+v, %v", got, err)
	}
}

func TestASessionThatNeverProvedTwoFactorHasNoTimeForIt(t *testing.T) {
	if got, err := accounts.New(&users{}).SignedIn(context.Background(), "tok"); err != nil || !got.TwoFactorAt.IsZero() {
		t.Fatalf("SignedIn = %+v, %v", got, err)
	}
}

func TestHasTwoFactorAsksTheUsersContext(t *testing.T) {
	if has, err := accounts.New(&users{has: true}).HasTwoFactor(context.Background(), uuid.New()); !has || err != nil {
		t.Fatalf("HasTwoFactor = %v, %v", has, err)
	}
}
