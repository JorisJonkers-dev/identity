package web_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/audit/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/audit/app"
	"github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
)

var (
	errBoom = errors.New("boom")
	t0      = time.Date(2026, 10, 16, 12, 0, 0, 123456000, time.UTC)
	admin   = uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
	ada     = uuid.MustParse("00000000-0000-4000-8000-0000000000a2")
)

// useCases answers with what a test sets and records what it was asked.
type useCases struct {
	page   app.Page
	err    error
	actor  app.Actor
	filter domain.Filter
}

func (u *useCases) List(_ context.Context, actor app.Actor, f domain.Filter) (app.Page, error) {
	u.actor, u.filter = actor, f
	return u.page, u.err
}

func asAdmin() context.Context {
	return principal.With(context.Background(), principal.Principal{UserID: admin, SecondFactor: true})
}

func list(t *testing.T, uc *useCases, params oas.ListAuditEventsParams) oas.ListAuditEventsRes {
	t.Helper()
	res, err := web.New(uc).ListAuditEvents(asAdmin(), params)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestTheListingAsksAsTheSignedInUser(t *testing.T) {
	t.Parallel()
	uc := &useCases{}
	list(t, uc, oas.ListAuditEventsParams{})
	if uc.actor != (app.Actor{ID: admin, SecondFactor: true}) || uc.filter != (domain.Filter{}) {
		t.Fatalf("asked as %+v with %+v", uc.actor, uc.filter)
	}
}

func TestTheListingFiltersByUserClientAndKind(t *testing.T) {
	t.Parallel()
	uc := &useCases{}
	list(t, uc, oas.ListAuditEventsParams{
		User: oas.NewOptID(oas.ID(ada)), Client: oas.NewOptClientId("grafana"), Kind: oas.NewOptAuditKind(oas.AuditKindAccess),
	})
	if uc.filter != (domain.Filter{User: ada, Client: "grafana", Kind: domain.KindAccess}) {
		t.Fatalf("filter = %+v", uc.filter)
	}
}

func TestAPagesNextReadsBackAsWhereTheNextPageBegins(t *testing.T) {
	t.Parallel()
	next := domain.Cursor{At: t0, ID: ada}
	page := list(t, &useCases{page: app.Page{Next: next}}, oas.ListAuditEventsParams{}).(*oas.AuditPage)
	uc := &useCases{}
	list(t, uc, oas.ListAuditEventsParams{Before: page.Next})
	if !uc.filter.Before.At.Equal(t0) || uc.filter.Before.ID != ada {
		t.Fatalf("before = %+v", uc.filter.Before)
	}
}

func TestTheLastPageHasNoNext(t *testing.T) {
	t.Parallel()
	if page := list(t, &useCases{}, oas.ListAuditEventsParams{}).(*oas.AuditPage); page.Next.Set || len(page.Events) != 0 {
		t.Fatalf("page = %+v", page)
	}
}

func TestACursorThatIsNotOneIsABadRequest(t *testing.T) {
	t.Parallel()
	for name, cursor := range map[string]string{
		"not base64":   "!!!",
		"no separator": base64.RawURLEncoding.EncodeToString([]byte("123")),
		"not a time":   base64.RawURLEncoding.EncodeToString([]byte("soon." + ada.String())),
		"not an id":    base64.RawURLEncoding.EncodeToString([]byte("123.nobody")),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res := list(t, &useCases{}, oas.ListAuditEventsParams{Before: oas.NewOptAuditCursor(oas.AuditCursor(cursor))})
			if p, ok := res.(*oas.ProblemStatusCode); !ok || p.StatusCode != http.StatusBadRequest {
				t.Fatalf("answer = %+v", res)
			}
		})
	}
}

func TestAnEventCarriesWhatItNames(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	entry := app.Entry{
		Event: domain.Event{
			ID: id, At: t0, Action: domain.SignInFailed, Client: "grafana", GroupName: "Family", Role: "viewer",
			Email: "ada@example.org", Reason: domain.ReasonNoAccess,
		},
		Kind:        domain.KindSignIn,
		Actor:       &app.Person{ID: admin, Name: "Root", Email: "root@example.org"},
		User:        &app.Person{ID: ada},
		ClientLabel: "Grafana",
	}
	got := list(t, &useCases{page: app.Page{Entries: []app.Entry{entry}}}, oas.ListAuditEventsParams{}).(*oas.AuditPage).Events[0]
	want := oas.AuditEvent{
		ID: oas.ID(id), At: t0, Kind: oas.AuditKindSignIn, Action: oas.AuditActionSignInFailed,
		Actor:     oas.NewOptAuditPerson(oas.AuditPerson{ID: oas.ID(admin), Name: oas.NewOptString("Root"), Email: oas.NewOptEmail("root@example.org")}),
		User:      oas.NewOptAuditPerson(oas.AuditPerson{ID: oas.ID(ada)}),
		Client:    oas.NewOptAuditClient(oas.AuditClient{ID: "grafana", Label: "Grafana"}),
		GroupName: oas.NewOptString("Family"), Role: oas.NewOptAppRole("viewer"),
		Email: oas.NewOptString("ada@example.org"), Reason: oas.NewOptAuditEventReason(oas.AuditEventReasonNoAccess),
	}
	if got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
}

func TestAnEventLeavesOutWhatItDoesNotName(t *testing.T) {
	t.Parallel()
	entry := app.Entry{Event: domain.Event{ID: uuid.New(), At: t0, Action: domain.SignedOut}, Kind: domain.KindSignIn}
	got := list(t, &useCases{page: app.Page{Entries: []app.Entry{entry}}}, oas.ListAuditEventsParams{}).(*oas.AuditPage).Events[0]
	if got.Actor.Set || got.User.Set || got.Client.Set || got.GroupName.Set || got.Role.Set || got.Email.Set || got.Reason.Set {
		t.Fatalf("event = %+v", got)
	}
}

func TestARefusedListingSaysWhy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		err    error
		detail string
	}{
		{"not an Admin", app.ErrNotAdmin, "Only Admins read the audit log."},
		{"no Second Factor", app.ErrSecondFactorNeeded, "Sign out, then sign in again with a passkey or authenticator app to read the audit log. If you have neither, add one on the Security page first."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := list(t, &useCases{err: tc.err}, oas.ListAuditEventsParams{})
			if p, ok := res.(*oas.ProblemStatusCode); !ok || p.StatusCode != http.StatusForbidden || p.Response.Detail.Value != tc.detail {
				t.Fatalf("answer = %+v", res)
			}
		})
	}
}

func TestAFailedListingIsPassedOn(t *testing.T) {
	t.Parallel()
	if _, err := web.New(&useCases{err: errBoom}).ListAuditEvents(asAdmin(), oas.ListAuditEventsParams{}); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}
