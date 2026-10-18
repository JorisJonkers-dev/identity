// Package secretstore is the app.SecretVault over Vault KV: each Secret Client's id and secret, at
// the path deploy-kit derives, under the keys its Application reads (deploy-kit chapter 17).
package secretstore

import (
	"context"
	"errors"

	"github.com/JorisJonkers-dev/identity/internal/platform/vault"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

// KV is Vault's KV engine.
type KV interface {
	Read(ctx context.Context, path string) (map[string]string, error)
	Create(ctx context.Context, path string, data map[string]string) error
	Write(ctx context.Context, path string, data map[string]string) error
}

// Store is the app.SecretVault.
type Store struct{ kv KV }

// New returns a Store over kv.
func New(kv KV) Store { return Store{kv: kv} }

var _ app.SecretVault = Store{}

var errNoSecretKey = errors.New("the data at the path has no secret key")

func (s Store) Read(ctx context.Context, path string) (string, error) {
	data, err := s.kv.Read(ctx, path)
	if errors.Is(err, vault.ErrNoSecret) {
		return "", app.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if data["secret"] == "" {
		return "", errNoSecretKey
	}
	return data["secret"], nil
}

func (s Store) Create(ctx context.Context, path, clientID, secret string) error {
	return s.kv.Create(ctx, path, map[string]string{"id": clientID, "secret": secret})
}

func (s Store) Write(ctx context.Context, path, clientID, secret string) error {
	return s.kv.Write(ctx, path, map[string]string{"id": clientID, "secret": secret})
}
