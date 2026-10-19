package web_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
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
	page      app.Page
	dashboard app.Dashboard
	err       error
	actor     app.Actor
	filter    domain.Filter
}

func (u *useCases) Dashboard(_ context.Context, actor app.Actor) (app.Dashboard, error) {
	u.actor = actor
	return u.dashboard, u.err
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
	res, err := web.New(uc, nil).ListAuditEvents(asAdmin(), params)
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
	if got.Actor.Set || got.User.Set || got.Client.Set || got.GroupName.Set || got.Role.Set || got.Email.Set || got.Reason.Set || got.Minutes.Set || got.MachineName.Set {
		t.Fatalf("event = %+v", got)
	}
}

func TestAMachineIdentityEventCarriesItsName(t *testing.T) {
	t.Parallel()
	entry := app.Entry{Event: domain.Event{ID: uuid.New(), At: t0, Action: domain.MachineCreated, Machine: uuid.New(), MachineName: "Backup"}, Kind: domain.KindAccess}
	got := list(t, &useCases{page: app.Page{Entries: []app.Entry{entry}}}, oas.ListAuditEventsParams{}).(*oas.AuditPage).Events[0]
	if got.MachineName != oas.NewOptString("Backup") {
		t.Fatalf("machine name = %+v", got.MachineName)
	}
}

func TestAStepUpChangeCarriesItsWindowEvenWhenItIsOff(t *testing.T) {
	t.Parallel()
	entry := app.Entry{Event: domain.Event{ID: uuid.New(), At: t0, Action: domain.StepUpChanged, Minutes: 0}, Kind: domain.KindAccess}
	got := list(t, &useCases{page: app.Page{Entries: []app.Entry{entry}}}, oas.ListAuditEventsParams{}).(*oas.AuditPage).Events[0]
	if got.Minutes != oas.NewOptAuditEventMinutes(0) {
		t.Fatalf("minutes = %+v", got.Minutes)
	}
}

func TestARefusedListingSaysWhy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		err    error
		detail string
	}{
		{"not an Admin", app.ErrNotAdmin, "Only Admins can see this page."},
		{"no Second Factor", app.ErrSecondFactorNeeded, "This page needs a Second Factor. If you have a passkey or authenticator app, sign out and sign in again with it. If you have neither, add a Second Factor on the Security page first, then sign out and sign in again."},
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
	if _, err := web.New(&useCases{err: errBoom}, nil).ListAuditEvents(asAdmin(), oas.ListAuditEventsParams{}); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func getDashboard(t *testing.T, uc *useCases, grafana *url.URL) oas.GetDashboardRes {
	t.Helper()
	res, err := web.New(uc, grafana).GetDashboard(asAdmin())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestTheDashboardCarriesItsCounts(t *testing.T) {
	t.Parallel()
	var hours [24]app.Hour
	for i := range hours {
		hours[i] = app.Hour{At: t0.Add(time.Duration(i) * time.Hour), SignIns: i, Failed: i % 2}
	}
	uc := &useCases{dashboard: app.Dashboard{
		Users: 14, UsersThisWeek: 1, SignIns: 186, FailedSignIns: 4, Hours: hours, ActiveSessions: 23, SessionUsers: 12, TokensExpiring: 3,
		Unassigned: []app.Person{{ID: ada, Name: "Noor Bakker", Email: "noor@example.org", CreatedAt: t0}},
	}}
	d := getDashboard(t, uc, nil).(*oas.Dashboard)
	if d.Users != 14 || d.UsersThisWeek != 1 || d.SignIns != 186 || d.FailedSignIns != 4 || d.ActiveSessions != 23 || d.SessionUsers != 12 || d.TokensExpiring != 3 {
		t.Fatalf("dashboard = %+v", d)
	}
	if len(d.Unassigned) != 1 || d.Unassigned[0] != (oas.WaitingUser{ID: oas.ID(ada), Email: "noor@example.org", Name: "Noor Bakker", ConfirmedAt: t0}) {
		t.Fatalf("unassigned = %+v", d.Unassigned)
	}
	if len(d.Hours) != 24 || d.Hours[23] != (oas.SignInHour{At: t0.Add(23 * time.Hour), SignIns: 23, Failed: 1}) {
		t.Fatalf("hours = %+v", d.Hours)
	}
	if d.GrafanaUrl.Set || uc.actor != (app.Actor{ID: admin, SecondFactor: true}) {
		t.Fatalf("grafana %+v, asked as %+v", d.GrafanaUrl, uc.actor)
	}
}

func TestTheDashboardLinksToGrafanaWhenItIsConfigured(t *testing.T) {
	t.Parallel()
	grafana, _ := url.Parse("https://grafana.example.org/d/identity")
	if d := getDashboard(t, &useCases{}, grafana).(*oas.Dashboard); d.GrafanaUrl.Value.String() != grafana.String() {
		t.Fatalf("grafana = %+v", d.GrafanaUrl)
	}
}

func TestARefusedDashboardSaysWhy(t *testing.T) {
	t.Parallel()
	res := getDashboard(t, &useCases{err: app.ErrNotAdmin}, nil)
	if p, ok := res.(*oas.ProblemStatusCode); !ok || p.StatusCode != http.StatusForbidden || p.Response.Detail.Value != "Only Admins can see this page." {
		t.Fatalf("answer = %+v", res)
	}
}

func TestAFailedDashboardIsPassedOn(t *testing.T) {
	t.Parallel()
	if _, err := web.New(&useCases{err: errBoom}, nil).GetDashboard(asAdmin()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}
