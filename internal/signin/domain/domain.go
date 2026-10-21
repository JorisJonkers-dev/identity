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
	// BackChannelLogoutURI takes a logout token when a session that signed into the Client ends.
	BackChannelLogoutURI string
	Credential           Credential
	// SecretPath is the Secret Store path where a Secret Client's Application reads its secret.
	SecretPath string
	Roles      []string
}

// Public reports whether the Client is a public one. A Secret Client without a secret yet is not
// public: it cannot authenticate at all until identity has given it one.
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
	// Resource is the Client a Connected App's request is for (identity#23); empty for any other.
	Resource string
	// TwoFactorWithin is how fresh a two-factor proof the app asked for (identity#142); 0 when it
	// asked for none.
	TwoFactorWithin time.Duration
	// SignIn is who completed the request; nil until someone has.
	SignIn *SignIn
}

// The freshest and the stalest two-factor proof an app may ask for with acr_values=mfa.
const (
	FreshestProof = time.Minute
	StalestProof  = time.Hour
)

// AskedProof is the two-factor proof window an app asks for with max_age, in seconds, kept between
// FreshestProof and StalestProof; StalestProof when it names none.
func AskedProof(maxAge *uint) time.Duration {
	if maxAge == nil {
		return StalestProof
	}
	seconds := min(*maxAge, uint(StalestProof/time.Second))
	return max(time.Duration(seconds)*time.Second, FreshestProof) //nolint:gosec // at most an hour of seconds
}

// SignIn is a User's Single sign-on session vouching for an AuthRequest.
type SignIn struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	// At is when the User last proved their password, the ID token's auth_time.
	At time.Time
	// Methods is how the session was proved, as RFC 8176 names them.
	Methods []string
	// TwoFactorAt is when the session last proved two-factor; zero when it never did.
	TwoFactorAt time.Time
}
