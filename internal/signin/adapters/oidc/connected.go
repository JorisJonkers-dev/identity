package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	connectedapp "github.com/JorisJonkers-dev/identity/internal/connected/app"
	connecteddomain "github.com/JorisJonkers-dev/identity/internal/connected/domain"
)

// A tool such as an MCP client registers itself (RFC 7591), and a User consents to it acting for
// them on one Client, the MCP resource its requests name (RFC 8707). Its access tokens are opaque
// and checked against the database; its refresh tokens rotate on every use (identity#23).

// ConnectedApps is the connected context as the Provider needs it.
type ConnectedApps interface {
	Register(ctx context.Context, name string, redirectURIs []string) (connecteddomain.Registration, error)
	Registration(ctx context.Context, clientID string) (connecteddomain.Registration, error)
	Resource(ctx context.Context, resource string) (connectedapp.Client, error)
	Consented(ctx context.Context, userID uuid.UUID, registrationID, clientID string) (uuid.UUID, error)
	IssueAccess(ctx context.Context, appID uuid.UUID) (uuid.UUID, time.Time, error)
	IssueRefresh(ctx context.Context, appID uuid.UUID) (string, error)
	Peek(ctx context.Context, secret string) (connectedapp.RefreshToken, error)
	Refresh(ctx context.Context, secret string) (connectedapp.RefreshToken, error)
	Check(ctx context.Context, tokenID uuid.UUID, clientID string) (connectedapp.AccessToken, []string, error)
}

const (
	registerPath = "/oidc/register"
	// appTokenIDPrefix marks a Connected App's access token id, as the Provider passes token ids
	// around; appTokenPrefix marks the sealed token itself, so the Gate knows it at a glance.
	appTokenIDPrefix = "app-"
	appTokenPrefix   = "idapp_"
	// maxRegistration bounds a registration's body.
	maxRegistration = 64 << 10
)

// isRegistration reports whether a client id is a registered tool's.
func isRegistration(clientID string) bool {
	return strings.HasPrefix(clientID, connectedapp.RegistrationPrefix)
}

// registeredClient is a registered tool as zitadel/oidc reads a client: public, so PKCE, on the
// User's own machine or at an https address, with opaque access tokens and refresh tokens. Its
// ID token says nothing about the User; the tool acts for them, it does not sign them in.
type registeredClient struct{ connecteddomain.Registration }

var _ op.Client = registeredClient{}

func (c registeredClient) GetID() string                  { return c.ClientID }
func (c registeredClient) RedirectURIs() []string         { return c.Registration.RedirectURIs }
func (registeredClient) PostLogoutRedirectURIs() []string { return nil }
func (registeredClient) ApplicationType() op.ApplicationType {
	return op.ApplicationTypeNative
}
func (registeredClient) AuthMethod() oidc.AuthMethod { return oidc.AuthMethodNone }
func (registeredClient) ResponseTypes() []oidc.ResponseType {
	return []oidc.ResponseType{oidc.ResponseTypeCode}
}

func (registeredClient) GrantTypes() []oidc.GrantType {
	return []oidc.GrantType{oidc.GrantTypeCode, oidc.GrantTypeRefreshToken}
}

func (registeredClient) LoginURL(id string) string            { return loginPath + "?authRequestID=" + id }
func (registeredClient) AccessTokenType() op.AccessTokenType  { return op.AccessTokenTypeBearer }
func (registeredClient) IDTokenLifetime() time.Duration       { return idTokenLifetime }
func (registeredClient) DevMode() bool                        { return false }
func (registeredClient) IsScopeAllowed(string) bool           { return false }
func (registeredClient) IDTokenUserinfoClaimsAssertion() bool { return false }
func (registeredClient) ClockSkew() time.Duration             { return 0 }
func (registeredClient) RestrictAdditionalIdTokenScopes() func([]string) []string { //nolint:revive // zitadel/oidc's name
	return none
}

func (registeredClient) RestrictAdditionalAccessTokenScopes() func([]string) []string { return none }

type resourceKey struct{}

