// Package claims states a Principal as the Principal contract's claims (contract/principal): the
// one mapping the Gate's signed header, access tokens and introspection answers share.
package claims

import (
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
)

// Version is the contract version identity states Principals in.
const Version = "1.1"

// The kind of caller and the way in a Principal states.
const (
	KindUser         = "user"
	KindMachine      = "machine"
	ViaSession       = "session"
	ViaPersonalToken = "personal-token"
	ViaTool          = "tool"
	ViaMachine       = "machine"
)

// Principal is who is calling, as identity knows it.
type Principal struct {
	Subject string
	Kind    string
	// Email and Name are a User's; a Machine Identity has neither.
	Email string
	Name  string
	Roles []string
	Via   string
	// Actor is the tool acting for the User, for via tool.
	Actor Actor
	// SessionID is the Single sign-on session, for via session.
	SessionID string
	// TwoFactorAt is when the session last proved two-factor; zero when it never did.
	TwoFactorAt time.Time
}

// Actor is a tool acting for a User: its registration's client id and the name it registered with.
type Actor struct {
	ID   string
	Name string
}

// Claims is a Principal as the contract's claims, in the order the Gate signs them.
type Claims struct {
	Version     string   `json:"ver"`
	Kind        string   `json:"kind"`
	Email       string   `json:"email,omitempty"`
	Name        string   `json:"name,omitempty"`
	Roles       []string `json:"roles"`
	Via         string   `json:"via"`
	Actor       *Act     `json:"act,omitempty"`
	TwoFactor   bool     `json:"two_factor"`
	TwoFactorAt int64    `json:"two_factor_at,omitempty"`
	SessionID   string   `json:"sid,omitempty"`
}

// Act is RFC 8693's act claim, with the tool's name beside its id.
type Act struct {
	Subject string `json:"sub"`
	Name    string `json:"name"`
}

// Of is p as the contract's claims.
func Of(p Principal) Claims {
	c := Claims{Version: Version, Kind: p.Kind, Email: p.Email, Name: p.Name, Roles: p.Roles, Via: p.Via}
	if c.Roles == nil {
		c.Roles = []string{}
	}
	if p.Via == ViaTool {
		c.Actor = &Act{Subject: p.Actor.ID, Name: p.Actor.Name}
	}
	if !p.TwoFactorAt.IsZero() {
		c.TwoFactor, c.TwoFactorAt = true, p.TwoFactorAt.Unix()
	}
	if p.Via == ViaSession {
		c.SessionID = p.SessionID
	}
	return c
}

// Private is p as the claims an access token or an introspection answer carries beside the ones
// OIDC itself defines.
func Private(p Principal) map[string]any {
	c := Of(p)
	out := map[string]any{"ver": c.Version, "kind": c.Kind, "roles": c.Roles, "via": c.Via, "two_factor": c.TwoFactor}
	if c.Email != "" {
		out["email"] = c.Email
	}
	if c.Name != "" {
		out["name"] = c.Name
	}
	if c.Actor != nil {
		out["act"] = c.Actor
	}
	if c.TwoFactor {
		out["two_factor_at"] = c.TwoFactorAt
	}
	if c.SessionID != "" {
		out["sid"] = c.SessionID
	}
	return out
}

// Introspect fills resp with p, as identity answers clientID's check of a token that expires at
// expires and, unless issued is zero, was issued at issued.
func Introspect(resp *oidc.IntrospectionResponse, p Principal, clientID string, issued, expires time.Time) {
	resp.Subject, resp.ClientID, resp.Audience, resp.TokenType = p.Subject, clientID, oidc.Audience{clientID}, "Bearer"
	if !issued.IsZero() {
		resp.IssuedAt = oidc.FromTime(issued)
	}
	resp.Expiration = oidc.FromTime(expires)
	resp.Email, resp.EmailVerified, resp.Name = p.Email, true, p.Name
	resp.Claims = Private(p)
}
