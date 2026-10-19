//go:build integration

package access_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var wiki = signindomain.Client{ID: "wiki", SignIn: signindomain.Native, Host: "wiki.test", Label: "Wiki", Roles: []string{"user"}}

type launcher struct {
	Apps []struct {
		ID    string
		Roles []string
	}
}

// grafanaRoles is the roles Ada's Launcher shows on Grafana.
func grafanaRoles(t *testing.T, ada *itest.Browser) []string {
	t.Helper()
	var l launcher
	ada.Call(t, http.MethodGet, "/api/v1/launcher", nil, http.StatusOK, &l)
	for _, a := range l.Apps {
		if a.ID == "grafana" {
			return a.Roles
		}
	}
	return nil
}

// heldBack is identity with Ada in Family, which requires two-factor and gives Grafana viewer, and
// in Friends, which gives Grafana user; Ada has no two-factor.
func heldBack(t *testing.T) (admin, ada *itest.Browser, family string) {
	t.Helper()
	env := itest.Start(t, grafana, wiki)
	admin = env.Admin(t, "root@example.org")
	ada = env.SignedUp(t, "ada@example.org")
	adaID := admin.UserID(t, ada.Email)
	family = createFamily(t, admin).ID
	var friends group
	admin.Call(t, http.MethodPost, "/api/v1/access/groups", map[string]string{"name": "Friends"}, http.StatusCreated, &friends)
	for g, role := range map[string]string{family: "viewer", friends.ID: "user"} {
		admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+g+"/members/"+adaID, nil, http.StatusNoContent, nil)
		admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+g+"/roles/grafana/"+role, nil, http.StatusNoContent, nil)
	}
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+family+"/two-factor", map[string]bool{"required": true}, http.StatusNoContent, nil)
	return admin, ada, family
}

func TestAnAdminMakesAGroupRequireTwoFactor(t *testing.T) {
	t.Parallel()
	admin, _, family := heldBack(t)
	if g, _ := find(read(t, admin), family); !g.RequiresTwoFactor {
		t.Fatalf("Family = %+v", g)
	}
}

func TestAnAdminStopsAGroupRequiringTwoFactor(t *testing.T) {
	t.Parallel()
	admin, ada, family := heldBack(t)
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+family+"/two-factor", map[string]bool{"required": false}, http.StatusNoContent, nil)
	if got := grafanaRoles(t, ada); !slices.Equal(got, []string{"user", "viewer"}) {
		t.Fatalf("roles = %v", got)
	}
}

func TestTheAdminsGroupAlwaysRequiresTwoFactor(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t).Admin(t, "root@example.org")
	o := read(t, admin)
	i := slices.IndexFunc(o.Groups, func(g group) bool { return g.Admins })
	if !o.Groups[i].RequiresTwoFactor {
		t.Fatalf("admins = %+v", o.Groups[i])
	}
	refused(t, admin, http.MethodPut, "/api/v1/access/groups/"+o.Groups[i].ID+"/two-factor", map[string]bool{"required": false},
		http.StatusUnprocessableEntity, "The admins Group always requires two-factor.")
}

func TestAMemberWithoutTwoFactorHoldsNoneOfATwoFactorGroupsRolesButKeepsTheOthers(t *testing.T) {
	t.Parallel()
	_, ada, _ := heldBack(t)
	if got := grafanaRoles(t, ada); !slices.Equal(got, []string{"user"}) {
		t.Fatalf("roles = %v", got)
	}
}

func TestSettingUpTwoFactorGivesTheRolesBackAtOnce(t *testing.T) {
	t.Parallel()
	_, ada, _ := heldBack(t)
	ada.AddAuthenticator(t)
	if got := grafanaRoles(t, ada); !slices.Equal(got, []string{"user", "viewer"}) {
		t.Fatalf("roles = %v", got)
	}
}