// resources reads a registered tool's `resource` into the authorize request's context, and asks
// for `offline_access` whatever scope it sent, so it gets a refresh token and no claims.
func resources(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || !isRegistration(r.Form.Get("client_id")) {
			next.ServeHTTP(w, r)
			return
		}
		r.Form.Set("scope", oidc.ScopeOfflineAccess)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), resourceKey{}, r.Form.Get("resource"))))
	})
}

// errInactive is a token introspection answers as inactive, which is no failure to log.
var errInactive = oidc.ErrInvalidRequest().WithDescription("The token is not active.")

var errNotResource = oidc.ErrInvalidTarget().WithDescription("This tool cannot connect to that app.")

// resourceOf is the Client a registered tool's authorize request names as its resource.
func (s *storage) resourceOf(ctx context.Context) (string, error) {
	resource, _ := ctx.Value(resourceKey{}).(string)
	c, err := s.apps.Resource(ctx, resource)
	if errors.Is(err, connectedapp.ErrNotMCPResource) {
		return "", errNotResource
	}
	if err != nil {
		return "", s.fail(ctx, err)
	}
	return c.ID, nil
}

func (s *storage) registration(ctx context.Context, clientID string) (op.Client, error) {
	r, err := s.apps.Registration(ctx, clientID)
	if errors.Is(err, connectedapp.ErrNotFound) {
		return nil, oidc.ErrInvalidClient()
	}
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	return registeredClient{r}, nil
}

// appSealer seals a Connected App's access token id behind its own prefix, so the Gate and secret
// scanners know the token at a glance, and opens only such a token.
type appSealer struct{ sealed op.Crypto }

var errNotAppToken = errors.New("not a Connected App's token")

func (a appSealer) Encrypt(plain string) (string, error) {
	sealed, err := a.sealed.Encrypt(plain)
	if err != nil || !strings.HasPrefix(plain, appTokenIDPrefix) {
		return sealed, err
	}
	return appTokenPrefix + sealed, nil
}

func (a appSealer) Decrypt(token string) (string, error) {
	rest, ok := strings.CutPrefix(token, appTokenPrefix)
	if !ok {
		return "", errNotAppToken
	}
	plain, err := a.sealed.Decrypt(rest)
	if err != nil || !strings.HasPrefix(plain, appTokenIDPrefix) {
		return "", errNotAppToken
	}
	return plain, nil
}

// connectedTokens issues a Connected App an access token, and a refresh token when asked: after
// the User's consent completed its request, or for the refresh token it spent.
func (s *storage) connectedTokens(ctx context.Context, req op.TokenRequest, withRefresh bool) (string, string, time.Time, error) {
	appID, err := s.appOf(ctx, req)
	if err != nil {
		return "", "", time.Time{}, err
	}
	id, expires, err := s.apps.IssueAccess(ctx, appID)
	if err != nil {
		return "", "", time.Time{}, s.fail(ctx, err)
	}
	var refresh string
	if withRefresh {
		if refresh, err = s.apps.IssueRefresh(ctx, appID); err != nil {
			return "", "", time.Time{}, s.fail(ctx, err)
		}
	}
	return appTokenIDPrefix + id.String(), refresh, expires, nil
}

var errConsentGone = oidc.ErrInvalidGrant().WithDescription("The app was disconnected. Connect it again.")

func (s *storage) appOf(ctx context.Context, req op.TokenRequest) (uuid.UUID, error) {
	if r, ok := req.(*refreshRequest); ok {
		return s.spend(ctx, r)
	}
	r, _ := req.(authRequest)
	appID, err := s.apps.Consented(ctx, uuid.MustParse(r.GetSubject()), r.ClientID, r.Resource)
	if err != nil {
		s.logger.InfoContext(ctx, "a Connected App's code outlived its consent", "error", err)
		return uuid.Nil, errConsentGone
	}
	return appID, nil
}

