package oidc

import (
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"math/big"

	"github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/op"
)

// Ring is the signing key and the keys JWKS publishes. It holds one key for now; keeping keys
// wrapped in Postgres and rotating them is identity#30's (docs/adr/0013).
type Ring struct {
	key signingKey
}

// NewRing returns a Ring signing with private under RS256, the algorithm every OIDC client supports.
func NewRing(private *rsa.PrivateKey) *Ring {
	return &Ring{key: signingKey{id: keyID(&private.PublicKey), private: private}}
}

// Signing is the key new tokens are signed with.
func (r *Ring) Signing() op.SigningKey { return r.key }

// Published is every key a token still in use may be signed with.
func (r *Ring) Published() []op.Key { return []op.Key{publicKey(r.key)} }

// keyID names a key by a hash of its public half, so the same key always has the same kid.
func keyID(pub *rsa.PublicKey) string {
	h := sha256.New()
	h.Write(pub.N.Bytes())
	h.Write(big.NewInt(int64(pub.E)).Bytes())
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))[:16]
}

type signingKey struct {
	id      string
	private *rsa.PrivateKey
}

func (k signingKey) SignatureAlgorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (k signingKey) Key() any                                    { return k.private }
func (k signingKey) ID() string                                  { return k.id }

type publicKey signingKey

func (k publicKey) ID() string                         { return k.id }
func (k publicKey) Algorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (k publicKey) Use() string                        { return "sig" }
func (k publicKey) Key() any                           { return &k.private.PublicKey }
