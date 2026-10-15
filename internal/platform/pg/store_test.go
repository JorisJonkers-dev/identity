package pg_test

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/pg"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/pg/pgtest"
)

const unreachable = "postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"

func TestStoreAgainstMigratedDatabase(t *testing.T) {
	t.Parallel()
	url := pgtest.URL(t)
	if err := pg.Migrate(t.Context(), url); err != nil {
		t.Fatalf("migrating an up-to-date database again: %v", err)
	}
	store, err := pg.Open(t.Context(), url)
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

func TestMigrateFailsOnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := pg.Migrate(ctx, unreachable); err == nil {
		t.Fatal("Migrate against an unreachable database succeeded")
	}
}

func TestMigrateRefusesAnEmptyMigrationSet(t *testing.T) {
	t.Parallel()
	if err := pg.MigrateFS(t.Context(), pgtest.URL(t), fstest.MapFS{"migrations/.keep": {}}); err == nil {
		t.Fatal("an empty migration set was accepted")
	}
}

func TestMigrateRefusesAMissingMigrationsDir(t *testing.T) {
	t.Parallel()
	if err := pg.MigrateFS(t.Context(), unreachable, fstest.MapFS{}); err == nil {
		t.Fatal("a filesystem without migrations/ was accepted")
	}
}
