// Package oidc is the sign-in context's OIDC adapter: zitadel/oidc's Provider over the sign-in use
// cases (docs/adr/0001). identity writes no protocol code; it answers what the library's storage
// asks, and serves the one page the library leaves to it, the check for a Single sign-on session.
package oidc

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	connectedapp "github.com/JorisJonkers-dev/identity/internal/connected/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// storage is the op.Storage. Every failure the library may show a browser or a Client is an OIDC
// error with a generic description; the cause goes to the log.
type storage struct {
	svc      *app.Service
	machines *app.MachineSignIn
	apps     ConnectedApps
	ring     *Ring
	logger   *slog.Logger
	now      func() time.Time
}

var (
	_ op.Storage                   = (*storage)(nil)
	_ op.CanSetUserinfoFromRequest = (*storage)(nil)
)

// fail turns err into what a browser or a Client may see.
func (s *storage) fail(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, app.ErrNotFound):
		return oidc.ErrInvalidRequest().WithDescription("This sign-in has expired or was already used. Start again from the app.")
	case errors.Is(err, app.ErrInvalidClient):
		return oidc.ErrInvalidClient()
	default:
		s.logger.ErrorContext(ctx, "sign-in storage", "error", err)
		return oidc.ErrServerError()
	}
}

func (s *storage) CreateAuthRequest(ctx context.Context, req *oidc.AuthRequest, _ string) (op.AuthRequest, error) {
	r := domain.AuthRequest{
		ClientID: req.ClientID, RedirectURI: req.RedirectURI, Scopes: req.Scopes, State: req.State, Nonce: req.Nonce,
		ResponseType: string(req.ResponseType), ResponseMode: string(req.ResponseMode),
		CodeChallenge: req.CodeChallenge, CodeChallengeMethod: string(req.CodeChallengeMethod),
	}
	if isRegistration(req.ClientID) {
		resource, err := s.resourceOf(ctx)
		if err != nil {
			return nil, err
		}
		r.Resource = resource
	}
	started, err := s.svc.Start(ctx, r)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	return authRequest{started}, nil
}

func (s *storage) AuthRequestByID(ctx context.Context, id string) (op.AuthRequest, error) {
	rid, err := uuid.Parse(id)
	if err != nil {
		return nil, s.fail(ctx, app.ErrNotFound)
	}
	r, err := s.svc.Request(ctx, rid)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	return authRequest{r}, nil
}

func (s *storage) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	r, err := s.svc.Redeem(ctx, code)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	return authRequest{r}, nil
}

func (s *storage) SaveAuthCode(ctx context.Context, id, code string) error {
	rid, err := uuid.Parse(id)
	if err != nil {
		return s.fail(ctx, app.ErrNotFound)
	}
	if err := s.svc.SaveCode(ctx, rid, code); err != nil {
		return s.fail(ctx, err)
	}
	return nil
}

func (s *storage) DeleteAuthRequest(ctx context.Context, id string) error {
	rid, err := uuid.Parse(id)
	if err != nil {
		return s.fail(ctx, app.ErrNotFound)
	}
	if err := s.svc.Finish(ctx, rid); err != nil {
		return s.fail(ctx, err)
	}
	return nil
}

// CreateAccessToken names a JWT access token; Clients verify it against JWKS, so nothing is stored.
// A Connected App's is opaque, and kept.
func (s *storage) CreateAccessToken(ctx context.Context, req op.TokenRequest) (string, time.Time, error) {
	if connected(req) {
		id, _, expires, err := s.connectedTokens(ctx, req, false)
		return id, expires, err
	}
	return uuid.NewString(), s.now().Add(accessTokenLifetime), nil
}

var errNoRefreshTokens = oidc.ErrUnsupportedGrantType().WithDescription("Refresh tokens are not supported. Sign in again for a new token.")

// CreateAccessAndRefreshTokens issues refresh tokens to Connected Apps only (identity#23).
func (s *storage) CreateAccessAndRefreshTokens(ctx context.Context, req op.TokenRequest, _ string) (string, string, time.Time, error) {
	if connected(req) {
		return s.connectedTokens(ctx, req, true)
	}
	return "", "", time.Time{}, errNoRefreshTokens
}

