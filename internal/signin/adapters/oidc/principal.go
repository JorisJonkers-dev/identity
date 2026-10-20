package oidc

import (
	"context"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"

	"github.com/JorisJonkers-dev/identity/internal/platform/principalclaims"
)

// errNoPrincipal is a token request identity makes no access token for: only a sign-in's code
// and a Machine Identity's client credentials or token exchange reach a JWT.
var errNoPrincipal = oidc.ErrServerError().WithDescription("This request does not make an access token.")

// sessionClaims is the Principal a User's access token states (contract/principal): their App
// Roles on the Client they signed in to, and the session that signed them in.
func (s *storage) sessionClaims(ctx context.Context, r authRequest) (map[string]any, error) {
	roles, err := s.roles(ctx, r.GetSubject(), r.ClientID)
	if err != nil {
		return nil, err
	}
	return s.userClaims(ctx, r.GetSubject(), roles, principalclaims.Principal{
		Via: principalclaims.ViaSession, SessionID: r.SignIn.SessionID.String(), TwoFactorAt: r.SignIn.TwoFactorAt,
	})
}

// userClaims is p for the User subject, holding roles, with their email and name as they are now.
func (s *storage) userClaims(ctx context.Context, subject string, roles []string, p principalclaims.Principal) (map[string]any, error) {
	claims, err := s.claims(ctx, subject)
	if err != nil {
		return nil, err
	}
	p.Kind, p.Email, p.Name, p.Roles = principalclaims.KindUser, claims.Email, claims.Name, roles
	return principalclaims.Claims(p), nil
}

// answer is an introspection answer for an opaque token of subject's, for clientID until expires,
// stating claims (contract/principal).
func answer(resp *oidc.IntrospectionResponse, subject, clientID string, expires time.Time, claims map[string]any) {
	resp.Subject, resp.ClientID, resp.Audience, resp.TokenType = subject, clientID, oidc.Audience{clientID}, "Bearer"
	resp.Expiration, resp.EmailVerified, resp.Claims = oidc.FromTime(expires), true, claims
}
