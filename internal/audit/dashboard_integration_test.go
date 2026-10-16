//go:build integration

package audit_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
)

type hour struct {
	At              time.Time
	SignIns, Failed int
}

type dashboard struct {
	Users, UsersThisWeek         int
	Unassigned                   []struct{ ID, Name, Email string }
	SignIns, FailedSignIns       int
	Hours                        []hour
	ActiveSessions, SessionUsers int
	GrafanaURL                   string `json:"grafanaUrl"`
}

func read(t *testing.T, admin *itest.Browser) dashboard {
	t.Helper()
	var d dashboard
	admin.Call(t, http.MethodGet, "/api/v1/dashboard", nil, http.StatusOK, &d)
	return d
}

func TestTheDashboardCountsUsersAndThoseWhoJoinedThisWeek(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	env.SignedUp(t, "ada@example.org")
	if d := read(t, admin); d.Users != 2 || d.UsersThisWeek != 2 {
		t.Fatalf("users %d, this week %d", d.Users, d.UsersThisWeek)
	}
}

func TestTheDashboardListsTheUsersWaitingForAGroup(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	d := read(t, admin)
	if len(d.Unassigned) != 1 || d.Unassigned[0].Email != ada.Email || d.Unassigned[0].Name != "Grace Hopper" {
		t.Fatalf("unassigned = %+v", d.Unassigned)
	}
}

func TestTheDashboardCountsSignInsAndFailedSignInsOverADay(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	env.Browser().Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": "nobody@example.org", "password": itest.Password}, http.StatusUnauthorized, nil)
	d := read(t, admin)
	// The Admin signed in twice: once with the password, once again with the authenticator's code.
	if d.SignIns != 2 || d.FailedSignIns != 1 || len(d.Hours) != 24 {
		t.Fatalf("sign-ins %d, failed %d, %d hours", d.SignIns, d.FailedSignIns, len(d.Hours))
	}
	if now := d.Hours[23]; now.SignIns != 2 || now.Failed != 1 || time.Since(now.At) > time.Hour {
		t.Fatalf("this hour = %+v", now)
	}
}

func TestTheDashboardCountsActiveSessionsAndWhoseTheyAre(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	ada.Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": ada.Email, "password": itest.Password}, http.StatusNoContent, nil)
	if d := read(t, admin); d.ActiveSessions != 3 || d.SessionUsers != 2 {
		t.Fatalf("sessions %d across %d Users", d.ActiveSessions, d.SessionUsers)
	}
}

func TestOnlyAnAdminSeesTheDashboard(t *testing.T) {
	t.Parallel()
	itest.Start(t).SignedUp(t, "ada@example.org").Call(t, http.MethodGet, "/api/v1/dashboard", nil, http.StatusForbidden, nil)
}
