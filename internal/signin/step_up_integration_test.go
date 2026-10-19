//go:build integration

package signin_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var wiki = signindomain.Client{
	ID: "wiki", SignIn: signindomain.Native, Host: "wiki.test", Label: "Wiki", Roles: []string{"user"},
	Credential: signindomain.PKCE, RedirectURIs: []string{"https://wiki.test/callback"},
}

const callback = "https://" + itest.Host + "/oidc/authorize/callback?id="

type stepUp struct{ Minutes int }

// world is identity with the Wiki, an Admin, and Ada, who holds the Wiki's user role.
type world struct {
	env        *itest.Env
	admin, ada *itest.Browser
	adaID      string
}

func start(t *testing.T) world {
	t.Helper()
	env := itest.Start(t, wiki)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	w := world{env: env, admin: admin, ada: ada, adaID: admin.UserID(t, ada.Email)}
	admin.Call(t, http.MethodPut, "/api/v1/access/users/"+w.adaID+"/roles/wiki/user", nil, http.StatusNoContent, nil)
	return w
}

func (w world) stepUp(t *testing.T, minutes int) {
	t.Helper()
	w.admin.Call(t, http.MethodPut, "/api/v1/apps/wiki/step-up", map[string]int{"minutes": minutes}, http.StatusNoContent, nil)
}

