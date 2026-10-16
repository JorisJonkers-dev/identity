//go:build integration

// Package users_test checks the users context's behaviours the way a browser reaches them: over
// HTTP, against the real routes, Postgres, Vault and mail (docs/testing-standards.md).
package users_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
	"github.com/pquerna/otp/totp"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
)

type profile struct{ Email, Name string }

type problem struct{ Detail string }

type needsSecondFactor struct{ Methods []string }

func signIn(t *testing.T, b *itest.Browser, password string, want int, out any) {
	t.Helper()
	b.Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": b.Email, "password": password}, want, out)
}

func TestAPersonWhoSignedUpAndConfirmedByMailSeesTheirProfile(t *testing.T) {
	t.Parallel()
	ada := itest.Start(t).SignedUp(t, "ada@example.org")
	var p profile
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, &p)
	if p.Email != ada.Email || p.Name != "Grace Hopper" {
		t.Fatalf("profile = %+v", p)
	}
}

func TestASignUpAnswersTheSameWhetherOrNotTheEmailIsTaken(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada := env.SignedUp(t, "ada@example.org")
	again := map[string]string{"email": ada.Email, "name": "Someone Else", "password": itest.Password}
	env.Browser().Call(t, http.MethodPost, "/api/v1/signup", again, http.StatusAccepted, nil)
}

func TestAWrongPasswordGetsTheSameAnswerAsAnUnknownEmail(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada := env.SignedUp(t, "ada@example.org")
	var wrong, unknown problem
	signIn(t, ada, "not the password at all", http.StatusUnauthorized, &wrong)
	env.Browser().Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": "nobody@example.org", "password": itest.Password},
		http.StatusUnauthorized, &unknown)
	if wrong.Detail == "" || wrong != unknown {
		t.Fatalf("wrong password %q, unknown email %q: want one answer", wrong.Detail, unknown.Detail)
	}
}

func TestSigningOutEndsTheSessionInEveryBrowser(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	here := env.SignedUp(t, "ada@example.org")
	elsewhere := env.Browser()
	elsewhere.Email = here.Email
	signIn(t, elsewhere, itest.Password, http.StatusNoContent, nil)
	here.Call(t, http.MethodDelete, "/api/v1/session", nil, http.StatusNoContent, nil)
	here.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
	elsewhere.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
}

func TestAUserRenamesThemself(t *testing.T) {
	t.Parallel()
	ada := itest.Start(t).SignedUp(t, "ada@example.org")
	var p profile
	ada.Call(t, http.MethodPatch, "/api/v1/profile", map[string]string{"name": "Ada King"}, http.StatusOK, &p)
	if p.Name != "Ada King" {
		t.Fatalf("name = %q", p.Name)
	}
}

func TestChangingThePasswordEndsOtherSessionsAndTheNewOneSignsIn(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	here := env.SignedUp(t, "ada@example.org")
	elsewhere := env.Browser()
	elsewhere.Email = here.Email
	signIn(t, elsewhere, itest.Password, http.StatusNoContent, nil)
	const next = "an entirely new password"
	here.Call(t, http.MethodPost, "/api/v1/profile/password", map[string]string{"current": itest.Password, "next": next}, http.StatusNoContent, nil)
	elsewhere.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
	here.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, nil)
	signIn(t, elsewhere, next, http.StatusNoContent, nil)
}

func TestAPasswordChangeNeedsTheCurrentPassword(t *testing.T) {
	t.Parallel()
	ada := itest.Start(t).SignedUp(t, "ada@example.org")
	var p problem
	ada.Call(t, http.MethodPost, "/api/v1/profile/password", map[string]string{"current": "not the password at all", "next": "an entirely new password"},
		http.StatusUnprocessableEntity, &p)
	if p.Detail != "Your current password is wrong. Enter it again." {
		t.Fatalf("detail = %q", p.Detail)
	}
}

// withAuthenticator is Ada after adding an authenticator app, with its secret.
func withAuthenticator(t *testing.T) (*itest.Env, *itest.Browser, string) {
	t.Helper()
	env := itest.Start(t)
	ada := env.SignedUp(t, "ada@example.org")
	var setup struct{ Secret string }
	ada.Call(t, http.MethodPost, "/api/v1/profile/second-factors/totp", nil, http.StatusOK, &setup)
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	ada.Call(t, http.MethodPost, "/api/v1/profile/second-factors/totp/confirmation",
		map[string]string{"label": "Phone", "code": code, "password": itest.Password}, http.StatusNoContent, nil)
	return env, ada, setup.Secret
}

