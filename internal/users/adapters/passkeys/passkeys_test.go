package passkeys_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/descope/virtualwebauthn"
	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/users/adapters/passkeys"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
)

var rp = virtualwebauthn.RelyingParty{Name: "jorisjonkers.dev", ID: "auth.test", Origin: "https://auth.test"}

func publicKey(t *testing.T, options []byte) string {
	t.Helper()
	var wrapped struct {
		PublicKey json.RawMessage `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &wrapped); err != nil || len(wrapped.PublicKey) == 0 {
		t.Fatalf("options %s have no publicKey: %v", options, err)
	}
	return string(wrapped.PublicKey)
}

// register makes a passkey for u on the virtual authenticator and returns it.
func register(t *testing.T, c *passkeys.Ceremonies, auth virtualwebauthn.Authenticator, cred virtualwebauthn.Credential, u app.PasskeyUser) app.NewPasskey {
	t.Helper()
	options, ceremony, err := c.BeginRegistration(u)
	if err != nil {
		t.Fatal(err)
	}
	attestation, err := virtualwebauthn.ParseAttestationOptions(publicKey(t, options))
	if err != nil {
		t.Fatal(err)
	}
	response := virtualwebauthn.CreateAttestationResponse(rp, auth, cred, *attestation)
	made, err := c.FinishRegistration(u, ceremony, []byte(response))
	if err != nil {
		t.Fatalf("FinishRegistration = %v", err)
	}
	return made
}

func TestAPasskeyIsMadeThenSignsIn(t *testing.T) {
	c, err := passkeys.New("auth.test", "https://auth.test")
	if err != nil {
		t.Fatal(err)
	}
	auth := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	auth.AddCredential(cred)
	u := app.PasskeyUser{ID: uuid.New(), Email: "ada@example.org", Name: "Ada"}

	made := register(t, c, auth, cred, u)
	if string(made.CredentialID) != string(cred.ID) || len(made.Credential) == 0 {
		t.Fatalf("made = %+v", made)
	}

	// A passkey the User has is excluded from the next registration.
	u.Credentials = [][]byte{made.Credential}
	options, _, err := c.BeginRegistration(u)
	if err != nil {
		t.Fatal(err)
	}
	attestation, _ := virtualwebauthn.ParseAttestationOptions(publicKey(t, options))
	if !cred.IsExcludedForAttestation(*attestation) {
		t.Fatal("the existing passkey was not excluded")
	}

	options, ceremony, err := c.BeginLogin(u)
	if err != nil {
		t.Fatal(err)
	}
	assertion, err := virtualwebauthn.ParseAssertionOptions(publicKey(t, options))
	if err != nil {
		t.Fatal(err)
	}
	cred.Counter = 1
	response := virtualwebauthn.CreateAssertionResponse(rp, auth, cred, *assertion)
	used, err := c.FinishLogin(u, ceremony, []byte(response))
	if err != nil || string(used.CredentialID) != string(cred.ID) {
		t.Fatalf("FinishLogin = %+v, %v", used, err)
	}

	// A copy of the key signing with a counter that did not move on is refused.
	cloned := app.PasskeyUser{ID: u.ID, Email: u.Email, Name: u.Name, Credentials: [][]byte{used.Credential}}
	options, again, _ := c.BeginLogin(cloned)
	repeat, _ := virtualwebauthn.ParseAssertionOptions(publicKey(t, options))
	if _, err := c.FinishLogin(cloned, again, []byte(virtualwebauthn.CreateAssertionResponse(rp, auth, cred, *repeat))); !errors.Is(err, passkeys.ErrRefused) {
		t.Fatalf("a counter that did not move on = %v", err)
	}
	cred.Counter = 2
	options, next, _ := c.BeginLogin(cloned)
	moved, _ := virtualwebauthn.ParseAssertionOptions(publicKey(t, options))
	if _, err := c.FinishLogin(cloned, next, []byte(virtualwebauthn.CreateAssertionResponse(rp, auth, cred, *moved))); err != nil {
		t.Fatalf("a counter that moved on = %v", err)
	}

	// The same assertion against a fresh ceremony is refused: its challenge is another one's.
	_, other, _ := c.BeginLogin(u)
	if _, err := c.FinishLogin(u, other, []byte(response)); !errors.Is(err, passkeys.ErrRefused) {
		t.Fatalf("a replayed assertion = %v", err)
	}
	// An assertion for a site with another origin is refused.
	evil := virtualwebauthn.RelyingParty{Name: "x", ID: "auth.test", Origin: "https://evil.test"}
	_, fresh, _ := c.BeginLogin(u)
	if _, err := c.FinishLogin(u, fresh, []byte(virtualwebauthn.CreateAssertionResponse(evil, auth, cred, *assertion))); !errors.Is(err, passkeys.ErrRefused) {
		t.Fatalf("an assertion from another origin = %v", err)
	}
}

func TestWhatIsNotAPasskeyIsRefused(t *testing.T) {
	c, _ := passkeys.New("auth.test", "https://auth.test")
	u := app.PasskeyUser{ID: uuid.New(), Email: "ada@example.org", Name: "Ada"}
	_, ceremony, _ := c.BeginRegistration(u)
	if _, err := c.FinishRegistration(u, ceremony, []byte(`{}`)); !errors.Is(err, passkeys.ErrRefused) {
		t.Errorf("an empty registration = %v", err)
	}
	if _, err := c.FinishRegistration(u, []byte("not json"), []byte(`{}`)); err == nil {
		t.Error("a broken ceremony was accepted")
	}
	if _, err := c.FinishLogin(u, []byte("not json"), []byte(`{}`)); err == nil {
		t.Error("a broken login ceremony was accepted")
	}
	if _, err := c.FinishLogin(u, ceremony, []byte(`{}`)); !errors.Is(err, passkeys.ErrRefused) {
		t.Errorf("an empty assertion = %v", err)
	}

	// An attestation answered for another challenge is refused.
	auth := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	options, _, _ := c.BeginRegistration(u)
	attestation, _ := virtualwebauthn.ParseAttestationOptions(publicKey(t, options))
	_, otherCeremony, _ := c.BeginRegistration(u)
	if _, err := c.FinishRegistration(u, otherCeremony, []byte(virtualwebauthn.CreateAttestationResponse(rp, auth, cred, *attestation))); !errors.Is(err, passkeys.ErrRefused) {
		t.Errorf("an attestation for another challenge = %v", err)
	}

	broken := app.PasskeyUser{ID: u.ID, Credentials: [][]byte{[]byte("not json")}}
	if _, _, err := c.BeginRegistration(broken); err == nil {
		t.Error("BeginRegistration with a broken stored passkey")
	}
	if _, _, err := c.BeginLogin(broken); err == nil {
		t.Error("BeginLogin with a broken stored passkey")
	}
	if _, err := c.FinishRegistration(broken, ceremony, nil); err == nil {
		t.Error("FinishRegistration with a broken stored passkey")
	}
	if _, _, err := c.BeginLogin(u); err == nil {
		t.Error("a login for a User with no passkey began")
	}
	if _, err := passkeys.New("not a domain!", "https://auth.test"); err == nil {
		t.Error("a relying party without an id was accepted")
	}
}
