package app

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// ErrNoSigningKey is a key set with no key that signs now, which Load never returns.
var ErrNoSigningKey = errors.New("no signing key signs now")

// StoredKey is a signing key as it is kept: its public half, and its private half sealed.
type StoredKey struct {
	domain.SigningKey
	Sealed string
}

// KeyStore keeps the signing keys and the sealed secrets every replica shares.
type KeyStore interface {
	// WithKeys runs add holding a lock every replica takes, with the keys stored; a key add returns,
	// when it says it made one, is stored before the lock is let go. It returns the keys stored after.
	WithKeys(ctx context.Context, add func(keys []StoredKey) (StoredKey, bool, error)) ([]StoredKey, error)
	// DeleteEndedBefore removes the keys that stopped signing before t.
	DeleteEndedBefore(ctx context.Context, t time.Time) error
	// Secret is the sealed secret called name, made by mint and kept the first time it is asked for.
	Secret(ctx context.Context, name string, mint func() (string, error)) (string, error)
}

// Sealer keeps a secret as Vault transit ciphertext (docs/adr/0013).
type Sealer interface {
	Seal(ctx context.Context, plaintext []byte) (string, error)
	Open(ctx context.Context, ciphertext string) ([]byte, error)
}

// KeySet is the keys the Provider uses now: the one it signs with, and every one JWKS publishes.
type KeySet struct {
	Signing   domain.SigningKey
	Private   *rsa.PrivateKey
	Published []domain.SigningKey
}

// Keys keeps the Provider's signing keys in the store, their private halves sealed, and makes a new
// one when none signs (docs/adr/0013).
type Keys struct {
	store    KeyStore
	sealer   Sealer
	generate func() (*rsa.PrivateKey, error)
	now      func() time.Time
	random   io.Reader
}

// NewKeys returns Keys. generate makes a new private key; random is where secrets come from.
func NewKeys(store KeyStore, sealer Sealer, generate func() (*rsa.PrivateKey, error), now func() time.Time, random io.Reader) *Keys {
	return &Keys{store: store, sealer: sealer, generate: generate, now: now, random: random}
}

// Load is the keys as they stand now. When no key signs, the first replica to ask makes one, and
// every other reads it; keys no token can need any more are dropped.
func (k *Keys) Load(ctx context.Context) (KeySet, error) {
	now := k.now()
	stored, err := k.store.WithKeys(ctx, func(keys []StoredKey) (StoredKey, bool, error) {
		if _, ok := domain.Current(publics(keys), now); ok {
			return StoredKey{}, false, nil
		}
		made, err := k.make(ctx, now)
		return made, err == nil, err
	})
	if err != nil {
		return KeySet{}, err
	}
	keys := publics(stored)
	i, ok := domain.CurrentIndex(keys, now)
	if !ok {
		return KeySet{}, ErrNoSigningKey
	}
	private, err := k.open(ctx, stored[i])
	if err != nil {
		return KeySet{}, err
	}
	if err := k.store.DeleteEndedBefore(ctx, now.Add(-domain.KeyOverlap)); err != nil {
		return KeySet{}, err
	}
	return KeySet{Signing: keys[i], Private: private, Published: domain.Published(keys, now)}, nil
}

// make is a new key that signs from now for KeyLifetime, its private half sealed.
func (k *Keys) make(ctx context.Context, now time.Time) (StoredKey, error) {
	private, err := k.generate()
	if err != nil {
		return StoredKey{}, fmt.Errorf("make a signing key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return StoredKey{}, err
	}
	sealed, err := k.sealer.Seal(ctx, der)
	if err != nil {
		return StoredKey{}, err
	}
	pub := &private.PublicKey
	return StoredKey{
		SigningKey: domain.SigningKey{ID: domain.KeyID(pub), Public: pub, SignsFrom: now, SignsUntil: now.Add(domain.KeyLifetime)},
		Sealed:     sealed,
	}, nil
}

// open unseals a stored key's private half.
func (k *Keys) open(ctx context.Context, s StoredKey) (*rsa.PrivateKey, error) {
	der, err := k.sealer.Open(ctx, s.Sealed)
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, err
	}
	private, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("signing key %s is not RSA", s.ID)
	}
	return private, nil
}

// Secret is the 32-byte secret called name, the same on every replica and across restarts: made
// and sealed the first time any replica asks.
func (k *Keys) Secret(ctx context.Context, name string) ([32]byte, error) {
	var secret [32]byte
	sealed, err := k.store.Secret(ctx, name, func() (string, error) {
		raw := make([]byte, len(secret))
		if _, err := io.ReadFull(k.random, raw); err != nil {
			return "", err
		}
		return k.sealer.Seal(ctx, raw)
	})
	if err != nil {
		return secret, err
	}
	raw, err := k.sealer.Open(ctx, sealed)
	if err != nil {
		return secret, err
	}
	if len(raw) != len(secret) {
		return secret, fmt.Errorf("secret %s is %d bytes, not %d", name, len(raw), len(secret))
	}
	copy(secret[:], raw)
	return secret, nil
}

func publics(stored []StoredKey) []domain.SigningKey {
	keys := make([]domain.SigningKey, 0, len(stored))
	for _, s := range stored {
		keys = append(keys, s.SigningKey)
	}
	return keys
}
