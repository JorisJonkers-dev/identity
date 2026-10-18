//go:build dev && integration

package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
)

// seeded runs the seed against a database of the test's own and the shared dev-mode Vault, and
// returns the database's URL.
func seeded(t *testing.T) string {
	t.Helper()
	url := pgtest.URL(t)
	addr, token := itest.Vault(t)
	env := map[string]string{"DATABASE_URL": url, "VAULT_ADDR": addr, "VAULT_TOKEN": token, "CLIENTS_FILE": "../../dev/clients.yaml"}
	if err := run(t.Context(), func(k string) string { return env[k] }, io.Discard, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	return url
}

func count(t *testing.T, url, query string) int {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var n int
	if err := conn.QueryRow(t.Context(), query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestTheSeedHoldsTheCanvasPeopleGroupsAndAccess(t *testing.T) {
	url := seeded(t)
	for _, c := range []struct {
		name, query string
		want        int
	}{
		{"people", "SELECT count(*) FROM users", 12},
		{"Groups, admins among them", "SELECT count(*) FROM groups", 4},
		{"Admins", "SELECT count(*) FROM memberships m JOIN groups g ON g.id = m.group_id WHERE g.admins", 2},
		{"people waiting for a Group", "SELECT count(*) FROM users u WHERE NOT EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = u.id)", 2},
		{"authenticator apps", "SELECT count(*) FROM totp_factors", 3},
		{"the Removed app's Assignments", "SELECT count(*) FROM assignments WHERE client_id = 'old-wiki'", 3},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := count(t, url, c.query); got != c.want {
				t.Fatalf("%d, want %d", got, c.want)
			}
		})
	}
}

func TestTheSeedHoldsTokensAndADayOfAuditEvents(t *testing.T) {
	url := seeded(t)
	for _, c := range []struct {
		name, query string
		atLeast     int
	}{
		{"tokens ending within 7 days", "SELECT count(*) FROM personal_tokens WHERE revoked_at IS NULL AND expires_at > now() AND expires_at < now() + interval '7 days'", 2},
		{"a revoked token", "SELECT count(*) FROM personal_tokens WHERE revoked_at IS NOT NULL", 1},
		{"sign-ins in the last day", "SELECT count(*) FROM audit_events WHERE action = 'signed-in' AND at > now() - interval '24 hours'", 100},
		{"failed sign-ins in the last day", "SELECT count(*) FROM audit_events WHERE action = 'sign-in-failed' AND at > now() - interval '24 hours'", 4},
		{"hours with sign-ins", "SELECT count(DISTINCT date_trunc('hour', at)) FROM audit_events WHERE action = 'signed-in' AND at > now() - interval '24 hours'", 20},
		{"access changes in the last hours", "SELECT count(*) FROM audit_events WHERE action IN ('member-added', 'role-assigned') AND at > now() - interval '3 hours'", 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := count(t, url, c.query); got < c.atLeast {
				t.Fatalf("%d, want at least %d", got, c.atLeast)
			}
		})
	}
}

func TestSeedingAgainChangesNothing(t *testing.T) {
	url := seeded(t)
	before := count(t, url, "SELECT count(*) FROM audit_events")
	addr, token := itest.Vault(t)
	env := map[string]string{"DATABASE_URL": url, "VAULT_ADDR": addr, "VAULT_TOKEN": token, "CLIENTS_FILE": "../../dev/clients.yaml"}
	var out bytes.Buffer
	if err := run(t.Context(), func(k string) string { return env[k] }, &out, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Already seeded") || count(t, url, "SELECT count(*) FROM audit_events") != before {
		t.Fatalf("a second seed said %q and changed the Audit Events", out.String())
	}
}