// code is what Ada's authenticator shows now, as identity's clock has it.
func (w world) code(t *testing.T, secret string) string {
	t.Helper()
	c, err := totp.GenerateCode(secret, w.env.Now())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAnAdminSetsStepUpForAnApp(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.stepUp(t, 15)
	var got stepUp
	w.admin.Call(t, http.MethodGet, "/api/v1/apps/wiki/step-up", nil, http.StatusOK, &got)
	if got.Minutes != 15 {
		t.Fatalf("step-up = %+v", got)
	}
}

func TestAnAdminTurnsStepUpOff(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.stepUp(t, 15)
	w.stepUp(t, 0)
	var got stepUp
	w.admin.Call(t, http.MethodGet, "/api/v1/apps/wiki/step-up", nil, http.StatusOK, &got)
	if got.Minutes != 0 {
		t.Fatalf("step-up = %+v", got)
	}
}

func TestStepUpIsOffUntilAnAdminSetsIt(t *testing.T) {
	t.Parallel()
	var got stepUp
	start(t).admin.Call(t, http.MethodGet, "/api/v1/apps/wiki/step-up", nil, http.StatusOK, &got)
	if got.Minutes != 0 {
		t.Fatalf("step-up = %+v", got)
	}
}

func TestStepUpLastsOnlyFiveFifteenOrSixtyMinutes(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.admin.Call(t, http.MethodPut, "/api/v1/apps/wiki/step-up", map[string]int{"minutes": 30}, http.StatusBadRequest, nil)
}

func TestOnlyAnAdminSetsStepUp(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.ada.Call(t, http.MethodPut, "/api/v1/apps/wiki/step-up", map[string]int{"minutes": 5}, http.StatusForbidden, nil)
}

func TestStepUpForAnAppThatIsNotRegisteredIsRefused(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.admin.Call(t, http.MethodPut, "/api/v1/apps/grafana/step-up", map[string]int{"minutes": 5}, http.StatusNotFound, nil)
}

func TestAnAppWithoutStepUpSignsInWithThePasswordAlone(t *testing.T) {
	t.Parallel()
	w := start(t)
	if next := w.ada.Follow(t, w.ada.Authorize(t, wiki)); !strings.HasPrefix(next, callback) {
		t.Fatalf("login went to %s", next)
	}
}

func TestAStepUpAppAsksForTwoFactorWhenTheProofIsOlderThanItsWindow(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.ada.AddAuthenticator(t)
	w.stepUp(t, 15)
	w.env.Pass(16 * time.Minute)
	login := w.ada.Authorize(t, wiki)
	id := login[strings.Index(login, "authRequestID=")+len("authRequestID="):]
	if next := w.ada.Follow(t, login); next != "/signin/step-up?authRequestID="+id {
		t.Fatalf("login went to %s", next)
	}
}

func TestAProofInsideTheWindowSignsInWithoutAsking(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.ada.AddAuthenticator(t)
	w.stepUp(t, 15)
	w.env.Pass(14 * time.Minute)
	if next := w.ada.Follow(t, w.ada.Authorize(t, wiki)); !strings.HasPrefix(next, callback) {
		t.Fatalf("login went to %s", next)
	}
}

func TestProvingTwoFactorAgainFinishesTheSameSignIn(t *testing.T) {
	t.Parallel()
	w := start(t)
	secret := w.ada.AddAuthenticator(t)
	w.stepUp(t, 5)
	w.env.Pass(6 * time.Minute)
	login := w.ada.Authorize(t, wiki)
	w.ada.Follow(t, login)
	var needs struct{ Methods []string }
	w.ada.Call(t, http.MethodPost, "/api/v1/session/step-up", nil, http.StatusOK, &needs)
	if len(needs.Methods) != 1 || needs.Methods[0] != "totp" {
		t.Fatalf("methods = %v", needs.Methods)
	}
	w.ada.Call(t, http.MethodPost, "/api/v1/session/step-up/code", map[string]string{"code": w.code(t, secret)}, http.StatusNoContent, nil)
	if next := w.ada.Follow(t, login); !strings.HasPrefix(next, callback) {
		t.Fatalf("login went to %s", next)
	}
}

func TestOneFreshProofCoversEveryStepUpAppForItsWindow(t *testing.T) {
	t.Parallel()
	w := start(t)
	secret := w.ada.AddAuthenticator(t)
	w.stepUp(t, 5)
	w.env.Pass(6 * time.Minute)
	w.ada.Call(t, http.MethodPost, "/api/v1/session/step-up", nil, http.StatusOK, nil)
	w.ada.Call(t, http.MethodPost, "/api/v1/session/step-up/code", map[string]string{"code": w.code(t, secret)}, http.StatusNoContent, nil)
	w.env.Pass(4 * time.Minute)
	if next := w.ada.Follow(t, w.ada.Authorize(t, wiki)); !strings.HasPrefix(next, callback) {
		t.Fatalf("login went to %s", next)
	}
}

func TestAWrongStepUpCodeIsRefusedAndTheAppStillAsks(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.ada.AddAuthenticator(t)
	w.stepUp(t, 5)
	w.env.Pass(6 * time.Minute)
	w.ada.Call(t, http.MethodPost, "/api/v1/session/step-up", nil, http.StatusOK, nil)
	w.ada.Call(t, http.MethodPost, "/api/v1/session/step-up/code", map[string]string{"code": "000000"}, http.StatusUnprocessableEntity, nil)
	if next := w.ada.Follow(t, w.ada.Authorize(t, wiki)); !strings.HasPrefix(next, "/signin/step-up?") {
		t.Fatalf("login went to %s", next)
	}
}

func TestAStepUpCodeWithoutStartingStepUpIsRefused(t *testing.T) {
	t.Parallel()
	w := start(t)
	secret := w.ada.AddAuthenticator(t)
	w.ada.Call(t, http.MethodPost, "/api/v1/session/step-up/code", map[string]string{"code": w.code(t, secret)}, http.StatusUnauthorized, nil)
}

func TestAUserWithNoTwoFactorIsSentToSetItUpAndNotToTheApp(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.stepUp(t, 15)
	if next := w.ada.Follow(t, w.ada.Authorize(t, wiki)); next != "/security/two-factor?for=step-up" {
		t.Fatalf("login went to %s", next)
	}
}

func TestStartingStepUpWithNoTwoFactorSaysToSetItUp(t *testing.T) {
	t.Parallel()
	var p struct{ Detail string }
	start(t).ada.Call(t, http.MethodPost, "/api/v1/session/step-up", nil, http.StatusConflict, &p)
	if p.Detail != "Add a passkey or an authenticator app on the Security page first." {
		t.Fatalf("detail = %q", p.Detail)
	}
}

func TestChangingStepUpIsAnAuditEvent(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.stepUp(t, 15)
	var page struct {
		Events []struct {
			Action  string
			Actor   *struct{ Email string }
			Client  *struct{ ID string }
			Minutes int
		}
	}
	w.admin.Call(t, http.MethodGet, "/api/v1/audit?kind=access", nil, http.StatusOK, &page)
	e := page.Events[0]
	if e.Action != "step-up-changed" || e.Actor == nil || e.Actor.Email != w.admin.Email || e.Client == nil || e.Client.ID != "wiki" || e.Minutes != 15 {
		t.Fatalf("event = %+v", e)
	}
}
