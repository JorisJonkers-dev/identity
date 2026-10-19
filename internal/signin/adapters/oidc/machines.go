package oidc

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

// A Machine Identity signs in at the token endpoint with its id as client_id (identity#22): inside
// the cluster by exchanging its ServiceAccount token (RFC 8693), outside it with the client
// credentials grant and a secret. Either way it names the one Client it wants a token for in
// `audience`, and the token carries its own App Roles there.

var (
	_ op.TokenExchangeStorage               = (*storage)(nil)
	_ op.TokenExchangeTokensVerifierStorage = (*storage)(nil)
	_ op.ClientCredentialsStorage           = (*storage)(nil)
	_ op.CanGetPrivateClaimsFromRequest     = (*storage)(nil)
)

// tokenForm is what grants needs from a token request's form, read before zitadel/oidc reads it.
type tokenForm struct {
	grant    string
	audience string
}

type tokenFormKey struct{}

// grants reads the grant type and audience of a token request into its context: a Machine
// Identity exchanging a ServiceAccount token has no secret to send, and the client credentials
// grant has no parameter of its own for the Client the token is for.
func grants(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			next.ServeHTTP(w, r)
			return
		}
		f := tokenForm{grant: r.PostForm.Get("grant_type"), audience: r.PostForm.Get("audience")}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tokenFormKey{}, f)))
	})
}

func formOf(ctx context.Context) tokenForm {
	f, _ := ctx.Value(tokenFormKey{}).(tokenForm)
	return f
}

// machineClient is a Machine Identity as zitadel/oidc reads a client: no browser, only the token
// exchange and client credentials grants, and JWT access tokens.
type machineClient struct{ app.MachineIdentity }

var _ op.Client = machineClient{}

func (m machineClient) GetID() string                  { return m.ID.String() }
func (machineClient) RedirectURIs() []string           { return nil }
func (machineClient) PostLogoutRedirectURIs() []string { return nil }
func (machineClient) ApplicationType() op.ApplicationType {
	return op.ApplicationTypeWeb
}
func (machineClient) AuthMethod() oidc.AuthMethod        { return oidc.AuthMethodBasic }
func (machineClient) ResponseTypes() []oidc.ResponseType { return nil }
func (machineClient) GrantTypes() []oidc.GrantType {
	return []oidc.GrantType{oidc.GrantTypeTokenExchange, oidc.GrantTypeClientCredentials}
}
func (machineClient) LoginURL(string) string               { return "" }
func (machineClient) AccessTokenType() op.AccessTokenType  { return op.AccessTokenTypeJWT }
func (machineClient) IDTokenLifetime() time.Duration       { return 0 }
func (machineClient) DevMode() bool                        { return false }
func (machineClient) IsScopeAllowed(string) bool           { return false }
func (machineClient) IDTokenUserinfoClaimsAssertion() bool { return false }
func (machineClient) ClockSkew() time.Duration             { return 0 }
func (machineClient) RestrictAdditionalIdTokenScopes() func([]string) []string { //nolint:revive // zitadel/oidc's name
	return none
}
func (machineClient) RestrictAdditionalAccessTokenScopes() func([]string) []string { return none }

func none([]string) []string { return nil }

// machine is the Machine Identity whose id is clientID, or false when it names none.
func (s *storage) machine(ctx context.Context, clientID string) (app.MachineIdentity, bool, error) {
	id, isID := machineID(clientID)
	if !isID {
		return app.MachineIdentity{}, false, nil
	}
	m, err := s.machines.Machine(ctx, id)
	if errors.Is(err, app.ErrUnknownMachine) {
		return app.MachineIdentity{}, false, nil
	}
	return m, err == nil, err
}

// machineID is clientID as a Machine Identity's id; a Client's id is a name, never a UUID.
func machineID(clientID string) (uuid.UUID, bool) {
	id, err := uuid.Parse(clientID)
	return id, err == nil
}

// authorizeMachine checks a Machine Identity at the token endpoint. Exchanging a ServiceAccount
// token needs no secret: ValidateTokenExchangeRequest holds the token to this Machine Identity.
func (s *storage) authorizeMachine(ctx context.Context, m app.MachineIdentity, secret string) error {
	if secret == "" && formOf(ctx).grant == string(oidc.GrantTypeTokenExchange) {
		return nil
	}
	if err := s.machines.Authorize(ctx, m.ID, secret); err != nil {
		return s.fail(ctx, err)
	}
	return nil
}

var (
	errSubjectToken    = oidc.ErrInvalidRequest().WithDescription("subject_token must be a ServiceAccount token, of type " + string(oidc.JWTTokenType) + ".")
	errNoActor         = oidc.ErrInvalidRequest().WithDescription("actor_token is not supported.")
	errOneAudience     = oidc.ErrInvalidTarget().WithDescription("Name one Client in audience.")
	errNoMachineAccess = oidc.ErrAccessDenied().WithDescription("The Machine Identity holds no App Role on that Client.")
)

