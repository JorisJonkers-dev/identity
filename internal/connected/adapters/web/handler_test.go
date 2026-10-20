package web_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/connected/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/connected/app"
	"github.com/JorisJonkers-dev/identity/internal/connected/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
)

var (
	errBoom = errors.New("boom")
	ada     = uuid.MustParse("00000000-0000-4000-8000-0000000000a2")
	t0      = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
)

// useCases answers with err when set, and records who asked.
type useCases struct {
	apps  []domain.ConnectedApp
	err   error
	actor app.Actor
	on    bool
}

func (u *useCases) ConnectedApps(context.Context, uuid.UUID) ([]domain.ConnectedApp, error) {
	return u.apps, u.err
}

func (u *useCases) Revoke(context.Context, uuid.UUID, uuid.UUID) error { return u.err }

func (u *useCases) MCPState(_ context.Context, actor app.Actor, _ string) (app.MCPState, error) {
	u.actor = actor
	return app.MCPState{On: true, ConnectedApps: 2, TokenCheck: app.ByIntrospection}, u.err
}

func (u *useCases) SetMCPResource(_ context.Context, actor app.Actor, _ string, on bool) error {
	u.actor, u.on = actor, on
	return u.err
}

func (u *useCases) Label(_ context.Context, clientID string) string { return "Label of " + clientID }

func signedIn() context.Context {
	return principal.With(context.Background(), principal.Principal{UserID: ada, TwoFactor: true})
}

func status(t *testing.T, res any) int {
	t.Helper()
	p, ok := res.(*oas.ProblemStatusCode)
	if !ok {
		t.Fatalf("answer %T is no problem", res)
	}
	return p.StatusCode
}

func TestAUsersConnectedAppsAreListedWithTheirClientsLabelAndLastUse(t *testing.T) {
	used := t0.Add(time.Hour)
	uc := &useCases{apps: []domain.ConnectedApp{
		{ID: uuid.New(), ClientID: "kb", Name: "Claude Code", CreatedAt: t0, LastUsedAt: &used},
		{ID: uuid.New(), ClientID: "kb", Name: "Cursor", CreatedAt: t0},
	}}
	res, err := web.New(uc).ListMyConnectedApps(signedIn())
	list, ok := res.(*oas.ConnectedAppList)
	if err != nil || !ok || len(list.Apps) != 2 || list.Apps[0].Client.Label != "Label of kb" || !list.Apps[0].LastUsedAt.Value.Equal(used) || list.Apps[1].LastUsedAt.Set {
		t.Fatalf("list = %+v, %v", res, err)
	}
}

func TestListingConnectedAppsFailsWhenTheUseCaseDoes(t *testing.T) {
	if _, err := web.New(&useCases{err: errBoom}).ListMyConnectedApps(signedIn()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAUserDisconnectsAConnectedApp(t *testing.T) {
	res, err := web.New(&useCases{}).RevokeMyConnectedApp(signedIn(), oas.RevokeMyConnectedAppParams{})
	if _, ok := res.(*oas.RevokeMyConnectedAppNoContent); err != nil || !ok {
		t.Fatalf("revoke = %+v, %v", res, err)
	}
}

func TestDisconnectingAConnectedAppThatIsGoneIsNotFound(t *testing.T) {
	res, _ := web.New(&useCases{err: app.ErrNotFound}).RevokeMyConnectedApp(signedIn(), oas.RevokeMyConnectedAppParams{})
	if status(t, res) != http.StatusNotFound {
		t.Fatalf("revoke = %+v", res)
	}
}

func TestDisconnectingFailsWhenTheUseCaseDoes(t *testing.T) {
	if _, err := web.New(&useCases{err: errBoom}).RevokeMyConnectedApp(signedIn(), oas.RevokeMyConnectedAppParams{}); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnAdminReadsWhetherAClientTakesConnectedApps(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).GetMCPResource(signedIn(), oas.GetMCPResourceParams{ClientId: "kb"})
	if m, ok := res.(*oas.MCPResource); err != nil || !ok || !m.On || m.ConnectedApps != 2 || m.TokenCheck != oas.MCPResourceTokenCheckIntrospection || uc.actor != (app.Actor{ID: ada, TwoFactor: true}) {
		t.Fatalf("get = %+v, %v", res, err)
	}
}

func TestAnAdminLetsAClientTakeConnectedApps(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).SetMCPResource(signedIn(), &oas.MCPResourceSetting{On: true}, oas.SetMCPResourceParams{ClientId: "kb"})
	if _, ok := res.(*oas.SetMCPResourceNoContent); err != nil || !ok || !uc.on {
		t.Fatalf("set = %+v, %v", res, err)
	}
}

func TestTheMCPSettingIsRefusedByWhatTheCallerBroke(t *testing.T) {
	for _, c := range []struct {
		name   string
		err    error
		status int
	}{
		{"not an Admin", app.ErrNotAdmin, http.StatusForbidden},
		{"no two-factor", app.ErrTwoFactorProofNeeded, http.StatusForbidden},
		{"a Client that is gone", app.ErrNotFound, http.StatusNotFound},
		{"a Client that can check no tool's token", app.ErrUnchecked, http.StatusConflict},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := web.New(&useCases{err: c.err})
			got, _ := h.GetMCPResource(signedIn(), oas.GetMCPResourceParams{ClientId: "kb"})
			set, _ := h.SetMCPResource(signedIn(), &oas.MCPResourceSetting{}, oas.SetMCPResourceParams{ClientId: "kb"})
			if status(t, got) != c.status || status(t, set) != c.status {
				t.Fatalf("get %+v, set %+v", got, set)
			}
		})
	}
}

func TestTheMCPSettingFailsWhenTheUseCaseDoes(t *testing.T) {
	if _, err := web.New(&useCases{err: errBoom}).GetMCPResource(signedIn(), oas.GetMCPResourceParams{ClientId: "kb"}); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}
