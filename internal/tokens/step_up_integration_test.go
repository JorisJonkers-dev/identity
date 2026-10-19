//go:build integration

package tokens_test

import (
	"net/http"
	"testing"
)

type stepUpState struct{ Minutes, Tokens int }

func (w world) stepUp(t *testing.T, minutes int) {
	t.Helper()
	w.admin.Call(t, http.MethodPut, "/api/v1/apps/grafana/step-up", map[string]int{"minutes": minutes}, http.StatusNoContent, nil)
}

func TestATokenForAStepUpAppIsRefused(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.stepUp(t, 15)
	var p problem
	w.ada.Call(t, http.MethodPost, "/api/v1/profile/tokens", map[string]any{"name": "grafana-cli", "clientId": "grafana", "days": 30}, http.StatusUnprocessableEntity, &p)
	if p.Detail != "Grafana asks for two-factor each time it opens, so a token cannot open it." {
		t.Fatalf("detail = %q", p.Detail)
	}
}

func TestAStepUpAppIsNotOfferedForTokens(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.stepUp(t, 15)
	var mine struct{ Clients []struct{ ID string } }
	w.ada.Call(t, http.MethodGet, "/api/v1/profile/tokens", nil, http.StatusOK, &mine)
	if len(mine.Clients) != 0 {
		t.Fatalf("clients = %+v", mine.Clients)
	}
}

func TestTheAppPageCountsTheTokensStepUpWouldRevoke(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.mint(t)
	w.mint(t)
	var s stepUpState
	w.admin.Call(t, http.MethodGet, "/api/v1/apps/grafana/step-up", nil, http.StatusOK, &s)
	if s.Tokens != 2 || s.Minutes != 0 {
		t.Fatalf("step-up = %+v", s)
	}
}

func TestTurningStepUpOnRevokesTheAppsTokensAtOnce(t *testing.T) {
	t.Parallel()
	w := start(t)
	m := w.mint(t)
	w.stepUp(t, 5)
	if got := w.env.Introspect(t, "grafana", grafanaSecret, m.Secret); got["active"] != false {
		t.Fatalf("introspection = %+v", got)
	}
	var mine struct{ Tokens []struct{ State string } }
	w.ada.Call(t, http.MethodGet, "/api/v1/profile/tokens", nil, http.StatusOK, &mine)
	if len(mine.Tokens) != 1 || mine.Tokens[0].State != "revoked" {
		t.Fatalf("tokens = %+v", mine.Tokens)
	}
}

func TestEachTokenStepUpRevokesIsAnAuditEventByTheAdmin(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.mint(t)
	w.stepUp(t, 5)
	var page struct {
		Events []struct {
			Action string
			Actor  *struct{ Email string }
			User   *struct{ Email string }
		}
	}
	w.admin.Call(t, http.MethodGet, "/api/v1/audit?kind=tokens", nil, http.StatusOK, &page)
	e := page.Events[0]
	if e.Action != "token-revoked" || e.Actor == nil || e.Actor.Email != w.admin.Email || e.User == nil || e.User.Email != w.ada.Email {
		t.Fatalf("event = %+v", e)
	}
}
