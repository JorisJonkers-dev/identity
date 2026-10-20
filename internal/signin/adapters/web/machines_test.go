package web_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

// machineSecrets answers with a secret, or err, and records who asked for which.
type machineSecrets struct {
	err   error
	actor app.Actor
	id    uuid.UUID
}

func (m *machineSecrets) RotateSecret(_ context.Context, actor app.Actor, id uuid.UUID) (string, error) {
	m.actor, m.id = actor, id
	return "new-secret", m.err
}

var backupID = uuid.MustParse("00000000-0000-4000-8000-0000000000c1")

func TestAnAdminGetsAMachineIdentitysNewSecretOnce(t *testing.T) {
	t.Parallel()
	m := &machineSecrets{}
	res, err := web.New(nil, nil, m).RotateMachineSecret(asRoot(), oas.RotateMachineSecretParams{MachineId: oas.ID(backupID)})
	s, ok := res.(*oas.MachineSecret)
	if err != nil || !ok || s.Secret != "new-secret" {
		t.Fatalf("RotateMachineSecret = %+v, %v", res, err)
	}
	if m.actor != (app.Actor{ID: root, TwoFactor: true}) || m.id != backupID {
		t.Fatalf("asked %+v for %v", m.actor, m.id)
	}
}

func TestARefusedMachineSecretSaysWhy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		err    error
		status int
		detail string
	}{
		{"not an Admin", app.ErrNotAdmin, 403, "Only Admins can make a new secret for a Machine Identity."},
		{"no two-factor", app.ErrTwoFactorProofNeeded, 403, "This page needs two-factor. If you have a passkey or authenticator app, sign out and sign in again with it. If you have neither, add a two-factor method on the Security page first, then sign out and sign in again."},
		{"a Machine Identity that is gone", app.ErrNotFound, 404, "That Machine Identity no longer exists. Reload the page."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := web.New(nil, nil, &machineSecrets{err: tc.err}).RotateMachineSecret(asRoot(), oas.RotateMachineSecretParams{})
			p, ok := res.(*oas.ProblemStatusCode)
			if err != nil || !ok || p.StatusCode != tc.status || p.Response.Detail.Value != tc.detail {
				t.Fatalf("RotateMachineSecret = %+v, %v", res, err)
			}
		})
	}
}

func TestAFailedMachineSecretIsPassedOn(t *testing.T) {
	t.Parallel()
	if _, err := web.New(nil, nil, &machineSecrets{err: errBoom}).RotateMachineSecret(asRoot(), oas.RotateMachineSecretParams{}); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}
