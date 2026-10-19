//go:build integration

package users_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
	"github.com/pquerna/otp/totp"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
)

const (
	resetSubject = "Reset your password"
	newPassword  = "a brand new passphrase"
)

type resetCheck struct{ Methods []string }

// resetLink asks for a reset of b's password and returns the token mailed to them.
func resetLink(t *testing.T, env *itest.Env, b *itest.Browser) string {
	t.Helper()
	env.Browser().Call(t, http.MethodPost, "/api/v1/password-resets", map[string]string{"email": b.Email}, http.StatusAccepted, nil)
	return itest.TokenIn(t, itest.Mailed(t, b.Email, resetSubject), "/reset")
}

func completeReset(t *testing.T, env *itest.Env, token string, want int, out any) {
	t.Helper()
	env.Browser().Call(t, http.MethodPost, "/api/v1/password-resets/completion", map[string]string{"token": token, "password": newPassword}, want, out)
}

func TestAForgottenPasswordIsResetThroughTheMailedLink(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada := env.SignedUp(t, "ada@example.org")
	token := resetLink(t, env, ada)
	var c resetCheck
	env.Browser().Call(t, http.MethodPost, "/api/v1/password-resets/check", map[string]string{"token": token}, http.StatusOK, &c)
	if len(c.Methods) != 0 {
		t.Fatalf("methods = %v", c.Methods)
	}
	completeReset(t, env, token, http.StatusNoContent, nil)
	env.Browser().Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": ada.Email, "password": newPassword}, http.StatusNoContent, nil)
	env.Browser().Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": ada.Email, "password": itest.Password}, http.StatusUnauthorized, nil)
}

func TestAResetEndsEverySessionAndMailsANotice(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada := env.SignedUp(t, "ada@example.org")
	completeReset(t, env, resetLink(t, env, ada), http.StatusNoContent, nil)
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
	itest.Mailed(t, ada.Email, "Your password was reset")
}

func TestAResetForAnUnknownEmailAnswersTheSame(t *testing.T) {
	t.Parallel()
	itest.Start(t).Browser().Call(t, http.MethodPost, "/api/v1/password-resets", map[string]string{"email": "nobody@example.org"}, http.StatusAccepted, nil)
}

func TestAResetLinkWorksOnce(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	token := resetLink(t, env, env.SignedUp(t, "ada@example.org"))
	completeReset(t, env, token, http.StatusNoContent, nil)
	var p problem
	completeReset(t, env, token, http.StatusUnprocessableEntity, &p)
	if p.Detail != "This link has expired or was already used. Ask for a new one." {
		t.Fatalf("detail = %q", p.Detail)
	}
}

func TestAResetLinkWorksForThirtyMinutes(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	token := resetLink(t, env, env.SignedUp(t, "ada@example.org"))
	env.Pass(30*time.Minute + time.Second)
	env.Browser().Call(t, http.MethodPost, "/api/v1/password-resets/check", map[string]string{"token": token}, http.StatusUnprocessableEntity, nil)
}

func TestAUserWithTwoFactorProvesItBeforeTheNewPasswordIsAccepted(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada := env.SignedUp(t, "ada@example.org")
	secret := ada.AddAuthenticator(t)
	env.Pass(time.Minute)
	token := resetLink(t, env, ada)
	var c resetCheck
	env.Browser().Call(t, http.MethodPost, "/api/v1/password-resets/check", map[string]string{"token": token}, http.StatusOK, &c)
	if len(c.Methods) != 1 || c.Methods[0] != "totp" {
		t.Fatalf("methods = %v", c.Methods)
	}
	var p problem
	completeReset(t, env, token, http.StatusForbidden, &p)
	if p.Detail != "Confirm it is you with two-factor first." {
		t.Fatalf("detail = %q", p.Detail)
	}
	code, err := totp.GenerateCode(secret, env.Now())
	if err != nil {
		t.Fatal(err)
	}
	env.Browser().Call(t, http.MethodPost, "/api/v1/password-resets/code", map[string]string{"token": token, "code": code}, http.StatusNoContent, nil)
	completeReset(t, env, token, http.StatusNoContent, nil)
}

func TestAWrongResetCodeIsRefused(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada := env.SignedUp(t, "ada@example.org")
	ada.AddAuthenticator(t)
	token := resetLink(t, env, ada)
	env.Browser().Call(t, http.MethodPost, "/api/v1/password-resets/check", map[string]string{"token": token}, http.StatusOK, nil)
	env.Browser().Call(t, http.MethodPost, "/api/v1/password-resets/code", map[string]string{"token": token, "code": "000000"}, http.StatusUnprocessableEntity, nil)
	completeReset(t, env, token, http.StatusForbidden, nil)
}

func TestResetRequestsAreLimitedPerAddress(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada := env.SignedUp(t, "ada@example.org")
	for range 4 {
		env.Browser().Call(t, http.MethodPost, "/api/v1/password-resets", map[string]string{"email": ada.Email}, http.StatusAccepted, nil)
	}
	itest.Mailed(t, ada.Email, resetSubject)
	time.Sleep(time.Second)
	if n := itest.MailCount(t, ada.Email, resetSubject); n != 3 {
		t.Fatalf("%d reset mails, want 3", n)
	}
}

func TestAResetIsAnAuditEvent(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	completeReset(t, env, resetLink(t, env, ada), http.StatusNoContent, nil)
	var page struct{ Events []struct{ Action string } }
	admin.Call(t, http.MethodGet, "/api/v1/audit?kind=people&user="+admin.UserID(t, ada.Email), nil, http.StatusOK, &page)
	if len(page.Events) == 0 || page.Events[0].Action != "password-reset" {
		t.Fatalf("events = %+v", page.Events)
	}
}

func TestAPasskeyProvesAReset(t *testing.T) {
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

	token := resetLink(t, env, ada)
	browser := env.Browser()
	browser.Call(t, http.MethodPost, "/api/v1/password-resets/check", map[string]string{"token": token}, http.StatusOK, nil)
	var asked struct{ Options string }
	browser.Call(t, http.MethodPost, "/api/v1/password-resets/passkey", map[string]string{"token": token}, http.StatusOK, &asked)
	assertion, err := virtualwebauthn.ParseAssertionOptions(publicKey(t, asked.Options))
	if err != nil {
		t.Fatal(err)
	}
	browser.Call(t, http.MethodPost, "/api/v1/password-resets/passkey/assertion", map[string]string{
		"token": token, "response": virtualwebauthn.CreateAssertionResponse(relyingParty, authenticator, credential, *assertion),
	}, http.StatusNoContent, nil)
	completeReset(t, env, token, http.StatusNoContent, nil)
}
