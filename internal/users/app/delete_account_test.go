package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
)

func TestAUserDeletesTheirAccountWithTheirPassword(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	session, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	if err := f.DeleteAccount(context.Background(), id, session, pw); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.store.users[id]; ok || len(f.store.sessions) != 0 {
		t.Fatal("the User or a session is still there")
	}
}

func TestDeletingAnAccountIsAnAuditEvent(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	session, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	*f.events = nil
	_ = f.DeleteAccount(context.Background(), id, session, pw)
	if len(*f.events) != 1 || (*f.events)[0] != (audit.Event{Action: audit.AccountDeleted, User: id}) {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestDeletingAnAccountEndsItsSessionsAndTellsTheirApps(t *testing.T) {
	var told []uuid.UUID
	f := newFixture(app.WithSignedOut(func(_ context.Context, _ uuid.UUID, ids []uuid.UUID) { told = append(told, ids...) }))
	id := f.signedUp(t, "ada@example.org")
	session, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	sessionID := f.store.sessions[sum(session)].ID
	_ = f.DeleteAccount(context.Background(), id, session, pw)
	if len(told) != 1 || told[0] != sessionID {
		t.Fatalf("told %v", told)
	}
}

func TestDeletingAnAccountForgetsItsPersonalTokens(t *testing.T) {
	var forgotten []uuid.UUID
	f := newFixture(app.WithTokensDeleted(func(_ context.Context, ids []uuid.UUID) { forgotten = append(forgotten, ids...) }))
	id := f.signedUp(t, "ada@example.org")
	token := uuid.New()
	f.store.tokens = map[uuid.UUID][]uuid.UUID{id: {token}}
	session, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	_ = f.DeleteAccount(context.Background(), id, session, pw)
	if len(forgotten) != 1 || forgotten[0] != token {
		t.Fatalf("forgot %v", forgotten)
	}
}

func TestDeletingAnAccountWithNoTokensForgetsNone(t *testing.T) {
	called := false
	f := newFixture(app.WithTokensDeleted(func(context.Context, []uuid.UUID) { called = true }))
	id := f.signedUp(t, "ada@example.org")
	session, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	if err := f.DeleteAccount(context.Background(), id, session, pw); err != nil || called {
		t.Fatalf("err %v, called %v", err, called)
	}
}

func TestDeletingAnAccountNeedsThePassword(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	session, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	if err := f.DeleteAccount(context.Background(), id, session, "wrong"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := f.store.users[id]; !ok {
		t.Fatal("the User was deleted")
	}
}

func TestAUserWithTwoFactorDeletesOnlyFromASessionThatProvedItWithinFiveMinutes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		after time.Duration
		want  error
	}{
		{"just proved", 5 * time.Minute, nil},
		{"proved too long ago", 5*time.Minute + time.Second, app.ErrFreshTwoFactorNeeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			id, _, session := f.enrolled(t, "ada@example.org")
			f.now = t0.Add(tc.after)
			if err := f.DeleteAccount(context.Background(), id, session, pw); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAUserWithTwoFactorWhoseSessionNeverProvedItCannotDelete(t *testing.T) {
	f := newFixture()
	id, _, session := f.enrolled(t, "ada@example.org")
	s := f.store.sessions[sum(session)]
	s.TwoFactorAt = nil
	f.store.sessions[sum(session)] = s
	if err := f.DeleteAccount(context.Background(), id, session, pw); !errors.Is(err, app.ErrFreshTwoFactorNeeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeletingAnAccountFailsWhenTheStoreDoes(t *testing.T) {
	for _, method := range []string{"TOTPFactors", "SessionByHash", "DeleteUser"} {
		t.Run(method, func(t *testing.T) {
			f := newFixture()
			id, _, session := f.enrolled(t, "ada@example.org")
			f.store.fail[method] = errBoomSF
			if err := f.DeleteAccount(context.Background(), id, session, pw); !errors.Is(err, errBoomSF) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
