package oidc

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

var robot = app.MachineIdentity{ID: uuid.MustParse("00000000-0000-4000-8000-0000000000c1"), Name: "Backup", Namespace: "media", ServiceAccount: "backup"}

// machines is the access context holding robot, with viewer on web; fail breaks reading Machine
// Identities, failRoles reading their roles.
type machines struct{ fail, failRoles error }

func (m machines) Machine(_ context.Context, id uuid.UUID) (app.MachineIdentity, error) {
	if m.fail != nil {
		return app.MachineIdentity{}, m.fail
	}
	if id != robot.ID {
		return app.MachineIdentity{}, app.ErrNotFound
	}
	return robot, nil
}

func (m machines) MachineByBinding(_ context.Context, namespace, name string) (app.MachineIdentity, error) {
	if namespace == "media" && name == "other" {
		return app.MachineIdentity{ID: uuid.New(), Namespace: namespace, ServiceAccount: name}, nil
	}
	if namespace != robot.Namespace || name != robot.ServiceAccount {
		return app.MachineIdentity{}, app.ErrNotFound
	}
	return robot, nil
}

func (m machines) MachineRoles(_ context.Context, _ uuid.UUID, clientID string) ([]string, error) {
	if clientID == "web" {
		return []string{"viewer"}, m.failRoles
	}
	return nil, m.failRoles
}

// clusterTokens verifies "sa:<namespace>:<name>" as the cluster's ServiceAccount tokens.
type clusterTokens struct{}

func (clusterTokens) Verify(_ context.Context, token string) (string, string, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 3 || parts[0] != "sa" {
		return "", "", app.ErrInvalidServiceAccountToken
	}
	return parts[1], parts[2], nil
}

// machineHashes holds robot's secret, "robot-secret".
type machineHashes struct{}

func (machineHashes) Provision(context.Context, string, func() ([]byte, error)) error { return nil }
func (machineHashes) Rotate(context.Context, string, func() ([]byte, error)) error    { return nil }
func (machineHashes) EndPrevious(context.Context, string, []byte) error               { return nil }

func (machineHashes) Hashes(_ context.Context, id string) ([]byte, []byte, error) {
	if id != robot.ID.String() {
		return nil, nil, app.ErrNotFound
	}
	h := sha256.Sum256([]byte("robot-secret"))
	return h[:], nil, nil
}

// serveMachines runs a Provider whose Machine Identities are robot, with m failing as set.
func serveMachines(t *testing.T, m machines) string {
	t.Helper()
	var h http.Handler
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }))
	t.Cleanup(srv.Close)
	svc := app.New(newRequests(), testClients, &accounts{}, newAccess(), noLogouts{}, discard{}, time.Now, rand.Reader)
	svc.SetClientSecrets(testSecrets)
	signIn := app.NewMachineSignIn(m, clusterTokens{}, machineHashes{}, testClients, nil, discard{}, rand.Reader)
	var err error
	h, err = New(Config{Issuer: srv.URL, Insecure: true, SessionCookie: cookieName}, svc, signIn, newApps(), NewRing(testKey()), slog.New(slog.DiscardHandler), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return srv.URL
}

// tokenAnswer is the token endpoint's answer, with the access token's claims when it gave one.
type tokenAnswer struct {
	status int
	body   map[string]any
	claims map[string]any
}

func postToken(t *testing.T, issuer string, form url.Values, clientID, secret string) tokenAnswer {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, issuer+"/oidc/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if clientID != "" {
		req.SetBasicAuth(clientID, secret)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	a := tokenAnswer{status: res.StatusCode}
	_ = json.Unmarshal(raw, &a.body)
	if token, ok := a.body["access_token"].(string); ok {
		payload, _ := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[1])
		_ = json.Unmarshal(payload, &a.claims)
	}
	return a
}

func exchangeForm(subject, audience string) url.Values {
	return url.Values{
		"grant_type": {string(oidc.GrantTypeTokenExchange)}, "subject_token": {subject},
		"subject_token_type": {string(oidc.JWTTokenType)}, "audience": {audience},
	}
}

func wantRoles(t *testing.T, a tokenAnswer) {
	t.Helper()
	roles, _ := a.claims["roles"].([]any)
	if a.status != http.StatusOK || a.claims["sub"] != robot.ID.String() || a.claims["aud"] == nil || len(roles) != 1 || roles[0] != "viewer" {
		t.Fatalf("token = %d %v, claims %v", a.status, a.body, a.claims)
	}
}

