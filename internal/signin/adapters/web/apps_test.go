package web_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

var (
	errBoom = errors.New("boom")
	root    = uuid.MustParse("00000000-0000-4000-8000-0000000000a3")
)

// secrets records what it was asked and answers err.
type secrets struct {
	err    error
	actor  app.Actor
	client string
}

func (s *secrets) Rotate(_ context.Context, actor app.Actor, clientID string) error {
	s.actor, s.client = actor, clientID
	return s.err
}

// stepUps records what it was asked and answers minutes and err.
type stepUps struct {
	err     error
	minutes int
	actor   app.Actor
	client  string
	set     int
}

func (s *stepUps) StepUp(_ context.Context, actor app.Actor, clientID string) (app.StepUpState, error) {
	s.actor, s.client = actor, clientID
	return app.StepUpState{Minutes: s.minutes, Tokens: 3, ConnectedApps: 4}, s.err
}

func (s *stepUps) Set(_ context.Context, actor app.Actor, clientID string, minutes int) error {
	s.actor, s.client, s.set = actor, clientID, minutes
	return s.err
}

func asRoot() context.Context {
	return principal.With(context.Background(), principal.Principal{UserID: root, SecondFactor: true})
}

func TestAnAdminGivesAClientANewSecret(t *testing.T) {
	t.Parallel()
	s := &secrets{}
	res, err := web.New(s, nil, nil).RotateClientSecret(asRoot(), oas.RotateClientSecretParams{ClientId: "grafana"})
	if _, ok := res.(*oas.RotateClientSecretNoContent); err != nil || !ok {
		t.Fatalf("RotateClientSecret = %+v, %v", res, err)
	}
	if s.actor != (app.Actor{ID: root, SecondFactor: true}) || s.client != "grafana" {
		t.Fatalf("asked %+v for %q", s.actor, s.client)
	}
}

func TestARefusedRotationSaysWhy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		err    error
		status int
		detail string
	}{
		{"not an Admin", app.ErrNotAdmin, 403, "Only Admins give apps new secrets."},
		{"no Second Factor", app.ErrSecondFactorNeeded, 403, "This page needs a Second Factor. If you have a passkey or authenticator app, sign out and sign in again with it. If you have neither, add a Second Factor on the Security page first, then sign out and sign in again."},
		{"an app that is gone", app.ErrNotFound, 404, "That app is not registered any more. Reload the page."},
		{"an app without a secret", app.ErrNoSecret, 422, "That app signs in without a secret, so it has none to replace."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := web.New(&secrets{err: tc.err}, nil, nil).RotateClientSecret(asRoot(), oas.RotateClientSecretParams{ClientId: "grafana"})
			p, ok := res.(*oas.ProblemStatusCode)
			if err != nil || !ok || p.StatusCode != tc.status || p.Response.Detail.Value != tc.detail {
				t.Fatalf("RotateClientSecret = %+v, %v", res, err)
			}
		})
	}
}

func TestAFailedRotationIsPassedOn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"Vault failing", errBoom},
		{"a Client with nowhere to keep its secret", app.ErrNoSecretPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := web.New(&secrets{err: tc.err}, nil, nil).RotateClientSecret(asRoot(), oas.RotateClientSecretParams{}); !errors.Is(err, tc.err) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAnAdminReadsAClientsStepUp(t *testing.T) {
	t.Parallel()
	s := &stepUps{minutes: 15}
	res, err := web.New(nil, s, nil).GetStepUp(asRoot(), oas.GetStepUpParams{ClientId: "grafana"})
	if got, ok := res.(*oas.StepUpState); err != nil || !ok || got.Minutes != 15 || got.Tokens != 3 || got.ConnectedApps != 4 || s.client != "grafana" || s.actor != (app.Actor{ID: root, SecondFactor: true}) {
		t.Fatalf("GetStepUp = %+v, %v", res, err)
	}
}

func TestAnAdminSetsAClientsStepUp(t *testing.T) {
	t.Parallel()
	s := &stepUps{}
	res, err := web.New(nil, s, nil).SetStepUp(asRoot(), &oas.StepUp{Minutes: 60}, oas.SetStepUpParams{ClientId: "grafana"})
	if _, ok := res.(*oas.SetStepUpNoContent); err != nil || !ok || s.set != 60 || s.client != "grafana" || s.actor != (app.Actor{ID: root, SecondFactor: true}) {
		t.Fatalf("SetStepUp = %+v, %v", res, err)
	}
}

func TestARefusedStepUpSaysWhy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		err    error
		status int
		detail string
	}{
		{"not an Admin", app.ErrNotAdmin, 403, "Only Admins can change this."},
		{"a window that is not offered", app.ErrInvalidStepUp, 422, "Choose Off, 5 minutes, 15 minutes or 60 minutes."},
		{"an app that is gone", app.ErrNotFound, 404, "That app is not registered any more. Reload the page."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := web.New(nil, &stepUps{err: tc.err}, nil).SetStepUp(asRoot(), &oas.StepUp{}, oas.SetStepUpParams{ClientId: "grafana"})
			p, ok := res.(*oas.ProblemStatusCode)
			if err != nil || !ok || p.StatusCode != tc.status || p.Response.Detail.Value != tc.detail {
				t.Fatalf("SetStepUp = %+v, %v", res, err)
			}
		})
	}
}

func TestAStepUpThatCannotBeReadIsPassedOn(t *testing.T) {
	t.Parallel()
	if _, err := web.New(nil, &stepUps{err: errBoom}, nil).GetStepUp(asRoot(), oas.GetStepUpParams{}); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}
