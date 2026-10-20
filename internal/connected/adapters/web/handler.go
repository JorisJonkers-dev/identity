// Package web is the connected context's inbound HTTP adapter: a User's Connected Apps, and the
// Admin's mark on a Client that takes them, of the generated ogen Handler.
package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/connected/app"
	"github.com/JorisJonkers-dev/identity/internal/connected/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/httpx"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
)

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	ConnectedApps(ctx context.Context, userID uuid.UUID) ([]domain.ConnectedApp, error)
	Revoke(ctx context.Context, userID, id uuid.UUID) error
	MCPState(ctx context.Context, actor app.Actor, clientID string) (bool, int, error)
	SetMCPResource(ctx context.Context, actor app.Actor, clientID string, on bool) error
	Label(ctx context.Context, clientID string) string
}

// Connected implements the Connected App operations.
type Connected struct{ uc UseCases }

// New returns Connected calling uc.
func New(uc UseCases) *Connected { return &Connected{uc: uc} }

// ListMyConnectedApps implements listMyConnectedApps.
func (h *Connected) ListMyConnectedApps(ctx context.Context) (oas.ListMyConnectedAppsRes, error) {
	apps, err := h.uc.ConnectedApps(ctx, principal.From(ctx).UserID)
	if err != nil {
		return nil, err
	}
	out := make([]oas.ConnectedApp, 0, len(apps))
	for _, a := range apps {
		c := oas.ConnectedApp{
			ID: oas.ID(a.ID), Name: a.Name, CreatedAt: a.CreatedAt,
			Client: oas.TokenClient{ID: oas.ClientId(a.ClientID), Label: h.uc.Label(ctx, a.ClientID)},
		}
		if a.LastUsedAt != nil {
			c.LastUsedAt = oas.NewOptDateTime(*a.LastUsedAt)
		}
		out = append(out, c)
	}
	return &oas.ConnectedAppList{Apps: out}, nil
}

// RevokeMyConnectedApp implements revokeMyConnectedApp.
func (h *Connected) RevokeMyConnectedApp(ctx context.Context, params oas.RevokeMyConnectedAppParams) (oas.RevokeMyConnectedAppRes, error) {
	err := h.uc.Revoke(ctx, principal.From(ctx).UserID, uuid.UUID(params.ConnectedAppId))
	if errors.Is(err, app.ErrNotFound) {
		return problem(http.StatusNotFound, "That app is already disconnected. Reload the page."), nil
	}
	if err != nil {
		return nil, err
	}
	return &oas.RevokeMyConnectedAppNoContent{}, nil
}

// GetMCPResource implements getMCPResource.
func (h *Connected) GetMCPResource(ctx context.Context, params oas.GetMCPResourceParams) (oas.GetMCPResourceRes, error) {
	on, n, err := h.uc.MCPState(ctx, actor(ctx), string(params.ClientId))
	if err != nil {
		return refusal(err)
	}
	return &oas.MCPResource{On: on, ConnectedApps: oas.Count(n)}, nil //nolint:gosec // a count
}

// SetMCPResource implements setMCPResource.
func (h *Connected) SetMCPResource(ctx context.Context, req *oas.MCPResourceSetting, params oas.SetMCPResourceParams) (oas.SetMCPResourceRes, error) {
	if err := h.uc.SetMCPResource(ctx, actor(ctx), string(params.ClientId), req.On); err != nil {
		return refusal(err)
	}
	return &oas.SetMCPResourceNoContent{}, nil
}

func actor(ctx context.Context) app.Actor {
	p := principal.From(ctx)
	return app.Actor{ID: p.UserID, SecondFactor: p.SecondFactor}
}

// refusal answers a rule the caller broke with a problem saying which, and passes anything else on.
func refusal(err error) (*oas.ProblemStatusCode, error) {
	switch {
	case errors.Is(err, app.ErrNotAdmin):
		return problem(http.StatusForbidden, "Only Admins can change this."), nil
	case errors.Is(err, app.ErrSecondFactorNeeded):
		return problem(http.StatusForbidden, "This page needs a Second Factor. If you have a passkey or authenticator app, sign out and sign in again with it. If you have neither, add a Second Factor on the Security page first, then sign out and sign in again."), nil
	case errors.Is(err, app.ErrNotFound):
		return problem(http.StatusNotFound, "That app is not registered any more. Reload the page."), nil
	default:
		return nil, err
	}
}

func problem(status int, detail string) *oas.ProblemStatusCode {
	return &oas.ProblemStatusCode{StatusCode: status, Response: httpx.Problem(status, detail)}
}