func TestWithAnAuthenticatorSigningInAsksForItsCode(t *testing.T) {
	t.Parallel()
	env, ada, _ := withAuthenticator(t)
	again := env.Browser()
	again.Email = ada.Email
	var needs needsSecondFactor
	signIn(t, again, itest.Password, http.StatusAccepted, &needs)
	if !slices.Equal(needs.Methods, []string{"totp"}) {
		t.Fatalf("methods = %v", needs.Methods)
	}
	again.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
}

func TestTheAuthenticatorsNextCodeFinishesTheSignIn(t *testing.T) {
	t.Parallel()
	env, ada, secret := withAuthenticator(t)
	again := env.Browser()
	again.Email = ada.Email
	signIn(t, again, itest.Password, http.StatusAccepted, nil)
	next, _ := totp.GenerateCode(secret, time.Now().Add(30*time.Second)) // the code that added the app is spent
	again.Call(t, http.MethodPost, "/api/v1/session/second-factor", map[string]string{"code": next}, http.StatusNoContent, nil)
	again.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, nil)
}

func TestAWrongAuthenticatorCodeIsRefused(t *testing.T) {
	t.Parallel()
	env, ada, _ := withAuthenticator(t)
	again := env.Browser()
	again.Email = ada.Email
	signIn(t, again, itest.Password, http.StatusAccepted, nil)
	var p problem
	again.Call(t, http.MethodPost, "/api/v1/session/second-factor", map[string]string{"code": "000000"}, http.StatusUnprocessableEntity, &p)
	if p.Detail != "That code is wrong or was already used. Enter the code your authenticator app shows now." {
		t.Fatalf("detail = %q", p.Detail)
	}
	again.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
}

var relyingParty = virtualwebauthn.RelyingParty{Name: "jorisjonkers.dev", ID: itest.Host, Origin: "https://" + itest.Host}

// publicKey is the WebAuthn options identity sent, without the publicKey wrapper when there is one.
func publicKey(t *testing.T, options string) string {
	t.Helper()
	var wrapped struct {
		PublicKey json.RawMessage `json:"publicKey"`
	}
	if err := json.Unmarshal([]byte(options), &wrapped); err == nil && wrapped.PublicKey != nil {
		return string(wrapped.PublicKey)
	}
	return options
}

func TestAPasskeyAddedInTheProfileFinishesTheNextSignIn(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada := env.SignedUp(t, "ada@example.org")
	authenticator := virtualwebauthn.NewAuthenticator()
	credential := itest.PasskeyCredential(t)
	authenticator.AddCredential(credential)

	var made struct{ Options string }
	ada.Call(t, http.MethodPost, "/api/v1/profile/second-factors/passkey", nil, http.StatusOK, &made)
	attestation, err := virtualwebauthn.ParseAttestationOptions(publicKey(t, made.Options))
	if err != nil {
		t.Fatal(err)
	}
	ada.Call(t, http.MethodPost, "/api/v1/profile/second-factors/passkey/confirmation", map[string]string{
		"label": "MacBook", "password": itest.Password,
		"response": virtualwebauthn.CreateAttestationResponse(relyingParty, authenticator, credential, *attestation),
	}, http.StatusNoContent, nil)

	again := env.Browser()
	again.Email = ada.Email
	var needs needsSecondFactor
	signIn(t, again, itest.Password, http.StatusAccepted, &needs)
	if !slices.Equal(needs.Methods, []string{"passkey"}) {
		t.Fatalf("methods = %v", needs.Methods)
	}
	var asked struct{ Options string }
	again.Call(t, http.MethodPost, "/api/v1/session/passkey", nil, http.StatusOK, &asked)
	assertion, err := virtualwebauthn.ParseAssertionOptions(publicKey(t, asked.Options))
	if err != nil {
		t.Fatal(err)
	}
	again.Call(t, http.MethodPost, "/api/v1/session/passkey/assertion", map[string]string{
		"response": virtualwebauthn.CreateAssertionResponse(relyingParty, authenticator, credential, *assertion),
	}, http.StatusNoContent, nil)
	again.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, nil)
}

func TestTheProfileNeedsASignIn(t *testing.T) {
	t.Parallel()
	itest.Start(t).Browser().Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
}
