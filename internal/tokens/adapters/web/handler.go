// Package web is the tokens context's inbound HTTP adapter: the Personal Token operations of the
// generated ogen Handler, for the signed-in User and for Admins.
package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/platform/httpx"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
	"github.com/JorisJonkers-dev/identity/internal/tokens/app"
	"github.com/JorisJonkers-dev/identity/internal/tokens/domain"
)

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	Mint(ctx context.Context, userID uuid.UUID, name, clientID string, days int) (app.Minted, error)
	Mine(ctx context.Context, userID uuid.UUID) ([]domain.Token, error)
	Mintable(ctx context.Context, userID uuid.UUID) ([]app.Client, error)
	RevokeMine(ctx context.Context, userID, tokenID uuid.UUID) error
	Of(ctx context.Context, actor app.Actor, userID uuid.UUID) ([]domain.Token, error)
	RevokeAs(ctx context.Context, actor app.Actor, userID, tokenID uuid.UUID) error
	Label(ctx context.Context, clientID string) string
	RolesOf(ctx context.Context, t domain.Token) ([]string, error)
	State(t domain.Token) domain.State
}

// Tokens implements the Personal Token operations.
type Tokens struct{ uc UseCases }

// New returns Tokens calling uc.
func New(uc UseCases) *Tokens { return &Tokens{uc: uc} }

// ListMyTokens implements listMyTokens.
func (h *Tokens) ListMyTokens(ctx context.Context) (oas.ListMyTokensRes, error) {
	me := principal.From(ctx).UserID
	mine, err := h.uc.Mine(ctx, me)
	if err != nil {
		return nil, err
	}
	clients, err := h.uc.Mintable(ctx, me)
	if err != nil {
		return nil, err
	}
	tokens, err := h.list(ctx, mine)
	if err != nil {
		return nil, err
	}
	res := &oas.MyTokens{Tokens: tokens, Clients: make([]oas.TokenClient, 0, len(clients))}
	for _, c := range clients {
		res.Clients = append(res.Clients, oas.TokenClient{ID: oas.ClientId(c.ID), Label: c.Label})
	}
	return res, nil
}

// MintToken implements mintToken.
func (h *Tokens) MintToken(ctx context.Context, req *oas.TokenDraft) (oas.MintTokenRes, error) {
	m, err := h.uc.Mint(ctx, principal.From(ctx).UserID, req.Name, string(req.ClientId), int(req.Days))
	if errors.Is(err, app.ErrStepUpClient) {
		return problem(http.StatusUnprocessableEntity, h.uc.Label(ctx, string(req.ClientId))+" asks for two-factor each time it opens, so a token cannot open it."), nil
	}
	if err != nil {
		return refusal(err)
	}
	t, err := h.token(ctx, m.Token)
	if err != nil {
		return nil, err
	}
	return &oas.MintedToken{Token: t, Secret: m.Secret}, nil
}

// RevokeMyToken implements revokeMyToken.
func (h *Tokens) RevokeMyToken(ctx context.Context, params oas.RevokeMyTokenParams) (oas.RevokeMyTokenRes, error) {
	if err := h.uc.RevokeMine(ctx, principal.From(ctx).UserID, uuid.UUID(params.PatId)); err != nil {
		return refusal(err)
	}
	return &oas.RevokeMyTokenNoContent{}, nil
}

// ListUserTokens implements listUserTokens.
func (h *Tokens) ListUserTokens(ctx context.Context, params oas.ListUserTokensParams) (oas.ListUserTokensRes, error) {
	tokens, err := h.uc.Of(ctx, actor(ctx), uuid.UUID(params.UserId))
	if err != nil {
		return refusal(err)
	}
	list, err := h.list(ctx, tokens)
	if err != nil {
		return nil, err
	}
	return &oas.TokenList{Tokens: list}, nil
}

// RevokeUserToken implements revokeUserToken.
func (h *Tokens) RevokeUserToken(ctx context.Context, params oas.RevokeUserTokenParams) (oas.RevokeUserTokenRes, error) {
	if err := h.uc.RevokeAs(ctx, actor(ctx), uuid.UUID(params.UserId), uuid.UUID(params.PatId)); err != nil {
		return refusal(err)
	}
	return &oas.RevokeUserTokenNoContent{}, nil
}

func (h *Tokens) list(ctx context.Context, tokens []domain.Token) ([]oas.PersonalToken, error) {
	out := make([]oas.PersonalToken, 0, len(tokens))
	for _, t := range tokens {
		pt, err := h.token(ctx, t)
		if err != nil {
			return nil, err
		}
		out = append(out, pt)
	}
	return out, nil
}

func (h *Tokens) token(ctx context.Context, t domain.Token) (oas.PersonalToken, error) {
	held, err := h.uc.RolesOf(ctx, t)
	if err != nil {
		return oas.PersonalToken{}, err
	}
	roles := make([]oas.AppRole, 0, len(held))
	for _, r := range held {
		roles = append(roles, oas.AppRole(r))
	}
	out := oas.PersonalToken{
		ID: oas.ID(t.ID), Name: t.Name, Client: oas.TokenClient{ID: oas.ClientId(t.ClientID), Label: h.uc.Label(ctx, t.ClientID)},
		Roles: roles, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, State: oas.PersonalTokenState(h.uc.State(t)),
	}
	if t.LastUsedAt != nil {
		out.LastUsedAt = oas.NewOptDateTime(*t.LastUsedAt)
	}
	return out, nil
}

func actor(ctx context.Context) app.Actor {
	p := principal.From(ctx)
	return app.Actor{ID: p.UserID, SecondFactor: p.SecondFactor}
}

// refusal answers a rule the caller broke with a problem saying which, and passes anything else on.
func refusal(err error) (*oas.ProblemStatusCode, error) {
	switch {
	case errors.Is(err, domain.ErrInvalidName):
		return problem(http.StatusUnprocessableEntity, "Enter a token name of 1 to 64 characters."), nil
	case errors.Is(err, domain.ErrInvalidLifetime):
		return problem(http.StatusUnprocessableEntity, "Choose 30, 90 or 365 days."), nil
	case errors.Is(err, app.ErrUnknownClient):
		return problem(http.StatusUnprocessableEntity, "That app is not registered."), nil
	case errors.Is(err, app.ErrNoAccess):
		return problem(http.StatusUnprocessableEntity, "You have no role in that app, so a token for it could open nothing."), nil
	case errors.Is(err, app.ErrNotFound):
		return problem(http.StatusNotFound, "That token no longer exists. Reload the page."), nil
	case errors.Is(err, app.ErrNotAdmin):
		return problem(http.StatusForbidden, "Only Admins manage other people's tokens."), nil
	case errors.Is(err, app.ErrSecondFactorNeeded):
		return problem(http.StatusForbidden, "This page needs a Second Factor. If you have a passkey or authenticator app, sign out and sign in again with it. If you have neither, add a Second Factor on the Security page first, then sign out and sign in again."), nil
	default:
		return nil, err
	}
}

func problem(status int, detail string) *oas.ProblemStatusCode {
	return &oas.ProblemStatusCode{StatusCode: status, Response: httpx.Problem(status, detail)}
}