// spend spends a refresh token once zitadel/oidc accepted the request it came with; spending it twice
// disconnects its Connected App.
func (s *storage) spend(ctx context.Context, r *refreshRequest) (uuid.UUID, error) {
	t, err := s.apps.Refresh(ctx, r.secret)
	if errors.Is(err, connectedapp.ErrInvalidToken) {
		return uuid.Nil, oidc.ErrInvalidGrant()
	}
	if err != nil {
		return uuid.Nil, s.fail(ctx, err)
	}
	return t.AppID, nil
}

// connected reports whether a token request is a Connected App's.
func connected(req op.TokenRequest) bool {
	switch r := req.(type) {
	case *refreshRequest:
		return true
	case authRequest:
		return r.Resource != ""
	}
	return false
}

// refreshRequest is a spent refresh token as zitadel/oidc reads it, for the Connected App's next
// tokens.
type refreshRequest struct {
	connectedapp.RefreshToken
	secret string
	scopes []string
}

var _ op.RefreshTokenRequest = (*refreshRequest)(nil)

func newRefreshRequest(t connectedapp.RefreshToken, secret string) *refreshRequest {
	return &refreshRequest{RefreshToken: t, secret: secret, scopes: []string{oidc.ScopeOfflineAccess}}
}

func (*refreshRequest) GetAMR() []string                   { return nil }
func (r *refreshRequest) GetAudience() []string            { return []string{r.RegistrationID} }
func (*refreshRequest) GetAuthTime() time.Time             { return time.Time{} }
func (r *refreshRequest) GetClientID() string              { return r.RegistrationID }
func (r *refreshRequest) GetScopes() []string              { return r.scopes }
func (r *refreshRequest) GetSubject() string               { return r.UserID.String() }
func (r *refreshRequest) SetCurrentScopes(scopes []string) { r.scopes = scopes }

// appTokenID is a Connected App's access token id, from the id the Provider carries.
func appTokenID(tokenID string) (uuid.UUID, bool) {
	rest, ok := strings.CutPrefix(tokenID, appTokenIDPrefix)
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(rest)
	return id, err == nil
}

// introspectAppToken answers a Client's check of a Connected App's access token: active only for
// that Client, while live, with the App Roles its User holds there now.
func (s *storage) introspectAppToken(ctx context.Context, resp *oidc.IntrospectionResponse, id uuid.UUID, clientID string) error {
	t, roles, err := s.apps.Check(ctx, id, clientID)
	if errors.Is(err, connectedapp.ErrInvalidToken) {
		return errInactive
	}
	if err != nil {
		return s.fail(ctx, err)
	}
	claims, err := s.claims(ctx, t.UserID.String())
	if err != nil {
		return err
	}
	resp.Subject, resp.ClientID, resp.Audience, resp.TokenType = t.UserID.String(), clientID, oidc.Audience{clientID}, "Bearer"
	resp.Expiration = oidc.FromTime(t.ExpiresAt)
	resp.Email, resp.EmailVerified, resp.Name = claims.Email, true, claims.Name
	resp.Claims = map[string]any{"roles": roles}
	return nil
}

// registration is what a tool sends to register itself (RFC 7591 §2), of which identity keeps the
// name and redirect URIs; it refuses anything a public code-flow client does not ask for.
type registrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

type registrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

