//go:build integration

// Package access_test checks the access context's behaviours the way a browser reaches them: over
// HTTP, against the real routes, Postgres, Vault and mail (docs/testing-standards.md).
package access_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var (
	grafana = signindomain.Client{
		ID: "grafana", SignIn: signindomain.Native, Host: "grafana.test", Label: "Grafana", Roles: []string{"user", "viewer", "editor"},
	}
	stats = signindomain.Client{ID: "stats", SignIn: signindomain.Gated, Host: "stats.test", Label: "Stats", Roles: []string{"user", "editor"}}
)

type group struct {
	ID                string
	Name              string
	Admins            bool
	Members           []string
	RequiresTwoFactor bool
}

type assignment struct{ GroupID, ClientID, Role string }

type userAssignment struct{ UserID, ClientID, Role string }

type overview struct {
	Groups          []group
	Assignments     []assignment
	UserAssignments []userAssignment
}

type problem struct{ Detail string }

func read(t *testing.T, admin *itest.Browser) overview {
	t.Helper()
	var o overview
	admin.Call(t, http.MethodGet, "/api/v1/access", nil, http.StatusOK, &o)
	return o
}

func find(o overview, id string) (group, bool) {
	i := slices.IndexFunc(o.Groups, func(g group) bool { return g.ID == id })
	if i < 0 {
		return group{}, false
	}
	return o.Groups[i], true
}

// createFamily is the Family Group, made by admin.
func createFamily(t *testing.T, admin *itest.Browser) group {
	t.Helper()
	var g group
	admin.Call(t, http.MethodPost, "/api/v1/access/groups", map[string]string{"name": "Family"}, http.StatusCreated, &g)
	return g
}

func refused(t *testing.T, b *itest.Browser, method, path string, body any, status int, detail string) {
	t.Helper()
	var p problem
	b.Call(t, method, path, body, status, &p)
	if p.Detail != detail {
		t.Fatalf("detail = %q, want %q", p.Detail, detail)
	}
}

func TestAnAdminCreatesAGroup(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t).Admin(t, "root@example.org")
	family := createFamily(t, admin)
	if g, ok := find(read(t, admin), family.ID); !ok || g.Name != "Family" || g.Admins {
		t.Fatalf("Family = %+v, %v", g, ok)
	}
}

func TestTwoGroupsCannotShareANameWhateverItsCase(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t).Admin(t, "root@example.org")
	createFamily(t, admin)
	refused(t, admin, http.MethodPost, "/api/v1/access/groups", map[string]string{"name": "FAMILY"},
		http.StatusConflict, "A Group with that name already exists. Choose another name.")
}

func TestAnAdminRenamesAGroup(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t).Admin(t, "root@example.org")
	family := createFamily(t, admin)
	admin.Call(t, http.MethodPatch, "/api/v1/access/groups/"+family.ID, map[string]string{"name": "Kin"}, http.StatusNoContent, nil)
	if g, _ := find(read(t, admin), family.ID); g.Name != "Kin" {
		t.Fatalf("name = %q, want Kin", g.Name)
	}
}

func TestAnAdminDeletesAGroup(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t).Admin(t, "root@example.org")
	family := createFamily(t, admin)
	admin.Call(t, http.MethodDelete, "/api/v1/access/groups/"+family.ID, nil, http.StatusNoContent, nil)
	if _, ok := find(read(t, admin), family.ID); ok {
		t.Fatal("Family is still there")
	}
}

func TestTheAdminsGroupCannotBeDeleted(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t).Admin(t, "root@example.org")
	i := slices.IndexFunc(read(t, admin).Groups, func(g group) bool { return g.Admins })
	refused(t, admin, http.MethodDelete, "/api/v1/access/groups/"+read(t, admin).Groups[i].ID, nil,
		http.StatusUnprocessableEntity, "The admins Group cannot be deleted or given a role.")
}

