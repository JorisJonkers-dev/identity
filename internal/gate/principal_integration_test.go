//go:build integration

package gate_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// kb is a gated app that takes Connected Apps on its MCP endpoint.
var kb = signindomain.Client{ID: "kb", SignIn: signindomain.Gated, Host: "kb.test", Label: "Knowledge", Roles: []string{"user"}}

// plainHeaders is every plain header the contract has the Gate set.
var plainHeaders = []string{
	"X-Auth-Request-User", "X-Auth-Request-Email", "X-Auth-Request-Name", "X-Auth-Request-Roles",
	"X-Auth-Request-Kind", "X-Auth-Request-Via", "X-Auth-Request-Actor",
}

// gatedForAda is identity with Stats and Knowledge gated, Knowledge taking Connected Apps, and Ada
// holding the user role on both.
func gatedForAda(t *testing.T) (*itest.Env, *itest.Browser, string) {
	t.Helper()
	env := itest.Start(t, stats, kb)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	id := admin.UserID(t, ada.Email)
	for _, client := range []string{"stats", "kb"} {
		admin.Call(t, http.MethodPut, "/api/v1/access/users/"+id+"/roles/"+client+"/user", nil, http.StatusNoContent, nil)
	}
	admin.Call(t, http.MethodPut, "/api/v1/apps/kb/mcp", map[string]bool{"on": true}, http.StatusNoContent, nil)
	return env, ada, id
}

func allowed(t *testing.T, a itest.GateAnswer) {
	t.Helper()
	if a.StatusCode != http.StatusOK {
		t.Fatalf("check = %d %v", a.StatusCode, a.Header)
	}
}

func TestTheGateSendsASessionsSignedPrincipalForTheClient(t *testing.T) {
	t.Parallel()
	env, ada, id := gatedForAda(t)
	answer := env.GateCheckCookie(t, "stats.test", ada.GateSignIn(t, "stats.test"))
	allowed(t, answer)

	p := env.Principal(t, answer, "stats")

	if p["subject"] != id || p["kind"] != "user" || p["email"] != ada.Email || p["via"] != "session" || p["sid"] == nil ||
		!slices.Equal(p["roles"].([]string), []string{"user"}) || p["twoFactor"] != false || p["actor"] != nil {
		t.Fatalf("principal = %v", p)
	}
}

func TestTheGatesPrincipalSaysWhenTheSessionProvedTwoFactor(t *testing.T) {
	t.Parallel()
	env, ada, _ := gatedForAda(t)
	ada.AddAuthenticator(t)
	answer := env.GateCheckCookie(t, "stats.test", ada.GateSignIn(t, "stats.test"))
	allowed(t, answer)

	p := env.Principal(t, answer, "stats")

	if p["twoFactor"] != true || p["twoFactorAt"] == nil {
		t.Fatalf("principal = %v", p)
	}
}

func TestTheGatesPrincipalForAPersonalTokenNamesNoSession(t *testing.T) {
	t.Parallel()
	env, _, secret := tokenFor(t, "stats")
	answer := env.GateCheck(t, "stats.test", "Bearer "+secret)
	allowed(t, answer)

	p := env.Principal(t, answer, "stats")

	if p["via"] != "personal-token" || p["sid"] != nil || p["actor"] != nil || p["twoFactor"] != false {
		t.Fatalf("principal = %v", p)
	}
}

func TestTheGatesPrincipalNamesAToolActingForAUser(t *testing.T) {
	t.Parallel()
	env, ada, id := gatedForAda(t)
	tool, token := ada.ConnectedApp(t, "Claude Code", "https://kb.test/mcp")
	answer := env.GateCheck(t, "kb.test", "Bearer "+token)
	allowed(t, answer)

	p := env.Principal(t, answer, "kb")

	actor, _ := p["actor"].(map[string]any)
	if p["subject"] != id || p["via"] != "tool" || actor["id"] != tool || actor["name"] != "Claude Code" || p["sid"] != nil {
		t.Fatalf("principal = %v", p)
	}
	if got := answer.Header.Get("X-Auth-Request-Actor"); got != "Claude Code" {
		t.Fatalf("X-Auth-Request-Actor = %q", got)
	}
}

func TestEveryPlainHeaderIsOnEveryAllowedAnswerEvenWhenEmpty(t *testing.T) {
	t.Parallel()
	env, ada, _ := gatedForAda(t)
	_, token := ada.ConnectedApp(t, "Claude Code", "https://kb.test/mcp")
	answers := map[string]itest.GateAnswer{
		"a session": env.GateCheckCookie(t, "stats.test", ada.GateSignIn(t, "stats.test")),
		"a tool":    env.GateCheck(t, "kb.test", "Bearer "+token),
	}
	for name, a := range answers {
		t.Run(name, func(t *testing.T) {
			allowed(t, a)
			for _, header := range plainHeaders {
				if _, ok := a.Header[header]; !ok {
					t.Errorf("%s is missing", header)
				}
			}
		})
	}
}