type registrationError struct {
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

// grantTypes and responseTypes are what a registered tool may ask for: the code flow and refresh.
func grantTypes() []string {
	return []string{string(oidc.GrantTypeCode), string(oidc.GrantTypeRefreshToken)}
}

func responseTypes() []string { return []string{string(oidc.ResponseTypeCode)} }

// register is the registration endpoint. Anyone may register; a registration holds nothing until a
// signed-in User consents to it, and one nobody consents to is gone after a day.
func register(apps ConnectedApps, s *storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registrationRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRegistration)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, registrationError{"invalid_client_metadata", "The registration is not valid JSON."})
			return
		}
		if !publicCodeFlow(req) {
			writeJSON(w, http.StatusBadRequest, registrationError{"invalid_client_metadata", "Only public clients using the authorization code flow with PKCE can register."})
			return
		}
		reg, err := apps.Register(r.Context(), req.ClientName, req.RedirectURIs)
		if errors.Is(err, connecteddomain.ErrInvalidRegistration) {
			writeJSON(w, http.StatusBadRequest, registrationError{"invalid_redirect_uri", err.Error()})
			return
		}
		if err != nil {
			s.logger.ErrorContext(r.Context(), "registering a client", "error", err)
			writeJSON(w, http.StatusInternalServerError, registrationError{"server_error", "The registration failed. Try again."})
			return
		}
		writeJSON(w, http.StatusCreated, registrationResponse{
			ClientID: reg.ClientID, ClientIDIssuedAt: reg.CreatedAt.Unix(), ClientName: reg.Name, RedirectURIs: reg.RedirectURIs,
			TokenEndpointAuthMethod: string(oidc.AuthMethodNone), GrantTypes: grantTypes(), ResponseTypes: responseTypes(),
		})
	}
}

func publicCodeFlow(req registrationRequest) bool {
	if req.TokenEndpointAuthMethod != "" && req.TokenEndpointAuthMethod != string(oidc.AuthMethodNone) {
		return false
	}
	for _, g := range req.GrantTypes {
		if !slices.Contains(grantTypes(), g) {
			return false
		}
	}
	for _, t := range req.ResponseTypes {
		if !slices.Contains(responseTypes(), t) {
			return false
		}
	}
	return true
}

// metadata is the authorization server's metadata (RFC 8414): the discovery document, with the
// registration endpoint a tool registers at.
func metadata(provider *op.Provider, s *storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := op.CreateDiscoveryConfig(r.Context(), provider, s)
		cfg.RegistrationEndpoint = op.IssuerFromContext(r.Context()) + registerPath
		w.Header().Set("Access-Control-Allow-Origin", "*")
		op.Discover(w, cfg)
	}
}

// protectedResource is the metadata of an MCP resource (RFC 9728), which the Gate's challenge
// points a tool to: which authorization server issues its tokens.
type protectedResource struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ResourceName           string   `json:"resource_name,omitempty"`
}

func resourceMetadata(apps ConnectedApps, s *storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resource := "https://" + r.PathValue("host") + "/"
		c, err := apps.Resource(r.Context(), resource)
		if errors.Is(err, connectedapp.ErrNotMCPResource) {
			writeJSON(w, http.StatusNotFound, registrationError{"not_found", "No service takes connected apps at this host."})
			return
		}
		if err != nil {
			s.logger.ErrorContext(r.Context(), "protected resource metadata", "error", err)
			writeJSON(w, http.StatusInternalServerError, registrationError{"server_error", ""})
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		writeJSON(w, http.StatusOK, protectedResource{
			Resource: resource, AuthorizationServers: []string{op.IssuerFromContext(r.Context())},
			BearerMethodsSupported: []string{"header"}, ResourceName: c.Label,
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// AppTokens opens a Connected App's access token for the Gate, which takes it as a bearer token
// on its MCP resource's host.
type AppTokens struct {
	sealer appSealer
	apps   ConnectedApps
}

// NewAppTokens returns AppTokens opening what the Provider sealed with key.
func NewAppTokens(key [32]byte, apps ConnectedApps) *AppTokens {
	return &AppTokens{sealer: appSealer{sealed: op.NewAES256GCMCrypto(key, "")}, apps: apps}
}

// Check is the connected context's check of the token for a Client; ErrInvalidToken for a token
// that does not open as a Connected App's.
func (a *AppTokens) Check(ctx context.Context, token, clientID string) (connectedapp.AccessToken, []string, error) {
	plain, err := a.sealer.Decrypt(token)
	if err != nil {
		return connectedapp.AccessToken{}, nil, connectedapp.ErrInvalidToken
	}
	tokenID, _, _ := strings.Cut(plain, ":")
	id, ok := appTokenID(tokenID)
	if !ok {
		return connectedapp.AccessToken{}, nil, connectedapp.ErrInvalidToken
	}
	return a.apps.Check(ctx, id, clientID)
}
