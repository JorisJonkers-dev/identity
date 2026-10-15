// Package domain holds what the sign-in context knows: Clients as deploy-kit declares them, and
// authorization requests from /authorize until their code is redeemed.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// SignInMode is how a Client signs Users in (deploy-kit chapter 17).
type SignInMode string

const (
	// Native Clients speak OIDC themselves.
	Native SignInMode = "native"
	// Gated Clients are signed in for by the Gate.
	Gated SignInMode = "gated"
	// Token Clients take bearer tokens only.
	Token SignInMode = "token"
)

// Credential is how a native Client authenticates at the token endpoint.
type Credential string

const (
	// Secret Clients are confidential: they hold a client secret.
	Secret Credential = "secret"
	// PKCE Clients are public: they hold nothing and prove each code with PKCE.
	PKCE Credential = "pkce"
)

// Client is a Client as identity reads it, from a deploy-kit Client object; identity never creates
// one (docs/adr/0002).
type Client struct {
	ID                     string
	SignIn                 SignInMode
	Host                   string
	Label                  string
	Description            string
	LaunchURL              string
	IconURL                string
	RedirectURIs           []string
	PostLogoutRedirectURIs []string
	Credential             Credential
	// SecretPath is where a Secret Client's secret is kept (identity#32).
	SecretPath string
	// SecretHash is the SHA-256 of a Secret Client's secret; empty until identity has one.
	SecretHash []byte
	Roles      []string
}

// Public reports whether the Client is a public one. A Secret Client without a secret yet is not
// public: it cannot authenticate at all.
func (c Client) Public() bool { return c.Credential == PKCE }

// AuthRequest is one authorization request, as /authorize received it.
type AuthRequest struct {
	ID                  uuid.UUID
	ClientID            string
	RedirectURI         string
	Scopes              []string
	State               string
	Nonce               string
	ResponseType        string
	ResponseMode        string
	CodeChallenge       string
	CodeChallengeMethod string
	CreatedAt           time.Time
	ExpiresAt           time.Time
	// SignIn is who completed the request; nil until someone has.
	SignIn *SignIn
}

// SignIn is a User's Single sign-on session vouching for an AuthRequest.
type SignIn struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	// At is when the User last proved their password, the ID token's auth_time.
	At time.Time
}
