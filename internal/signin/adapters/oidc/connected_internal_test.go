package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	connectedapp "github.com/JorisJonkers-dev/identity/internal/connected/app"
	connecteddomain "github.com/JorisJonkers-dev/identity/internal/connected/domain"
)

// fakeApps is the connected context in memory: web.test is the one MCP resource, the Client web;
// boom.test fails to resolve. fail makes the named method fail.
type fakeApps struct {
	mu       sync.Mutex
	n        int
	regs     map[string]connecteddomain.Registration
	consents map[string]uuid.UUID
	apps     map[uuid.UUID]consented
	access   map[uuid.UUID]connectedapp.AccessToken
	refresh  map[string]*spendable
	refuse   error
	fail     map[string]error
}

type consented struct {
	user        uuid.UUID
	reg, client string
}

type spendable struct {
	connectedapp.RefreshToken
	spent bool
}

func newApps() *fakeApps {
	return &fakeApps{
		regs: map[string]connecteddomain.Registration{}, consents: map[string]uuid.UUID{}, apps: map[uuid.UUID]consented{},
		access: map[uuid.UUID]connectedapp.AccessToken{}, refresh: map[string]*spendable{}, fail: map[string]error{},
	}
}

func (f *fakeApps) Register(_ context.Context, name string, uris []string) (connecteddomain.Registration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail["Register"]; err != nil {
		return connecteddomain.Registration{}, err
	}
	f.n++
	r, err := connecteddomain.NewRegistration(fmt.Sprintf("mcp-%d", f.n), name, uris, tokenIssued)
	if err == nil {
		f.regs[r.ClientID] = r
	}
	return r, err
}

func (f *fakeApps) Registration(_ context.Context, id string) (connecteddomain.Registration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail["Registration"]; err != nil {
		return connecteddomain.Registration{}, err
	}
	r, ok := f.regs[id]
	if !ok {
		return connecteddomain.Registration{}, connectedapp.ErrNotFound
	}
	return r, nil
}

func (f *fakeApps) Resource(_ context.Context, resource string) (connectedapp.Client, error) {
	switch {
	case strings.HasPrefix(resource, "https://web.test/"):
		return connectedapp.Client{ID: "web", Label: "Web"}, nil
	case strings.HasPrefix(resource, "https://boom.test/"):
		return connectedapp.Client{}, errBoom
	}
	return connectedapp.Client{}, connectedapp.ErrNotMCPResource
}

func consentKey(user uuid.UUID, reg, client string) string {
	return user.String() + " " + reg + " " + client
}

func (f *fakeApps) Consented(_ context.Context, user uuid.UUID, reg, client string) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse != nil {
		return uuid.Nil, f.refuse
	}
	id, ok := f.consents[consentKey(user, reg, client)]
	if !ok {
		return uuid.Nil, connectedapp.ErrNotFound
	}
	return id, nil
}

func (f *fakeApps) Consent(_ context.Context, user uuid.UUID, reg, client string) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse != nil {
		return uuid.Nil, f.refuse
	}
	id := uuid.New()
	f.consents[consentKey(user, reg, client)] = id
	f.apps[id] = consented{user: user, reg: reg, client: client}
	return id, nil
}

// revoke ends a Connected App as the connected context does: its consent and tokens go.
func (f *fakeApps) revoke(appID uuid.UUID) {
	a := f.apps[appID]
	delete(f.consents, consentKey(a.user, a.reg, a.client))
	delete(f.apps, appID)
	for id, t := range f.access {
		if t.AppID == appID {
			delete(f.access, id)
		}
	}
}

func (f *fakeApps) IssueAccess(_ context.Context, appID uuid.UUID) (uuid.UUID, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail["IssueAccess"]; err != nil {
		return uuid.Nil, time.Time{}, err
	}
	a := f.apps[appID]
	id, expires := uuid.New(), time.Now().Add(time.Hour)
	f.access[id] = connectedapp.AccessToken{ID: id, AppID: appID, UserID: a.user, ClientID: a.client, ExpiresAt: expires}
	return id, expires, nil
}

