package domain

import (
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"math/big"
	"time"
)

const (
	// KeyLifetime is how long a signing key signs before a new one takes over (docs/adr/0013).
	KeyLifetime = 90 * 24 * time.Hour
	// KeyOverlap is how long a key stays published after it stops signing, so every token it
	// signed can still be verified.
	KeyOverlap = 7 * 24 * time.Hour
)

// SigningKey is a key the Provider signs with, by its public half: what JWKS publishes.
type SigningKey struct {
	ID         string
	Public     *rsa.PublicKey
	SignsFrom  time.Time
	SignsUntil time.Time
}

// Signs reports whether the key signs at now.
func (k SigningKey) Signs(now time.Time) bool {
	return !now.Before(k.SignsFrom) && now.Before(k.SignsUntil)
}

// Published reports whether the key is in JWKS at now: from when it signs until KeyOverlap after.
func (k SigningKey) Published(now time.Time) bool {
	return !now.Before(k.SignsFrom) && now.Before(k.SignsUntil.Add(KeyOverlap))
}

// Current is the key that signs at now: of two, the one that started last.
func Current(keys []SigningKey, now time.Time) (SigningKey, bool) {
	i, ok := CurrentIndex(keys, now)
	if !ok {
		return SigningKey{}, false
	}
	return keys[i], true
}

// CurrentIndex is where in keys the key that signs at now is.
func CurrentIndex(keys []SigningKey, now time.Time) (int, bool) {
	current := -1
	for i, k := range keys {
		if k.Signs(now) && (current < 0 || k.SignsFrom.After(keys[current].SignsFrom)) {
			current = i
		}
	}
	return current, current >= 0
}

// Published is every key JWKS publishes at now.
func Published(keys []SigningKey, now time.Time) []SigningKey {
	var out []SigningKey
	for _, k := range keys {
		if k.Published(now) {
			out = append(out, k)
		}
	}
	return out
}

// KeyID names a key by a hash of its public half, so the same key always has the same kid.
func KeyID(pub *rsa.PublicKey) string {
	h := sha256.New()
	h.Write(pub.N.Bytes())
	h.Write(big.NewInt(int64(pub.E)).Bytes())
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))[:16]
}