func TestAnAdminAddsAPersonToAGroup(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := admin.UserID(t, env.SignedUp(t, "ada@example.org").Email)
	family := createFamily(t, admin)
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+family.ID+"/members/"+ada, nil, http.StatusNoContent, nil)
	if g, _ := find(read(t, admin), family.ID); !slices.Contains(g.Members, ada) {
		t.Fatalf("Family's members = %v, want Ada among them", g.Members)
	}
}

func TestAnAdminTakesAPersonOutOfAGroup(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := admin.UserID(t, env.SignedUp(t, "ada@example.org").Email)
	family := createFamily(t, admin)
	member := "/api/v1/access/groups/" + family.ID + "/members/" + ada
	admin.Call(t, http.MethodPut, member, nil, http.StatusNoContent, nil)
	admin.Call(t, http.MethodDelete, member, nil, http.StatusNoContent, nil)
	if g, _ := find(read(t, admin), family.ID); len(g.Members) != 0 {
		t.Fatalf("Family's members = %v, want none", g.Members)
	}
}

func TestTheLastAdminCannotLeaveTheAdminsGroup(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t).Admin(t, "root@example.org")
	o := read(t, admin)
	i := slices.IndexFunc(o.Groups, func(g group) bool { return g.Admins })
	refused(t, admin, http.MethodDelete, "/api/v1/access/groups/"+o.Groups[i].ID+"/members/"+admin.UserID(t, admin.Email), nil,
		http.StatusUnprocessableEntity, "Add another Admin before you remove the last one.")
}

func TestAnAdminGivesAGroupAnAppRole(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t, grafana).Admin(t, "root@example.org")
	family := createFamily(t, admin)
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+family.ID+"/roles/grafana/viewer", nil, http.StatusNoContent, nil)
	if as := read(t, admin).Assignments; !slices.Contains(as, assignment{family.ID, "grafana", "viewer"}) {
		t.Fatalf("Assignments = %+v", as)
	}
}

func TestAnAppRoleTheClientDoesNotDeclareIsRefused(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t, grafana).Admin(t, "root@example.org")
	family := createFamily(t, admin)
	refused(t, admin, http.MethodPut, "/api/v1/access/groups/"+family.ID+"/roles/grafana/owner", nil,
		http.StatusUnprocessableEntity, "That app has no such role.")
}

func TestAnAppRoleOnAnAppThatIsNotRegisteredIsRefused(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t).Admin(t, "root@example.org")
	family := createFamily(t, admin)
	refused(t, admin, http.MethodPut, "/api/v1/access/groups/"+family.ID+"/roles/grafana/viewer", nil,
		http.StatusUnprocessableEntity, "That app is not registered.")
}

func TestAnAdminTakesAnAppRoleAway(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t, grafana).Admin(t, "root@example.org")
	family := createFamily(t, admin)
	role := "/api/v1/access/groups/" + family.ID + "/roles/grafana/viewer"
	admin.Call(t, http.MethodPut, role, nil, http.StatusNoContent, nil)
	admin.Call(t, http.MethodDelete, role, nil, http.StatusNoContent, nil)
	if as := read(t, admin).Assignments; len(as) != 0 {
		t.Fatalf("Assignments = %+v, want none", as)
	}
}

func TestAnAdminGivesAPersonAnAppRoleDirectly(t *testing.T) {
	t.Parallel()
	env := itest.Start(t, grafana)
	admin := env.Admin(t, "root@example.org")
	ada := admin.UserID(t, env.SignedUp(t, "ada@example.org").Email)
	admin.Call(t, http.MethodPut, "/api/v1/access/users/"+ada+"/roles/grafana/viewer", nil, http.StatusNoContent, nil)
	if as := read(t, admin).UserAssignments; !slices.Equal(as, []userAssignment{{ada, "grafana", "viewer"}}) {
		t.Fatalf("UserAssignments = %+v", as)
	}
}

func TestAnAdminTakesAPersonsDirectAppRoleAway(t *testing.T) {
	t.Parallel()
	env := itest.Start(t, grafana)
	admin := env.Admin(t, "root@example.org")
	role := "/api/v1/access/users/" + admin.UserID(t, env.SignedUp(t, "ada@example.org").Email) + "/roles/grafana/viewer"
	admin.Call(t, http.MethodPut, role, nil, http.StatusNoContent, nil)
	admin.Call(t, http.MethodDelete, role, nil, http.StatusNoContent, nil)
	if as := read(t, admin).UserAssignments; len(as) != 0 {
		t.Fatalf("UserAssignments = %+v, want none", as)
	}
}

