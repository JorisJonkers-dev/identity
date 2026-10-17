package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/audit/app"
	"github.com/JorisJonkers-dev/identity/internal/audit/domain"
)

var (
	errBoom = errors.New("boom")
	t0      = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	admin   = uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
	ada     = uuid.MustParse("00000000-0000-4000-8000-0000000000a2")
	gone    = uuid.MustParse("00000000-0000-4000-8000-0000000000a3")
	bob     = uuid.MustParse("00000000-0000-4000-8000-0000000000a4")
	asAdmin = app.Actor{ID: admin, SecondFactor: true}
)

// store keeps events in memory, newest last; fail makes the named method return its error.
type store struct {
	events []domain.Event
	limit  int
	filter domain.Filter
	cutoff time.Time
	hours  []app.Hour
	since  time.Time
	fail   map[string]error
}

func (s *store) SignInsByHour(_ context.Context, since time.Time) ([]app.Hour, error) {
	s.since = since
	return s.hours, s.fail["SignInsByHour"]
}

func (s *store) Append(_ context.Context, e domain.Event) error {
	if err := s.fail["Append"]; err != nil {
		return err
	}
	s.events = append(s.events, e)
	return nil
}

func (s *store) List(_ context.Context, f domain.Filter, limit int) ([]domain.Event, error) {
	s.filter, s.limit = f, limit
	out := make([]domain.Event, 0, len(s.events))
	for i := len(s.events) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.events[i])
	}
	return out, s.fail["List"]
}

func (s *store) DeleteBefore(_ context.Context, t time.Time) (int64, error) {
	s.cutoff = t
	return 3, s.fail["DeleteBefore"]
}

type admins struct{ err error }

func (a admins) IsAdmin(_ context.Context, id uuid.UUID) (bool, error) { return id == admin, a.err }

// directory knows Root, who joined a month ago, and Ada and Bob, who joined this week and wait
// for a Group; fail makes the named method return its error.
type directory struct{ fail map[string]error }

func (d directory) People(context.Context) ([]app.Person, error) {
	return []app.Person{
		{ID: admin, Name: "Root", Email: "root@example.org", CreatedAt: t0.Add(-30 * 24 * time.Hour)},
		{ID: ada, Name: "Ada", Email: "ada@example.org", CreatedAt: t0.Add(-2 * 24 * time.Hour)},
		{ID: bob, Name: "Bob", Email: "bob@example.org", CreatedAt: t0.Add(-time.Hour)},
	}, d.fail["People"]
}

func (d directory) Unassigned(context.Context) ([]uuid.UUID, error) {
	return []uuid.UUID{ada, bob}, d.fail["Unassigned"]
}

func (d directory) ActiveSessions(context.Context) (sessions, users int, err error) {
	return 5, 2, d.fail["ActiveSessions"]
}

func (d directory) TokensExpiring(context.Context) (int, error) {
	return 3, d.fail["TokensExpiring"]
}

func (directory) ClientLabel(_ context.Context, id string) string { return strings.ToUpper(id) }

type fixture struct {
	*app.Service
	store *store
	logs  *bytes.Buffer
}

func newFixture(random func([]byte) (int, error), opts ...func(*fixture)) *fixture {
	f := &fixture{store: &store{fail: map[string]error{}}, logs: &bytes.Buffer{}}
	f.Service = app.New(f.store, admins{}, directory{}, slog.New(slog.NewJSONHandler(f.logs, nil)), func() time.Time { return t0 }, readerFunc(random))
	for _, o := range opts {
		o(f)
	}
	return f
}

type readerFunc func([]byte) (int, error)

func (r readerFunc) Read(p []byte) (int, error) { return r(p) }

func counting() func([]byte) (int, error) {
	var n byte
	return func(p []byte) (int, error) {
		n++
		for i := range p {
			p[i] = n
		}
		return len(p), nil
	}
}

