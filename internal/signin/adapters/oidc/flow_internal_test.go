package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var t0 = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

const cookieName = "__Host-identity_session"

// env is a running Provider over in-memory stores, and a browser that follows no redirects.
type env struct {
	issuer   string
	session  string
	cookies  map[string]string // what the server set in this browser
	requests *requests
	accounts *accounts
	browser  *http.Client
}

// elsewhere is another browser holding the same session, as a victim lured to a link would.
func (e *env) elsewhere() *env {
	other := *e
	other.cookies = map[string]string{}
	return &other
}

func newEnv(t *testing.T) *env {
	t.Helper()
	r, a := newRequests(), &accounts{ada: uuid.New(), sessionID: uuid.New()}
	e := serve(t, r, a, "tok")
	e.requests, e.accounts = r, a
	return e
}

// serve runs a Provider over requests and accounts; session is the browser's session cookie.
func serve(t *testing.T, requests app.Requests, accounts app.Accounts, session string) *env {
	t.Helper()
	var h http.Handler
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }))
	t.Cleanup(srv.Close)
	svc := app.New(requests, testClients, accounts, time.Now, rand.Reader)
	var err error
	h, err = New(Config{Issuer: srv.URL, Insecure: true, SessionCookie: cookieName}, svc, NewRing(testKey()), slog.New(slog.DiscardHandler), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return &env{
		issuer:  srv.URL,
		session: session,
		cookies: map[string]string{},
		browser: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}
}

// page is a response, read in full.
type page struct {
	status int
	header http.Header
	body   []byte
}

func (e *env) get(t *testing.T, target string, signedIn bool) page {
	t.Helper()
	if strings.HasPrefix(target, "/") {
		target = e.issuer + target
	}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	for name, value := range e.cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value}) //nolint:gosec // a request's cookie carries no attributes
	}
	if signedIn {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: e.session}) //nolint:gosec // as above
	}
	return e.do(t, req)
}

func (e *env) do(t *testing.T, req *http.Request) page {
	t.Helper()
	res, err := e.browser.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	for _, c := range res.Cookies() {
		if c.MaxAge < 0 {
			delete(e.cookies, c.Name)
		} else {
			e.cookies[c.Name] = c.Value
		}
	}
	body, _ := io.ReadAll(res.Body)
	return page{status: res.StatusCode, header: res.Header, body: body}
}

func redirect(t *testing.T, p page, status int, prefix string) string {
	t.Helper()
	loc := p.header.Get("Location")
	if p.status != status || !strings.HasPrefix(loc, prefix) {
		t.Fatalf("got %d to %q (%s), want %d to %s…", p.status, loc, p.body, status, prefix)
	}
	return loc
}

func challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func authorizeQuery(clientID, redirectURI, verifier string) url.Values {
	q := url.Values{
		"client_id": {clientID}, "redirect_uri": {redirectURI}, "response_type": {"code"},
		"scope": {"openid email profile"}, "state": {"st"}, "nonce": {"n-" + clientID},
	}
	if verifier != "" {
		q.Set("code_challenge", challenge(verifier))
		q.Set("code_challenge_method", "S256")
	}
	return q
}

// loginURL starts an authorization request and returns where /authorize sends the browser.
func (e *env) loginURL(t *testing.T, q url.Values) string {
	t.Helper()
	return redirect(t, e.get(t, "/oidc/authorize?"+q.Encode(), false), http.StatusFound, loginPath+"?authRequestID=")
}

// code signs in through the Single sign-on session and returns the code the Client receives.
func (e *env) code(t *testing.T, q url.Values) string {
	t.Helper()
	callback := redirect(t, e.get(t, e.loginURL(t, q), true), http.StatusSeeOther, e.issuer+"/oidc/authorize/callback?id=")
	back := redirect(t, e.get(t, callback, false), http.StatusFound, q.Get("redirect_uri")+"?")
	u, _ := url.Parse(back)
	if u.Query().Get("state") != "st" {
		t.Fatalf("the Client got state %q back", u.Query().Get("state"))
	}
	return u.Query().Get("code")
}

type tokens struct {
	IDToken     string `json:"id_token"`
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
}

