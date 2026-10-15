// Package pg is the Postgres adapter: the connection pool, goose migrations and the sqlc queries.
package pg

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver goose runs on
	"github.com/pressly/goose/v3"

	"github.com/JorisJonkers-dev/template-go-vue/db"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/pg/queries"
)

// Store owns the connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects to Postgres and checks the connection.
func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("pg: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pg: ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases every pooled connection.
func (s *Store) Close() { s.pool.Close() }

// Ping reports whether the database answers; readiness hangs off it.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Queries returns the generated queries, for the persistence adapters.
func (s *Store) Queries() *queries.Queries { return queries.New(s.pool) }

// Migrate applies every pending migration embedded in the binary.
func Migrate(ctx context.Context, url string) error {
	return MigrateFS(ctx, url, db.Migrations)
}

// MigrateFS applies the migrations under fsys/migrations; tests pass synthetic sets.
func MigrateFS(ctx context.Context, url string, fsys fs.FS) error {
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("pg: open for migrate: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()
	sub, err := fs.Sub(fsys, "migrations")
	if err != nil {
		return fmt.Errorf("pg: migrations dir: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, sub)
	if err != nil {
		return fmt.Errorf("pg: migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("pg: migrate up: %w", err)
	}
	return nil
}
