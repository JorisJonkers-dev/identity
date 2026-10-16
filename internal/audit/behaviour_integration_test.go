//go:build integration

// Package audit_test checks Audit Events the way an Admin reaches them: over HTTP, against the real
// routes, Postgres, Vault and mail (docs/testing-standards.md).
package audit_test

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var grafana = signindomain.Client{
	ID: "grafana", SignIn: signindomain.Native, Host: "grafana.test", Label: "Grafana", Roles: []string{"user", "viewer"},
}

type person struct{ ID, Name, Email string }

type client struct{ ID, Label string }

type event struct {
	ID, Kind, Action               string
	At                             time.Time
	Actor, User                    *person
	Client                         *client
	GroupName, Role, Email, Reason string
}

type page struct {
	Events []event
	Next   string
}

func list(t *testing.T, admin *itest.Browser, query url.Values) page {
	t.Helper()
	var p page
	admin.Call(t, http.MethodGet, "/api/v1/audit?"+query.Encode(), nil, http.StatusOK, &p)
	return p
}

// only is the one event with action among the newest page, failing t unless there is exactly one.
func only(t *testing.T, admin *itest.Browser, query url.Values, action string) event {
	t.Helper()
	var found []event
	for _, e := range list(t, admin, query).Events {
		if e.Action == action {
			found = append(found, e)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d %s events, want 1: %+v", len(found), action, found)
	}
	return found[0]
}

func none(t *testing.T, admin *itest.Browser, query url.Values, action string) {
	t.Helper()
	for _, e := range list(t, admin, query).Events {
		if e.Action == action {
			t.Fatalf("unexpected %s event: %+v", action, e)
		}
	}
}

func kind(k string) url.Values { return url.Values{"kind": {k}} }

func createGroup(t *testing.T, admin *itest.Browser, name string) string {
	t.Helper()
	var g struct{ ID string }
	admin.Call(t, http.MethodPost, "/api/v1/access/groups", map[string]string{"name": name}, http.StatusCreated, &g)
	return g.ID
}

func TestEachAccessChangeIsOneAuditEventByTheAdmin(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, action, role string
		change             func(t *testing.T, admin *itest.Browser, group, ada string)
	}{
		{name: "making a Group", action: "group-created", change: func(*testing.T, *itest.Browser, string, string) {}},
		{name: "renaming a Group", action: "group-renamed", change: func(t *testing.T, admin *itest.Browser, group, _ string) {
			admin.Call(t, http.MethodPatch, "/api/v1/access/groups/"+group, map[string]string{"name": "Family"}, http.StatusNoContent, nil)
		}},
		{name: "deleting a Group", action: "group-deleted", change: func(t *testing.T, admin *itest.Browser, group, _ string) {
			admin.Call(t, http.MethodDelete, "/api/v1/access/groups/"+group, nil, http.StatusNoContent, nil)
		}},
		{name: "adding a member", action: "member-added", change: func(t *testing.T, admin *itest.Browser, group, ada string) {
			admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+group+"/members/"+ada, nil, http.StatusNoContent, nil)
		}},
		{name: "removing a member", action: "member-removed", change: func(t *testing.T, admin *itest.Browser, group, ada string) {
			admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+group+"/members/"+ada, nil, http.StatusNoContent, nil)
			admin.Call(t, http.MethodDelete, "/api/v1/access/groups/"+group+"/members/"+ada, nil, http.StatusNoContent, nil)
		}},
		{name: "giving an App Role", action: "role-assigned", role: "viewer", change: func(t *testing.T, admin *itest.Browser, group, _ string) {
			admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+group+"/roles/grafana/viewer", nil, http.StatusNoContent, nil)
		}},
		{name: "taking an App Role away", action: "role-removed", role: "viewer", change: func(t *testing.T, admin *itest.Browser, group, _ string) {
			admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+group+"/roles/grafana/viewer", nil, http.StatusNoContent, nil)
			admin.Call(t, http.MethodDelete, "/api/v1/access/groups/"+group+"/roles/grafana/viewer", nil, http.StatusNoContent, nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := itest.Start(t, grafana)
			admin := env.Admin(t, "root@example.org")
			ada := admin.UserID(t, env.SignedUp(t, "ada@example.org").Email)
			group := createGroup(t, admin, "Kin")
			tc.change(t, admin, group, ada)
			e := only(t, admin, kind("access"), tc.action)
			if e.Kind != "access" || e.Actor == nil || e.Actor.Email != admin.Email || e.GroupName == "" || e.Role != tc.role {
				t.Fatalf("event = %+v", e)
			}
		})
	}
}