func (e *env) exchange(t *testing.T, form url.Values, user, secret string) (int, tokens) {
	t.Helper()
	form.Set("grant_type", "authorization_code")
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, e.issuer+"/oidc/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if user != "" {
		req.SetBasicAuth(user, secret)
	}
	p := e.do(t, req)
	var tok tokens
	_ = json.Unmarshal(p.body, &tok)
	return p.status, tok
}

func (e *env) verify(t *testing.T, clientID, idToken string) *oidc.IDTokenClaims {
	t.Helper()
	keys := rp.NewRemoteKeySet(http.DefaultClient, e.issuer+"/oidc/keys")
	v := rp.NewIDTokenVerifier(e.issuer, clientID, keys, rp.WithNonce(func(context.Context) string { return "n-" + clientID }))
	claims, err := rp.VerifyIDToken[*oidc.IDTokenClaims](t.Context(), idToken, v)
	if err != nil {
		t.Fatalf("the ID token does not verify: %v", err)
	}
	return claims
}

func TestDiscoveryAndJWKSAreServedAtTheIssuer(t *testing.T) {
	e := newEnv(t)
	var d oidc.DiscoveryConfiguration
	res := e.get(t, "/.well-known/openid-configuration", false)
	if err := json.Unmarshal(res.body, &d); err != nil || res.status != http.StatusOK {
		t.Fatalf("discovery = %d, %v", res.status, err)
	}
	if d.Issuer != e.issuer || d.AuthorizationEndpoint != e.issuer+"/oidc/authorize" || d.JwksURI != e.issuer+"/oidc/keys" ||
		d.TokenEndpoint != e.issuer+"/oidc/token" || d.EndSessionEndpoint != e.issuer+"/oidc/end_session" {
		t.Fatalf("discovery = %+v", d)
	}
	var jwks struct {
		Keys []struct{ Kid, Alg, Use string } `json:"keys"`
	}
	_ = json.Unmarshal(e.get(t, "/oidc/keys", false).body, &jwks)
	if len(jwks.Keys) != 1 || jwks.Keys[0].Alg != "RS256" || jwks.Keys[0].Use != "sig" || jwks.Keys[0].Kid != NewRing(testKey()).Signing().ID() {
		t.Fatalf("JWKS = %+v", jwks)
	}
}

func TestAPublicClientSignsInWithPKCE(t *testing.T) {
	e := newEnv(t)
	q := authorizeQuery("spa", "https://spa.test/cb", "a-verifier-of-at-least-forty-three-characters-long")
	code := e.code(t, q)
	status, tok := e.exchange(t, url.Values{
		"code": {code}, "redirect_uri": {"https://spa.test/cb"}, "client_id": {"spa"},
		"code_verifier": {"a-verifier-of-at-least-forty-three-characters-long"},
	}, "", "")
	if status != http.StatusOK || tok.IDToken == "" || tok.AccessToken == "" {
		t.Fatalf("token = %d %+v", status, tok)
	}
	c := e.verify(t, "spa", tok.IDToken)
	if c.Subject != e.accounts.ada.String() || c.Email != "ada@example.org" || !bool(c.EmailVerified) || c.Name != "Ada" ||
		len(c.AuthenticationMethodsReferences) != 1 || c.AuthenticationMethodsReferences[0] != "pwd" ||
		!c.AuthTime.AsTime().Equal(t0) || c.Claims["sid"] != e.accounts.sessionID.String() {
		t.Fatalf("ID token claims = %+v", c)
	}

	// The code is spent.
	if status, tok := e.exchange(t, url.Values{
		"code": {code}, "redirect_uri": {"https://spa.test/cb"}, "client_id": {"spa"},
		"code_verifier": {"a-verifier-of-at-least-forty-three-characters-long"},
	}, "", ""); status == http.StatusOK || tok.Error != "invalid_grant" {
		t.Fatalf("a second exchange = %d %+v", status, tok)
	}
}

