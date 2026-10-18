package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

// ClientSecrets is the app.SecretHashes: every replica takes one advisory lock per Client to make
// or rotate its secret.
type ClientSecrets struct {
	q   queries.Querier
	tx  Transactor
	now func() time.Time
}

// NewClientSecrets returns ClientSecrets over q, locking through tx.
func NewClientSecrets(q queries.Querier, tx Transactor, now func() time.Time) *ClientSecrets {
	return &ClientSecrets{q: q, tx: tx, now: now}
}

var _ app.SecretHashes = (*ClientSecrets)(nil)

func (s *ClientSecrets) Provision(ctx context.Context, clientID string, mint func() ([]byte, error)) error {
	return s.tx.InTx(ctx, func(q *queries.Queries) error {
		if err := q.LockClientSecret(ctx, clientID); err != nil {
			return err
		}
		_, err := q.ClientSecret(ctx, clientID)
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		hash, err := mint()
		if err != nil {
			return err
		}
		return q.CreateClientSecret(ctx, queries.CreateClientSecretParams{ClientID: clientID, Hash: hash, CreatedAt: s.now()})
	})
}

func (s *ClientSecrets) Rotate(ctx context.Context, clientID string, mint func() ([]byte, error)) error {
	return s.tx.InTx(ctx, func(q *queries.Queries) error {
		if err := q.LockClientSecret(ctx, clientID); err != nil {
			return err
		}
		hash, err := mint()
		if err != nil {
			return err
		}
		return q.RotateClientSecret(ctx, queries.RotateClientSecretParams{ClientID: clientID, Hash: hash, CreatedAt: s.now()})
	})
}

func (s *ClientSecrets) Hashes(ctx context.Context, clientID string) (current, previous []byte, err error) {
	row, err := s.q.ClientSecret(ctx, clientID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, app.ErrNotFound
	}
	return row.Hash, row.PreviousHash, err
}

func (s *ClientSecrets) EndPrevious(ctx context.Context, clientID string, current []byte) error {
	return s.q.EndPreviousClientSecret(ctx, queries.EndPreviousClientSecretParams{ClientID: clientID, Hash: current})
}