func (f *fakeApps) IssueRefresh(_ context.Context, appID uuid.UUID) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail["IssueRefresh"]; err != nil {
		return "", err
	}
	a := f.apps[appID]
	secret := "rt-" + uuid.NewString()
	f.refresh[secret] = &spendable{RefreshToken: connectedapp.RefreshToken{ID: uuid.New(), AppID: appID, UserID: a.user, ClientID: a.client, RegistrationID: a.reg}}
	return secret, nil
}

func (f *fakeApps) Refresh(_ context.Context, secret string) (connectedapp.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail["Refresh"]; err != nil {
		return connectedapp.RefreshToken{}, err
	}
	t, ok := f.refresh[secret]
	if !ok {
		return connectedapp.RefreshToken{}, connectedapp.ErrInvalidToken
	}
	if t.spent {
		f.revoke(t.AppID)
		return connectedapp.RefreshToken{}, connectedapp.ErrInvalidToken
	}
	t.spent = true
	return t.RefreshToken, nil
}

func (f *fakeApps) Check(_ context.Context, id uuid.UUID, clientID string) (connectedapp.AccessToken, []string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail["Check"]; err != nil {
		return connectedapp.AccessToken{}, nil, err
	}
	t, ok := f.access[id]
	if !ok || t.ClientID != clientID {
		return connectedapp.AccessToken{}, nil, connectedapp.ErrInvalidToken
	}
	return t, []string{"user"}, nil
}

// registered is a tool registered with the env's Provider, listening on a loopback port.
func (e *env) registered(t *testing.T) string {
	t.Helper()
	r, err := e.apps.Register(t.Context(), "Claude Code", []string{"http://127.0.0.1:33418/callback"})
	if err != nil {
		t.Fatal(err)
	}
	return r.ClientID
}

// toolQuery is a registered tool's authorize request for the MCP resource at resource.
func toolQuery(clientID, resource string) url.Values {
	return url.Values{
		"client_id": {clientID}, "redirect_uri": {"http://127.0.0.1:33418/callback"}, "response_type": {"code"},
		"state": {"st"}, "code_challenge": {challenge("tool-verifier")}, "code_challenge_method": {"S256"},
		"resource": {resource},
	}
}

func (e *env) consent(t *testing.T, reg string) uuid.UUID {
	t.Helper()
	id, err := e.apps.Consent(t.Context(), e.accounts.ada, reg, "web")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type toolTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
}

func (e *env) post(t *testing.T, path string, form url.Values) page {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, e.issuer+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return e.do(t, req)
}

func (e *env) toolToken(t *testing.T, form url.Values) (int, toolTokens) {
	t.Helper()
	p := e.post(t, "/oidc/token", form)
	var tok toolTokens
	_ = json.Unmarshal(p.body, &tok)
	return p.status, tok
}

// connectedTokens is a consented tool's first tokens, by the code flow.
func (e *env) connectedTokens(t *testing.T) (string, toolTokens) {
	t.Helper()
	reg := e.registered(t)
	e.consent(t, reg)
	code := e.code(t, toolQuery(reg, "https://web.test/"))
	status, tok := e.toolToken(t, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {reg},
		"redirect_uri": {"http://127.0.0.1:33418/callback"}, "code_verifier": {"tool-verifier"},
	})
	if status != http.StatusOK {
		t.Fatalf("token: %d %+v", status, tok)
	}
	return reg, tok
}

func (e *env) refreshWith(t *testing.T, reg, refresh string) (int, toolTokens) {
	t.Helper()
	return e.toolToken(t, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {reg}})
}

