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
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// GateSignIn is b signing in to a gated host through the Gate's button, and answers the cookie the
// host's /.auth/callback sets.
func (b *Browser) GateSignIn(t *testing.T, host string) string {
	t.Helper()
	next, err := url.Parse(b.Follow(t, "/gate/start?"+url.Values{"host": {host}, "state": {"s"}, "rd": {"/"}}.Encode()))
	if err != nil || next.Host != host {
		t.Fatalf("start went to %v", next)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, next.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: "__Host-identity_gate_state", Value: "s"}) //nolint:gosec // a request's cookie carries no attributes
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

// Principal is the signed Principal a Gate answer carries for clientID: verified against identity's
// published keys, typed, bound to the Client, lasting 60 seconds, and validated against the
// contract's schema. It answers the Principal as the contract states it.
func (e *Env) Principal(t *testing.T, a GateAnswer, clientID string) map[string]any {
	t.Helper()
	jws, err := jose.ParseSigned(a.Header.Get("X-Identity-Principal"), []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		t.Fatalf("X-Identity-Principal: %v", err)
	}
	h := jws.Signatures[0].Protected
	if h.ExtraHeaders[jose.HeaderType] != "identity-principal+jwt" {
		t.Fatalf("typ = %v", h.ExtraHeaders[jose.HeaderType])
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
	var c struct {
		Issuer      string                      `json:"iss"`
		Subject     string                      `json:"sub"`
		Audience    string                      `json:"aud"`
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
	if err := json.Unmarshal(payload, &c); err != nil {
		t.Fatal(err)
	}
	if c.Issuer != "https://"+Host || c.Audience != clientID || c.Expiry-c.IssuedAt != 60 || !e.Now().Before(time.Unix(c.Expiry, 0)) {
		t.Fatalf("claims = %+v", c)
	}
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
