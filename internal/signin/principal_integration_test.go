//go:build integration

package signin_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
)

// channels is identity with the Wiki, and Grafana taking Connected Apps; Ada holds user on both.
type channels struct {
	env        *itest.Env
	admin, ada *itest.Browser
	adaID      string
	// secret is the one identity provisioned for Grafana, which it introspects with.
	secret string
}

func startChannels(t *testing.T) channels {
	t.Helper()
	gr := grafana(t)
	env := itest.Start(t, gr, wiki)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	c := channels{env: env, admin: admin, ada: ada, adaID: admin.UserID(t, ada.Email), secret: itest.ReadSecret(t, gr.SecretPath)["secret"]}
	for _, client := range []string{"wiki", "grafana"} {
		admin.Call(t, http.MethodPut, "/api/v1/access/users/"+c.adaID+"/roles/"+client+"/user", nil, http.StatusNoContent, nil)
	}
	admin.Call(t, http.MethodPut, "/api/v1/apps/grafana/mcp", map[string]bool{"on": true}, http.StatusNoContent, nil)
	return c
}

func TestAUsersAccessTokenStatesTheirPrincipalAndTheSessionThatSignedThemIn(t *testing.T) {
	t.Parallel()
	c := startChannels(t)

	p := c.env.AccessTokenPrincipal(t, c.ada.AccessToken(t, wiki), "wiki")

	if p["subject"] != c.adaID || p["kind"] != "user" || p["email"] != c.ada.Email || p["via"] != "session" || p["sid"] == nil || p["actor"] != nil {
		t.Fatalf("principal = %v", p)
	}
}

func TestAUsersAccessTokenSaysWhenTheSessionProvedTwoFactor(t *testing.T) {
	t.Parallel()
	c := startChannels(t)
	c.ada.AddAuthenticator(t)

	p := c.env.AccessTokenPrincipal(t, c.ada.AccessToken(t, wiki), "wiki")

	if p["twoFactor"] != true || p["twoFactorAt"] == nil {
		t.Fatalf("principal = %v", p)
	}
}

func TestAMachineIdentitysAccessTokenStatesAMachineWithNoEmailOrName(t *testing.T) {
	t.Parallel()
	c := startChannels(t)
	var m struct{ ID string }
	c.admin.Call(t, http.MethodPost, "/api/v1/access/machines", map[string]string{"name": "Backup", "namespace": "media", "serviceAccount": "backup"}, http.StatusCreated, &m)
	c.admin.Call(t, http.MethodPut, "/api/v1/access/machines/"+m.ID+"/roles/grafana/user", nil, http.StatusNoContent, nil)
	status, tokens := c.env.Token(t, url.Values{
		"grant_type":         {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token":      {itest.ServiceAccountToken(t, "media", "backup", "https://"+itest.Host)},
		"subject_token_type": {"urn:ietf:params:oauth:token-type:jwt"},
		"audience":           {"grafana"},
	}, m.ID, "")
	if status != http.StatusOK {
		t.Fatalf("exchange = %d %v", status, tokens)
	}

	p := c.env.AccessTokenPrincipal(t, tokens["access_token"].(string), "grafana")

	if p["subject"] != m.ID || p["kind"] != "machine" || p["via"] != "machine" || p["email"] != nil || p["name"] != nil {
		t.Fatalf("principal = %v", p)
	}
}

func TestAPersonalTokensIntrospectionStatesItsUserByPersonalToken(t *testing.T) {
	t.Parallel()
	c := startChannels(t)
	var minted struct{ Secret string }
	c.ada.Call(t, http.MethodPost, "/api/v1/profile/tokens", map[string]any{"name": "script", "clientId": "grafana", "days": 30}, http.StatusCreated, &minted)

	p := itest.IntrospectionPrincipal(t, c.env.Introspect(t, "grafana", c.secret, minted.Secret), "grafana")

	if p["subject"] != c.adaID || p["via"] != "personal-token" || p["sid"] != nil || p["actor"] != nil {
		t.Fatalf("principal = %v", p)
	}
}

func TestAConnectedAppsIntrospectionNamesTheToolActingForTheUser(t *testing.T) {
	t.Parallel()
	c := startChannels(t)
	tool, token := c.ada.ConnectedApp(t, "Claude Code", "https://grafana.test/mcp")

	p := itest.IntrospectionPrincipal(t, c.env.Introspect(t, "grafana", c.secret, token), "grafana")

	actor, _ := p["actor"].(map[string]any)
	if p["subject"] != c.adaID || p["via"] != "tool" || actor["id"] != tool || actor["name"] != "Claude Code" {
		t.Fatalf("principal = %v", p)
	}
}
