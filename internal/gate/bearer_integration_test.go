//go:build integration

// Package gate_test checks the Gate the way Traefik reaches it: over HTTP, against the real routes,
// Postgres, Vault and mail (docs/testing-standards.md).
package gate_test

import (
	"net/http"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var (
	stats  = signindomain.Client{ID: "stats", SignIn: signindomain.Gated, Host: "stats.test", Label: "Stats", Roles: []string{"user"}}
	backup = signindomain.Client{ID: "backup", SignIn: signindomain.Token, Host: "backup.test", Roles: []string{"user"}}
)

// tokenFor is a Personal Token Ada minted for clientID, after an Admin gave her Group its user role.
func tokenFor(t *testing.T, clientID string) (*itest.Env, *itest.Browser, string) {
	t.Helper()
	env := itest.Start(t, stats, backup)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	var g struct{ ID string }
	admin.Call(t, http.MethodPost, "/api/v1/access/groups", map[string]string{"name": "Scripts"}, http.StatusCreated, &g)
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+g.ID+"/members/"+admin.UserID(t, ada.Email), nil, http.StatusNoContent, nil)
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+g.ID+"/roles/"+clientID+"/user", nil, http.StatusNoContent, nil)
	var m struct {
		Token  struct{ ID string }
		Secret string
	}
	ada.Call(t, http.MethodPost, "/api/v1/profile/tokens", map[string]any{"name": "script", "clientId": clientID, "days": 30}, http.StatusCreated, &m)
	return env, ada, m.Secret
}

func TestAPersonalTokenOpensAGatedHost(t *testing.T) {
	t.Parallel()
	env, ada, secret := tokenFor(t, "stats")
	res := env.GateCheck(t, "stats.test", "Bearer "+secret)
	if res.StatusCode != http.StatusOK || res.Header.Get("X-Auth-Request-Email") != ada.Email || res.Header.Get("X-Auth-Request-Roles") != "user" {
		t.Fatalf("check = %d %v", res.StatusCode, res.Header)
	}
}

func TestAPersonalTokenOpensATokenHost(t *testing.T) {
	t.Parallel()
	env, _, secret := tokenFor(t, "backup")
	if res := env.GateCheck(t, "backup.test", "Bearer "+secret); res.StatusCode != http.StatusOK {
		t.Fatalf("check = %d", res.StatusCode)
	}
}

func TestAPersonalTokenForAnotherAppIsRefused(t *testing.T) {
	t.Parallel()
	env, _, secret := tokenFor(t, "backup")
	res := env.GateCheck(t, "stats.test", "Bearer "+secret)
	if res.StatusCode != http.StatusUnauthorized || res.Header.Get("WWW-Authenticate") != `Bearer realm="stats.test", error="invalid_token"` {
		t.Fatalf("check = %d %v", res.StatusCode, res.Header)
	}
}

func TestARevokedPersonalTokenIsRefused(t *testing.T) {
	t.Parallel()
	env, ada, secret := tokenFor(t, "backup")
	var mine struct{ Tokens []struct{ ID string } }
	ada.Call(t, http.MethodGet, "/api/v1/profile/tokens", nil, http.StatusOK, &mine)
	ada.Call(t, http.MethodDelete, "/api/v1/profile/tokens/"+mine.Tokens[0].ID, nil, http.StatusNoContent, nil)
	if res := env.GateCheck(t, "backup.test", "Bearer "+secret); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("check = %d", res.StatusCode)
	}
}

func TestATokenHostWithoutAPersonalTokenAsksForOne(t *testing.T) {
	t.Parallel()
	env := itest.Start(t, backup)
	res := env.GateCheck(t, "backup.test", "")
	if res.StatusCode != http.StatusUnauthorized || res.Header.Get("WWW-Authenticate") != `Bearer realm="backup.test"` {
		t.Fatalf("check = %d %v", res.StatusCode, res.Header)
	}
}