func TestADirectAppRoleTheClientDoesNotDeclareIsRefused(t *testing.T) {
	t.Parallel()
	env := itest.Start(t, grafana)
	admin := env.Admin(t, "root@example.org")
	ada := admin.UserID(t, env.SignedUp(t, "ada@example.org").Email)
	refused(t, admin, http.MethodPut, "/api/v1/access/users/"+ada+"/roles/grafana/owner", nil,
		http.StatusUnprocessableEntity, "That app has no such role.")
}

func TestADirectAppRoleForSomeoneWhoDoesNotExistIsRefused(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t, grafana).Admin(t, "root@example.org")
	refused(t, admin, http.MethodPut, "/api/v1/access/users/"+uuid.NewString()+"/roles/grafana/viewer", nil,
		http.StatusNotFound, "That Group or person no longer exists. Reload the page.")
}

func TestAPersonSeesAnAppTheyHoldADirectRoleOnInTheLauncher(t *testing.T) {
	t.Parallel()
	env := itest.Start(t, grafana)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	admin.Call(t, http.MethodPut, "/api/v1/access/users/"+admin.UserID(t, ada.Email)+"/roles/grafana/viewer", nil, http.StatusNoContent, nil)
	var l struct{ Apps []struct{ ID string } }
	ada.Call(t, http.MethodGet, "/api/v1/launcher", nil, http.StatusOK, &l)
	if len(l.Apps) != 1 || l.Apps[0].ID != "grafana" {
		t.Fatalf("apps = %+v", l.Apps)
	}
}

func TestDirectAndGroupAppRolesCombineAtTheGate(t *testing.T) {
	t.Parallel()
	env := itest.Start(t, stats)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	adaID := admin.UserID(t, ada.Email)
	family := createFamily(t, admin)
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+family.ID+"/members/"+adaID, nil, http.StatusNoContent, nil)
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+family.ID+"/roles/stats/user", nil, http.StatusNoContent, nil)
	admin.Call(t, http.MethodPut, "/api/v1/access/users/"+adaID+"/roles/stats/editor", nil, http.StatusNoContent, nil)
	var m struct{ Secret string }
	ada.Call(t, http.MethodPost, "/api/v1/profile/tokens", map[string]any{"name": "script", "clientId": "stats", "days": 30}, http.StatusCreated, &m)
	if res := env.GateCheck(t, "stats.test", "Bearer "+m.Secret); res.Header.Get("X-Auth-Request-Roles") != "user,editor" {
		t.Fatalf("check = %d %v", res.StatusCode, res.Header)
	}
}

func TestAnAdminSignsAUserOutEverywhere(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	admin.Call(t, http.MethodPost, "/api/v1/access/users/"+admin.UserID(t, ada.Email)+"/sign-out", nil, http.StatusNoContent, nil)
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
}

func TestAUserWhoIsNotAnAdminCannotOpenTheAccessPage(t *testing.T) {
	t.Parallel()
	ada := itest.Start(t).SignedUp(t, "ada@example.org")
	refused(t, ada, http.MethodGet, "/api/v1/access", nil, http.StatusForbidden, "Only Admins manage access.")
}

func TestAnAdminWhoseSessionProvedNoTwoFactorIsToldToAddOne(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.SignedUp(t, "root@example.org")
	env.Promote(t, admin.Email)
	refused(t, admin, http.MethodGet, "/api/v1/access", nil, http.StatusForbidden,
		"Sign out, then sign in again with a passkey or authenticator app to manage access. If you have neither, add one on the Security page first.")
}

func TestTheAccessPageNeedsASignIn(t *testing.T) {
	t.Parallel()
	itest.Start(t).Browser().Call(t, http.MethodGet, "/api/v1/access", nil, http.StatusUnauthorized, nil)
}