func TestAddingAMemberTwiceIsOneAuditEvent(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := admin.UserID(t, env.SignedUp(t, "ada@example.org").Email)
	group := createGroup(t, admin, "Kin")
	for range 2 {
		admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+group+"/members/"+ada, nil, http.StatusNoContent, nil)
	}
	if e := only(t, admin, kind("access"), "member-added"); e.User == nil || e.User.ID != ada {
		t.Fatalf("event = %+v", e)
	}
}

func TestAnAssignmentNamesItsClient(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t, grafana).Admin(t, "root@example.org")
	group := createGroup(t, admin, "Kin")
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+group+"/roles/grafana/viewer", nil, http.StatusNoContent, nil)
	if e := only(t, admin, kind("access"), "role-assigned"); e.Client == nil || *e.Client != (client{ID: "grafana", Label: "Grafana"}) {
		t.Fatalf("event = %+v", e)
	}
}

func TestASignInIsAnAuditEvent(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := admin.UserID(t, env.SignedUp(t, "ada@example.org").Email)
	if e := only(t, admin, url.Values{"user": {ada}}, "signed-in"); e.Kind != "sign-in" || e.User == nil || e.User.ID != ada {
		t.Fatalf("event = %+v", e)
	}
}

func TestAWrongPasswordIsAFailedSignInForThatUser(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	env.Browser().Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": ada.Email, "password": "not the password"}, http.StatusUnauthorized, nil)
	e := only(t, admin, kind("sign-in"), "sign-in-failed")
	if e.User == nil || e.User.Email != ada.Email || e.Email != ada.Email || e.Reason != "password" {
		t.Fatalf("event = %+v", e)
	}
}

func TestAFailedSignInWithAnUnknownEmailNamesOnlyTheEmail(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	env.Browser().Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": "nobody@example.org", "password": itest.Password}, http.StatusUnauthorized, nil)
	e := only(t, admin, kind("sign-in"), "sign-in-failed")
	if e.User != nil || e.Email != "nobody@example.org" || e.Reason != "password" {
		t.Fatalf("event = %+v", e)
	}
}

func TestAWrongAuthenticatorCodeIsAFailedSignIn(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	other := env.Browser()
	other.Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": admin.Email, "password": itest.Password}, http.StatusAccepted, nil)
	other.Call(t, http.MethodPost, "/api/v1/session/second-factor", map[string]string{"code": "000000"}, http.StatusUnprocessableEntity, nil)
	if e := only(t, admin, kind("sign-in"), "sign-in-failed"); e.User == nil || e.User.Email != admin.Email || e.Reason != "second-factor" {
		t.Fatalf("event = %+v", e)
	}
}

func TestSigningOutIsAnAuditEvent(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	ada.Call(t, http.MethodDelete, "/api/v1/session", nil, http.StatusNoContent, nil)
	id := admin.UserID(t, ada.Email)
	if e := only(t, admin, url.Values{"user": {id}}, "signed-out"); e.Actor != nil || e.User == nil || e.User.ID != id {
		t.Fatalf("event = %+v", e)
	}
}

func TestAnAdminSigningSomeoneOutIsAnAuditEventByTheAdmin(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := admin.UserID(t, env.SignedUp(t, "ada@example.org").Email)
	admin.Call(t, http.MethodPost, "/api/v1/access/users/"+ada+"/sign-out", nil, http.StatusNoContent, nil)
	if e := only(t, admin, url.Values{"user": {ada}}, "signed-out"); e.Actor == nil || e.Actor.Email != admin.Email {
		t.Fatalf("event = %+v", e)
	}
}

func TestEachProfileChangeIsOneAuditEvent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, action string
		change       func(t *testing.T, ada *itest.Browser)
	}{
		{name: "confirming a sign-up", action: "user-created", change: func(*testing.T, *itest.Browser) {}},
		{name: "renaming oneself", action: "user-renamed", change: func(t *testing.T, ada *itest.Browser) {
			ada.Call(t, http.MethodPatch, "/api/v1/profile", map[string]string{"name": "Ada Lovelace"}, http.StatusOK, nil)
		}},
		{name: "changing the password", action: "password-changed", change: func(t *testing.T, ada *itest.Browser) {
			ada.Call(t, http.MethodPost, "/api/v1/profile/password", map[string]string{"current": itest.Password, "next": "another long password"}, http.StatusNoContent, nil)
		}},
		{name: "adding an authenticator app", action: "second-factor-added", change: func(t *testing.T, ada *itest.Browser) {
			addAuthenticator(t, ada)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := itest.Start(t)
			admin := env.Admin(t, "root@example.org")
			ada := env.SignedUp(t, "ada@example.org")
			tc.change(t, ada)
			id := admin.UserID(t, ada.Email)
			if e := only(t, admin, url.Values{"user": {id}}, tc.action); e.Kind != "people" || e.User == nil || e.User.ID != id {
				t.Fatalf("event = %+v", e)
			}
		})
	}
}

