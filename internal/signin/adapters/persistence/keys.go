package persistence

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// Transactor runs queries in one transaction.
type Transactor interface {
	InTx(ctx context.Context, fn func(*queries.Queries) error) error
}

// Keys is the app.KeyStore: every replica takes one advisory lock to read or add keys and secrets.
type Keys struct {
	q   queries.Querier
	tx  Transactor
	now func() time.Time
}

// NewKeys returns Keys over q, locking through tx.
func NewKeys(q queries.Querier, tx Transactor, now func() time.Time) *Keys {
	return &Keys{q: q, tx: tx, now: now}
}

var _ app.KeyStore = (*Keys)(nil)

func (k *Keys) WithKeys(ctx context.Context, add func(keys []app.StoredKey) (app.StoredKey, bool, error)) ([]app.StoredKey, error) {
	var stored []app.StoredKey
	err := k.tx.InTx(ctx, func(q *queries.Queries) error {
		if err := q.LockKeys(ctx); err != nil {
			return err
		}
		var err error
		if stored, err = keys(ctx, q); err != nil {
			return err
		}
		added, made, err := add(stored)
		if err != nil || !made {
			return err
		}
		der, err := x509.MarshalPKIXPublicKey(added.Public)
		if err != nil {
			return err
		}
		if err := q.CreateSigningKey(ctx, queries.CreateSigningKeyParams{
			ID: added.ID, PublicKey: der, SealedPrivate: added.Sealed, SignsFrom: added.SignsFrom, SignsUntil: added.SignsUntil,
		}); err != nil {
			return err
		}
		stored = append(stored, added)
		return nil
	})
	return stored, err
}

func keys(ctx context.Context, q *queries.Queries) ([]app.StoredKey, error) {
	rows, err := q.ListSigningKeys(ctx)
	if err != nil {
		return nil, err
	}
	stored := make([]app.StoredKey, 0, len(rows))
	for _, r := range rows {
		parsed, err := x509.ParsePKIXPublicKey(r.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("signing key %s: %w", r.ID, err)
		}
		pub, ok := parsed.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("signing key %s is not RSA", r.ID)
		}
		stored = append(stored, app.StoredKey{
			SigningKey: domain.SigningKey{ID: r.ID, Public: pub, SignsFrom: r.SignsFrom, SignsUntil: r.SignsUntil},
			Sealed:     r.SealedPrivate,
		})
	}
	return stored, nil
}

func (k *Keys) DeleteEndedBefore(ctx context.Context, t time.Time) error {
	return k.q.DeleteSigningKeysEndedBefore(ctx, t)
}

func (k *Keys) Secret(ctx context.Context, name string, mint func() (string, error)) (string, error) {
	var sealed string
	err := k.tx.InTx(ctx, func(q *queries.Queries) error {
		if err := q.LockKeys(ctx); err != nil {
			return err
		}
		var err error
		sealed, err = q.SealedSecret(ctx, name)
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if sealed, err = mint(); err != nil {
			return err
		}
		return q.CreateSealedSecret(ctx, queries.CreateSealedSecretParams{Name: name, Sealed: sealed, CreatedAt: k.now()})
	})
	return sealed, err
}
