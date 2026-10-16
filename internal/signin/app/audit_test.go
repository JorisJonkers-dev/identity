package app_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

func TestASignInToAClientIsRecorded(t *testing.T) {
	f := newFixture()
	r := f.started(t)
	if err := f.Complete(context.Background(), r.ID, "tok"); err != nil {
		t.Fatal(err)
	}
	want := []audit.Event{{Action: audit.SignedIn, User: f.accounts.ada, Client: "spa"}}
	if !slices.Equal(*f.events, want) {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestASignInRefusedForWantOfAnAppRoleIsRecordedAsFailed(t *testing.T) {
	f := newFixture()
	r, _ := f.Start(context.Background(), domain.AuthRequest{ClientID: "web"})
	if err := f.Complete(context.Background(), r.ID, "tok"); !errors.Is(err, app.ErrNoAccess) {
		t.Fatal(err)
	}
	want := []audit.Event{{Action: audit.SignInFailed, User: f.accounts.ada, Client: "web", Reason: audit.ReasonNoAccess}}
	if !slices.Equal(*f.events, want) {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestASignInThatCannotBeKeptRecordsNothing(t *testing.T) {
	for _, method := range []string{"CompleteRequest", "AddSessionClient"} {
		t.Run(method, func(t *testing.T) {
			f := newFixture()
			r := f.started(t)
			f.requests.fail[method] = errBoom
			if err := f.Complete(context.Background(), r.ID, "tok"); !errors.Is(err, errBoom) {
				t.Fatal(err)
			}
			if len(*f.events) != 0 {
				t.Fatalf("events = %+v", *f.events)
			}
		})
	}
}
