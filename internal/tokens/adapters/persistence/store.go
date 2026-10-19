// Package persistence is the tokens context's Postgres adapter: app.Store over the sqlc queries.
package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/identity/internal/tokens/app"
	"github.com/JorisJonkers-dev/identity/internal/tokens/domain"
)

// Store is the app.Store.
type Store struct{ q queries.Querier }

// New returns a Store over q.
func New(q queries.Querier) *Store { return &Store{q: q} }

var _ app.Store = (*Store)(nil)

func (s *Store) Create(ctx context.Context, t domain.Token, hash []byte) error {
	return s.q.CreatePersonalToken(ctx, queries.CreatePersonalTokenParams{
		ID: t.ID, UserID: t.UserID, ClientID: t.ClientID, Name: t.Name, TokenHash: hash, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt,
	})
}

// ByHash is the token's id and User only: what a Client's check needs next.
func (s *Store) ByHash(ctx context.Context, hash []byte) (domain.Token, error) {
	r, err := s.q.PersonalTokenByHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Token{}, app.ErrNotFound
	}
	if err != nil {
		return domain.Token{}, err
	}
	return domain.Token{ID: r.ID, UserID: r.UserID}, nil
}

func (s *Store) ByID(ctx context.Context, id uuid.UUID) (domain.Token, error) {
	r, err := s.q.PersonalTokenByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Token{}, app.ErrNotFound
	}
	if err != nil {
		return domain.Token{}, err
	}
	return domain.Token(r), nil
}

func (s *Store) OfUser(ctx context.Context, userID uuid.UUID) ([]domain.Token, error) {
	rows, err := s.q.ListPersonalTokens(ctx, userID)
	if err != nil {
		return nil, err
	}
	tokens := make([]domain.Token, 0, len(rows))
	for _, r := range rows {
		tokens = append(tokens, domain.Token(r))
	}
	return tokens, nil
}

func (s *Store) Revoke(ctx context.Context, id, userID uuid.UUID, at time.Time) (bool, error) {
	n, err := s.q.RevokePersonalToken(ctx, queries.RevokePersonalTokenParams{ID: id, UserID: userID, RevokedAt: &at})
	return n > 0, err
}

func (s *Store) RevokeForClient(ctx context.Context, clientID string, at time.Time) ([]domain.Token, error) {
	rows, err := s.q.RevokeClientTokens(ctx, queries.RevokeClientTokensParams{ClientID: clientID, RevokedAt: &at})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Token, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Token{ID: r.ID, UserID: r.UserID, ClientID: clientID})
	}
	return out, nil
}

func (s *Store) LiveForClient(ctx context.Context, clientID string, now time.Time) (int, error) {
	n, err := s.q.CountLiveClientTokens(ctx, queries.CountLiveClientTokensParams{ClientID: clientID, ExpiresAt: now})
	return int(n), err
}

func (s *Store) Touch(ctx context.Context, id uuid.UUID, at time.Time) error {
	return s.q.TouchPersonalToken(ctx, queries.TouchPersonalTokenParams{ID: id, LastUsedAt: &at})
}

func (s *Store) CountExpiring(ctx context.Context, now, before time.Time) (int, error) {
	n, err := s.q.CountPersonalTokensExpiring(ctx, queries.CountPersonalTokensExpiringParams{ExpiresAt: now, ExpiresAt_2: before})
	return int(n), err
}