func TestAConfidentialClientSignsInWithoutAPasswordWhileTheSessionHolds(t *testing.T) {
	e := newEnv(t)
	code := e.code(t, authorizeQuery("web", "https://web.test/cb", ""))
	status, tok := e.exchange(t, url.Values{"code": {code}, "redirect_uri": {"https://web.test/cb"}}, "web", "s3cret")
	if status != http.StatusOK {
		t.Fatalf("token = %d %+v", status, tok)
	}
	c := e.verify(t, "web", tok.IDToken)
	if c.Subject != e.accounts.ada.String() || c.Claims["sid"] != e.accounts.sessionID.String() {
		t.Fatalf("ID token claims = %+v", c)
	}

	// Userinfo answers for the access token.
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, e.issuer+"/oidc/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	res := e.do(t, req)
	var info oidc.UserInfo
	_ = json.Unmarshal(res.body, &info)
	if info.Subject != e.accounts.ada.String() || info.Email != "ada@example.org" {
		t.Fatalf("userinfo = %d %+v", res.status, info)
	}
}

func TestWithoutASessionTheBrowserIsSentToSignIn(t *testing.T) {
	e := newEnv(t)
	login := e.loginURL(t, authorizeQuery("spa", "https://spa.test/cb", "v"))
	id := strings.TrimPrefix(login, loginPath+"?authRequestID=")
	redirect(t, e.get(t, login, false), http.StatusSeeOther, "/signin?authRequestID="+id)
}

func TestAPublicClientWithoutPKCEGetsNoTokens(t *testing.T) {
	e := newEnv(t)
	code := e.code(t, authorizeQuery("spa", "https://spa.test/cb", ""))
	if status, tok := e.exchange(t, url.Values{"code": {code}, "redirect_uri": {"https://spa.test/cb"}, "client_id": {"spa"}}, "", ""); status == http.StatusOK || tok.Error != "invalid_request" {
		t.Fatalf("token = %d %+v", status, tok)
	}
}

func TestAWrongSecretGetsNoTokens(t *testing.T) {
	e := newEnv(t)
	code := e.code(t, authorizeQuery("web", "https://web.test/cb", ""))
	if status, tok := e.exchange(t, url.Values{"code": {code}, "redirect_uri": {"https://web.test/cb"}}, "web", "wrong"); status == http.StatusOK || tok.Error != "invalid_client" {
		t.Fatalf("token = %d %+v", status, tok)
	}
}

func TestAnUnknownClientCannotStart(t *testing.T) {
	e := newEnv(t)
	res := e.get(t, "/oidc/authorize?"+authorizeQuery("nobody", "https://evil.test/cb", "v").Encode(), false)
	if res.status == http.StatusFound && strings.HasPrefix(res.header.Get("Location"), "https://evil.test") {
		t.Fatal("an unknown Client was sent a response")
	}
}

func TestEndSessionSignsOutEverywhere(t *testing.T) {
	e := newEnv(t)
	code := e.code(t, authorizeQuery("web", "https://web.test/cb", ""))
	_, tok := e.exchange(t, url.Values{"code": {code}, "redirect_uri": {"https://web.test/cb"}}, "web", "s3cret")
	res := e.get(t, "/oidc/end_session?id_token_hint="+url.QueryEscape(tok.IDToken), false)
	if len(e.accounts.signedOut) != 1 || e.accounts.signedOut[0] != e.accounts.ada {
		t.Fatalf("end_session = %d, signed out %v", res.status, e.accounts.signedOut)
	}
}

func TestTheLoginPageRefusesWhatItCannotComplete(t *testing.T) {
	e := newEnv(t)
	for _, c := range []struct {
		target string
		status int
	}{
		{loginPath + "?authRequestID=not-a-uuid", http.StatusBadRequest},
		{loginPath + "?authRequestID=" + uuid.NewString(), http.StatusBadRequest},
	} {
		if res := e.get(t, c.target, true); res.status != c.status {
			t.Errorf("%s = %d, want %d", c.target, res.status, c.status)
		}
	}
	login := e.loginURL(t, authorizeQuery("spa", "https://spa.test/cb", "v"))
	gone := e.loginURL(t, authorizeQuery("spa", "https://spa.test/cb", "v"))
	e.requests.byID = map[uuid.UUID]domain.AuthRequest{}
	if res := e.get(t, gone, true); res.status != http.StatusBadRequest || !strings.Contains(string(res.body), "expired") {
		t.Fatalf("an expired request = %d %s", res.status, res.body)
	}
	e.requests.byID[uuid.MustParse(strings.TrimPrefix(login, loginPath+"?authRequestID="))] = domain.AuthRequest{ExpiresAt: time.Now().Add(time.Hour)}
	e.cookies[bindingCookie] = strings.TrimPrefix(login, loginPath+"?authRequestID=")
	e.requests.fail = errBoom
	if res := e.get(t, login, true); res.status != http.StatusInternalServerError {
		t.Fatalf("a failing store = %d", res.status)
	}
}

