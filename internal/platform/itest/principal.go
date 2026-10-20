//go:build integration

package itest

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// GateSignIn is b signing in to a gated host through the Gate's button, and answers the cookie the
// host's /.auth/callback sets.
func (b *Browser) GateSignIn(t *testing.T, host string) string {
	t.Helper()
	return b.GateCallback(t, b.Follow(t, "/gate/start?"+url.Values{"host": {host}, "state": {"s"}, "rd": {"/"}}.Encode()), "s")
}

// GateStepUp is an app on host sending b to step up for a two-factor proof no older than minutes. It
// answers where /gate/start then sends b, and the state the host set for the callback.
func (b *Browser) GateStepUp(t *testing.T, host string, minutes int) (string, string) {
	t.Helper()
	target := "https://" + host + "/.auth/step-up?" + url.Values{"minutes": {strconv.Itoa(minutes)}, "rd": {"/pay"}}.Encode()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	var state string
	for _, c := range res.Cookies() {
		if c.Name == "__Host-identity_gate_state" {
			state = c.Value
		}
	}
	if res.StatusCode != http.StatusSeeOther || state == "" {
		t.Fatalf("step-up = %d %v", res.StatusCode, res.Header)
	}
	return b.Follow(t, res.Header.Get("Location")), state
}

// GateCallback follows a code back to its host, in a browser holding the state the host set, and
// answers the host cookie the callback sets.
func (b *Browser) GateCallback(t *testing.T, callback, state string) string {
	t.Helper()
	next, err := url.Parse(callback)
	if err != nil || next.Path != "/.auth/callback" {
		t.Fatalf("start went to %s", callback)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, next.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: "__Host-identity_gate_state", Value: state}) //nolint:gosec // a request's cookie carries no attributes
	res, err := b.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	for _, c := range res.Cookies() {
		if c.Name == "__Host-identity_gate" {
			return c.Value
		}
	}
	t.Fatalf("the callback = %d and set no host cookie", res.StatusCode)
	return ""
}

// ConnectedApp is b connecting a newly registered tool, called name, to the MCP resource at
// resource, and answers the tool's client id and its access token.
func (b *Browser) ConnectedApp(t *testing.T, name, resource string) (string, string) {
	t.Helper()
	const redirect, verifier = "http://127.0.0.1:33418/callback", "a-tool-verifier-of-enough-length-for-pkce-0123456789"
	var reg struct {
		ClientID string `json:"client_id"`
	}
	b.env.Browser().Call(t, http.MethodPost, "/oidc/register", map[string]any{"client_name": name, "redirect_uris": []string{redirect}}, http.StatusCreated, &reg)
	sum := sha256.Sum256([]byte(verifier))
	consent := b.Follow(t, b.Follow(t, "/oidc/authorize?"+url.Values{
		"client_id": {reg.ClientID}, "redirect_uri": {redirect}, "response_type": {"code"}, "state": {"st"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}, "resource": {resource},
	}.Encode()))
	var next struct{ Redirect string }
	b.Call(t, http.MethodPost, "/oidc/consent", map[string]any{"authRequestID": strings.TrimPrefix(consent, "/connect?authRequestID="), "allow": true}, http.StatusOK, &next)
	back, _ := url.Parse(b.Follow(t, next.Redirect))
	status, tokens := b.env.Token(t, url.Values{
		"grant_type": {"authorization_code"}, "code": {back.Query().Get("code")}, "client_id": {reg.ClientID},
		"redirect_uri": {redirect}, "code_verifier": {verifier},
	}, "", "")
	if status != http.StatusOK {
		t.Fatalf("token = %d %v", status, tokens)
	}
	return reg.ClientID, tokens["access_token"].(string)
}

// contract is contract/principal, read from the source tree.
func contract(file string) string {
	_, here, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(here), "../../../contract/principal", file)
}

// principalClaims is the contract's claims, as any channel carries them.
type principalClaims struct {
	Active      *bool                       `json:"active"`
	Issuer      string                      `json:"iss"`
	Subject     string                      `json:"sub"`
	Audience    oidc.Audience               `json:"aud"`
	ClientID    string                      `json:"client_id"`
	IssuedAt    int64                       `json:"iat"`
	Expiry      int64                       `json:"exp"`
	Version     string                      `json:"ver"`
	Kind        string                      `json:"kind"`
	Email       string                      `json:"email"`
	Name        string                      `json:"name"`
	Roles       []string                    `json:"roles"`
	Via         string                      `json:"via"`
	Actor       *struct{ Sub, Name string } `json:"act"`
	TwoFactor   bool                        `json:"two_factor"`
	TwoFactorAt int64                       `json:"two_factor_at"`
	SessionID   string                      `json:"sid"`
}

