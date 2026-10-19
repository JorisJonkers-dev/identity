package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// changeLink is the token in the last mail's link to the new address.
func (f *fixture) changeLink(t *testing.T) string {
	t.Helper()
	text := f.mailer.sent[len(f.mailer.sent)-1].Text
	_, after, ok := strings.Cut(text, "https://auth.test/account/email?token=")
	if !ok {
		t.Fatalf("no link in %q", text)
	}
	return strings.Fields(after)[0]
}

// changing is Ada after asking to change her email to lovelace@example.org.
func changing(t *testing.T) (*fixture, uuid.UUID) {
	t.Helper()
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	if err := f.AskEmailChange(context.Background(), id, "lovelace@example.org", pw); err != nil {
		t.Fatal(err)
	}
	return f, id
}

func TestAskingForANewEmailMailsTheNewAddressALink(t *testing.T) {
	f, _ := changing(t)
	m := f.mailer.sent[len(f.mailer.sent)-1]
	if m.To != "lovelace@example.org" || m.Subject != "Confirm your new email" || !strings.Contains(m.Text, "for 24 hours") {
		t.Fatalf("mail = %+v", m)
	}
}

func TestANewEmailWaitsUntilItsLinkIsFollowed(t *testing.T) {
	f, id := changing(t)
	if pending, err := f.PendingEmail(context.Background(), id); pending != "lovelace@example.org" || err != nil {
		t.Fatalf("PendingEmail = %q, %v", pending, err)
	}
	if u := f.store.users[id]; u.Email.String() != "ada@example.org" {
		t.Fatalf("email = %s before the link", u.Email)
	}
}

func TestFollowingTheLinkChangesTheEmailAndTellsTheOldAddress(t *testing.T) {
	f, id := changing(t)
	if err := f.ConfirmEmailChange(context.Background(), f.changeLink(t)); err != nil {
		t.Fatal(err)
	}
	m := f.mailer.sent[len(f.mailer.sent)-1]
	if f.store.users[id].Email.String() != "lovelace@example.org" || m.To != "ada@example.org" || !strings.Contains(m.Text, "lovelace@example.org") {
		t.Fatalf("email %s, notice %+v", f.store.users[id].Email, m)
	}
}

func TestChangingTheEmailIsAnAuditEventNamingTheOldAddress(t *testing.T) {
	f, id := changing(t)
	*f.events = nil
	_ = f.ConfirmEmailChange(context.Background(), f.changeLink(t))
	if len(*f.events) != 1 || (*f.events)[0] != (audit.Event{Action: audit.EmailChanged, User: id, Email: "ada@example.org"}) {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestAskingForANewEmailNeedsThePasswordAndAValidAddress(t *testing.T) {
	for _, tc := range []struct {
		name, email, password string
		want                  error
	}{
		{"a wrong password", "lovelace@example.org", "wrong", app.ErrInvalidCredentials},
		{"an address that is not one", "lovelace", pw, domain.ErrInvalidEmail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			id := f.signedUp(t, "ada@example.org")
			sent := len(f.mailer.sent)
			if err := f.AskEmailChange(context.Background(), id, tc.email, tc.password); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v", err)
			}
			if len(f.mailer.sent) != sent || len(f.store.changes) != 0 {
				t.Fatal("a refused change was kept or mailed")
			}
		})
	}
}

func TestAnEmailChangeLinkFails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before func(f *fixture, id uuid.UUID)
	}{
		{"once spent", func(f *fixture, _ uuid.UUID) { _ = f.ConfirmEmailChange(context.Background(), f.changeLink(t)) }},
		{"after a day", func(f *fixture, _ uuid.UUID) { f.now = t0.Add(24 * time.Hour) }},
		{"once cancelled", func(f *fixture, id uuid.UUID) { _ = f.CancelEmailChange(context.Background(), id) }},
		{"when the address was taken since", func(f *fixture, _ uuid.UUID) { f.signedUp(t, "lovelace@example.org") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, id := changing(t)
			link := f.changeLink(t)
			tc.before(f, id)
			if err := f.ConfirmEmailChange(context.Background(), link); !errors.Is(err, app.ErrInvalidToken) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAnExpiredChangeIsNoLongerPending(t *testing.T) {
	f, id := changing(t)
	f.now = t0.Add(24 * time.Hour)
	if pending, err := f.PendingEmail(context.Background(), id); pending != "" || err != nil {
		t.Fatalf("PendingEmail = %q, %v", pending, err)
	}
}

func TestAUserWithNoChangeHasNonePending(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	if pending, err := f.PendingEmail(context.Background(), id); pending != "" || err != nil {
		t.Fatalf("PendingEmail = %q, %v", pending, err)
	}
}

func TestAnEmailChangeFailsWhenTheStoreDoes(t *testing.T) {
	for _, tc := range []struct {
		method string
		call   func(f *fixture, id uuid.UUID, link string) error
	}{
		{"SaveEmailChange", func(f *fixture, id uuid.UUID, _ string) error {
			return f.AskEmailChange(context.Background(), id, "new@example.org", pw)
		}},
		{"UserByID", func(f *fixture, id uuid.UUID, _ string) error {
			return f.AskEmailChange(context.Background(), id, "new@example.org", pw)
		}},
		{"EmailChangeOf", func(f *fixture, id uuid.UUID, _ string) error {
			_, err := f.PendingEmail(context.Background(), id)
			return err
		}},
		{"TakeEmailChange", func(f *fixture, _ uuid.UUID, link string) error {
			return f.ConfirmEmailChange(context.Background(), link)
		}},
		{"UpdateEmail", func(f *fixture, _ uuid.UUID, link string) error {
			return f.ConfirmEmailChange(context.Background(), link)
		}},
		{"DeleteEmailChange", func(f *fixture, id uuid.UUID, _ string) error { return f.CancelEmailChange(context.Background(), id) }},
	} {
		t.Run(tc.method, func(t *testing.T) {
			f, id := changing(t)
			link := f.changeLink(t)
			f.store.fail[tc.method] = errBoomSF
			if err := tc.call(f, id, link); !errors.Is(err, errBoomSF) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestConfirmingFailsWhenTheUserCannotBeRead(t *testing.T) {
	f, _ := changing(t)
	link := f.changeLink(t)
	f.store.fail["UserByID"] = errBoomSF
	if err := f.ConfirmEmailChange(context.Background(), link); !errors.Is(err, errBoomSF) {
		t.Fatalf("err = %v", err)
	}
}

func TestNoEmailChangeIsMadeWithoutRandomness(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	f.dry = true
	if err := f.AskEmailChange(context.Background(), id, "new@example.org", pw); err == nil {
		t.Fatal("a change was made without randomness")
	}
}