func TestAMachineIdentityExchangesItsServiceAccountTokenForItsRolesOnOneClient(t *testing.T) {
	a := postToken(t, serveMachines(t, machines{}), exchangeForm("sa:media:backup", "web"), robot.ID.String(), "")
	wantRoles(t, a)
}

func TestAMachineIdentitySignsInWithItsSecretForItsRolesOnOneClient(t *testing.T) {
	form := url.Values{"grant_type": {string(oidc.GrantTypeClientCredentials)}, "audience": {"web"}}
	wantRoles(t, postToken(t, serveMachines(t, machines{}), form, robot.ID.String(), "robot-secret"))
}

func TestATokenExchangeIsRefused(t *testing.T) {
	for _, c := range []struct {
		name      string
		form      url.Values
		clientID  string
		wantError string
	}{
		{"for another Machine Identity's ServiceAccount", exchangeForm("sa:media:other", "web"), robot.ID.String(), "invalid_request"},
		{"with a token the cluster did not issue", exchangeForm("forged", "web"), robot.ID.String(), "invalid_request"},
		{"for a Client it holds no role on", exchangeForm("sa:media:backup", "spa"), robot.ID.String(), "access_denied"},
		{"for a Client not in the cluster", exchangeForm("sa:media:backup", "wiki"), robot.ID.String(), "invalid_target"},
		{"naming no Client", exchangeForm("sa:media:backup", ""), robot.ID.String(), "invalid_target"},
		{"naming two Clients", func() url.Values {
			f := exchangeForm("sa:media:backup", "web")
			f.Add("audience", "spa")
			return f
		}(), robot.ID.String(), "invalid_target"},
		{"for an ID token", func() url.Values {
			f := exchangeForm("sa:media:backup", "web")
			f.Set("requested_token_type", string(oidc.IDTokenType))
			return f
		}(), robot.ID.String(), "invalid_request"},
		{"with an actor", func() url.Values {
			f := exchangeForm("sa:media:backup", "web")
			f.Set("actor_token", "sa:media:backup")
			f.Set("actor_token_type", string(oidc.JWTTokenType))
			return f
		}(), robot.ID.String(), "invalid_request"},
		{"of an access token", func() url.Values {
			f := exchangeForm("sa:media:backup", "web")
			f.Set("subject_token_type", string(oidc.AccessTokenType))
			return f
		}(), robot.ID.String(), "invalid_request"},
		{"from a client that is no Machine Identity, without its secret", exchangeForm("sa:media:backup", "web"), "web", "invalid_client"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := postToken(t, serveMachines(t, machines{}), c.form, c.clientID, "")
			if a.status == http.StatusOK || a.body["error"] != c.wantError {
				t.Fatalf("token = %d %v", a.status, a.body)
			}
		})
	}
}

func TestAClientCredentialsSignInIsRefused(t *testing.T) {
	for _, c := range []struct {
		name, clientID, secret, audience, wantError string
	}{
		{"with a wrong secret", robot.ID.String(), "wrong", "web", "invalid_client"},
		{"for no Machine Identity", uuid.NewString(), "robot-secret", "web", "invalid_client"},
		{"for a Client that is no Machine Identity", "web", "s3cret", "web", "invalid_client"},
		{"naming no Client", robot.ID.String(), "robot-secret", "", "invalid_target"},
	} {
		t.Run(c.name, func(t *testing.T) {
			form := url.Values{"grant_type": {string(oidc.GrantTypeClientCredentials)}, "audience": {c.audience}}
			a := postToken(t, serveMachines(t, machines{}), form, c.clientID, c.secret)
			if a.status == http.StatusOK || a.body["error"] != c.wantError {
				t.Fatalf("token = %d %v", a.status, a.body)
			}
		})
	}
}

func TestAMachineSignInFailsWhenItsRolesCannotBeRead(t *testing.T) {
	issuer := serveMachines(t, machines{failRoles: errors.New("boom")})
	if a := postToken(t, issuer, exchangeForm("sa:media:backup", "web"), robot.ID.String(), ""); a.status == http.StatusOK || a.body["error"] != "server_error" {
		t.Fatalf("token = %d %v", a.status, a.body)
	}
}

