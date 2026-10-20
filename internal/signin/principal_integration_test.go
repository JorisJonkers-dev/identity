//go:build integration

package signin_test

import (
	"slices"
	"testing"
)

func TestAUsersAccessTokenStatesThePrincipalOfTheSessionThatSignedThemIn(t *testing.T) {
	t.Parallel()
	w := start(t)

	p := w.env.AccessPrincipal(t, w.ada.AccessToken(t, wiki), "wiki")

	if p["subject"] != w.adaID || p["kind"] != "user" || p["email"] != w.ada.Email || p["name"] != "Grace Hopper" ||
		p["via"] != "session" || p["sid"] == nil || !slices.Equal(p["roles"].([]string), []string{"user"}) ||
		p["twoFactor"] != false || p["actor"] != nil || p["ver"] != "1.1" {
		t.Fatalf("principal = %v", p)
	}
}

func TestAUsersAccessTokenSaysWhenTheSessionProvedTwoFactor(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.ada.AddAuthenticator(t)

	p := w.env.AccessPrincipal(t, w.ada.AccessToken(t, wiki), "wiki")

	if p["twoFactor"] != true || p["twoFactorAt"] == nil {
		t.Fatalf("principal = %v", p)
	}
}
