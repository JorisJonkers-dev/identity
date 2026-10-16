//go:build integration

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
)

func TestPromoteMakesTheFirstAdminAndNeedsForceForMore(t *testing.T) {
	url := pgtest.URL(t)
	conn, err := pgx.Connect(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	for _, email := range []string{"ada@example.org", "bob@example.org"} {
		if _, err := conn.Exec(t.Context(), `INSERT INTO users (id, email, name, password_hash, created_at) VALUES ($1, $2, 'x', 'h', $3)`,
			uuid.New(), email, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	vars := env(map[string]string{"DATABASE_URL": url})
	run := func(args ...string) (int, string, string) {
		var out, errs strings.Builder
		code := command(t.Context(), vars, args, &out, &errs)
		return code, out.String(), errs.String()
	}
	admins := func() int {
		var n int
		_ = conn.QueryRow(t.Context(), `SELECT count(*) FROM memberships m JOIN groups g ON g.id = m.group_id WHERE g.admins`).Scan(&n)
		return n
	}

	if code, _, errs := run("admin", "promote", "nobody@example.org"); code != 1 || !strings.Contains(errs, "no User has the email") {
		t.Fatalf("an unknown email = %d, %q", code, errs)
	}
	if code, out, _ := run("admin", "promote", "Ada@Example.org"); code != 0 || out != "Ada@Example.org is now an Admin.\n" || admins() != 1 {
		t.Fatalf("the first promote = %d, %q, %d Admins", code, out, admins())
	}
	if code, _, errs := run("admin", "promote", "bob@example.org"); code != 1 || !strings.Contains(errs, "--force") || admins() != 1 {
		t.Fatalf("a second promote = %d, %q, %d Admins", code, errs, admins())
	}
	if code, _, _ := run("admin", "promote", "bob@example.org", "--force"); code != 0 || admins() != 2 {
		t.Fatalf("a forced promote = %d, %d Admins", code, admins())
	}
}