func (f *fixture) lines(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range bytes.Lines(f.logs.Bytes()) {
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

func TestARecordedEventIsStoredWithAnIDAndTheTime(t *testing.T) {
	t.Parallel()
	f := newFixture(counting())
	f.Record(context.Background(), domain.Event{Action: domain.GroupCreated, Actor: admin})
	if len(f.store.events) != 1 || f.store.events[0].ID == uuid.Nil || !f.store.events[0].At.Equal(t0) {
		t.Fatalf("stored = %+v", f.store.events)
	}
}

func TestARecordedEventIsLoggedAsJSONWithOnlyWhatItNames(t *testing.T) {
	t.Parallel()
	f := newFixture(counting())
	f.Record(context.Background(), domain.Event{
		Action: domain.RoleAssigned, Actor: admin, User: ada, Group: gone, Client: "grafana",
		GroupName: "Family", Role: "viewer", Email: "ada@example.org", Reason: domain.ReasonNoAccess,
	})
	f.Record(context.Background(), domain.Event{Action: domain.SignedOut})
	lines := f.lines(t)
	full, bare := lines[0]["audit"].(map[string]any), lines[1]["audit"].(map[string]any)
	want := map[string]any{
		"id": f.store.events[0].ID.String(), "at": "2026-10-16T12:00:00Z", "kind": "access", "action": "role-assigned",
		"actor_id": admin.String(), "user_id": ada.String(), "group_id": gone.String(), "client_id": "grafana",
		"group_name": "Family", "role": "viewer", "email": "ada@example.org", "reason": "no-access",
	}
	if lines[0]["msg"] != "audit event" || len(full) != len(want) || len(bare) != 4 {
		t.Fatalf("logged %v and %v", full, bare)
	}
	for k, v := range want {
		if full[k] != v {
			t.Fatalf("%s = %v, want %v", k, full[k], v)
		}
	}
}

func TestAnEventTheDatabaseRefusesIsStillLoggedAndTheRefusalWithIt(t *testing.T) {
	t.Parallel()
	f := newFixture(counting())
	f.store.fail["Append"] = errBoom
	f.Record(context.Background(), domain.Event{Action: domain.GroupCreated})
	lines := f.lines(t)
	if len(lines) != 2 || lines[0]["msg"] != "audit event" || lines[1]["msg"] != "audit event not stored" || lines[1]["error"] != "boom" {
		t.Fatalf("logged %v", lines)
	}
}

func TestAnEventWithoutAnIDIsNeitherStoredNorLoggedAsAnEvent(t *testing.T) {
	t.Parallel()
	f := newFixture(func([]byte) (int, error) { return 0, errBoom })
	f.Record(context.Background(), domain.Event{Action: domain.GroupCreated})
	lines := f.lines(t)
	if len(f.store.events) != 0 || len(lines) != 1 || lines[0]["msg"] != "audit event not recorded" || lines[0]["action"] != "group-created" {
		t.Fatalf("stored %v, logged %v", f.store.events, lines)
	}
}

func TestAListingNamesThePeopleAndClientsItsEventsRefer(t *testing.T) {
	t.Parallel()
	f := newFixture(counting())
	f.Record(context.Background(), domain.Event{Action: domain.MemberAdded, Actor: admin, User: ada, Client: "grafana"})
	p, err := f.List(context.Background(), asAdmin, domain.Filter{})
	if err != nil || len(p.Entries) != 1 {
		t.Fatalf("List = %+v, %v", p, err)
	}
	e := p.Entries[0]
	if e.Kind != domain.KindAccess || e.Actor.Name != "Root" || e.User.Email != "ada@example.org" || e.ClientLabel != "GRAFANA" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestAListingNamesAUserWhoIsGoneByTheirIDAlone(t *testing.T) {
	t.Parallel()
	f := newFixture(counting())
	f.Record(context.Background(), domain.Event{Action: domain.SignedIn, User: gone})
	p, _ := f.List(context.Background(), asAdmin, domain.Filter{})
	if e := p.Entries[0]; e.User == nil || *e.User != (app.Person{ID: gone}) || e.Actor != nil || e.ClientLabel != "" {
		t.Fatalf("entry = %+v", e)
	}
}

func TestAListingPassesItsFilterOn(t *testing.T) {
	t.Parallel()
	f := newFixture(counting())
	want := domain.Filter{User: ada, Client: "grafana", Kind: domain.KindAccess, Before: domain.Cursor{At: t0, ID: gone}}
	if _, err := f.List(context.Background(), asAdmin, want); err != nil || f.store.filter != want {
		t.Fatalf("filter = %+v, %v", f.store.filter, err)
	}
}

func TestAFullPageSaysWhereTheNextBegins(t *testing.T) {
	t.Parallel()
	f := newFixture(counting())
	for range 51 {
		f.Record(context.Background(), domain.Event{Action: domain.GroupCreated})
	}
	p, _ := f.List(context.Background(), asAdmin, domain.Filter{})
	last := p.Entries[49]
	if len(p.Entries) != 50 || f.store.limit != 51 || p.Next != (domain.Cursor{At: last.At, ID: last.ID}) {
		t.Fatalf("%d entries, next %+v", len(p.Entries), p.Next)
	}
}

func TestTheLastPageHasNoNext(t *testing.T) {
	t.Parallel()
	f := newFixture(counting())
	for range 50 {
		f.Record(context.Background(), domain.Event{Action: domain.GroupCreated})
	}
	if p, _ := f.List(context.Background(), asAdmin, domain.Filter{}); len(p.Entries) != 50 || !p.Next.Zero() {
		t.Fatalf("%d entries, next %+v", len(p.Entries), p.Next)
	}
}

func TestOnlyAnAdminListsEvents(t *testing.T) {
	t.Parallel()
	if _, err := newFixture(counting()).List(context.Background(), app.Actor{ID: ada, SecondFactor: true}, domain.Filter{}); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("List = %v", err)
	}
}

func TestAnAdminListsEventsOnlyFromASessionProvedWithASecondFactor(t *testing.T) {
	t.Parallel()
	if _, err := newFixture(counting()).List(context.Background(), app.Actor{ID: admin}, domain.Filter{}); !errors.Is(err, app.ErrSecondFactorNeeded) {
		t.Fatalf("List = %v", err)
	}
}

func TestAListingFailsWithWhatItReads(t *testing.T) {
	t.Parallel()
	for name, broken := range map[string]func(*fixture){
		"who is an Admin": func(f *fixture) {
			f.Service = app.New(f.store, admins{err: errBoom}, directory{}, slog.New(slog.DiscardHandler), time.Now, readerFunc(counting()))
		},
		"the events": func(f *fixture) { f.store.fail["List"] = errBoom },
		"the people": func(f *fixture) {
			f.Service = app.New(f.store, admins{}, directory{fail: map[string]error{"People": errBoom}}, slog.New(slog.DiscardHandler), time.Now, readerFunc(counting()))
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := newFixture(counting(), broken).List(context.Background(), asAdmin, domain.Filter{}); !errors.Is(err, errBoom) {
				t.Fatalf("List = %v", err)
			}
		})
	}
}

func TestPruningRemovesEventsOlderThanAYear(t *testing.T) {
	t.Parallel()
	f := newFixture(counting())
	n, err := f.Prune(context.Background())
	if err != nil || n != 3 || !f.store.cutoff.Equal(t0.Add(-365*24*time.Hour)) {
		t.Fatalf("Prune = %d, %v; cutoff %v", n, err, f.store.cutoff)
	}
}
