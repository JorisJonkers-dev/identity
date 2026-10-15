// Package pg is the Postgres adapter: the connection pool and the sqlc queries. The schema is
// migrated by liquibase-runner in its own image, never by this binary (docs/adr/0010).
package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
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