// introspection is what /oidc/introspect answers web about a token.
func (e *env) introspection(t *testing.T, token string) map[string]any {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, e.issuer+"/oidc/introspect", strings.NewReader(url.Values{"token": {token}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("web", "s3cret")
	var got map[string]any
	_ = json.Unmarshal(e.do(t, req).body, &got)
	return got
}

func (e *env) registration(t *testing.T, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, e.issuer+registerPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	p := e.do(t, req)
	var got map[string]any
	_ = json.Unmarshal(p.body, &got)
	return p.status, got
}

func TestAToolRegistersItselfAsAPublicCodeFlowClient(t *testing.T) {
	e := newEnv(t)
	status, got := e.registration(t, `{"client_name":"Claude Code","redirect_uris":["http://localhost:33418/callback"],"grant_types":["authorization_code","refresh_token"],"response_types":["code"],"token_endpoint_auth_method":"none"}`)
	if status != http.StatusCreated || !strings.HasPrefix(fmt.Sprint(got["client_id"]), "mcp-") || got["client_name"] != "Claude Code" ||
		got["token_endpoint_auth_method"] != "none" || fmt.Sprint(got["grant_types"]) != "[authorization_code refresh_token]" {
		t.Fatalf("register = %d %v", status, got)
	}
}

func TestARegistrationIsRefused(t *testing.T) {
	for _, c := range []struct {
		name, body, want string
	}{
		{"that is not JSON", `{`, "invalid_client_metadata"},
		{"for a client with a secret", `{"redirect_uris":["https://a.test/cb"],"token_endpoint_auth_method":"client_secret_basic"}`, "invalid_client_metadata"},
		{"for another grant", `{"redirect_uris":["https://a.test/cb"],"grant_types":["client_credentials"]}`, "invalid_client_metadata"},
		{"for another response type", `{"redirect_uris":["https://a.test/cb"],"response_types":["token"]}`, "invalid_client_metadata"},
		{"for a redirect off the User's machine over http", `{"redirect_uris":["http://a.test/cb"]}`, "invalid_redirect_uri"},
	} {
		t.Run(c.name, func(t *testing.T) {
			status, got := newEnv(t).registration(t, c.body)
			if status != http.StatusBadRequest || got["error"] != c.want {
				t.Fatalf("register = %d %v", status, got)
			}
		})
	}
}

func TestARegistrationFailsAsAServerErrorWhenTheStoreDoes(t *testing.T) {
	e := newEnv(t)
	e.apps.fail["Register"] = errBoom
	if status, got := e.registration(t, `{"redirect_uris":["https://a.test/cb"]}`); status != http.StatusInternalServerError || got["error"] != "server_error" {
		t.Fatalf("register = %d %v", status, got)
	}
}

func TestTheAuthorizationServerMetadataNamesTheRegistrationEndpoint(t *testing.T) {
	e := newEnv(t)
	p := e.get(t, "/.well-known/oauth-authorization-server", false)
	var got oidc.DiscoveryConfiguration
	_ = json.Unmarshal(p.body, &got)
	if p.status != http.StatusOK || got.Issuer != e.issuer || got.RegistrationEndpoint != e.issuer+registerPath || p.header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("metadata = %d %+v", p.status, got)
	}
}

func TestAnMCPResourcesMetadataNamesTheIssuer(t *testing.T) {
	e := newEnv(t)
	p := e.get(t, "/.well-known/oauth-protected-resource/web.test", false)
	var got protectedResource
	_ = json.Unmarshal(p.body, &got)
	if p.status != http.StatusOK || got.Resource != "https://web.test/" || len(got.AuthorizationServers) != 1 || got.AuthorizationServers[0] != e.issuer || got.ResourceName != "Web" {
		t.Fatalf("metadata = %d %+v", p.status, got)
	}
}

func TestAHostTakingNoConnectedAppsHasNoResourceMetadata(t *testing.T) {
	if p := newEnv(t).get(t, "/.well-known/oauth-protected-resource/spa.test", false); p.status != http.StatusNotFound {
		t.Fatalf("status = %d", p.status)
	}
}

func TestResourceMetadataFailsAsAServerErrorWhenTheLookupDoes(t *testing.T) {
	if p := newEnv(t).get(t, "/.well-known/oauth-protected-resource/boom.test", false); p.status != http.StatusInternalServerError {
		t.Fatalf("status = %d", p.status)
	}
}

func TestAToolsSignInAsksTheUserToConsentFirst(t *testing.T) {
	e := newEnv(t)
	login := e.loginURL(t, toolQuery(e.registered(t), "https://web.test/mcp"))
	redirect(t, e.get(t, login, true), http.StatusSeeOther, consentPath+"?authRequestID=")
}

func TestAToolAskingForAServiceTakingNoConnectedAppsIsTurnedBack(t *testing.T) {
	for _, c := range []struct{ name, resource, want string }{
		{"one that takes none", "https://spa.test/", "invalid_target"},
		{"no service at all", "", "invalid_target"},
		{"one whose lookup fails", "https://boom.test/", "server_error"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			back := redirect(t, e.get(t, "/oidc/authorize?"+toolQuery(e.registered(t), c.resource).Encode(), false), http.StatusFound, "http://127.0.0.1:33418/callback?")
			if u, _ := url.Parse(back); u.Query().Get("error") != c.want {
				t.Fatalf("redirected to %s", back)
			}
		})
	}
}

