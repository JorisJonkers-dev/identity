package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
)

var root = uuid.MustParse("00000000-0000-4000-8000-00000000a0a0")

// withRoot is a fixture where root is an Admin, and Ada is signed up.
func withRoot(t *testing.T) (*fixture, uuid.UUID) {
	t.Helper()
	f := newFixture()
	f.admins.ids[root] = true
	return f, f.signedUp(t, "ada@example.org")
}

var asRoot = app.Actor{ID: root, TwoFactor: true}

func TestAnAdminRenamesAPersonAsAnAuditEventNamingThem(t *testing.T) {
	f, ada := withRoot(t)
	*f.events = nil
	if err := f.RenameAs(context.Background(), asRoot, ada, " Ada King "); err != nil {
		t.Fatal(err)
	}
	if f.store.users[ada].Name.String() != "Ada King" || len(*f.events) != 1 || (*f.events)[0] != (audit.Event{Action: audit.UserRenamed, Actor: root, User: ada}) {
		t.Fatalf("name %q, events %+v", f.store.users[ada].Name, *f.events)
	}
}

func TestAnAdminsRenameNeedsAName(t *testing.T) {
	f, ada := withRoot(t)
	if err := f.RenameAs(context.Background(), asRoot, ada, " "); err == nil {
		t.Fatal("an empty name was kept")
	}
}

func TestAnAdminStartsAPersonsEmailChange(t *testing.T) {
	f, ada := withRoot(t)
	if err := f.AskEmailChangeAs(context.Background(), asRoot, ada, "lovelace@example.org"); err != nil {
		t.Fatal(err)
	}
	if pending, _ := f.PendingEmail(context.Background(), ada); pending != "lovelace@example.org" || f.mailer.sent[len(f.mailer.sent)-1].To != "lovelace@example.org" {
		t.Fatalf("pending %q, mail %+v", pending, f.mailer.sent[len(f.mailer.sent)-1])
	}
}

func TestAnAdminsEmailChangeIsRefusedForSomethingThatIsNotAnEmailOrSomeoneWhoIsGone(t *testing.T) {
	for _, tc := range []struct {
		name, email string
		gone        bool
	}{{"not an email", "lovelace", false}, {"someone who is gone", "lovelace@example.org", true}} {
		t.Run(tc.name, func(t *testing.T) {
			f, ada := withRoot(t)
			if tc.gone {
				ada = uuid.New()
			}
			if err := f.AskEmailChangeAs(context.Background(), asRoot, ada, tc.email); err == nil {
				t.Fatal("the change was started")
			}
		})
	}
}

func TestAnAdminDeletesAPersonAsAnAuditEventNamingTheAdmin(t *testing.T) {
	f, ada := withRoot(t)
	*f.events = nil
	if err := f.DeleteAs(context.Background(), asRoot, ada); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.store.users[ada]; ok || len(*f.events) != 1 || (*f.events)[0] != (audit.Event{Action: audit.AccountDeleted, Actor: root, User: ada}) {
		t.Fatalf("still there %v, events %+v", ok, *f.events)
	}
}

func TestOnlyAnAdminWhoProvedTwoFactorManagesPeople(t *testing.T) {
	for _, tc := range []struct {
		name  string
		actor app.Actor
		want  error
	}{
		{"someone who is not an Admin", app.Actor{ID: uuid.New(), TwoFactor: true}, app.ErrNotAdmin},
		{"an Admin without a two-factor method", app.Actor{ID: root}, app.ErrAdminTwoFactorProofNeeded},
	} {
		for _, op := range []struct {
			name string
			call func(f *fixture, ada uuid.UUID) error
		}{
			{"renaming", func(f *fixture, ada uuid.UUID) error { return f.RenameAs(context.Background(), tc.actor, ada, "X") }},
			{"an email change", func(f *fixture, ada uuid.UUID) error {
				return f.AskEmailChangeAs(context.Background(), tc.actor, ada, "x@example.org")
			}},
			{"deleting", func(f *fixture, ada uuid.UUID) error { return f.DeleteAs(context.Background(), tc.actor, ada) }},
		} {
			t.Run(tc.name+", "+op.name, func(t *testing.T) {
				f, ada := withRoot(t)
				if err := op.call(f, ada); !errors.Is(err, tc.want) {
					t.Fatalf("err = %v, want %v", err, tc.want)
				}
			})
		}
	}
}

func TestPeopleAreManagedByNoOneWithoutAnAdminCheck(t *testing.T) {
	f := newFixture(app.WithAdmins(nil))
	ada := f.signedUp(t, "ada@example.org")
	if err := f.RenameAs(context.Background(), asRoot, ada, "X"); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("err = %v", err)
	}
}

func TestManagingAPersonFailsWhenTheAdminCheckOrTheStoreDoes(t *testing.T) {
	t.Run("the Admin check", func(t *testing.T) {
		f, ada := withRoot(t)
		f.admins.err = errBoomSF
		if err := f.RenameAs(context.Background(), asRoot, ada, "X"); !errors.Is(err, errBoomSF) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("the rename", func(t *testing.T) {
		f, ada := withRoot(t)
		f.store.fail["UpdateName"] = errBoomSF
		if err := f.RenameAs(context.Background(), asRoot, ada, "X"); !errors.Is(err, errBoomSF) {
			t.Fatalf("err = %v", err)
		}
	})
}