// introspect is the status /oidc/introspect answers a client authenticating with secret.
func introspect(t *testing.T, issuer, clientID, secret string) int {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, issuer+"/oidc/introspect", strings.NewReader("token=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, secret)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	return res.StatusCode
}

func TestAMachineIdentityAuthenticatesAnywhereElseOnlyWithItsSecret(t *testing.T) {
	issuer := serveMachines(t, machines{})
	if got := introspect(t, issuer, robot.ID.String(), "robot-secret"); got != http.StatusOK {
		t.Fatalf("with its secret = %d", got)
	}
	if got := introspect(t, issuer, robot.ID.String(), ""); got != http.StatusUnauthorized {
		t.Fatalf("without a secret = %d", got)
	}
}

func TestAClientLookupFailsWhenMachineIdentitiesCannotBeRead(t *testing.T) {
	s := &storage{logger: slog.New(slog.DiscardHandler), machines: app.NewMachineSignIn(machines{fail: errors.New("boom")}, clusterTokens{}, machineHashes{}, testClients, nil, discard{}, rand.Reader)}
	if _, err := s.GetClientByClientID(context.Background(), robot.ID.String()); err == nil {
		t.Fatal("a client was found")
	}
}

// bareRequest is a token request naming no Client, which no grant identity serves makes.
type bareRequest struct{}

func (bareRequest) GetSubject() string    { return uuid.NewString() }
func (bareRequest) GetAudience() []string { return nil }
func (bareRequest) GetScopes() []string   { return nil }

func TestATokenRequestNamingNoClientGetsNoRoles(t *testing.T) {
	s, _, _ := newStorage()
	if _, err := s.GetPrivateClaimsFromRequest(context.Background(), bareRequest{}, nil); err == nil {
		t.Fatal("roles for no Client")
	}
}

func TestAMachineSignInFailsWhenMachineIdentitiesCannotBeRead(t *testing.T) {
	issuer := serveMachines(t, machines{fail: errors.New("boom")})
	for _, c := range []struct {
		name   string
		form   url.Values
		secret string
	}{
		{"exchanging", exchangeForm("sa:media:backup", "web"), ""},
		{"with a secret", url.Values{"grant_type": {string(oidc.GrantTypeClientCredentials)}, "audience": {"web"}}, "robot-secret"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if a := postToken(t, issuer, c.form, robot.ID.String(), c.secret); a.status == http.StatusOK {
				t.Fatalf("token = %d %v", a.status, a.body)
			}
		})
	}
}

func TestATokenRequestWhoseFormCannotBeReadIsLeftToTheProvider(t *testing.T) {
	issuer := serveMachines(t, machines{})
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, issuer+"/oidc/token", bytes.NewReader([]byte("grant_type=%zz")))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestAMachineIdentityGetsNoIDToken(t *testing.T) {
	s := &storage{}
	if err := s.SetUserinfoFromTokenExchangeRequest(context.Background(), nil, nil); err == nil {
		t.Fatal("an ID token for a Machine Identity")
	}
}

func TestTheProviderKnowsAMachineIdentityAsAClientOfTheTokenEndpointOnly(t *testing.T) {
	c := machineClient{robot}
	if c.GetID() != robot.ID.String() || c.RedirectURIs() != nil || c.PostLogoutRedirectURIs() != nil || c.ResponseTypes() != nil ||
		c.LoginURL("x") != "" || c.IDTokenLifetime() != 0 || c.DevMode() || c.IsScopeAllowed("openid") || c.IDTokenUserinfoClaimsAssertion() ||
		c.ClockSkew() != 0 || c.AuthMethod() != oidc.AuthMethodBasic || c.ApplicationType() != 0 ||
		!slices.Equal(c.GrantTypes(), []oidc.GrantType{oidc.GrantTypeTokenExchange, oidc.GrantTypeClientCredentials}) ||
		c.RestrictAdditionalIdTokenScopes()([]string{"email"}) != nil || c.RestrictAdditionalAccessTokenScopes()([]string{"email"}) != nil {
		t.Fatalf("machineClient = %+v", c)
	}
}

// usersToken is an exchange of a User's own access token, which zitadel/oidc verifies itself
// before the storage sees it.
type usersToken struct{ op.TokenExchangeRequest }

func (usersToken) GetExchangeSubjectTokenType() oidc.TokenType { return oidc.AccessTokenType }
func (usersToken) GetExchangeSubject() string                  { return robot.ID.String() }
func (usersToken) GetClientID() string                         { return robot.ID.String() }

func TestAUsersAccessTokenIsNeverExchangedForAMachineToken(t *testing.T) {
	s := &storage{}
	if err := s.ValidateTokenExchangeRequest(context.Background(), usersToken{}); err == nil {
		t.Fatal("a User's access token was exchanged")
	}
}

func TestAMachineIdentityMayAskForAnAccessTokenByName(t *testing.T) {
	f := exchangeForm("sa:media:backup", "web")
	f.Set("requested_token_type", string(oidc.AccessTokenType))
	wantRoles(t, postToken(t, serveMachines(t, machines{}), f, robot.ID.String(), ""))
}
