// Package persistence is the Gate's Postgres adapter: app.Store over the sqlc queries.
package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/identity/internal/gate/app"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
)

// Store is the app.Store.
type Store struct{ q queries.Querier }

// New returns a Store over q.
func New(q queries.Querier) *Store { return &Store{q: q} }

var _ app.Store = (*Store)(nil)

func (s *Store) CreateCode(ctx context.Context, c app.Code) error {
	return s.q.CreateGateCode(ctx, queries.CreateGateCodeParams{
		CodeHash: c.Hash, Host: c.Host, SessionID: c.SessionID, StateHash: c.StateHash, ReturnPath: c.ReturnPath, ExpiresAt: c.ExpiresAt,
	})
}

func (s *Store) TakeCode(ctx context.Context, hash []byte) (app.Code, error) {
	r, err := s.q.TakeGateCode(ctx, hash)
	if err != nil {
		return app.Code{}, notFound(err)
	}
	return app.Code{Hash: hash, Host: r.Host, SessionID: r.SessionID, StateHash: r.StateHash, ReturnPath: r.ReturnPath, ExpiresAt: r.ExpiresAt}, nil
}

func (s *Store) CreateSession(ctx context.Context, hash []byte, gs app.Session, at time.Time) error {
	return s.q.CreateGateSession(ctx, queries.CreateGateSessionParams{TokenHash: hash, Host: gs.Host, SessionID: gs.SessionID, CreatedAt: at})
}

func (s *Store) SessionByHash(ctx context.Context, hash []byte) (app.Session, error) {
	r, err := s.q.GateSessionByHash(ctx, hash)
	if err != nil {
		return app.Session{}, notFound(err)
	}
	return app.Session{Host: r.Host, SessionID: r.SessionID}, nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}
