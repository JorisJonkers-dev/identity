//go:build integration

package pg_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
)

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
	if ok, err := store.Queries().Ping(t.Context()); err != nil || ok != 1 {
		t.Fatalf("Ping query = %d, %v; want 1", ok, err)
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

func TestEveryChangesetRollsBack(t *testing.T) {
	t.Parallel()
	if err := pgtest.Reversible(t); err != nil {
		t.Fatal(err)
	}
}
