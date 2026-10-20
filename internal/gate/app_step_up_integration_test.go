//go:build integration

package gate_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestAnAppsStepUpWithAFreshEnoughProofComesStraightBack(t *testing.T) {
	t.Parallel()
	env, ada, _ := gatedForAda(t)
	ada.AddAuthenticator(t)

	next, state := ada.GateStepUp(t, "stats.test", 5)

	if !strings.HasPrefix(next, "https://stats.test/.auth/callback?code=") {
		t.Fatalf("start went to %s", next)
	}
	if p := env.Principal(t, env.GateCheckCookie(t, "stats.test", ada.GateCallback(t, next, state)), "stats"); p["twoFactor"] != true {
		t.Fatalf("principal = %v", p)
	}
}

func TestAnAppsStepUpWithAStaleProofProvesItAgainAndStatesTheNewProof(t *testing.T) {
	t.Parallel()
	env, ada, _ := gatedForAda(t)
	secret := ada.AddAuthenticator(t)
	env.Pass(10 * time.Minute)
	before := env.Principal(t, env.GateCheckCookie(t, "stats.test", ada.GateSignIn(t, "stats.test")), "stats")

	next, state := ada.GateStepUp(t, "stats.test", 5)
	start, _ := url.Parse(next)
	if start.Path != "/signin/step-up" {
		t.Fatalf("start went to %s", next)
	}
	ada.Call(t, http.MethodPost, "/api/v1/session/step-up", nil, http.StatusOK, nil)
	code, err := totp.GenerateCode(secret, env.Now())
	if err != nil {
		t.Fatal(err)
	}
	ada.Call(t, http.MethodPost, "/api/v1/session/step-up/code", map[string]string{"code": code}, http.StatusNoContent, nil)
	cookie := ada.GateCallback(t, ada.Follow(t, start.Query().Get("next")), state)

	after := env.Principal(t, env.GateCheckCookie(t, "stats.test", cookie), "stats")
	if after["twoFactorAt"] == before["twoFactorAt"] || after["twoFactorAt"].(string) < before["twoFactorAt"].(string) {
		t.Fatalf("two-factor at %v, before the step-up %v", after["twoFactorAt"], before["twoFactorAt"])
	}
}

func TestAnAppsStepUpSendsAUserWithNoTwoFactorToSetItUp(t *testing.T) {
	t.Parallel()
	_, ada, _ := gatedForAda(t)

	if next, _ := ada.GateStepUp(t, "stats.test", 5); next != "/security/two-factor?for=step-up" {
		t.Fatalf("start went to %s", next)
	}
}
