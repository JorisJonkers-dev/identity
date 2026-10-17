package oidc

import (
	"context"
	"crypto/rsa"
	"sync/atomic"

	"github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// Ring is the key the Provider signs with and the keys JWKS publishes, read from the store and
// swapped whole on each refresh, so a request never sees half of a rotation (docs/adr/0013).
type Ring struct {
	keys atomic.Pointer[keySet]
	load func(ctx context.Context) (app.KeySet, error)
}

type keySet struct {
	signing   signingKey
	published []op.Key
}

// NewRing returns a Ring that signs with private and never changes, for tests.
func NewRing(private *rsa.PrivateKey) *Ring {
	r := &Ring{}
	id := domain.KeyID(&private.PublicKey)
	r.keys.Store(&keySet{signing: signingKey{id: id, private: private}, published: []op.Key{publicKey{id: id, public: &private.PublicKey}}})
	return r
}

// LoadRing returns a Ring holding the keys load gives now; Refresh reads them again.
func LoadRing(ctx context.Context, load func(ctx context.Context) (app.KeySet, error)) (*Ring, error) {
	r := &Ring{load: load}
	return r, r.Refresh(ctx)
}

// Refresh reads the keys again, so a new key starts signing on time and an old one leaves JWKS. A
// failed read keeps the keys the Ring has.
func (r *Ring) Refresh(ctx context.Context) error {
	if r.load == nil {
		return nil
	}
	ks, err := r.load(ctx)
	if err != nil {
		return err
	}
	published := make([]op.Key, 0, len(ks.Published))
	for _, k := range ks.Published {
		published = append(published, publicKey{id: k.ID, public: k.Public})
	}
	r.keys.Store(&keySet{signing: signingKey{id: ks.Signing.ID, private: ks.Private}, published: published})
	return nil
}

// Signing is the key new tokens are signed with.
func (r *Ring) Signing() op.SigningKey { return r.keys.Load().signing }

// Published is every key a token still in use may be signed with.
func (r *Ring) Published() []op.Key { return r.keys.Load().published }

type signingKey struct {
	id      string
	private *rsa.PrivateKey
}

func (k signingKey) SignatureAlgorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (k signingKey) Key() any                                    { return k.private }
func (k signingKey) ID() string                                  { return k.id }

type publicKey struct {
	id     string
	public *rsa.PublicKey
}

func (k publicKey) ID() string                         { return k.id }
func (k publicKey) Algorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (k publicKey) Use() string                        { return "sig" }
func (k publicKey) Key() any                           { return k.public }
