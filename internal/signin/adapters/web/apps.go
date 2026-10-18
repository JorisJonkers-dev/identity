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

// Apps implements the Client operations.
type Apps struct{ uc UseCases }

// New returns Apps calling uc.
func New(uc UseCases) *Apps { return &Apps{uc: uc} }

// RotateClientSecret implements rotateClientSecret.
func (h *Apps) RotateClientSecret(ctx context.Context, params oas.RotateClientSecretParams) (oas.RotateClientSecretRes, error) {
	p := principal.From(ctx)
	if err := h.uc.Rotate(ctx, app.Actor{ID: p.UserID, SecondFactor: p.SecondFactor}, string(params.ClientId)); err != nil {
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
