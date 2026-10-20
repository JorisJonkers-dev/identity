package claims

import (
	"reflect"
	"testing"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
)

var proved = time.Unix(1_792_497_600, 0)

func TestASessionsClaimsNameTheSessionAndItsTwoFactorProof(t *testing.T) {
	got := Private(Principal{
		Subject: "a1", Kind: KindUser, Email: "ada@example.org", Name: "Ada", Roles: []string{"user"},
		Via: ViaSession, SessionID: "s1", TwoFactorAt: proved,
	})

	want := map[string]any{
		"ver": Version, "kind": "user", "email": "ada@example.org", "name": "Ada", "roles": []string{"user"},
		"via": "session", "sid": "s1", "two_factor": true, "two_factor_at": proved.Unix(),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("claims = %v", got)
	}
}

func TestAToolsClaimsNameTheToolAsActorAndNoSession(t *testing.T) {
	got := Private(Principal{
		Subject: "a1", Kind: KindUser, Email: "ada@example.org", Name: "Ada", Roles: []string{"user"},
		Via: ViaTool, Actor: Actor{ID: "mcp_1", Name: "Claude Code"}, SessionID: "s1",
	})

	if !reflect.DeepEqual(got["act"], &Act{Subject: "mcp_1", Name: "Claude Code"}) || got["sid"] != nil || got["two_factor"] != false {
		t.Fatalf("claims = %v", got)
	}
}

func TestAMachinesClaimsCarryNoEmailNameOrActor(t *testing.T) {
	got := Private(Principal{Subject: "m1", Kind: KindMachine, Roles: []string{"backup"}, Via: ViaMachine})

	want := map[string]any{"ver": Version, "kind": "machine", "roles": []string{"backup"}, "via": "machine", "two_factor": false}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("claims = %v", got)
	}
}

func TestAPrincipalHoldingNoRoleStatesAnEmptyList(t *testing.T) {
	if got := Of(Principal{Kind: KindMachine, Via: ViaMachine}); got.Roles == nil || len(got.Roles) != 0 {
		t.Fatalf("roles = %#v", got.Roles)
	}
}

func TestAnIntrospectionAnswerStatesWhenItsTokenWasIssuedOnlyWhenKnown(t *testing.T) {
	expires := proved.Add(time.Hour)
	tool, personal := new(oidc.IntrospectionResponse), new(oidc.IntrospectionResponse)

	Introspect(tool, Principal{Subject: "a1", Kind: KindUser, Via: ViaTool}, "wiki", time.Time{}, expires)
	Introspect(personal, Principal{Subject: "a1", Kind: KindUser, Via: ViaPersonalToken}, "wiki", proved, expires)

	if tool.IssuedAt != 0 || personal.IssuedAt != oidc.FromTime(proved) || tool.Expiration != oidc.FromTime(expires) ||
		tool.ClientID != "wiki" || tool.Claims["via"] != "tool" {
		t.Fatalf("answers = %+v, %+v", tool, personal)
	}
}
