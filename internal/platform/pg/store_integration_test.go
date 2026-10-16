//go:build integration

package pg_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
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

func TestInTxCommitsOrRollsBack(t *testing.T) {
	t.Parallel()
	store, err := pg.Open(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	boom := errors.New("boom")
	if err := store.InTx(t.Context(), func(q *queries.Queries) error {
		return q.CreateGroup(t.Context(), queries.CreateGroupParams{ID: uuid.New(), Name: "Kept"})
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.InTx(t.Context(), func(q *queries.Queries) error {
		if err := q.CreateGroup(t.Context(), queries.CreateGroupParams{ID: uuid.New(), Name: "Dropped"}); err != nil {
			return err
		}
		return boom
	}); !errors.Is(err, boom) {
		t.Fatalf("InTx = %v, want boom", err)
	}
	groups, err := store.Queries().ListGroups(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, g := range groups {
		names = append(names, g.Name)
	}
	if len(names) != 2 || names[0] != "admins" || names[1] != "Kept" {
		t.Fatalf("groups = %v, want [admins Kept]", names)
	}
}
