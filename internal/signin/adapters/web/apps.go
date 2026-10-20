// Package web is the sign-in context's inbound HTTP adapter: the Admin operations on Clients of the
// generated ogen Handler.
package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/platform/httpx"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	Rotate(ctx context.Context, actor app.Actor, clientID string) error
}

// StepUps is the step-up use cases.
type StepUps interface {
	StepUp(ctx context.Context, actor app.Actor, clientID string) (app.StepUpState, error)
	Set(ctx context.Context, actor app.Actor, clientID string, minutes int) error
}

// MachineSecrets gives Machine Identities new client secrets (identity#22).
type MachineSecrets interface {
	RotateSecret(ctx context.Context, actor app.Actor, id uuid.UUID) (string, error)
}

// Apps implements the Client operations, and the Machine Identity secret beside them.
type Apps struct {
	uc       UseCases
	stepUps  StepUps
	machines MachineSecrets
}

// New returns Apps calling uc, stepUps and machines.
func New(uc UseCases, stepUps StepUps, machines MachineSecrets) *Apps {
	return &Apps{uc: uc, stepUps: stepUps, machines: machines}
}

func actor(ctx context.Context) app.Actor {
	p := principal.From(ctx)
	return app.Actor{ID: p.UserID, TwoFactor: p.TwoFactor}
}

// GetStepUp implements getStepUp.
func (h *Apps) GetStepUp(ctx context.Context, params oas.GetStepUpParams) (oas.GetStepUpRes, error) {
	s, err := h.stepUps.StepUp(ctx, actor(ctx), string(params.ClientId))
	if err != nil {
		return stepUpRefusal(err)
	}
	return &oas.StepUpState{Minutes: oas.StepUpStateMinutes(s.Minutes), Tokens: oas.Count(s.Tokens), ConnectedApps: oas.Count(s.ConnectedApps)}, nil //nolint:gosec // 0 to 60, and counts
}

// SetStepUp implements setStepUp.
func (h *Apps) SetStepUp(ctx context.Context, req *oas.StepUp, params oas.SetStepUpParams) (oas.SetStepUpRes, error) {
	if err := h.stepUps.Set(ctx, actor(ctx), string(params.ClientId), int(req.Minutes)); err != nil {
		return stepUpRefusal(err)
	}
	return &oas.SetStepUpNoContent{}, nil
}

func stepUpRefusal(err error) (*oas.ProblemStatusCode, error) {
	switch {
	case errors.Is(err, app.ErrNotAdmin):
		return problem(http.StatusForbidden, "Only Admins can change this."), nil
	case errors.Is(err, app.ErrInvalidStepUp):
		return problem(http.StatusUnprocessableEntity, "Choose Off, 5 minutes, 15 minutes or 60 minutes."), nil
	default:
		return refusal(err)
	}
}

// RotateClientSecret implements rotateClientSecret.
func (h *Apps) RotateClientSecret(ctx context.Context, params oas.RotateClientSecretParams) (oas.RotateClientSecretRes, error) {
	if err := h.uc.Rotate(ctx, actor(ctx), string(params.ClientId)); err != nil {
		return refusal(err)
	}
	return &oas.RotateClientSecretNoContent{}, nil
}

// RotateMachineSecret implements rotateMachineSecret.
func (h *Apps) RotateMachineSecret(ctx context.Context, params oas.RotateMachineSecretParams) (oas.RotateMachineSecretRes, error) {
	secret, err := h.machines.RotateSecret(ctx, actor(ctx), uuid.UUID(params.MachineId))
	switch {
	case errors.Is(err, app.ErrNotAdmin):
		return problem(http.StatusForbidden, "Only Admins can make a new secret for a Machine Identity."), nil
	case errors.Is(err, app.ErrNotFound):
		return problem(http.StatusNotFound, "That Machine Identity no longer exists. Reload the page."), nil
	case err != nil:
		return refusal(err)
	}
	return &oas.MachineSecret{Secret: secret}, nil
}

// refusal answers a rule the caller broke with a problem saying which, and passes anything else on.
func refusal(err error) (*oas.ProblemStatusCode, error) {
	switch {
	case errors.Is(err, app.ErrNotAdmin):
		return problem(http.StatusForbidden, "Only Admins give apps new secrets."), nil
	case errors.Is(err, app.ErrTwoFactorProofNeeded):
		return problem(http.StatusForbidden, "This page needs two-factor. If you have a passkey or authenticator app, sign out and sign in again with it. If you have neither, add a two-factor method on the Security page first, then sign out and sign in again."), nil
	case errors.Is(err, app.ErrNotFound):
		return problem(http.StatusNotFound, "That app is not registered any more. Reload the page."), nil
	case errors.Is(err, app.ErrNoSecret):
		return problem(http.StatusUnprocessableEntity, "That app signs in without a secret, so it has none to replace."), nil
	default:
		return nil, err
	}
}

func problem(status int, detail string) *oas.ProblemStatusCode {
	return &oas.ProblemStatusCode{StatusCode: status, Response: httpx.Problem(status, detail)}
}
