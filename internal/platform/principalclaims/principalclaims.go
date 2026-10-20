// Package principalclaims states a Principal as the contract's claims (contract/principal): the
// same on the Gate's signed header, in access tokens and in introspection answers.
package principalclaims

import "time"

// Version is the contract version identity states its Principals in.
const Version = "1.1"

// The kinds of caller and the ways in the contract names.
const (
	KindUser         = "user"
	KindMachine      = "machine"
	ViaSession       = "session"
	ViaPersonalToken = "personal-token"
	ViaTool          = "tool"
	ViaMachine       = "machine"
)

// Principal is who is calling, before it is stated as claims.
type Principal struct {
	Kind  string
	Email string
	Name  string
	Roles []string
	Via   string
	// Actor is the tool acting for the User, for ViaTool.
	Actor *Actor
	// TwoFactorAt is when the session last proved two-factor; zero when it never did.
	TwoFactorAt time.Time
	// SessionID is the Single sign-on session, for ViaSession.
	SessionID string
}

// Actor is a tool acting for a User: its registration's client id and the name it registered with.
type Actor struct {
	ID   string
	Name string
}

// Claims is p as the contract's claims. Each channel adds sub, iss, aud, iat and exp itself.
func Claims(p Principal) map[string]any {
	roles := p.Roles
	if roles == nil {
		roles = []string{}
	}
	c := map[string]any{"ver": Version, "kind": p.Kind, "roles": roles, "via": p.Via, "two_factor": !p.TwoFactorAt.IsZero()}
	for name, value := range map[string]string{"email": p.Email, "name": p.Name, "sid": p.SessionID} {
		if value != "" {
			c[name] = value
		}
	}
	if p.Actor != nil {
		c["act"] = map[string]any{"sub": p.Actor.ID, "name": p.Actor.Name}
	}
	if !p.TwoFactorAt.IsZero() {
		c["two_factor_at"] = p.TwoFactorAt.Unix()
	}
	return c
}