func TestAnUnknownToolCannotStartASignIn(t *testing.T) {
	e := newEnv(t)
	if p := e.get(t, "/oidc/authorize?"+toolQuery("mcp-unknown", "https://web.test/").Encode(), false); p.status == http.StatusFound {
		t.Fatalf("an unknown tool was sent to sign in: %s", p.header.Get("Location"))
	}
}

func TestAToolWhoseRegistrationCannotBeReadCannotStartASignIn(t *testing.T) {
	e := newEnv(t)
	reg := e.registered(t)
	e.apps.fail["Registration"] = errBoom
	if p := e.get(t, "/oidc/authorize?"+toolQuery(reg, "https://web.test/").Encode(), false); p.status == http.StatusFound {
		t.Fatalf("a tool was sent to sign in: %s", p.header.Get("Location"))
	}
}

func TestAToolForAServiceThatStoppedTakingConnectedAppsIsTurnedAwayAtSignIn(t *testing.T) {
	e := newEnv(t)
	login := e.loginURL(t, toolQuery(e.registered(t), "https://web.test/"))
	e.apps.refuse = connectedapp.ErrStepUpClient
	if p := e.get(t, login, true); p.status != http.StatusBadRequest || !strings.Contains(string(p.body), "cannot connect") {
		t.Fatalf("login = %d %s", p.status, p.body)
	}
}

func TestAConsentedToolGetsAnOpaqueAccessTokenAndARefreshToken(t *testing.T) {
	_, tok := newEnv(t).connectedTokens(t)
	if !strings.HasPrefix(tok.AccessToken, appTokenPrefix) || strings.Count(tok.AccessToken, ".") == 2 || tok.RefreshToken == "" || tok.TokenType != "Bearer" || tok.ExpiresIn <= 0 {
		t.Fatalf("tokens = %+v", tok)
	}
}

func TestTheClientFindsAConnectedAppsTokenActiveWithTheUsersRoles(t *testing.T) {
	e := newEnv(t)
	_, tok := e.connectedTokens(t)
	got := e.introspection(t, tok.AccessToken)
	if got["active"] != true || got["sub"] != e.accounts.ada.String() || got["email"] != "ada@example.org" || fmt.Sprint(got["roles"]) != "[user]" {
		t.Fatalf("introspection = %v", got)
	}
}

func TestAConnectedAppsTokenIsInactiveOnceRevoked(t *testing.T) {
	e := newEnv(t)
	_, tok := e.connectedTokens(t)
	for id := range e.apps.apps {
		e.apps.revoke(id)
	}
	if got := e.introspection(t, tok.AccessToken); got["active"] != false {
		t.Fatalf("introspection = %v", got)
	}
}

