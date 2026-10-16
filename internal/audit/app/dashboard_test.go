package app_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/audit/app"
)

func dashboard(t *testing.T, f *fixture) app.Dashboard {
	t.Helper()
	d, err := f.Dashboard(context.Background(), asAdmin)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestTheDashboardCountsUsersAndThoseWhoJoinedThisWeek(t *testing.T) {
	t.Parallel()
	if d := dashboard(t, newFixture(counting())); d.Users != 3 || d.UsersThisWeek != 2 {
		t.Fatalf("users %d, this week %d", d.Users, d.UsersThisWeek)
	}
}

func TestTheDashboardListsTheUnassignedNewestFirst(t *testing.T) {
	t.Parallel()
	d := dashboard(t, newFixture(counting()))
	if len(d.Unassigned) != 2 || d.Unassigned[0].ID != bob || d.Unassigned[1].ID != ada {
		t.Fatalf("unassigned = %+v", d.Unassigned)
	}
}

func TestTheDashboardCountsLiveSessionsAndWhoseTheyAre(t *testing.T) {
	t.Parallel()
	if d := dashboard(t, newFixture(counting())); d.ActiveSessions != 5 || d.SessionUsers != 2 {
		t.Fatalf("sessions %d across %d", d.ActiveSessions, d.SessionUsers)
	}
}

func TestTheDashboardReadsSignInsFromTheStartOfTheHour23HoursAgo(t *testing.T) {
	t.Parallel()
	f := newFixture(counting())
	dashboard(t, f)
	if want := t0.Add(-23 * time.Hour); !f.store.since.Equal(want) {
		t.Fatalf("since %v, want %v", f.store.since, want)
	}
}

func TestTheDashboardLaysSignInsOutHourByHourEndingNow(t *testing.T) {
	t.Parallel()
	f := newFixture(counting())
	f.store.hours = []app.Hour{
		{At: t0.Add(-23 * time.Hour), SignIns: 4, Failed: 1},
		{At: t0, SignIns: 2},
	}
	d := dashboard(t, f)
	if d.Hours[0] != f.store.hours[0] || d.Hours[23] != f.store.hours[1] || d.Hours[12] != (app.Hour{At: t0.Add(-11 * time.Hour)}) {
		t.Fatalf("hours = %+v", d.Hours)
	}
	if d.SignIns != 6 || d.FailedSignIns != 1 {
		t.Fatalf("sign-ins %d, failed %d", d.SignIns, d.FailedSignIns)
	}
}

func TestTheDashboardLeavesOutHoursOutsideTheDay(t *testing.T) {
	t.Parallel()
	f := newFixture(counting())
	f.store.hours = []app.Hour{{At: t0.Add(-24 * time.Hour), SignIns: 9}, {At: t0.Add(time.Hour), SignIns: 9}}
	if d := dashboard(t, f); d.SignIns != 0 {
		t.Fatalf("sign-ins %d", d.SignIns)
	}
}

func TestOnlyAnAdminSeesTheDashboard(t *testing.T) {
	t.Parallel()
	if _, err := newFixture(counting()).Dashboard(context.Background(), app.Actor{ID: ada, SecondFactor: true}); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("Dashboard = %v", err)
	}
}

func TestTheDashboardFailsWithWhatItReads(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"People", "Unassigned", "ActiveSessions", "SignInsByHour"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			f := newFixture(counting())
			// Each fake fails only the methods it has, so the one named fails wherever it is.
			f.store.fail[method] = errBoom
			f.Service = app.New(f.store, admins{}, directory{fail: map[string]error{method: errBoom}}, slog.New(slog.DiscardHandler), func() time.Time { return t0 }, readerFunc(counting()))
			if _, err := f.Dashboard(context.Background(), asAdmin); !errors.Is(err, errBoom) {
				t.Fatalf("Dashboard = %v", err)
			}
		})
	}
}
