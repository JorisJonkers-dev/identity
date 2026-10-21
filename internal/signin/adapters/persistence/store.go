// Package persistence is the sign-in context's Postgres adapter: app.Requests over the sqlc queries.
package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// Store is the app.Requests.
type Store struct{ q queries.Querier }

// New returns a Store over q.
func New(q queries.Querier) *Store { return &Store{q: q} }

var _ app.Requests = (*Store)(nil)

func (s *Store) CreateRequest(ctx context.Context, r domain.AuthRequest) error {
	return s.q.CreateAuthRequest(ctx, queries.CreateAuthRequestParams{
		ID: r.ID, ClientID: r.ClientID, RedirectUri: r.RedirectURI, Scopes: r.Scopes, State: r.State, Nonce: r.Nonce,
		ResponseType: r.ResponseType, ResponseMode: r.ResponseMode,
		CodeChallenge: r.CodeChallenge, CodeChallengeMethod: r.CodeChallengeMethod,
		CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, Resource: pgtype.Text{String: r.Resource, Valid: r.Resource != ""},
		TwoFactorWithinSeconds: secondsOf(r.TwoFactorWithin),
	})
}

func (s *Store) RequestByID(ctx context.Context, id uuid.UUID) (domain.AuthRequest, error) {
	return toRequest(s.q.AuthRequestByID(ctx, id))
}

func (s *Store) TakeByCodeHash(ctx context.Context, codeHash []byte) (domain.AuthRequest, error) {
	return toRequest(s.q.TakeAuthRequestByCodeHash(ctx, codeHash))
}

func (s *Store) CompleteRequest(ctx context.Context, id uuid.UUID, in domain.SignIn) (bool, error) {
	params := queries.CompleteAuthRequestParams{ID: id, UserID: &in.UserID, SessionID: &in.SessionID, AuthTime: &in.At, Methods: in.Methods}
	if !in.TwoFactorAt.IsZero() {
		params.TwoFactorAt = &in.TwoFactorAt
	}
	n, err := s.q.CompleteAuthRequest(ctx, params)
	return n == 1, err
}

func (s *Store) SaveCode(ctx context.Context, id uuid.UUID, codeHash []byte) (bool, error) {
	n, err := s.q.SaveAuthCode(ctx, queries.SaveAuthCodeParams{ID: id, CodeHash: codeHash})
	return n == 1, err
}

func (s *Store) DeleteRequest(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteAuthRequest(ctx, id)
}

func toRequest(row queries.AuthRequest, err error) (domain.AuthRequest, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AuthRequest{}, app.ErrNotFound
	}
	if err != nil {
		return domain.AuthRequest{}, err
	}
	r := domain.AuthRequest{
		ID: row.ID, ClientID: row.ClientID, RedirectURI: row.RedirectUri, Scopes: row.Scopes, State: row.State,
		Nonce: row.Nonce, ResponseType: row.ResponseType, ResponseMode: row.ResponseMode,
		CodeChallenge: row.CodeChallenge, CodeChallengeMethod: row.CodeChallengeMethod,
		CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, Resource: row.Resource.String,
	}
	if row.TwoFactorWithinSeconds.Valid {
		r.TwoFactorWithin = time.Duration(row.TwoFactorWithinSeconds.Int32) * time.Second
	}
	// The three are written together by CompleteAuthRequest, so one stands for all.
	if row.UserID != nil {
		r.SignIn = &domain.SignIn{UserID: *row.UserID, SessionID: *row.SessionID, At: *row.AuthTime, Methods: row.Methods}
		if row.TwoFactorAt != nil {
			r.SignIn.TwoFactorAt = *row.TwoFactorAt
		}
	}
	return r, nil
}

func (s *Store) AddSessionClient(ctx context.Context, sessionID uuid.UUID, clientID string) error {
	return s.q.AddSessionClient(ctx, queries.AddSessionClientParams{SessionID: sessionID, ClientID: clientID})
}

func (s *Store) TakeSessionClients(ctx context.Context, sessionIDs []uuid.UUID) ([]app.SessionClient, error) {
	rows, err := s.q.TakeSessionClients(ctx, sessionIDs)
	if err != nil {
		return nil, err
	}
	out := make([]app.SessionClient, 0, len(rows))
	for _, r := range rows {
		out = append(out, app.SessionClient{SessionID: r.SessionID, ClientID: r.ClientID})
	}
	return out, nil
}

// secondsOf is d in whole seconds, or NULL for none. It is at most domain.StalestProof, so it fits.
func secondsOf(d time.Duration) pgtype.Int4 {
	return pgtype.Int4{Int32: int32(d / time.Second), Valid: d > 0} //nolint:gosec // an hour at most
}