func TestAConnectedAppsTokenIsInactiveWhenItsCheckFails(t *testing.T) {
	e := newEnv(t)
	_, tok := e.connectedTokens(t)
	e.apps.fail["Check"] = errBoom
	if got := e.introspection(t, tok.AccessToken); got["active"] != false {
		t.Fatalf("introspection = %v", got)
	}
}

func TestAConnectedAppsTokenIsInactiveWhenItsUserCannotBeRead(t *testing.T) {
	e := newEnv(t)
	_, tok := e.connectedTokens(t)
	e.accounts.failClaims = errBoom
	if got := e.introspection(t, tok.AccessToken); got["active"] != false {
		t.Fatalf("introspection = %v", got)
	}
}

func TestAConnectedAppsTokenAnswersNoUserinfo(t *testing.T) {
	e := newEnv(t)
	_, tok := e.connectedTokens(t)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, e.issuer+"/oidc/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	if p := e.do(t, req); p.status == http.StatusOK {
		t.Fatalf("userinfo = %s", p.body)
	}
}

func TestARefreshTokenGivesTheToolNewTokens(t *testing.T) {
	e := newEnv(t)
	reg, first := e.connectedTokens(t)
	status, next := e.refreshWith(t, reg, first.RefreshToken)
	if status != http.StatusOK || next.RefreshToken == "" || next.RefreshToken == first.RefreshToken || e.introspection(t, next.AccessToken)["active"] != true {
		t.Fatalf("refresh = %d %+v", status, next)
	}
}

func TestARefreshTokenSpentTwiceDisconnectsTheTool(t *testing.T) {
	e := newEnv(t)
	reg, first := e.connectedTokens(t)
	_, next := e.refreshWith(t, reg, first.RefreshToken)

	status, again := e.refreshWith(t, reg, first.RefreshToken)

	if status != http.StatusBadRequest || again.Error != "invalid_grant" || e.introspection(t, next.AccessToken)["active"] != false {
		t.Fatalf("reuse = %d %+v", status, again)
	}
}

func TestAnotherToolCannotSpendARefreshToken(t *testing.T) {
	e := newEnv(t)
	_, tok := e.connectedTokens(t)
	if status, got := e.refreshWith(t, e.registered(t), tok.RefreshToken); status != http.StatusBadRequest || got.Error != "invalid_grant" {
		t.Fatalf("refresh = %d %+v", status, got)
	}
}

func TestAToolGetsNoTokensWhenTheyCannotBeMade(t *testing.T) {
	for _, method := range []string{"IssueAccess", "IssueRefresh", "Refresh"} {
		t.Run(method, func(t *testing.T) {
			e := newEnv(t)
			reg, first := e.connectedTokens(t)
			e.apps.fail[method] = errBoom
			if status, got := e.refreshWith(t, reg, first.RefreshToken); status == http.StatusOK || got.AccessToken != "" {
				t.Fatalf("refresh = %d %+v", status, got)
			}
		})
	}
}

func TestACodeOutlivingItsConsentGivesNoTokens(t *testing.T) {
	e := newEnv(t)
	reg := e.registered(t)
	appID := e.consent(t, reg)
	code := e.code(t, toolQuery(reg, "https://web.test/"))
	e.apps.revoke(appID)
	status, got := e.toolToken(t, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {reg},
		"redirect_uri": {"http://127.0.0.1:33418/callback"}, "code_verifier": {"tool-verifier"},
	})
	if status != http.StatusBadRequest || got.Error != "invalid_grant" {
		t.Fatalf("token = %d %+v", status, got)
	}
}

func TestAConnectedAppsRefreshNeedsNoNewAccessTokenOnly(t *testing.T) {
	s, _, _ := newStorage()
	apps := s.apps.(*fakeApps)
	appID, _ := apps.Consent(t.Context(), uuid.New(), "mcp-1", "web")
	id, expires, err := s.CreateAccessToken(t.Context(), newRefreshRequest(connectedapp.RefreshToken{AppID: appID}))
	if err != nil || !strings.HasPrefix(id, appTokenIDPrefix) || expires.IsZero() || len(apps.refresh) != 0 {
		t.Fatalf("CreateAccessToken = %q, %v, %v", id, expires, err)
	}
}