// VerifyExchangeSubjectToken answers the Machine Identity a ServiceAccount token belongs to as the
// subject; zitadel/oidc asks it for any token it did not issue itself.
func (s *storage) VerifyExchangeSubjectToken(ctx context.Context, token string, tokenType oidc.TokenType) (string, string, map[string]any, error) {
	if tokenType != oidc.JWTTokenType {
		return "", "", nil, errSubjectToken
	}
	m, err := s.machines.Exchange(ctx, token)
	if err != nil {
		return "", "", nil, err
	}
	return token, m.ID.String(), nil, nil
}

func (s *storage) VerifyExchangeActorToken(context.Context, string, oidc.TokenType) (string, string, map[string]any, error) {
	return "", "", nil, errNoActor
}

// ValidateTokenExchangeRequest lets a Machine Identity exchange only its own ServiceAccount token,
// and only for an access token; which Client it is for is checked as the token is made.
func (s *storage) ValidateTokenExchangeRequest(_ context.Context, req op.TokenExchangeRequest) error {
	if req.GetExchangeSubjectTokenType() != oidc.JWTTokenType || req.GetExchangeSubject() != req.GetClientID() {
		return errSubjectToken
	}
	if t := req.GetRequestedTokenType(); t != "" && t != oidc.AccessTokenType {
		return errNoIDToken
	}
	req.SetRequestedTokenType(oidc.AccessTokenType)
	req.SetCurrentScopes(nil)
	return nil
}

// CreateTokenExchangeRequest keeps nothing: the access token is a JWT a Client verifies itself.
func (s *storage) CreateTokenExchangeRequest(context.Context, op.TokenExchangeRequest) error {
	return nil
}

func (s *storage) GetPrivateClaimsFromTokenExchangeRequest(ctx context.Context, req op.TokenExchangeRequest) (map[string]any, error) {
	return s.machineClaims(ctx, req.GetSubject(), req.GetAudience())
}

var errNoIDToken = oidc.ErrInvalidRequest().WithDescription("A Machine Identity gets an access token only.")

func (s *storage) SetUserinfoFromTokenExchangeRequest(context.Context, *oidc.UserInfo, op.TokenExchangeRequest) error {
	return errNoIDToken
}

// machineClaims is the App Roles a Machine Identity holds on the one Client audience names. The
// subject is a Machine Identity's id: VerifyExchangeSubjectToken or ClientCredentials set it.
func (s *storage) machineClaims(ctx context.Context, subject string, audience []string) (map[string]any, error) {
	if len(audience) != 1 {
		return nil, errOneAudience
	}
	roles, err := s.machines.Roles(ctx, uuid.MustParse(subject), audience[0])
	switch {
	case errors.Is(err, app.ErrNoAccess):
		return nil, errNoMachineAccess
	case errors.Is(err, app.ErrNotFound):
		return nil, errOneAudience
	case err != nil:
		return nil, s.fail(ctx, err)
	}
	return map[string]any{"roles": roles}, nil
}

// ClientCredentials signs a Machine Identity in with its client secret, outside the cluster.
func (s *storage) ClientCredentials(ctx context.Context, clientID, secret string) (op.Client, error) {
	m, ok, err := s.machine(ctx, clientID)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	if !ok {
		return nil, oidc.ErrInvalidClient()
	}
	if err := s.machines.Authorize(ctx, m.ID, secret); err != nil {
		return nil, s.fail(ctx, err)
	}
	return machineClient{m}, nil
}

// machineRequest is a client credentials request: the Machine Identity, for one Client.
type machineRequest struct {
	subject  string
	audience []string
}

func (r machineRequest) GetSubject() string    { return r.subject }
func (r machineRequest) GetAudience() []string { return r.audience }
func (machineRequest) GetScopes() []string     { return nil }

// ClientCredentialsTokenRequest is for the Client the request's audience names; clientID is a
// Machine Identity's, as ClientCredentials let only those through.
func (s *storage) ClientCredentialsTokenRequest(ctx context.Context, clientID string, _ []string) (op.TokenRequest, error) {
	audience := []string{}
	if a := formOf(ctx).audience; a != "" {
		audience = append(audience, a)
	}
	return machineRequest{subject: clientID, audience: audience}, nil
}

// GetPrivateClaimsFromRequest puts the App Roles in an access token: a Machine Identity's on the
// Client its request names, or a User's on the Client they signed in to.
func (s *storage) GetPrivateClaimsFromRequest(ctx context.Context, req op.TokenRequest, scopes []string) (map[string]any, error) {
	if m, ok := req.(machineRequest); ok {
		return s.machineClaims(ctx, m.subject, m.audience)
	}
	clientID := ""
	if r, ok := req.(interface{ GetClientID() string }); ok {
		clientID = r.GetClientID()
	}
	return s.GetPrivateClaimsFromScopes(ctx, req.GetSubject(), clientID, scopes)
}
