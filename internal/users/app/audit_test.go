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

// only is the one recorded event with action, failing t unless there is exactly one.
func (f *fixture) only(t *testing.T, action audit.Action) audit.Event {
	t.Helper()
	var found []audit.Event
	for _, e := range *f.events {
		if e.Action == action {
			found = append(found, e)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d %s events, want 1: %+v", len(found), action, *f.events)
	}
	return found[0]
}

func (f *fixture) none(t *testing.T, action audit.Action) {
	t.Helper()
	for _, e := range *f.events {
		if e.Action == action {
			t.Fatalf("unexpected %s event: %+v", action, e)
		}
	}
}

func TestConfirmingASignUpRecordsTheNewUser(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	if e := f.only(t, audit.UserCreated); e.User != id {
		t.Fatalf("event = %+v", e)
	}
}

func TestASignInRecordsWhoSignedIn(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	if _, err := f.SignIn(context.Background(), "ada@example.org", pw); err != nil {
		t.Fatal(err)
	}
	if e := f.only(t, audit.SignedIn); e.User != id || e.Actor != uuid.Nil {
		t.Fatalf("event = %+v", e)
	}
}

func TestAWrongPasswordRecordsAFailedSignInForThatUser(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	_, _ = f.SignIn(context.Background(), "ADA@example.org", "not the password")
	if e := f.only(t, audit.SignInFailed); e != (audit.Event{Action: audit.SignInFailed, User: id, Email: "ada@example.org", Reason: audit.ReasonPassword}) {
		t.Fatalf("event = %+v", e)
	}
}

func TestAnUnknownEmailRecordsAFailedSignInNamingOnlyTheEmail(t *testing.T) {
	f := newFixture()
	_, _ = f.SignIn(context.Background(), "nobody@example.org", pw)
	if e := f.only(t, audit.SignInFailed); e != (audit.Event{Action: audit.SignInFailed, Email: "nobody@example.org", Reason: audit.ReasonPassword}) {
		t.Fatalf("event = %+v", e)
	}
}

func TestAnEmailThatIsNoAddressRecordsAFailedSignInNamingNothing(t *testing.T) {
	f := newFixture()
	_, _ = f.SignIn(context.Background(), "not an address", pw)
	if e := f.only(t, audit.SignInFailed); e != (audit.Event{Action: audit.SignInFailed, Reason: audit.ReasonPassword}) {
		t.Fatalf("event = %+v", e)
	}
}

func TestASignInThatWaitsForASecondFactorRecordsNoSignInYet(t *testing.T) {
	f := newFixture()
	f.enrolled(t, "ada@example.org")
	*f.events = nil
	if _, err := f.SignIn(context.Background(), "ada@example.org", pw); !errors.Is(err, app.ErrSecondFactorNeeded) {
		t.Fatal(err)
	}
	if len(*f.events) != 0 {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestAWrongCodeRecordsAFailedSignIn(t *testing.T) {
	for _, tc := range []struct {
		name string
		// spent uses the right code once first, so the second use is refused.
		spent bool
	}{
		{name: "a code no authenticator shows"},
		{name: "a code already used", spent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			id, secret, _ := f.enrolled(t, "ada@example.org")
			f.now = t0.Add(time.Hour)
			wrong := "000000"
			if tc.spent {
				wrong = code(t, secret, f.now)
				first, _ := f.SignIn(context.Background(), "ada@example.org", pw)
				if _, err := f.CompleteSignIn(context.Background(), first, wrong); err != nil {
					t.Fatal(err)
				}
			}
			challenge, _ := f.SignIn(context.Background(), "ada@example.org", pw)
			*f.events = nil
			if _, err := f.CompleteSignIn(context.Background(), challenge, wrong); !errors.Is(err, app.ErrInvalidCode) {
				t.Fatalf("CompleteSignIn = %v", err)
			}
			if e := f.only(t, audit.SignInFailed); e != (audit.Event{Action: audit.SignInFailed, User: id, Reason: audit.ReasonSecondFactor}) {
				t.Fatalf("event = %+v", e)
			}
		})
	}
}

func TestARightCodeRecordsTheSignIn(t *testing.T) {
	f := newFixture()
	id, secret, _ := f.enrolled(t, "ada@example.org")
	f.now = t0.Add(time.Hour)
	challenge, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	*f.events = nil
	if _, err := f.CompleteSignIn(context.Background(), challenge, code(t, secret, f.now)); err != nil {
		t.Fatal(err)
	}
	if e := f.only(t, audit.SignedIn); e.User != id {
		t.Fatalf("event = %+v", e)
	}
}

func TestARefusedPasskeyRecordsAFailedSignIn(t *testing.T) {
	for _, tc := range []struct {
		name      string
		options   bool
		assertion func(id uuid.UUID) string
		stale     bool
	}{
		{name: "before the passkey step began", assertion: func(id uuid.UUID) string { return "signed:login-" + id.String() + "|cred-1" }},
		{name: "an assertion WebAuthn refuses", options: true, assertion: func(uuid.UUID) string { return "signed:other|cred-1" }},
		{name: "a passkey the User does not have", options: true, assertion: func(id uuid.UUID) string { return "signed:login-" + id.String() + "|ghost" }},
		{name: "a counter that went stale", options: true, stale: true, assertion: func(id uuid.UUID) string { return "signed:login-" + id.String() + "|cred-1" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			id, _ := f.withPasskey(t)
			f.now = t0.Add(time.Hour)
			challenge, _ := f.SignIn(context.Background(), "ada@example.org", pw)
			if tc.options {
				_, _ = f.PasskeyOptions(context.Background(), challenge)
			}
			if tc.stale {
				f.wa.staleUse = func() { f.store.passkeys[0].Credential = []byte(`{"id":"cred-1","count":8}`) }
			}
			*f.events = nil
			if _, err := f.CompleteSignInWithPasskey(context.Background(), challenge, []byte(tc.assertion(id))); !errors.Is(err, app.ErrInvalidPasskey) {
				t.Fatalf("CompleteSignInWithPasskey = %v", err)
			}
			if e := f.only(t, audit.SignInFailed); e != (audit.Event{Action: audit.SignInFailed, User: id, Reason: audit.ReasonSecondFactor}) {
				t.Fatalf("event = %+v", e)
			}
		})
	}
}

func TestSigningOutRecordsItOnce(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	session, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	_, _ = f.SignIn(context.Background(), "ada@example.org", pw)
	if err := f.SignOut(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if e := f.only(t, audit.SignedOut); e.User != id || e.Actor != uuid.Nil {
		t.Fatalf("event = %+v", e)
	}
}

func TestAnAdminSigningAUserOutIsRecordedAsTheAdmins(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	_, _ = f.SignIn(context.Background(), "ada@example.org", pw)
	admin := uuid.New()
	if err := f.SignOutEverywhere(context.Background(), admin, id); err != nil {
		t.Fatal(err)
	}
	if e := f.only(t, audit.SignedOut); e.User != id || e.Actor != admin {
		t.Fatalf("event = %+v", e)
	}
}

func TestSigningOutAUserWithNoSessionRecordsNothing(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	if err := f.SignOutEverywhere(context.Background(), uuid.New(), id); err != nil {
		t.Fatal(err)
	}
	f.none(t, audit.SignedOut)
}

func TestEachProfileChangeIsRecorded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		action audit.Action
		change func(t *testing.T, f *fixture, id uuid.UUID, session string) error
	}{
		{"renaming", audit.UserRenamed, func(_ *testing.T, f *fixture, id uuid.UUID, _ string) error {
			return f.Rename(context.Background(), id, "Ada King")
		}},
		{"changing the password", audit.PasswordChanged, func(_ *testing.T, f *fixture, id uuid.UUID, session string) error {
			return f.ChangePassword(context.Background(), id, session, pw, "another long password")
		}},
		{"adding an authenticator app", audit.SecondFactorAdded, func(t *testing.T, f *fixture, id uuid.UUID, session string) error {
			setup, err := f.StartTOTP(context.Background(), id)
			if err != nil {
				return err
			}
			return f.ConfirmTOTP(context.Background(), id, session, "Phone", code(t, setup.Secret, f.now), pw)
		}},
		{"adding a passkey", audit.SecondFactorAdded, func(_ *testing.T, f *fixture, id uuid.UUID, session string) error {
			if _, err := f.StartPasskey(context.Background(), id); err != nil {
				return err
			}
			return f.ConfirmPasskey(context.Background(), id, session, "MacBook", []byte("made:reg-"+id.String()), pw)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			id := f.signedUp(t, "ada@example.org")
			session, _ := f.SignIn(context.Background(), "ada@example.org", pw)
			if err := tc.change(t, f, id, session); err != nil {
				t.Fatal(err)
			}
			if e := f.only(t, tc.action); e != (audit.Event{Action: tc.action, User: id}) {
				t.Fatalf("event = %+v", e)
			}
		})
	}
}

func TestARefusedProfileChangeRecordsNothing(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	f.store.fail["UpdateName"] = errors.New("boom")
	_ = f.Rename(context.Background(), id, "Ada King")
	f.none(t, audit.UserRenamed)
}

func TestRemovingASecondFactorIsRecorded(t *testing.T) {
	for name, setUp := range map[string]func(*testing.T, *fixture) (uuid.UUID, uuid.UUID, string){
		"an authenticator app": func(t *testing.T, f *fixture) (uuid.UUID, uuid.UUID, string) {
			id, _, session := f.enrolled(t, "ada@example.org")
			return id, f.store.factors[0].ID, session
		},
		"a passkey": func(t *testing.T, f *fixture) (uuid.UUID, uuid.UUID, string) {
			id, session := f.withPasskey(t)
			return id, f.store.passkeys[0].ID, session
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			id, factor, session := setUp(t, f)
			if err := f.RemoveFactor(context.Background(), id, factor, session, pw); err != nil {
				t.Fatal(err)
			}
			if e := f.only(t, audit.SecondFactorRemoved); e != (audit.Event{Action: audit.SecondFactorRemoved, User: id}) {
				t.Fatalf("event = %+v", e)
			}
		})
	}
}

func TestRemovingASecondFactorTheUserDoesNotHaveRecordsNothing(t *testing.T) {
	f := newFixture()
	id, _, session := f.enrolled(t, "ada@example.org")
	_ = f.RemoveFactor(context.Background(), id, uuid.New(), session, pw)
	f.none(t, audit.SecondFactorRemoved)
}

func TestWithoutAnAuditNothingIsRecordedAndSigningInStillWorks(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	bare := app.New(f.store, f.hasher, f.mailer, func() time.Time { return f.now }, readerFunc(func(p []byte) (int, error) { return len(p), nil }), "https://auth.test")
	if _, err := bare.SignIn(context.Background(), "ada@example.org", pw); err != nil {
		t.Fatal(err)
	}
	f.none(t, audit.SignedIn)
}
