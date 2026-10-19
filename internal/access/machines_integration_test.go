//go:build integration

package access_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
)

type machine struct {
	ID             string
	Name           string
	Namespace      string
	ServiceAccount string
}

type machinesOverview struct {
	Machines    []machine
	Assignments []struct{ MachineID, ClientID, Role string }
}

// addMachine is a Machine Identity bound to media/<serviceAccount>, holding viewer on Grafana.
func addMachine(t *testing.T, admin *itest.Browser, name, serviceAccount string) machine {
	t.Helper()
	var m machine
	admin.Call(t, http.MethodPost, "/api/v1/access/machines", map[string]string{"name": name, "namespace": "media", "serviceAccount": serviceAccount}, http.StatusCreated, &m)
	admin.Call(t, http.MethodPut, "/api/v1/access/machines/"+m.ID+"/roles/grafana/viewer", nil, http.StatusNoContent, nil)
	return m
}

// exchange is a pod's token exchange: its ServiceAccount token for an access token for audience,
// with the Machine Identity's id as client_id in basic auth and no secret.
func exchange(t *testing.T, env *itest.Env, clientID, saToken, audience string) (int, map[string]any) {
	t.Helper()
	return env.Token(t, url.Values{
		"grant_type":         {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token":      {saToken},
		"subject_token_type": {"urn:ietf:params:oauth:token-type:jwt"},
		"audience":           {audience},
	}, clientID, "")
}

// claimsOf is an access token's claims, read without checking it: identity signed it.
func claimsOf(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", token)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}

func roles(claims map[string]any) []string {
	var out []string
	list, _ := claims["roles"].([]any)
	for _, r := range list {
		s, _ := r.(string)
		out = append(out, s)
	}
	return out
}

func audienceOf(claims map[string]any) []string {
	switch a := claims["aud"].(type) {
	case string:
		return []string{a}
	case []any:
		var out []string
		for _, s := range a {
			v, _ := s.(string)
			out = append(out, v)
		}
		return out
	}
	return nil
}

func TestAnAdminAddsAMachineIdentityAndGivesItARole(t *testing.T) {
	env := itest.Start(t, grafana)
	admin := env.Admin(t, "admin@example.org")

	m := addMachine(t, admin, "Backup", "backup")

	var o machinesOverview
	admin.Call(t, http.MethodGet, "/api/v1/access/machines", nil, http.StatusOK, &o)
	if len(o.Machines) != 1 || o.Machines[0] != m || m.Name != "Backup" || m.Namespace != "media" || m.ServiceAccount != "backup" {
		t.Fatalf("machines = %+v", o.Machines)
	}
	if len(o.Assignments) != 1 || o.Assignments[0].MachineID != m.ID || o.Assignments[0].Role != "viewer" {
		t.Fatalf("assignments = %+v", o.Assignments)
	}
}

func TestAPodExchangesItsServiceAccountTokenForAnAccessTokenCarryingItsRoles(t *testing.T) {
	env := itest.Start(t, grafana)
	m := addMachine(t, env.Admin(t, "admin@example.org"), "Backup", "backup")

	status, body := exchange(t, env, m.ID, itest.ServiceAccountToken(t, "media", "backup", "https://"+itest.Host), "grafana")

	if status != http.StatusOK {
		t.Fatalf("exchange = %d %v", status, body)
	}
	token, _ := body["access_token"].(string)
	claims := claimsOf(t, token)
	if claims["sub"] != m.ID || !slices.Equal(audienceOf(claims), []string{"grafana"}) || !slices.Equal(roles(claims), []string{"viewer"}) {
		t.Fatalf("claims = %v", claims)
	}
}

func TestAServiceAccountTokenIsRefused(t *testing.T) {
	for _, c := range []struct {
		name    string
		token   func(t *testing.T) string
		machine string
	}{
		{"for another audience", func(t *testing.T) string { return itest.ServiceAccountToken(t, "media", "backup", "vault") }, "Backup"},
		{"from a ServiceAccount no Machine Identity is bound to", func(t *testing.T) string {
			return itest.ServiceAccountToken(t, "media", "intruder", "https://"+itest.Host)
		}, "Backup"},
		{"from another Machine Identity's ServiceAccount", func(t *testing.T) string {
			return itest.ServiceAccountToken(t, "media", "sync", "https://"+itest.Host)
		}, "Backup"},
		{"that the cluster never signed", func(*testing.T) string { return "eyJhbGciOiJub25lIn0.e30." }, "Backup"},
	} {
		t.Run(c.name, func(t *testing.T) {
			env := itest.Start(t, grafana)
			admin := env.Admin(t, "admin@example.org")
			backup := addMachine(t, admin, "Backup", "backup")
			addMachine(t, admin, "Sync", "sync")

			status, body := exchange(t, env, backup.ID, c.token(t), "grafana")

			if status != http.StatusBadRequest || body["access_token"] != nil {
				t.Fatalf("exchange = %d %v", status, body)
			}
		})
	}
}

func TestAMachineIdentityGetsNoTokenForAClientItHoldsNoRoleOn(t *testing.T) {
	env := itest.Start(t, grafana, stats)
	m := addMachine(t, env.Admin(t, "admin@example.org"), "Backup", "backup")

	status, body := exchange(t, env, m.ID, itest.ServiceAccountToken(t, "media", "backup", "https://"+itest.Host), "stats")

	if status == http.StatusOK || body["error"] != "access_denied" {
		t.Fatalf("exchange = %d %v", status, body)
	}
}

func TestADeletedMachineIdentityCanNoLongerExchangeItsToken(t *testing.T) {
	env := itest.Start(t, grafana)
	admin := env.Admin(t, "admin@example.org")
	m := addMachine(t, admin, "Backup", "backup")

	admin.Call(t, http.MethodDelete, "/api/v1/access/machines/"+m.ID, nil, http.StatusNoContent, nil)

	status, _ := exchange(t, env, m.ID, itest.ServiceAccountToken(t, "media", "backup", "https://"+itest.Host), "grafana")
	if status == http.StatusOK {
		t.Fatal("a deleted Machine Identity got a token")
	}
}

func TestDeletingAMachineIdentityDeletesItsSecret(t *testing.T) {
	env := itest.Start(t, grafana)
	admin := env.Admin(t, "admin@example.org")
	m := addMachine(t, admin, "Backup", "backup")
	rotate(t, admin, m.ID)

	admin.Call(t, http.MethodDelete, "/api/v1/access/machines/"+m.ID, nil, http.StatusNoContent, nil)

	conn, err := pgx.Connect(t.Context(), env.DatabaseURL())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	var left int
	if err := conn.QueryRow(t.Context(), `SELECT count(*) FROM client_secrets WHERE client_id = $1`, m.ID).Scan(&left); err != nil || left != 0 {
		t.Fatalf("%d secrets left, %v", left, err)
	}
}

// credentials is an out-of-cluster sign-in with the client credentials grant, for a token to Grafana.
func credentials(t *testing.T, env *itest.Env, clientID, secret string) (int, map[string]any) {
	t.Helper()
	return env.Token(t, url.Values{"grant_type": {"client_credentials"}, "audience": {"grafana"}}, clientID, secret)
}

func rotate(t *testing.T, admin *itest.Browser, id string) string {
	t.Helper()
	var s struct{ Secret string }
	admin.Call(t, http.MethodPost, "/api/v1/access/machines/"+id+"/secret", nil, http.StatusOK, &s)
	return s.Secret
}

func TestAMachineIdentityOutsideTheClusterSignsInWithTheSecretAnAdminGaveIt(t *testing.T) {
	env := itest.Start(t, grafana)
	admin := env.Admin(t, "admin@example.org")
	m := addMachine(t, admin, "Backup", "backup")
	secret := rotate(t, admin, m.ID)

	status, body := credentials(t, env, m.ID, secret)

	if status != http.StatusOK {
		t.Fatalf("client credentials = %d %v", status, body)
	}
	token, _ := body["access_token"].(string)
	claims := claimsOf(t, token)
	if claims["sub"] != m.ID || !slices.Equal(audienceOf(claims), []string{"grafana"}) || !slices.Equal(roles(claims), []string{"viewer"}) {
		t.Fatalf("claims = %v", claims)
	}
}

func TestARotatedMachineSecretStopsOnceTheNewOneIsUsed(t *testing.T) {
	env := itest.Start(t, grafana)
	admin := env.Admin(t, "admin@example.org")
	m := addMachine(t, admin, "Backup", "backup")
	old := rotate(t, admin, m.ID)
	current := rotate(t, admin, m.ID)

	if status, _ := credentials(t, env, m.ID, current); status != http.StatusOK {
		t.Fatalf("the new secret = %d", status)
	}
	if status, _ := credentials(t, env, m.ID, old); status != http.StatusUnauthorized {
		t.Fatalf("the replaced secret = %d", status)
	}
}

func TestAWrongMachineSecretIsRefused(t *testing.T) {
	env := itest.Start(t, grafana)
	admin := env.Admin(t, "admin@example.org")
	m := addMachine(t, admin, "Backup", "backup")
	rotate(t, admin, m.ID)

	if status, _ := credentials(t, env, m.ID, "not-the-secret"); status != http.StatusUnauthorized {
		t.Fatalf("a wrong secret = %d", status)
	}
}

func TestAMachineSecretIsNeverLoggedNorInAnAuditEvent(t *testing.T) {
	env := itest.Start(t, grafana)
	admin := env.Admin(t, "admin@example.org")
	m := addMachine(t, admin, "Backup", "backup")
	secret := rotate(t, admin, m.ID)
	credentials(t, env, m.ID, secret)

	var events struct{ Events []map[string]any }
	admin.Call(t, http.MethodGet, "/api/v1/audit", nil, http.StatusOK, &events)
	raw, _ := json.Marshal(struct {
		Logs   []map[string]any
		Events []map[string]any
	}{env.Logs(t), events.Events})
	if strings.Contains(string(raw), secret) {
		t.Fatal("the secret is in a log line or an Audit Event")
	}
}

func TestMachineIdentitiesNeverShowAmongPeopleOrInTheUnassignedQueue(t *testing.T) {
	env := itest.Start(t, grafana)
	admin := env.Admin(t, "admin@example.org")
	var before, after struct{ Users []struct{ ID string } }
	admin.Call(t, http.MethodGet, "/api/v1/access", nil, http.StatusOK, &before)

	addMachine(t, admin, "Backup", "backup")

	admin.Call(t, http.MethodGet, "/api/v1/access", nil, http.StatusOK, &after)
	if len(after.Users) != len(before.Users) {
		t.Fatalf("people went from %d to %d", len(before.Users), len(after.Users))
	}
	var dashboard struct{ Unassigned []any }
	admin.Call(t, http.MethodGet, "/api/v1/dashboard", nil, http.StatusOK, &dashboard)
	if len(dashboard.Unassigned) != 0 {
		t.Fatalf("unassigned = %v", dashboard.Unassigned)
	}
}

func TestMachineIdentityChangesAreAuditEventsNamingIt(t *testing.T) {
	env := itest.Start(t, grafana)
	admin := env.Admin(t, "admin@example.org")
	m := addMachine(t, admin, "Backup", "backup")
	rotate(t, admin, m.ID)

	var page struct {
		Events []struct {
			Action      string
			MachineName string
		}
	}
	admin.Call(t, http.MethodGet, "/api/v1/audit?kind=access", nil, http.StatusOK, &page)
	var seen []string
	for _, e := range page.Events {
		if e.MachineName == "Backup" {
			seen = append(seen, e.Action)
		}
	}
	slices.Sort(seen)
	if !slices.Equal(seen, []string{"machine-created", "machine-secret-rotated", "role-assigned"}) {
		t.Fatalf("events naming Backup = %v", seen)
	}
}

func TestOnlyAnAdminManagesMachineIdentities(t *testing.T) {
	env := itest.Start(t, grafana)
	ada := env.SignedUp(t, "ada@example.org")
	refused(t, ada, http.MethodPost, "/api/v1/access/machines", map[string]string{"name": "Backup", "namespace": "media", "serviceAccount": "backup"}, http.StatusForbidden, "Only Admins manage access.")
}
