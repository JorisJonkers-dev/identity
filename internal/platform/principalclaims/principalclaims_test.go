package principalclaims_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/principalclaims"
)

func encoded(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestASessionsPrincipalStatesItsSessionAndWhenItProvedTwoFactor(t *testing.T) {
	at := time.Date(2026, 10, 20, 11, 45, 0, 0, time.UTC)

	c := principalclaims.Claims(principalclaims.Principal{
		Kind: principalclaims.KindUser, Email: "ada@example.org", Name: "Ada", Roles: []string{"user"},
		Via: principalclaims.ViaSession, TwoFactorAt: at, SessionID: "s1",
	})

	want := `{"email":"ada@example.org","kind":"user","name":"Ada","roles":["user"],"sid":"s1","two_factor":true,"two_factor_at":1792496700,"ver":"1.1","via":"session"}`
	if got := encoded(t, c); got != want {
		t.Fatalf("claims = %s", got)
	}
}

func TestAToolsPrincipalNamesTheToolAsActAndNoSession(t *testing.T) {
	c := principalclaims.Claims(principalclaims.Principal{
		Kind: principalclaims.KindUser, Email: "ada@example.org", Name: "Ada", Roles: []string{"user"},
		Via: principalclaims.ViaTool, Actor: &principalclaims.Actor{ID: "mcp_1", Name: "Claude Code"},
	})

	want := `{"act":{"name":"Claude Code","sub":"mcp_1"},"email":"ada@example.org","kind":"user","name":"Ada","roles":["user"],"two_factor":false,"ver":"1.1","via":"tool"}`
	if got := encoded(t, c); got != want {
		t.Fatalf("claims = %s", got)
	}
}

func TestAMachinesPrincipalCarriesNoEmailNameOrSession(t *testing.T) {
	c := principalclaims.Claims(principalclaims.Principal{Kind: principalclaims.KindMachine, Via: principalclaims.ViaMachine})

	want := `{"kind":"machine","roles":[],"two_factor":false,"ver":"1.1","via":"machine"}`
	if got := encoded(t, c); got != want {
		t.Fatalf("claims = %s", got)
	}
}