// TokenRequestByRefreshToken finds a Connected App's refresh token without spending it: zitadel/oidc
// still checks the client and scopes, and the token is spent only as new tokens are made.
func (s *storage) TokenRequestByRefreshToken(ctx context.Context, token string) (op.RefreshTokenRequest, error) {
	t, err := s.apps.Peek(ctx, token)
	if errors.Is(err, connectedapp.ErrInvalidToken) {
		return nil, op.ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	return newRefreshRequest(t, token), nil
}

func (s *storage) GetRefreshTokenInfo(context.Context, string, string) (string, string, error) {
	return "", "", op.ErrInvalidRefreshToken
}

// TerminateSession signs the User out everywhere (docs/adr/0007).
func (s *storage) TerminateSession(ctx context.Context, userID, _ string) error {
	id, err := uuid.Parse(userID)
	if err != nil {
		return s.fail(ctx, app.ErrNotFound)
	}
	if err := s.svc.SignOutEverywhere(ctx, id); err != nil {
		return s.fail(ctx, err)
	}
	return nil
}

// RevokeToken refuses: a JWT access token lives its 15 minutes, and there are no refresh tokens.
func (s *storage) RevokeToken(context.Context, string, string, string) *oidc.Error {
	return oidc.ErrRequestNotSupported().WithDescription("Tokens cannot be revoked. An access token expires after 15 minutes.")
}

func (s *storage) SigningKey(context.Context) (op.SigningKey, error) { return s.ring.Signing(), nil }

func (s *storage) SignatureAlgorithms(context.Context) ([]jose.SignatureAlgorithm, error) {
	return []jose.SignatureAlgorithm{s.ring.Signing().SignatureAlgorithm()}, nil
}

func (s *storage) KeySet(context.Context) ([]op.Key, error) { return s.ring.Published(), nil }

func (s *storage) GetClientByClientID(ctx context.Context, id string) (op.Client, error) {
	if isRegistration(id) {
		return s.registration(ctx, id)
	}
	if m, ok, err := s.machine(ctx, id); ok || err != nil {
		if err != nil {
			return nil, s.fail(ctx, err)
		}
		return machineClient{m}, nil
	}
	c, err := s.svc.Client(ctx, id)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	return client{c}, nil
}

func (s *storage) AuthorizeClientIDSecret(ctx context.Context, id, secret string) error {
	if m, ok, err := s.machine(ctx, id); ok || err != nil {
		if err != nil {
			return s.fail(ctx, err)
		}
		return s.authorizeMachine(ctx, m, secret)
	}
	if err := s.svc.AuthorizeSecret(ctx, id, secret); err != nil {
		return s.fail(ctx, err)
	}
	return nil
}

// SetUserinfoFromScopes is deprecated in favour of SetUserinfoFromRequest, which zitadel/oidc calls
// right after it.
func (s *storage) SetUserinfoFromScopes(context.Context, *oidc.UserInfo, string, string, []string) error {
	return nil
}

// SetUserinfoFromRequest fills the ID token: email and name as the scopes ask, the User's App Roles
// on this Client, and the session's sid.
func (s *storage) SetUserinfoFromRequest(ctx context.Context, info *oidc.UserInfo, req op.IDTokenRequest, scopes []string) error {
	claims, err := s.claims(ctx, req.GetSubject())
	if err != nil {
		return err
	}
	roles, err := s.roles(ctx, req.GetSubject(), req.GetClientID())
	if err != nil {
		return err
	}
	info.AppendClaims("roles", roles)
	info.Subject = req.GetSubject()
	for _, scope := range scopes {
		switch scope {
		case oidc.ScopeEmail:
			info.Email, info.EmailVerified = claims.Email, true
		case oidc.ScopeProfile:
			info.Name = claims.Name
		}
	}
	// An authorization request reaching here is done: its subject resolved above.
	if r, ok := req.(authRequest); ok {
		info.AppendClaims("sid", r.SignIn.SessionID.String())
	}
	return nil
}

// SetUserinfoFromToken answers userinfo for a verified access token: the User's own email and name.
func (s *storage) SetUserinfoFromToken(ctx context.Context, info *oidc.UserInfo, tokenID, subject, _ string) error {
	// A Personal Token or a Connected App's token is checked by its Client at introspection, never
	// answered here (docs/adr/0017).
	if _, isApp := appTokenID(tokenID); isApp || app.IsPersonalToken(tokenID) {
		return s.fail(ctx, app.ErrInactive)
	}
	claims, err := s.claims(ctx, subject)
	if err != nil {
		return err
	}
	info.Subject = subject
	info.Email, info.EmailVerified, info.Name = claims.Email, true, claims.Name
	return nil
}

func (s *storage) SetIntrospectionFromToken(ctx context.Context, resp *oidc.IntrospectionResponse, tokenID, subject, clientID string) error {
	if app.IsPersonalToken(tokenID) {
		return s.introspectPersonalToken(ctx, resp, tokenID, clientID)
	}
	if id, ok := appTokenID(tokenID); ok {
		return s.introspectAppToken(ctx, resp, id, clientID)
	}
	info := new(oidc.UserInfo)
	if err := s.SetUserinfoFromToken(ctx, info, tokenID, subject, clientID); err != nil {
		return err
	}
	resp.SetUserInfo(info)
	return nil
}

// introspectPersonalToken answers a Client's check of a Personal Token: active only for that
// Client, while live, with the App Roles its User holds there now (docs/adr/0017).
func (s *storage) introspectPersonalToken(ctx context.Context, resp *oidc.IntrospectionResponse, tokenID, clientID string) error {
	t, err := s.svc.CheckPersonalToken(ctx, tokenID, clientID)
	if err != nil {
		return s.fail(ctx, err)
	}
	claims, err := s.claims(ctx, t.UserID.String())
	if err != nil {
		return err
	}
	resp.Subject, resp.ClientID, resp.Audience, resp.TokenType = t.UserID.String(), clientID, oidc.Audience{clientID}, "Bearer"
	resp.IssuedAt, resp.Expiration = oidc.FromTime(t.CreatedAt), oidc.FromTime(t.ExpiresAt)
	resp.Email, resp.EmailVerified, resp.Name = claims.Email, true, claims.Name
	resp.Claims = map[string]any{"roles": t.Roles}
	return nil
}

func (s *storage) claims(ctx context.Context, subject string) (app.Claims, error) {
	id, err := uuid.Parse(subject)
	if err != nil {
		return app.Claims{}, s.fail(ctx, app.ErrNotFound)
	}
	c, err := s.svc.Claims(ctx, id)
	if err != nil {
		return app.Claims{}, s.fail(ctx, err)
	}
	return c, nil
}

// GetPrivateClaimsFromScopes puts the User's App Roles on this Client in the access token, and
// nothing about Groups (docs/adr/0004).
func (s *storage) GetPrivateClaimsFromScopes(ctx context.Context, subject, clientID string, _ []string) (map[string]any, error) {
	roles, err := s.roles(ctx, subject, clientID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"roles": roles}, nil
}

func (s *storage) roles(ctx context.Context, subject, clientID string) ([]string, error) {
	id, err := uuid.Parse(subject)
	if err != nil {
		return nil, s.fail(ctx, app.ErrNotFound)
	}
	roles, err := s.svc.Roles(ctx, id, clientID)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	return roles, nil
}

var errNoJWTProfile = oidc.ErrUnauthorizedClient().WithDescription("The JWT profile grant is not supported.")

func (s *storage) GetKeyByIDAndClientID(context.Context, string, string) (*jose.JSONWebKey, error) {
	return nil, errNoJWTProfile
}

func (s *storage) ValidateJWTProfileScopes(context.Context, string, []string) ([]string, error) {
	return nil, errNoJWTProfile
}

// Health is answered by /readyz, which pings Postgres.
func (s *storage) Health(context.Context) error { return nil }
