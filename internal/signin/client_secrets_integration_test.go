//go:build integration

// Package signin_test checks Secret Clients' secrets the way an Application and an Admin reach them:
// in Vault KV and over HTTP, against the real routes, Postgres, Vault and mail
// (docs/testing-standards.md).
package signin_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

func grafana(t *testing.T) signindomain.Client {
	t.Helper()
	return signindomain.Client{
		ID: "grafana", SignIn: signindomain.Native, Host: "grafana.test", Label: "Grafana", Roles: []string{"user"},
		Credential: signindomain.Secret, SecretPath: itest.SecretPath(t), RedirectURIs: []string{"https://grafana.test/callback"},
	}
}

func TestANewSecretClientGetsASecretItsApplicationReadsAndTheTokenEndpointAccepts(t *testing.T) {
	t.Parallel()
	c := grafana(t)
	env := itest.Start(t, c)
	written := itest.ReadSecret(t, c.SecretPath)
	if written["id"] != "grafana" || len(written["secret"]) < 43 {
		t.Fatalf("at %s = %d keys, id %q", c.SecretPath, len(written), written["id"])
	}
	if got := env.ClientAuth(t, "grafana", written["secret"]); got != http.StatusOK {
		t.Fatalf("the secret written = %d", got)
	}
	if got := env.ClientAuth(t, "grafana", "not-the-secret"); got != http.StatusUnauthorized {
		t.Fatalf("a wrong secret = %d", got)
	}
}

func TestASecretAlreadyAtThePathIsKeptAndAccepted(t *testing.T) {
	t.Parallel()
	c := grafana(t)
	itest.PutSecret(t, c.SecretPath, "grafana", "from-before-identity-started")
	env := itest.Start(t, c)
	if got := itest.ReadSecret(t, c.SecretPath)["secret"]; got != "from-before-identity-started" {
		t.Fatal("the secret at the path was replaced")
	}
	if got := env.ClientAuth(t, "grafana", "from-before-identity-started"); got != http.StatusOK {
		t.Fatalf("the existing secret = %d", got)
	}
}

func TestARestartChangesNoSecret(t *testing.T) {
	t.Parallel()
	c := grafana(t)
	itest.Start(t, c)
	first := itest.ReadSecret(t, c.SecretPath)["secret"]
	again := itest.Start(t, c)
	if itest.ReadSecret(t, c.SecretPath)["secret"] != first {
		t.Fatal("starting again replaced the secret")
	}
	if got := again.ClientAuth(t, "grafana", first); got != http.StatusOK {
		t.Fatalf("the secret after a restart = %d", got)
	}
}

func TestAnAdminRotatesASecretAndTheOldOneStopsOnceTheNewOneIsUsed(t *testing.T) {
	t.Parallel()
	c := grafana(t)
	env := itest.Start(t, c)
	old := itest.ReadSecret(t, c.SecretPath)["secret"]
	env.Admin(t, "root@example.org").Call(t, http.MethodPost, "/api/v1/apps/grafana/secret", nil, http.StatusNoContent, nil)
	renewed := itest.ReadSecret(t, c.SecretPath)
	if renewed["secret"] == old || renewed["id"] != "grafana" {
		t.Fatal("the Application reads no new secret")
	}
	if got := env.ClientAuth(t, "grafana", old); got != http.StatusOK {
		t.Fatalf("the old secret before the new one is used = %d", got)
	}
	if got := env.ClientAuth(t, "grafana", renewed["secret"]); got != http.StatusOK {
		t.Fatalf("the new secret = %d", got)
	}
	if got := env.ClientAuth(t, "grafana", old); got != http.StatusUnauthorized {
		t.Fatalf("the old secret once the new one is used = %d", got)
	}
}

func TestOnlyAnAdminWithASecondFactorRotatesASecret(t *testing.T) {
	t.Parallel()
	c := grafana(t)
	env := itest.Start(t, c)
	before := itest.ReadSecret(t, c.SecretPath)["secret"]
	env.SignedUp(t, "ada@example.org").Call(t, http.MethodPost, "/api/v1/apps/grafana/secret", nil, http.StatusForbidden, nil)
	if itest.ReadSecret(t, c.SecretPath)["secret"] != before {
		t.Fatal("a User who is not an Admin rotated the secret")
	}
}

func TestRotatingAPublicClientOrAMissingOneIsRefused(t *testing.T) {
	t.Parallel()
	spa := signindomain.Client{
		ID: "spa", SignIn: signindomain.Native, Host: "spa.test", Label: "SPA", Roles: []string{"user"},
		Credential: signindomain.PKCE, RedirectURIs: []string{"https://spa.test/callback"},
	}
	env := itest.Start(t, spa)
	admin := env.Admin(t, "root@example.org")
	admin.Call(t, http.MethodPost, "/api/v1/apps/spa/secret", nil, http.StatusUnprocessableEntity, nil)
	admin.Call(t, http.MethodPost, "/api/v1/apps/nobody/secret", nil, http.StatusNotFound, nil)
}

func TestARotationIsAnAuditEventAndNoSecretIsLoggedOrRecorded(t *testing.T) {
	t.Parallel()
	c := grafana(t)
	env := itest.Start(t, c)
	old := itest.ReadSecret(t, c.SecretPath)["secret"]
	admin := env.Admin(t, "root@example.org")
	admin.Call(t, http.MethodPost, "/api/v1/apps/grafana/secret", nil, http.StatusNoContent, nil)
	renewed := itest.ReadSecret(t, c.SecretPath)["secret"]
	env.ClientAuth(t, "grafana", "not-the-secret")

	var events struct {
		Events []struct {
			Action string
			Actor  struct{ Email string }
			Client struct{ ID string }
		}
	}
	admin.Call(t, http.MethodGet, "/api/v1/audit?kind=access", nil, http.StatusOK, &events)
	if len(events.Events) == 0 || events.Events[0].Action != "client-secret-rotated" ||
		events.Events[0].Actor.Email != admin.Email || events.Events[0].Client.ID != "grafana" {
		t.Fatalf("events = %+v", events.Events)
	}
	recorded, _ := json.Marshal(events)
	logged, _ := json.Marshal(env.Logs(t))
	for _, secret := range []string{old, renewed} {
		if strings.Contains(string(recorded), secret) || strings.Contains(string(logged), secret) {
			t.Fatal("a Client secret was logged or recorded")
		}
	}
}
