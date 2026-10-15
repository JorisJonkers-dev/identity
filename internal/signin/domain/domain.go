// Package domain holds what the sign-in context knows: Clients as deploy-kit declares them, and
// authorization requests from /authorize until their code is redeemed.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Client is a Client as identity reads it; identity never creates one (docs/adr/0002).
type Client struct {
	ID                     string
	RedirectURIs           []string
	PostLogoutRedirectURIs []string
	// SecretHash is the SHA-256 of a confidential Client's secret. A public Client has none and
	// must use PKCE.
	SecretHash []byte
}

// Public reports whether the Client has no secret.
func (c Client) Public() bool { return len(c.SecretHash) == 0 }

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
