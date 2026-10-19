// Package web is the sign-in context's inbound HTTP adapter: the Admin operations on Clients of the
// generated ogen Handler.
package web

import (
	"context"
	"errors"
	"net/http"

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
	StepUp(ctx context.Context, actor app.Actor, clientID string) (int, error)
	Set(ctx context.Context, actor app.Actor, clientID string, minutes int) error
}

// Apps implements the Client operations.
type Apps struct {
	uc      UseCases
	stepUps StepUps
}

// New returns Apps calling uc and stepUps.
func New(uc UseCases, stepUps StepUps) *Apps { return &Apps{uc: uc, stepUps: stepUps} }

func actor(ctx context.Context) app.Actor {
	p := principal.From(ctx)
	return app.Actor{ID: p.UserID, SecondFactor: p.SecondFactor}
}

// GetStepUp implements getStepUp.
func (h *Apps) GetStepUp(ctx context.Context, params oas.GetStepUpParams) (oas.GetStepUpRes, error) {
	minutes, err := h.stepUps.StepUp(ctx, actor(ctx), string(params.ClientId))
	if err != nil {
		return stepUpRefusal(err)
	}
	return &oas.StepUp{Minutes: oas.StepUpMinutes(minutes)}, nil //nolint:gosec // 0 to 60
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

// refusal answers a rule the caller broke with a problem saying which, and passes anything else on.
func refusal(err error) (*oas.ProblemStatusCode, error) {
	switch {
	case errors.Is(err, app.ErrNotAdmin):
		return problem(http.StatusForbidden, "Only Admins give apps new secrets."), nil
	case errors.Is(err, app.ErrSecondFactorNeeded):
		return problem(http.StatusForbidden, "This page needs a Second Factor. If you have a passkey or authenticator app, sign out and sign in again with it. If you have neither, add a Second Factor on the Security page first, then sign out and sign in again."), nil
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
