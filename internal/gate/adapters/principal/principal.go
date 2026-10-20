// Package principal signs the Gate's Verdict as the Principal contract's signed header
// (contract/principal): a JWT from identity's key ring, typed so it never passes as an access token.
package principal

import (
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/crypto"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/JorisJonkers-dev/identity/internal/gate/app"
	"github.com/JorisJonkers-dev/identity/internal/platform/principalclaims"
)

// Type is the signed header's JWT typ (RFC 8725).
const Type = "identity-principal+jwt"

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

// claimsOf is v as the Principal for clientID, issued at at: the contract's claims beside the
// registered ones.
func claimsOf(v app.Verdict, issuer, clientID string, at time.Time) map[string]any {
	p := principalclaims.Principal{
		Kind: v.Kind, Email: v.Email, Name: v.Name, Roles: v.Roles, Via: v.Via, TwoFactorAt: v.TwoFactorAt,
	}
	if v.Via == app.ViaTool {
		p.Actor = &principalclaims.Actor{ID: v.Actor.ID, Name: v.Actor.Name}
	}
	if v.Via == app.ViaSession {
		p.SessionID = v.SessionID.String()
	}
	c := principalclaims.Claims(p)
	c["iss"], c["sub"], c["aud"] = issuer, v.UserID.String(), clientID
	c["iat"], c["exp"] = at.Unix(), at.Add(app.PrincipalLifetime).Unix()
	return c
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
