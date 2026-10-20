// Package principal signs the Gate's Verdict as the Principal contract's signed header
// (contract/principal): a JWT from identity's key ring, typed so it never passes as an access token.
package principal

import (
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/crypto"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/JorisJonkers-dev/identity/internal/gate/app"
	principalclaims "github.com/JorisJonkers-dev/identity/internal/platform/claims"
)

const (
	// Type is the signed header's JWT typ (RFC 8725).
	Type = "identity-principal+jwt"
	// Version is the contract version the Gate states Principals in.
	Version = principalclaims.Version
)

// Keys is identity's key ring as the Gate signs with it.
type Keys interface {
	Signing() op.SigningKey
}

// Signer is the app.Principals the Gate signs with.
type Signer struct {
	issuer string
	keys   Keys
}

var _ app.Principals = (*Signer)(nil)

// New returns a Signer stating issuer as the Principal's iss.
func New(issuer string, keys Keys) *Signer {
	return &Signer{issuer: issuer, keys: keys}
}

// Sign is v as the Principal for clientID, issued at at and lasting app.PrincipalLifetime.
func (s *Signer) Sign(v app.Verdict, clientID string, at time.Time) (string, error) {
	return sign(s.keys.Signing(), Type, claimsOf(v, s.issuer, clientID, at))
}

// claims is the signed header's claims: the Principal's, bound to one Client for a short while.
type claims struct {
	Issuer   string `json:"iss"`
	Subject  string `json:"sub"`
	Audience string `json:"aud"`
	IssuedAt int64  `json:"iat"`
	Expiry   int64  `json:"exp"`
	principalclaims.Claims
}

func claimsOf(v app.Verdict, issuer, clientID string, at time.Time) claims {
	return claims{
		Issuer: issuer, Subject: v.UserID.String(), Audience: clientID,
		IssuedAt: at.Unix(), Expiry: at.Add(app.PrincipalLifetime).Unix(),
		Claims: principalclaims.Of(principalOf(v)),
	}
}

// principalOf is v as the Principal the contract states.
func principalOf(v app.Verdict) principalclaims.Principal {
	p := principalclaims.Principal{
		Subject: v.UserID.String(), Kind: v.Kind, Email: v.Email, Name: v.Name, Roles: v.Roles, Via: v.Via,
		Actor: principalclaims.Actor{ID: v.Actor.ID, Name: v.Actor.Name}, TwoFactorAt: v.TwoFactorAt,
	}
	if v.Via == app.ViaSession {
		p.SessionID = v.SessionID.String()
	}
	return p
}

func sign(key op.SigningKey, typ string, payload any) (string, error) {
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: key.SignatureAlgorithm(), Key: jose.JSONWebKey{Key: key.Key(), KeyID: key.ID()}},
		(&jose.SignerOptions{}).WithType(jose.ContentType(typ)),
	)
	if err != nil {
		return "", err
	}
	return crypto.Sign(payload, signer)
}
