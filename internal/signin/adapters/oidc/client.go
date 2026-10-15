package oidc

import (
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

const (
	accessTokenLifetime = 15 * time.Minute
	idTokenLifetime     = time.Hour
)

// client is a domain.Client as zitadel/oidc reads it: the code flow only, PKCE for a public Client,
// a secret for a confidential one, and JWT access tokens a Client can verify by itself.
type client struct{ domain.Client }

var _ op.Client = client{}

func (c client) GetID() string                    { return c.ID }
func (c client) RedirectURIs() []string           { return c.Client.RedirectURIs }
func (c client) PostLogoutRedirectURIs() []string { return c.Client.PostLogoutRedirectURIs }

func (c client) ApplicationType() op.ApplicationType {
	if c.Public() {
		return op.ApplicationTypeUserAgent
	}
	return op.ApplicationTypeWeb
}

func (c client) AuthMethod() oidc.AuthMethod {
	if c.Public() {
		return oidc.AuthMethodNone
	}
	return oidc.AuthMethodBasic
}

func (client) ResponseTypes() []oidc.ResponseType { return []oidc.ResponseType{oidc.ResponseTypeCode} }
func (client) GrantTypes() []oidc.GrantType       { return []oidc.GrantType{oidc.GrantTypeCode} }

// LoginURL is where /authorize sends the browser: identity's own check for a Single sign-on session.
func (client) LoginURL(id string) string { return loginPath + "?authRequestID=" + id }

func (client) AccessTokenType() op.AccessTokenType { return op.AccessTokenTypeJWT }
func (client) IDTokenLifetime() time.Duration      { return idTokenLifetime }
func (client) DevMode() bool                       { return false }

func (client) RestrictAdditionalIdTokenScopes() func([]string) []string     { return same } //nolint:revive // zitadel/oidc's name
func (client) RestrictAdditionalAccessTokenScopes() func([]string) []string { return same }

// IsScopeAllowed allows no scope beyond the standard ones.
func (client) IsScopeAllowed(string) bool { return false }

// IDTokenUserinfoClaimsAssertion puts email and name in the ID token, not only in userinfo.
func (client) IDTokenUserinfoClaimsAssertion() bool { return true }
func (client) ClockSkew() time.Duration             { return 0 }

func same(scopes []string) []string { return scopes }
