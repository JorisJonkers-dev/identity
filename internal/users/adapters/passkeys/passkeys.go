// Package passkeys is the users context's WebAuthn adapter: go-webauthn runs the ceremonies, as
// zitadel/oidc runs OIDC, and identity keeps what they produce. The app layer sees only bytes.
package passkeys

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/JorisJonkers-dev/identity/internal/users/app"
)

// ErrRefused is a browser answer WebAuthn does not accept.
var ErrRefused = errors.New("passkeys: the authenticator's answer is not accepted")

// Ceremonies is the app.WebAuthn.
type Ceremonies struct{ wa *webauthn.WebAuthn }

var _ app.WebAuthn = (*Ceremonies)(nil)

// New returns Ceremonies for the Identity Provider at origin, whose host is the relying party.
func New(rpID, origin string) (*Ceremonies, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPID: rpID, RPDisplayName: "jorisjonkers.dev", RPOrigins: []string{origin},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey: protocol.ResidentKeyRequirementPreferred, UserVerification: protocol.VerificationPreferred,
		},
	})
	if err != nil {
		return nil, err
	}
	return &Ceremonies{wa: wa}, nil
}

// user is a User as WebAuthn sees one: an opaque handle, names for display, and their credentials.
type user struct {
	id          []byte
	name, shown string
	creds       []webauthn.Credential
}

func (u user) WebAuthnID() []byte                         { return u.id }
func (u user) WebAuthnName() string                       { return u.name }
func (u user) WebAuthnDisplayName() string                { return u.shown }
func (u user) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func toUser(u app.PasskeyUser) (user, error) {
	creds := make([]webauthn.Credential, 0, len(u.Credentials))
	for _, raw := range u.Credentials {
		var c webauthn.Credential
		if err := json.Unmarshal(raw, &c); err != nil {
			return user{}, err
		}
		creds = append(creds, c)
	}
	return user{id: u.ID[:], name: u.Email, shown: u.Name, creds: creds}, nil
}

// BeginRegistration returns the options the browser passes to navigator.credentials.create, and
// the ceremony to keep until it answers. A passkey the User already has is excluded.
func (c *Ceremonies) BeginRegistration(in app.PasskeyUser) ([]byte, []byte, error) {
	u, err := toUser(in)
	if err != nil {
		return nil, nil, err
	}
	exclude := make([]protocol.CredentialDescriptor, 0, len(u.creds))
	for _, cr := range u.creds {
		exclude = append(exclude, cr.Descriptor())
	}
	return begun(c.wa.BeginRegistration(u, webauthn.WithExclusions(exclude)))
}

// FinishRegistration checks the browser's answer against the ceremony and returns the new passkey.
func (c *Ceremonies) FinishRegistration(in app.PasskeyUser, ceremony, response []byte) (app.NewPasskey, error) {
	u, session, err := prepare(in, ceremony)
	if err != nil {
		return app.NewPasskey{}, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(response))
	if err != nil {
		return app.NewPasskey{}, ErrRefused
	}
	cred, err := c.wa.CreateCredential(u, session, parsed)
	if err != nil {
		return app.NewPasskey{}, ErrRefused
	}
	return toPasskey(cred), nil
}

// BeginLogin returns the options for navigator.credentials.get over the User's passkeys.
func (c *Ceremonies) BeginLogin(in app.PasskeyUser) ([]byte, []byte, error) {
	u, err := toUser(in)
	if err != nil {
		return nil, nil, err
	}
	return begun(c.wa.BeginLogin(u))
}

// FinishLogin checks an assertion and returns the passkey it was made with, its counter moved on.
func (c *Ceremonies) FinishLogin(in app.PasskeyUser, ceremony, response []byte) (app.NewPasskey, error) {
	u, session, err := prepare(in, ceremony)
	if err != nil {
		return app.NewPasskey{}, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(response))
	if err != nil {
		return app.NewPasskey{}, ErrRefused
	}
	cred, err := c.wa.ValidateLogin(u, session, parsed)
	// A signature counter that did not move on means a second copy of the key signed: the library
	// only flags it, so it is refused here, and stays refused, as the flag is kept with the passkey.
	if err != nil || cred.Authenticator.CloneWarning {
		return app.NewPasskey{}, ErrRefused
	}
	return toPasskey(cred), nil
}

func prepare(in app.PasskeyUser, ceremony []byte) (user, webauthn.SessionData, error) {
	u, err := toUser(in)
	if err != nil {
		return user{}, webauthn.SessionData{}, err
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(ceremony, &session); err != nil {
		return user{}, webauthn.SessionData{}, err
	}
	return u, session, nil
}

func toPasskey(cred *webauthn.Credential) app.NewPasskey {
	raw, _ := json.Marshal(cred) //nolint:errchkjson // a Credential is plain data and always marshals
	return app.NewPasskey{CredentialID: cred.ID, Credential: raw}
}

// begun is a ceremony's options and its kept state, as JSON; both are plain data and always marshal.
func begun[O any](options O, session *webauthn.SessionData, err error) ([]byte, []byte, error) {
	if err != nil {
		return nil, nil, err
	}
	o, _ := json.Marshal(options) //nolint:errchkjson // see above
	k, _ := json.Marshal(session) //nolint:errchkjson // see above
	return o, k, nil
}