func TestARequestCompletesOnlyInTheBrowserThatStartedIt(t *testing.T) {
	attacker := newEnv(t)
	login := attacker.loginURL(t, authorizeQuery("spa", "https://spa.test/cb", "a-verifier-of-at-least-forty-three-characters-long"))
	id := strings.TrimPrefix(login, loginPath+"?authRequestID=")

	victim := attacker.elsewhere()
	if res := victim.get(t, login, true); res.status != http.StatusBadRequest {
		t.Fatalf("a request started elsewhere was completed with this browser's session: %d %s", res.status, res.header.Get("Location"))
	}
	if r, _ := attacker.requests.RequestByID(t.Context(), uuid.MustParse(id)); r.SignIn != nil {
		t.Fatal("the request was completed")
	}

	// Even a completed request gives its code only to the browser that started it.
	callback := redirect(t, attacker.get(t, login, true), http.StatusSeeOther, attacker.issuer+"/oidc/authorize/callback?id=")
	if res := victim.get(t, callback, false); res.status != http.StatusBadRequest {
		t.Fatalf("another browser fetched the code: %d %s", res.status, res.header.Get("Location"))
	}
	query := strings.TrimPrefix(callback, attacker.issuer+"/oidc/authorize/callback")
	for _, variant := range []string{"/oidc/authorize/callback/", "/oidc/authorize/%63allback", "//oidc/authorize/callback", "/oidc/./authorize/callback"} {
		res := victim.get(t, variant+query, false)
		if strings.HasPrefix(res.header.Get("Location"), "https://spa.test") {
			t.Fatalf("another browser fetched the code through %s", variant)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodHead, http.MethodPut} {
		req, _ := http.NewRequestWithContext(t.Context(), method, callback, nil)
		if res := victim.do(t, req); res.status != http.StatusBadRequest {
			t.Fatalf("another browser fetched the code with %s: %d %s", method, res.status, res.header.Get("Location"))
		}
	}
	for _, wrong := range []string{"/oidc/authorize/callback?id=" + uuid.NewString(), "/oidc/authorize/callback", "/oidc/authorize/callback?id="} {
		if res := attacker.get(t, wrong, false); res.status != http.StatusBadRequest {
			t.Fatalf("%s with a binding for another request = %d", wrong, res.status)
		}
	}
	redirect(t, attacker.get(t, callback, false), http.StatusFound, "https://spa.test/cb?")
	if _, bound := attacker.cookies[bindingCookie]; bound {
		t.Fatal("the binding outlived the callback")
	}
}

func TestTheCallbackServesOnlyTheRequestItChecked(t *testing.T) {
	e := newEnv(t)
	victimLogin := e.loginURL(t, authorizeQuery("spa", "https://spa.test/cb", "v"))
	victimID := strings.TrimPrefix(victimLogin, loginPath+"?authRequestID=")
	redirect(t, e.get(t, victimLogin, true), http.StatusSeeOther, e.issuer+"/oidc/authorize/callback?id=")

	// The attacker holds a binding for a request of their own, and names the victim's elsewhere.
	attacker := e.elsewhere()
	own := strings.TrimPrefix(attacker.loginURL(t, authorizeQuery("spa", "https://spa.test/cb", "v")), loginPath+"?authRequestID=")
	smuggled := []*http.Request{}
	post, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, e.issuer+"/oidc/authorize/callback?id="+own, strings.NewReader("id="+victimID))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	smuggled = append(smuggled, post)
	twice, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, e.issuer+"/oidc/authorize/callback?id="+own+"&id="+victimID, nil)
	smuggled = append(smuggled, twice)
	for _, req := range smuggled {
		req.AddCookie(&http.Cookie{Name: bindingCookie, Value: attacker.cookies[bindingCookie]}) //nolint:gosec // a request's cookie carries no attributes
		if res := attacker.do(t, req); strings.HasPrefix(res.header.Get("Location"), "https://spa.test") {
			t.Fatalf("%s %s fetched the victim's code", req.Method, req.URL)
		}
	}
}
