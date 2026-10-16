package app_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/gate/app"
)

func TestAGateSignInIsRecorded(t *testing.T) {
	f := newFixture()
	if _, err := f.Start(context.Background(), "sso", stats.Host, "s", "/"); err != nil {
		t.Fatal(err)
	}
	want := []audit.Event{{Action: audit.SignedIn, User: ada.UserID, Client: stats.ID}}
	if !slices.Equal(*f.events, want) {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestAGateSignInRefusedForWantOfAnAppRoleIsRecordedAsFailed(t *testing.T) {
	f := newFixture()
	f.roles.held["tribelt-stats"] = nil
	if _, err := f.Start(context.Background(), "sso", stats.Host, "s", "/"); !errors.Is(err, app.ErrNoAccess) {
		t.Fatal(err)
	}
	want := []audit.Event{{Action: audit.SignInFailed, User: ada.UserID, Client: stats.ID, Reason: audit.ReasonNoAccess}}
	if !slices.Equal(*f.events, want) {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestAGateSignInThatFailsOtherwiseRecordsNothing(t *testing.T) {
	for name, broken := range map[string]func(f *fixture){
		"the roles cannot be read": func(f *fixture) { f.roles.err = errBoom },
		"the code cannot be kept":  func(f *fixture) { f.store.fail["CreateCode"] = errBoom },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			broken(f)
			if _, err := f.Start(context.Background(), "sso", stats.Host, "s", "/"); !errors.Is(err, errBoom) {
				t.Fatal(err)
			}
			if len(*f.events) != 0 {
				t.Fatalf("events = %+v", *f.events)
			}
		})
	}
}
