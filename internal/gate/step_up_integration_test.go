//go:build integration

package gate_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
)

const start = "/gate/start?host=stats.test&state=s&rd=%2Freport"

// gatedStepUp is identity with Stats gated and step-up on for it at 5 minutes, and Ada holding its
// user role directly; addAuthenticator says whether she has two-factor, and returns its secret.
func gatedStepUp(t *testing.T, addAuthenticator bool) (env *itest.Env, admin, ada *itest.Browser, secret string) {
	t.Helper()
	env = itest.Start(t, stats)
	admin = env.Admin(t, "root@example.org")
	ada = env.SignedUp(t, "ada@example.org")
	admin.Call(t, http.MethodPut, "/api/v1/access/users/"+admin.UserID(t, ada.Email)+"/roles/stats/user", nil, http.StatusNoContent, nil)
	admin.Call(t, http.MethodPut, "/api/v1/apps/stats/step-up", map[string]int{"minutes": 5}, http.StatusNoContent, nil)
	if addAuthenticator {
		secret = ada.AddAuthenticator(t)
	}
	return env, admin, ada, secret
}

func TestAGatedStepUpAppSignsInWithAFreshProof(t *testing.T) {
	t.Parallel()
	_, _, ada, _ := gatedStepUp(t, true)
	if next := ada.Follow(t, start); !strings.HasPrefix(next, "https://stats.test/.auth/callback?code=") {
		t.Fatalf("start went to %s", next)
	}
}

func TestAGatedStepUpAppSendsAStaleProofToProveItAgainAndBack(t *testing.T) {
	t.Parallel()
	env, _, ada, secret := gatedStepUp(t, true)
	env.Pass(6 * time.Minute)
	if next := ada.Follow(t, start); next != "/signin/step-up?"+(url.Values{"next": {start}}).Encode() {
		t.Fatalf("start went to %s", next)
	}
	ada.Call(t, http.MethodPost, "/api/v1/session/step-up", nil, http.StatusOK, nil)
	code, err := totp.GenerateCode(secret, env.Now())
	if err != nil {
		t.Fatal(err)
	}
	ada.Call(t, http.MethodPost, "/api/v1/session/step-up/code", map[string]string{"code": code}, http.StatusNoContent, nil)
	if next := ada.Follow(t, start); !strings.HasPrefix(next, "https://stats.test/.auth/callback?code=") {
		t.Fatalf("start went to %s after proving it", next)
	}
}

func TestAGatedStepUpAppSendsAUserWithNoTwoFactorToSetItUp(t *testing.T) {
	t.Parallel()
	_, _, ada, _ := gatedStepUp(t, false)
	if next := ada.Follow(t, start); next != "/security/two-factor?for=step-up" {
		t.Fatalf("start went to %s", next)
	}
}

func TestTurningStepUpOffRestoresTheUsualGateSignIn(t *testing.T) {
	t.Parallel()
	_, admin, ada, _ := gatedStepUp(t, false)
	admin.Call(t, http.MethodPut, "/api/v1/apps/stats/step-up", map[string]int{"minutes": 0}, http.StatusNoContent, nil)
	if next := ada.Follow(t, start); !strings.HasPrefix(next, "https://stats.test/.auth/callback?code=") {
		t.Fatalf("start went to %s", next)
	}
}
