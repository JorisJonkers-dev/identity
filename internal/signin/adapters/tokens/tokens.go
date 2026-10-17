// Package tokens is the sign-in context's view of the tokens context: app.PersonalTokens over its
// use cases, translated at the boundary.
package tokens

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	tokensapp "github.com/JorisJonkers-dev/identity/internal/tokens/app"
	tokensdomain "github.com/JorisJonkers-dev/identity/internal/tokens/domain"
)

// Tokens is what sign-in needs from the tokens context.
type Tokens interface {
	Resolve(ctx context.Context, secret string) (tokensdomain.Token, error)
	Check(ctx context.Context, tokenID uuid.UUID, clientID string) (tokensapp.Active, error)
}

// PersonalTokens is the app.PersonalTokens.
type PersonalTokens struct{ tokens Tokens }

// New returns PersonalTokens over tokens.
func New(tokens Tokens) *PersonalTokens { return &PersonalTokens{tokens: tokens} }

var _ app.PersonalTokens = (*PersonalTokens)(nil)

func (p *PersonalTokens) Resolve(ctx context.Context, secret string) (tokenID, userID uuid.UUID, err error) {
	t, err := p.tokens.Resolve(ctx, secret)
	if errors.Is(err, tokensapp.ErrNotFound) {
		return uuid.Nil, uuid.Nil, app.ErrNotFound
	}
	return t.ID, t.UserID, err
}

func (p *PersonalTokens) Check(ctx context.Context, tokenID uuid.UUID, clientID string) (app.PersonalToken, error) {
	a, err := p.tokens.Check(ctx, tokenID, clientID)
	if errors.Is(err, tokensapp.ErrInactive) {
		return app.PersonalToken{}, app.ErrInactive
	}
	if err != nil {
		return app.PersonalToken{}, err
	}
	t := a.Token
	return app.PersonalToken{ID: t.ID, UserID: t.UserID, ClientID: t.ClientID, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, Roles: a.Roles}, nil
}