// verified is a JWT identity signed, typed typ, checked against its published keys: its claims.
func (e *Env) verified(t *testing.T, token, typ string) principalClaims {
	t.Helper()
	jws, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		t.Fatalf("%q: %v", token, err)
	}
	h := jws.Signatures[0].Protected
	if h.ExtraHeaders[jose.HeaderType] != typ {
		t.Fatalf("typ = %v, want %s", h.ExtraHeaders[jose.HeaderType], typ)
	}
	var keys jose.JSONWebKeySet
	e.Browser().Call(t, http.MethodGet, "/oidc/keys", nil, http.StatusOK, &keys)
	key := keys.Key(h.KeyID)
	if len(key) == 0 {
		t.Fatalf("no published key %q", h.KeyID)
	}
	payload, err := jws.Verify(key[0])
	if err != nil {
		t.Fatal(err)
	}
	var c principalClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		t.Fatal(err)
	}
	if c.Issuer != "https://"+Host || !e.Now().Before(time.Unix(c.Expiry, 0)) {
		t.Fatalf("claims = %+v", c)
	}
	return c
}

// stated is c as the contract states a Principal, validated against its schema.
func stated(t *testing.T, c principalClaims) map[string]any {
	t.Helper()
	p := map[string]any{"subject": c.Subject, "kind": c.Kind, "roles": c.Roles, "via": c.Via, "twoFactor": c.TwoFactor, "ver": c.Version}
	for field, value := range map[string]string{"email": c.Email, "name": c.Name, "sid": c.SessionID} {
		if value != "" {
			p[field] = value
		}
	}
	if c.Actor != nil {
		p["actor"] = map[string]any{"id": c.Actor.Sub, "name": c.Actor.Name}
	}
	if c.TwoFactorAt != 0 {
		p["twoFactorAt"] = time.Unix(c.TwoFactorAt, 0).UTC().Format(time.RFC3339)
	}
	validate(t, p)
	return p
}

// Principal is the signed Principal a Gate answer carries for clientID: verified against identity's
// published keys, typed, bound to the Client, lasting 60 seconds, and validated against the
// contract's schema. It answers the Principal as the contract states it.
func (e *Env) Principal(t *testing.T, a GateAnswer, clientID string) map[string]any {
	t.Helper()
	c := e.verified(t, a.Header.Get("X-Identity-Principal"), "identity-principal+jwt")
	if !slices.Equal(c.Audience, oidc.Audience{clientID}) || c.Expiry-c.IssuedAt != 60 {
		t.Fatalf("claims = %+v", c)
	}
	return stated(t, c)
}

// AccessTokenPrincipal is the Principal a JWT access token states for clientID: verified, typed JWT,
// naming clientID as client_id and audience, and validated against the contract's schema.
func (e *Env) AccessTokenPrincipal(t *testing.T, token, clientID string) map[string]any {
	t.Helper()
	c := e.verified(t, token, "JWT")
	if c.ClientID == "" || !slices.Equal(c.Audience, oidc.Audience{clientID}) {
		t.Fatalf("claims = %+v", c)
	}
	return stated(t, c)
}

// IntrospectionPrincipal is the Principal identity's introspection answer states for clientID:
// active, for clientID, and validated against the contract's schema.
func IntrospectionPrincipal(t *testing.T, answer map[string]any, clientID string) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(answer)
	var c principalClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if c.Active == nil || !*c.Active || c.ClientID != clientID || !slices.Equal(c.Audience, oidc.Audience{clientID}) {
		t.Fatalf("introspection = %v", answer)
	}
	return stated(t, c)
}

func validate(t *testing.T, p map[string]any) {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	schema, err := compiler.Compile(contract("principal.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p)
	doc, _ := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err := schema.Validate(doc); err != nil {
		t.Fatalf("the Principal %s breaks the contract: %v", raw, err)
	}
}

// AccessToken is b signing in to Client c through the code flow with PKCE, as a native app does,
// and answers the access token its code buys.
func (b *Browser) AccessToken(t *testing.T, c signindomain.Client) string {
	t.Helper()
	back, err := url.Parse(b.Follow(t, b.Follow(t, b.Authorize(t, c))))
	if err != nil || back.Query().Get("code") == "" {
		t.Fatalf("the sign-in came back to %v", back)
	}
	status, tokens := b.env.Token(t, url.Values{
		"grant_type": {"authorization_code"}, "code": {back.Query().Get("code")}, "client_id": {c.ID},
		"redirect_uri": {c.RedirectURIs[0]}, "code_verifier": {"dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"},
	}, "", "")
	if status != http.StatusOK {
		t.Fatalf("token = %d %v", status, tokens)
	}
	return tokens["access_token"].(string)
}
