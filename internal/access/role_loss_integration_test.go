//go:build integration

package access_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// signedInToWiki is identity with the Wiki, a native app taking back-channel logouts, and Ada
// signed in to it, holding its user role through how; how gives it.
type signedInToWiki struct {
	env        *itest.Env
	admin, ada *itest.Browser
	adaID      string
	logouts    *itest.Logouts
}

func signInToWiki(t *testing.T, how func(w signedInToWiki)) signedInToWiki {
	t.Helper()
	logouts := itest.NewLogouts(t)
	wiki := signindomain.Client{
		ID: "wiki", SignIn: signindomain.Native, Host: "wiki.test", Label: "Wiki", Roles: []string{"user", "editor"},
		Credential: signindomain.PKCE, RedirectURIs: []string{"https://wiki.test/callback"}, BackChannelLogoutURI: logouts.URI(),
	}
	env := itest.Start(t, wiki)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	w := signedInToWiki{env: env, admin: admin, ada: ada, adaID: admin.UserID(t, ada.Email), logouts: logouts}
	how(w)
	if back := ada.Follow(t, ada.Follow(t, ada.Authorize(t, wiki))); !strings.HasPrefix(back, "https://wiki.test/callback?code=") {
		t.Fatalf("Ada did not sign in to the Wiki: %s", back)
	}
	return w
}

func (w signedInToWiki) giveDirectly(t *testing.T, role string) {
	t.Helper()
	w.admin.Call(t, http.MethodPut, "/api/v1/access/users/"+w.adaID+"/roles/wiki/"+role, nil, http.StatusNoContent, nil)
}

// family is a Group Ada is in, giving the Wiki's user role.
func (w signedInToWiki) family(t *testing.T) string {
	t.Helper()
	g := createFamily(t, w.admin)
	w.admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+g.ID+"/members/"+w.adaID, nil, http.StatusNoContent, nil)
	w.admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+g.ID+"/roles/wiki/user", nil, http.StatusNoContent, nil)
	return g.ID
}

func (w signedInToWiki) signedOutOfTheWiki(t *testing.T) {
	t.Helper()
	claims := w.logouts.Next(t)
	if claims["sub"] != w.adaID || claims["sid"] == nil || claims["aud"] == nil {
		t.Fatalf("logout token = %v", claims)
	}
}

func TestTakingAUsersRoleAwaySignsThemOutOfThatApp(t *testing.T) {
	t.Parallel()
	w := signInToWiki(t, func(w signedInToWiki) { w.giveDirectly(t, "user") })

	w.admin.Call(t, http.MethodDelete, "/api/v1/access/users/"+w.adaID+"/roles/wiki/user", nil, http.StatusNoContent, nil)

	w.signedOutOfTheWiki(t)
}

func TestTakingOneOfTwoRolesAwaySignsTheUserOutOfThatApp(t *testing.T) {
	t.Parallel()
	w := signInToWiki(t, func(w signedInToWiki) { w.giveDirectly(t, "user"); w.giveDirectly(t, "editor") })

	w.admin.Call(t, http.MethodDelete, "/api/v1/access/users/"+w.adaID+"/roles/wiki/editor", nil, http.StatusNoContent, nil)

	w.signedOutOfTheWiki(t)
}

func TestTakingAGroupsRoleAwaySignsItsMembersOutOfThatApp(t *testing.T) {
	t.Parallel()
	var family string
	w := signInToWiki(t, func(w signedInToWiki) { family = w.family(t) })

	w.admin.Call(t, http.MethodDelete, "/api/v1/access/groups/"+family+"/roles/wiki/user", nil, http.StatusNoContent, nil)

	w.signedOutOfTheWiki(t)
}

func TestTakingAUserOutOfAGroupSignsThemOutOfItsApps(t *testing.T) {
	t.Parallel()
	var family string
	w := signInToWiki(t, func(w signedInToWiki) { family = w.family(t) })

	w.admin.Call(t, http.MethodDelete, "/api/v1/access/groups/"+family+"/members/"+w.adaID, nil, http.StatusNoContent, nil)

	w.signedOutOfTheWiki(t)
}

func TestDeletingAGroupSignsItsMembersOutOfItsApps(t *testing.T) {
	t.Parallel()
	var family string
	w := signInToWiki(t, func(w signedInToWiki) { family = w.family(t) })

	w.admin.Call(t, http.MethodDelete, "/api/v1/access/groups/"+family, nil, http.StatusNoContent, nil)

	w.signedOutOfTheWiki(t)
}

func TestAGroupStartingToRequireTwoFactorSignsMembersWithoutItOutOfItsApps(t *testing.T) {
	t.Parallel()
	var family string
	w := signInToWiki(t, func(w signedInToWiki) { family = w.family(t) })

	w.admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+family+"/two-factor", map[string]bool{"required": true}, http.StatusNoContent, nil)

	w.signedOutOfTheWiki(t)
}

func TestAUserWhoStillHoldsTheRoleAnotherWayStaysSignedIn(t *testing.T) {
	t.Parallel()
	var family string
	w := signInToWiki(t, func(w signedInToWiki) { w.giveDirectly(t, "user"); family = w.family(t) })

	w.admin.Call(t, http.MethodDelete, "/api/v1/access/groups/"+family+"/roles/wiki/user", nil, http.StatusNoContent, nil)

	w.logouts.None(t, time.Second)
}

func TestGivingARoleSignsNoOneOut(t *testing.T) {
	t.Parallel()
	w := signInToWiki(t, func(w signedInToWiki) { w.giveDirectly(t, "user") })

	w.giveDirectly(t, "editor")

	w.logouts.None(t, time.Second)
}