func TestRemovingTheLastTwoFactorMethodHoldsTheRolesBackAgain(t *testing.T) {
	t.Parallel()
	_, ada, _ := heldBack(t)
	ada.AddAuthenticator(t)
	var f struct{ Factors []struct{ ID string } }
	ada.Call(t, http.MethodGet, "/api/v1/profile/second-factors", nil, http.StatusOK, &f)
	ada.Call(t, http.MethodPost, "/api/v1/profile/second-factors/"+f.Factors[0].ID+"/removal", map[string]string{"password": itest.Password}, http.StatusNoContent, nil)
	if got := grafanaRoles(t, ada); !slices.Equal(got, []string{"user"}) {
		t.Fatalf("roles = %v", got)
	}
}

func TestADirectRoleIsNeverHeldBack(t *testing.T) {
	t.Parallel()
	admin, ada, _ := heldBack(t)
	admin.Call(t, http.MethodPut, "/api/v1/access/users/"+admin.UserID(t, ada.Email)+"/roles/grafana/editor", nil, http.StatusNoContent, nil)
	if got := grafanaRoles(t, ada); !slices.Equal(got, []string{"editor", "user"}) {
		t.Fatalf("roles = %v", got)
	}
}

func TestTheAccessPageSaysWhoHasTwoFactor(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	var o struct {
		Users []struct {
			Email     string
			TwoFactor bool
		}
	}
	admin.Call(t, http.MethodGet, "/api/v1/access", nil, http.StatusOK, &o)
	for _, u := range o.Users {
		if u.TwoFactor != (u.Email == admin.Email) {
			t.Fatalf("users = %+v (ada is %s)", o.Users, ada.Email)
		}
	}
}

func TestMarkingAGroupIsAnAuditEvent(t *testing.T) {
	t.Parallel()
	admin, _, _ := heldBack(t)
	var page struct {
		Events []struct {
			Action, GroupName string
			Actor             *struct{ Email string }
		}
	}
	admin.Call(t, http.MethodGet, "/api/v1/audit?kind=access", nil, http.StatusOK, &page)
	e := page.Events[0]
	if e.Action != "group-two-factor-required" || e.GroupName != "Family" || e.Actor == nil || e.Actor.Email != admin.Email {
		t.Fatalf("event = %+v", e)
	}
}

func TestMarkingAGroupMailsEachMemberItHoldsBackOnce(t *testing.T) {
	t.Parallel()
	admin, ada, family := heldBack(t)
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+family+"/two-factor", map[string]bool{"required": true}, http.StatusNoContent, nil)
	text := itest.Mailed(t, ada.Email, "Family now requires two-factor")
	if !strings.Contains(text, "/security/two-factor") {
		t.Fatalf("mail = %q", text)
	}
	if n := itest.MailCount(t, ada.Email, "Family now requires two-factor"); n != 1 {
		t.Fatalf("%d mails, want 1", n)
	}
	if n := itest.MailCount(t, admin.Email, "Family now requires two-factor"); n != 0 {
		t.Fatalf("the Admin, who has two-factor, got %d mails", n)
	}
}

func TestTheLauncherSaysHowManyAppsTwoFactorWouldGiveBack(t *testing.T) {
	t.Parallel()
	admin, ada, family := heldBack(t)
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+family+"/roles/wiki/user", nil, http.StatusNoContent, nil)
	var l struct{ HeldBack int }
	ada.Call(t, http.MethodGet, "/api/v1/launcher", nil, http.StatusOK, &l)
	if l.HeldBack != 1 {
		t.Fatalf("held back = %d", l.HeldBack)
	}
}

func TestTheLaunchersNoticeGoesOnceTwoFactorIsSetUp(t *testing.T) {
	t.Parallel()
	admin, ada, family := heldBack(t)
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+family+"/roles/wiki/user", nil, http.StatusNoContent, nil)
	ada.AddAuthenticator(t)
	var l struct{ HeldBack int }
	ada.Call(t, http.MethodGet, "/api/v1/launcher", nil, http.StatusOK, &l)
	if l.HeldBack != 0 {
		t.Fatalf("held back = %d", l.HeldBack)
	}
}