func TestAConnectedAppsRefreshRequestNamesItsToolAndUser(t *testing.T) {
	user := uuid.New()
	r := newRefreshRequest(connectedapp.RefreshToken{UserID: user, RegistrationID: "mcp-1"})
	if r.GetClientID() != "mcp-1" || r.GetSubject() != user.String() || r.GetAudience()[0] != "mcp-1" || r.GetAMR() != nil || !r.GetAuthTime().IsZero() || r.GetScopes()[0] != oidc.ScopeOfflineAccess {
		t.Fatalf("refresh request = %+v", r)
	}
}

func TestARefreshKeepsTheScopesTheToolNarrowedItTo(t *testing.T) {
	r := newRefreshRequest(connectedapp.RefreshToken{})
	r.SetCurrentScopes([]string{})
	if len(r.GetScopes()) != 0 {
		t.Fatalf("scopes = %v", r.GetScopes())
	}
}

func TestARegisteredClientRunsWithThePublicDefaults(t *testing.T) {
	c := registeredClient{connecteddomain.Registration{ClientID: "mcp-1"}}
	if c.PostLogoutRedirectURIs() != nil || c.DevMode() || c.IsScopeAllowed("openid") || c.ClockSkew() != 0 || c.IDTokenUserinfoClaimsAssertion() ||
		c.IDTokenLifetime() != idTokenLifetime || c.RestrictAdditionalAccessTokenScopes()([]string{"x"}) != nil {
		t.Fatalf("client = %+v", c)
	}
}

func TestOnlyAConnectedAppsSealedTokenOpensAsOne(t *testing.T) {
	c := newCrypto([32]byte{1}, func(context.Context, string) (string, string, bool) { return "", "", false })
	sealed, _ := c.Encrypt("app-1:ada")
	if !strings.HasPrefix(sealed, appTokenPrefix) {
		t.Fatalf("sealed = %q", sealed)
	}
	if plain, err := c.Decrypt(sealed); err != nil || plain != "app-1:ada" {
		t.Fatalf("Decrypt = %q, %v", plain, err)
	}
	other, _ := c.Encrypt("request-id")
	if _, err := (appSealer{sealed: op256(t)}).Decrypt(appTokenPrefix + other); err == nil {
		t.Fatal("a sealed request id opened as a Connected App's token")
	}
	if _, err := (appSealer{sealed: op256(t)}).Decrypt(sealed[len(appTokenPrefix):]); err == nil {
		t.Fatal("a token without the prefix opened as a Connected App's")
	}
}

func op256(t *testing.T) op.Crypto {
	t.Helper()
	return op.NewAES256GCMCrypto([32]byte{1}, "")
}

func TestTheGateOpensAConnectedAppsTokenTheProviderIssued(t *testing.T) {
	e := newEnv(t)
	_, tok := e.connectedTokens(t)
	got, roles, err := NewAppTokens([32]byte{}, e.apps).Check(t.Context(), tok.AccessToken, "web")
	if err != nil || got.UserID != e.accounts.ada || fmt.Sprint(roles) != "[user]" {
		t.Fatalf("Check = %+v, %v, %v", got, roles, err)
	}
}

func TestTheGateRefusesATokenThatIsNoConnectedAppsToken(t *testing.T) {
	sealed, _ := appSealer{sealed: op256(t)}.Encrypt("app-not-a-uuid:ada")
	for _, token := range []string{"idapp_garbage", "idpat_x", sealed} {
		t.Run(token, func(t *testing.T) {
			if _, _, err := NewAppTokens([32]byte{1}, newApps()).Check(t.Context(), token, "web"); !errors.Is(err, connectedapp.ErrInvalidToken) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
