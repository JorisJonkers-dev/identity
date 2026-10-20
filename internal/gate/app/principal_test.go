package app_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/gate/app"
)

// principals signs a Verdict as "signed-<n>", and remembers what it was asked to sign.
type principals struct {
	signed  []app.Verdict
	clients []string
	at      []time.Time
	err     error
}

func (p *principals) Sign(v app.Verdict, clientID string, at time.Time) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	p.signed, p.clients, p.at = append(p.signed, v), append(p.clients, clientID), append(p.at, at)
	return fmt.Sprintf("signed-%d", len(p.signed)), nil
}

func withPrincipals(f *fixture) *principals {
	p := &principals{}
	f.SetPrincipals(p)
	return p
}

func TestAnAllowedCheckCarriesThePrincipalSignedForItsClient(t *testing.T) {
	f := newFixture()
	p := withPrincipals(f)
	cookie := f.signedIn(t)

	v, err := f.Check(context.Background(), stats.Host, cookie)

	if err != nil || v.Signed != "signed-1" || len(p.clients) != 1 || p.clients[0] != stats.ID || !p.at[0].Equal(t0) {
		t.Fatalf("Check = %+v, %v; signed for %v at %v", v, err, p.clients, p.at)
	}
}

func TestASessionsPrincipalSaysItCameBySessionAndNamesTheSession(t *testing.T) {
	f := newFixture()
	p := withPrincipals(f)
	cookie := f.signedIn(t)

	_, _ = f.Check(context.Background(), stats.Host, cookie)

	got := p.signed[0]
	if got.Kind != app.KindUser || got.Via != app.ViaSession || got.SessionID != ada.SessionID || got.Actor != (app.Actor{}) {
		t.Fatalf("signed %+v", got)
	}
}

func TestASessionsPrincipalSaysWhenItProvedTwoFactor(t *testing.T) {
	f := newFixture()
	p := withPrincipals(f)
	f.accounts.vouchedProof = t0.Add(-time.Hour)
	cookie := f.signedIn(t)

	_, _ = f.Check(context.Background(), stats.Host, cookie)

	if got := p.signed[0].TwoFactorAt; !got.Equal(t0.Add(-time.Hour)) {
		t.Fatalf("two-factor at %v", got)
	}
}

func TestAPersonalTokensPrincipalSaysItCameByPersonalTokenWithNoSession(t *testing.T) {
	f := newFixture()
	p := withPrincipals(f)
	withTokens(f)

	_, _ = f.CheckBearer(context.Background(), stats.Host, "idpat_ada")

	got := p.signed[0]
	if got.Via != app.ViaPersonalToken || got.SessionID != uuid.Nil || p.clients[0] != stats.ID {
		t.Fatalf("signed %+v for %v", got, p.clients)
	}
}

func TestAToolsPrincipalNamesTheTool(t *testing.T) {
	f := newFixture()
	p := withPrincipals(f)
	withTokens(f)

	_, _ = f.CheckBearer(context.Background(), stats.Host, "idapp_claude")

	got := p.signed[0]
	if got.Via != app.ViaTool || got.Actor != claude || got.SessionID != uuid.Nil {
		t.Fatalf("signed %+v", got)
	}
}

func TestACachedCheckKeepsItsSignedPrincipalForAtMostHalfItsLifetime(t *testing.T) {
	f := newFixture()
	f.Service = app.New(f.store, clients{stats.Host: stats}, f.roles, f.accounts, f.events, func() time.Time { return f.now }, &counter{}, 5*time.Minute)
	p := withPrincipals(f)
	cookie := f.signedIn(t)
	ctx := context.Background()
	_, _ = f.Check(ctx, stats.Host, cookie)

	f.now = t0.Add(app.PrincipalLifetime/2 - time.Second)
	if v, _ := f.Check(ctx, stats.Host, cookie); v.Signed != "signed-1" {
		t.Fatalf("within half its lifetime the check signed again: %q", v.Signed)
	}
	f.now = t0.Add(app.PrincipalLifetime / 2)
	if v, _ := f.Check(ctx, stats.Host, cookie); v.Signed != "signed-2" || len(p.signed) != 2 {
		t.Fatalf("after half its lifetime the check kept %q", v.Signed)
	}
}

func TestACheckWhosePrincipalCannotBeSignedFailsAndIsAskedAgain(t *testing.T) {
	f := newFixture()
	p := withPrincipals(f)
	p.err = errBoom
	cookie := f.signedIn(t)
	ctx := context.Background()
	if _, err := f.Check(ctx, stats.Host, cookie); !errors.Is(err, errBoom) {
		t.Fatalf("Check = %v", err)
	}

	p.err = nil

	if v, err := f.Check(ctx, stats.Host, cookie); err != nil || v.Signed != "signed-1" {
		t.Fatalf("the failure was cached: %+v, %v", v, err)
	}
}

func TestABearerCheckWhosePrincipalCannotBeSignedFails(t *testing.T) {
	f := newFixture()
	withPrincipals(f).err = errBoom
	withTokens(f)

	if _, err := f.CheckBearer(context.Background(), stats.Host, "idpat_ada"); !errors.Is(err, errBoom) {
		t.Fatalf("CheckBearer = %v", err)
	}
}

func TestWithoutPrincipalsSetACheckCarriesNoSignedPrincipal(t *testing.T) {
	f := newFixture()
	cookie := f.signedIn(t)

	if v, err := f.Check(context.Background(), stats.Host, cookie); err != nil || v.Signed != "" {
		t.Fatalf("Check = %+v, %v", v, err)
	}
}

func TestAVerdictStatesEveryPlainHeaderEmptyWhenItHasNoValue(t *testing.T) {
	v := app.Verdict{UserID: ada.UserID, Email: ada.Email, Name: ada.Name, Roles: []string{"user", "editor"}, Kind: app.KindUser, Via: app.ViaSession}

	got := v.Headers()

	want := map[string]string{
		"X-Auth-Request-User": ada.UserID.String(), "X-Auth-Request-Email": ada.Email, "X-Auth-Request-Name": "Ada",
		"X-Auth-Request-Roles": "user,editor", "X-Auth-Request-Kind": "user", "X-Auth-Request-Via": "session", "X-Auth-Request-Actor": "",
	}
	if !maps.Equal(got, want) {
		t.Fatalf("headers = %v", got)
	}
}

func TestAToolsVerdictNamesTheToolInItsActorHeader(t *testing.T) {
	v := app.Verdict{Via: app.ViaTool, Actor: claude}

	if got := v.Headers()["X-Auth-Request-Actor"]; got != "Claude Code" {
		t.Fatalf("actor header = %q", got)
	}
}