func TestRemovingASecondFactorIsAnAuditEvent(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	var listed struct{ Factors []struct{ ID string } }
	admin.Call(t, http.MethodGet, "/api/v1/profile/second-factors", nil, http.StatusOK, &listed)
	admin.Call(t, http.MethodPost, "/api/v1/profile/second-factors/"+listed.Factors[0].ID+"/removal", map[string]string{"password": itest.Password}, http.StatusNoContent, nil)
	if e := only(t, admin, kind("people"), "second-factor-removed"); e.User == nil || e.User.Email != admin.Email {
		t.Fatalf("event = %+v", e)
	}
}

func addAuthenticator(t *testing.T, b *itest.Browser) {
	t.Helper()
	var setup struct{ Secret string }
	b.Call(t, http.MethodPost, "/api/v1/profile/second-factors/totp", nil, http.StatusOK, &setup)
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b.Call(t, http.MethodPost, "/api/v1/profile/second-factors/totp/confirmation",
		map[string]string{"label": "Phone", "code": code, "password": itest.Password}, http.StatusNoContent, nil)
}

func TestTheAuditLogFiltersByUser(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := admin.UserID(t, env.SignedUp(t, "ada@example.org").Email)
	for _, e := range list(t, admin, url.Values{"user": {ada}}).Events {
		if (e.User == nil || e.User.ID != ada) && (e.Actor == nil || e.Actor.ID != ada) {
			t.Fatalf("event of someone else: %+v", e)
		}
	}
	none(t, admin, url.Values{"user": {ada}}, "second-factor-added")
}

func TestTheAuditLogFiltersByClient(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t, grafana).Admin(t, "root@example.org")
	group := createGroup(t, admin, "Kin")
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+group+"/roles/grafana/viewer", nil, http.StatusNoContent, nil)
	got := list(t, admin, url.Values{"client": {"grafana"}}).Events
	if len(got) != 1 || got[0].Action != "role-assigned" {
		t.Fatalf("events = %+v", got)
	}
}

func TestTheAuditLogFiltersByKind(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t).Admin(t, "root@example.org")
	createGroup(t, admin, "Kin")
	for _, e := range list(t, admin, kind("sign-in")).Events {
		if e.Kind != "sign-in" {
			t.Fatalf("event of another kind: %+v", e)
		}
	}
	none(t, admin, kind("sign-in"), "group-created")
}

func TestTheAuditLogListsNewestFirstAPageAtATime(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t).Admin(t, "root@example.org")
	for i := range 51 {
		createGroup(t, admin, fmt.Sprintf("Group %02d", i))
	}
	first := list(t, admin, kind("access"))
	second := list(t, admin, url.Values{"kind": {"access"}, "before": {first.Next}})
	if len(first.Events) != 50 || len(second.Events) != 1 || second.Next != "" {
		t.Fatalf("pages of %d and %d, next %q", len(first.Events), len(second.Events), second.Next)
	}
	if second.Events[0].At.After(first.Events[49].At) || !slices.IsSortedFunc(first.Events, func(a, b event) int { return b.At.Compare(a.At) }) {
		t.Fatal("not newest first")
	}
}

func TestOnlyAnAdminReadsTheAuditLog(t *testing.T) {
	t.Parallel()
	itest.Start(t).SignedUp(t, "ada@example.org").Call(t, http.MethodGet, "/api/v1/audit", nil, http.StatusForbidden, nil)
}

func TestTheAuditLogNeedsASignIn(t *testing.T) {
	t.Parallel()
	itest.Start(t).Browser().Call(t, http.MethodGet, "/api/v1/audit", nil, http.StatusUnauthorized, nil)
}

func TestEveryAuditEventIsLoggedAsJSON(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	createGroup(t, admin, "Kin")
	e := only(t, admin, kind("access"), "group-created")
	for _, line := range env.Logs(t) {
		if line["msg"] != "audit event" {
			continue
		}
		if a, _ := line["audit"].(map[string]any); a["id"] == e.ID && a["action"] == "group-created" && a["group_name"] == "Kin" {
			return
		}
	}
	t.Fatalf("no log line for %+v", e)
}

func TestPruningRemovesAuditEventsOlderThanAYear(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	q := env.Queries()
	for _, age := range []time.Duration{366 * 24 * time.Hour, 364 * 24 * time.Hour} {
		err := q.AppendAuditEvent(t.Context(), queries.AppendAuditEventParams{
			ID: uuid.New(), At: time.Now().Add(-age), Kind: "access", Action: "group-created",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	env.Prune(t)
	var ages []time.Duration
	for _, e := range list(t, admin, kind("access")).Events {
		ages = append(ages, time.Since(e.At).Round(24*time.Hour))
	}
	if !slices.Equal(ages, []time.Duration{364 * 24 * time.Hour}) {
		t.Fatalf("ages left = %v, want only the 364-day-old event", ages)
	}
}
