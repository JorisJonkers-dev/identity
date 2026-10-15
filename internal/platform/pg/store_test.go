package pg_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
)

const unreachable = "postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"

func TestStoreAgainstMigratedDatabase(t *testing.T) {
	t.Parallel()
	store, err := pg.Open(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Ping(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Queries().ListNotes(t.Context(), 1); err != nil {
		t.Fatalf("the migrated schema has no notes table: %v", err)
	}
}

func TestSchemaIsAppliedByLiquibaseRunner(t *testing.T) {
	t.Parallel()
	conn, err := pgx.Connect(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	var tagged int
	if err := conn.QueryRow(t.Context(),
		"SELECT count(*) FROM databasechangelog WHERE author = 'liquibase-runner' AND tag = $1", pgtest.Tag,
	).Scan(&tagged); err != nil {
		t.Fatalf("no Liquibase changelog table: %v", err)
	}
	if tagged != 1 {
		t.Fatalf("runner tag rows = %d, want 1", tagged)
	}
}

func TestSchemaRefusesNoteTextOutsideTheDomainRule(t *testing.T) {
	t.Parallel()
	conn, err := pgx.Connect(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	for _, text := range []string{"", strings.Repeat("x", 501)} {
		if _, err := conn.Exec(t.Context(), "INSERT INTO notes (text) VALUES ($1)", text); err == nil {
			t.Fatalf("a note of %d characters was stored", len(text))
		}
	}
	if _, err := conn.Exec(t.Context(), "INSERT INTO notes (text) VALUES ($1)", strings.Repeat("x", 500)); err != nil {
		t.Fatalf("a note of 500 characters was refused: %v", err)
	}
}

func TestEveryChangesetRollsBack(t *testing.T) {
	t.Parallel()
	if err := pgtest.Reversible(t); err != nil {
		t.Fatal(err)
	}
}

func TestOpenFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	for _, url := range []string{unreachable, "::not a url::"} {
		if _, err := pg.Open(ctx, url); err == nil {
			t.Fatalf("Open(%q) succeeded", url)
		}
	}
}
