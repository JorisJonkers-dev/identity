package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInactive is a Personal Token a Client's check refuses (docs/adr/0017).
var ErrInactive = errors.New("the Personal Token is not active")

// tokenIDPrefix marks a Personal Token's id where the Provider passes token ids around, so it is
// told apart from an access token's.
const tokenIDPrefix = "pat-"

// PersonalToken is a live Personal Token as a Client's check sees it.
type PersonalToken struct {
	ID, UserID           uuid.UUID
	ClientID             string
	CreatedAt, ExpiresAt time.Time
	Roles                []string
}

// PersonalTokens is the tokens context as sign-in needs it.
type PersonalTokens interface {
	// Resolve is the token a secret names, live or not; ErrNotFound when none.
	Resolve(ctx context.Context, secret string) (tokenID, userID uuid.UUID, err error)
	// Check is ErrInactive unless the token is the Client's, live, and its User holds an App Role
	// there now.
	Check(ctx context.Context, tokenID uuid.UUID, clientID string) (PersonalToken, error)
}

// SetPersonalTokens lets Clients check Personal Tokens; until it is called, none resolves. It is
// set once, before the Provider serves.
func (s *Service) SetPersonalTokens(p PersonalTokens) { s.tokens = p }

// ResolvePersonalToken turns a Personal Token's secret into the token id and subject the Provider
// carries to its check; false when it names none.
func (s *Service) ResolvePersonalToken(ctx context.Context, secret string) (tokenID, subject string, ok bool) {
	if s.tokens == nil {
		return "", "", false
	}
	id, userID, err := s.tokens.Resolve(ctx, secret)
	if err != nil {
		return "", "", false
	}
	return tokenIDPrefix + id.String(), userID.String(), true
}

// IsPersonalToken reports whether a token id the Provider carries is a Personal Token's.
func IsPersonalToken(tokenID string) bool { return strings.HasPrefix(tokenID, tokenIDPrefix) }

// CheckPersonalToken is a Client's check of a Personal Token by the id ResolvePersonalToken gave.
func (s *Service) CheckPersonalToken(ctx context.Context, tokenID, clientID string) (PersonalToken, error) {
	id, err := uuid.Parse(strings.TrimPrefix(tokenID, tokenIDPrefix))
	if s.tokens == nil || !IsPersonalToken(tokenID) || err != nil {
		return PersonalToken{}, ErrInactive
	}
	return s.tokens.Check(ctx, id, clientID)
}
