package principal

import (
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/zitadel/oidc/v3/pkg/crypto"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	principalclaims "github.com/JorisJonkers-dev/identity/internal/platform/claims"
)

// The access-token and introspection vectors state the same Principals as the signed header, made
// by the code identity issues them with: principalclaims, inside zitadel/oidc's own access token.

// tokenVector is one access-token conformance vector: a JWT access token identity issued, read by
// an app of Client at Now, and the Principal or refusal any adapter must conclude from it.
type tokenVector struct {
	Description string    `json:"description"`
	Issuer      string    `json:"issuer"`
	Client      string    `json:"client"`
	Now         time.Time `json:"now"`
	Token       string    `json:"token"`
	Principal   *stated   `json:"principal,omitempty"`
	Refusal     string    `json:"refusal,omitempty"`
}

// introspectionVector is one introspection conformance vector: identity's answer to Client's check
// of a token, and the Principal or refusal any adapter must conclude from it.
type introspectionVector struct {
	Description string          `json:"description"`
	Client      string          `json:"client"`
	Answer      json.RawMessage `json:"answer"`
	Principal   *stated         `json:"principal,omitempty"`
	Refusal     string          `json:"refusal,omitempty"`
}

// accessTokenLifetime is how long identity's access tokens last.
const accessTokenLifetime = 15 * time.Minute

var (
	adaSession = principalclaims.Principal{
		Subject: "00000000-0000-4000-8000-0000000000a1", Kind: principalclaims.KindUser, Email: "ada@example.org",
		Name: "Ada Lovelace", Roles: []string{"user", "editor"}, Via: principalclaims.ViaSession,
		SessionID: "00000000-0000-4000-8000-0000000000c1",
	}
	backupMachine = principalclaims.Principal{
		Subject: "00000000-0000-4000-8000-0000000000b7", Kind: principalclaims.KindMachine,
		Roles: []string{"backup"}, Via: principalclaims.ViaMachine,
	}
)

// statedOf is what a reader of the contract must conclude from p.
func statedOf(p principalclaims.Principal) *stated {
	c := principalclaims.Of(p)
	s := &stated{
		Subject: p.Subject, Kind: c.Kind, Email: c.Email, Name: c.Name, Roles: c.Roles, Via: c.Via,
		TwoFactor: c.TwoFactor, SID: c.SessionID, Ver: c.Version,
	}
	if c.Actor != nil {
		s.Actor = &statedActor{ID: c.Actor.Subject, Name: c.Actor.Name}
	}
	if c.TwoFactor {
		s.TwoFactorAt = time.Unix(c.TwoFactorAt, 0).UTC().Format(time.RFC3339)
	}
	return s
}

// accessToken is p's access token for vectorClient, issued at signedAt to clientID, as zitadel/oidc
// makes it with the claims identity adds; edit changes it before it is signed.
func accessToken(t *testing.T, k key, p principalclaims.Principal, clientID string, edit func(*oidc.AccessTokenClaims)) string {
	t.Helper()
	c := oidc.NewAccessTokenClaims(vectorIssuer, p.Subject, []string{vectorClient}, signedAt.Add(accessTokenLifetime),
		"00000000-0000-4000-8000-0000000000d1", clientID, 0)
	c.IssuedAt, c.NotBefore = oidc.FromTime(signedAt), oidc.FromTime(signedAt)
	c.Claims = principalclaims.Private(p)
	if edit != nil {
		edit(c)
	}
	signer, err := op.SignerFromKey(k)
	if err != nil {
		t.Fatal(err)
	}
	token, err := crypto.Sign(c, signer)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func accessTokenVectors(t *testing.T) map[string]tokenVector {
	t.Helper()
	issuer, stranger := vectorKeys(t)
	twoFactor := adaSession
	twoFactor.TwoFactorAt = signedAt.Add(-15 * time.Minute)
	allowed := func(description string, p principalclaims.Principal, clientID string) tokenVector {
		return tokenVector{Description: description, Token: accessToken(t, issuer, p, clientID, nil), Principal: statedOf(p)}
	}
	refused := func(description, refusal, token string) tokenVector {
		return tokenVector{Description: description, Token: token, Refusal: refusal}
	}
	edited := func(edit func(*oidc.AccessTokenClaims)) string {
		return accessToken(t, issuer, adaSession, vectorClient, edit)
	}
	valid := edited(nil)
	parts := strings.Split(edited(func(c *oidc.AccessTokenClaims) { c.Claims["roles"] = []string{"admin"} }), ".")
	parts[2] = strings.Split(valid, ".")[2]
	header, err := New(vectorIssuer, ring{issuer}).Sign(adaSays, vectorClient, signedAt)
	if err != nil {
		t.Fatal(err)
	}

	vectors := map[string]tokenVector{
		"session":            allowed("A User's access token from the code flow, stating the session that signed them in.", adaSession, vectorClient),
		"session-two-factor": allowed("A User's access token whose session proved two-factor fifteen minutes before it was issued.", twoFactor, vectorClient),
		"machine":            allowed("A Machine Identity's access token, issued to the Machine Identity for the Client it named.", backupMachine, backupMachine.Subject),
		"wrong-typ": refused("A signed Principal header presented as an access token is refused.",
			"wrong-typ", header),
		"wrong-issuer": refused("An access token another issuer states is refused.",
			"wrong-issuer", edited(func(c *oidc.AccessTokenClaims) { c.Issuer = "https://auth.example.org" })),
		"wrong-audience": refused("An access token for another Client is refused.",
			"wrong-audience", edited(func(c *oidc.AccessTokenClaims) { c.Audience = oidc.Audience{"grafana"} })),
		"unknown-key": refused("An access token signed by a key the Issuer's JWKS does not publish is refused.",
			"unknown-key", accessToken(t, stranger, adaSession, vectorClient, nil)),
		"bad-signature": refused("An access token whose claims were changed after signing is refused.",
			"bad-signature", strings.Join(parts, ".")),
		"unknown-major": refused("An access token stated in a major version the reader does not know is refused.",
			"unknown-major", edited(func(c *oidc.AccessTokenClaims) { c.Claims["ver"] = "2.0" })),
	}
	expired := refused("An access token read at the moment it expires is refused.", "expired", valid)
	expired.Now = signedAt.Add(accessTokenLifetime)
	vectors["expired"] = expired
	for name, v := range vectors {
		v.Issuer, v.Client = vectorIssuer, vectorClient
		if v.Now.IsZero() {
			v.Now = signedAt.Add(30 * time.Second)
		}
		vectors[name] = v
	}
	return vectors
}

func introspectionVectors(t *testing.T) map[string]introspectionVector {
	t.Helper()
	answer := func(p principalclaims.Principal, issued time.Time) json.RawMessage {
		resp := new(oidc.IntrospectionResponse)
		principalclaims.Introspect(resp, p, vectorClient, issued, signedAt.Add(90*24*time.Hour))
		resp.Active = true
		raw, err := json.Marshal(resp)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	allowed := func(description string, p principalclaims.Principal, issued time.Time) introspectionVector {
		return introspectionVector{Description: description, Answer: answer(p, issued), Principal: statedOf(p)}
	}
	personal := adaSession
	personal.Via, personal.SessionID = principalclaims.ViaPersonalToken, ""
	tool := personal
	tool.Via, tool.Actor = principalclaims.ViaTool, principalclaims.Actor{ID: "mcp_6f1d0c2e", Name: "Claude Code"}
	var major map[string]any
	if err := json.Unmarshal(answer(personal, signedAt), &major); err != nil {
		t.Fatal(err)
	}
	major["ver"] = "2.0"
	majorAnswer, _ := json.Marshal(major)

	vectors := map[string]introspectionVector{
		"personal-token": allowed("A Personal Token, checked by the Client it is for: no session, and no two-factor.", personal, signedAt),
		"tool":           allowed("A Connected App's token: a tool acting for a User, named as the actor.", tool, time.Time{}),
		"inactive": {
			Description: "A token that is revoked, expired, for another Client or unknown is inactive, and states no one.",
			Answer:      json.RawMessage(`{"active":false}`), Refusal: "inactive",
		},
		"unknown-major": {
			Description: "An answer stated in a major version the reader does not know is refused.",
			Answer:      majorAnswer, Refusal: "unknown-major",
		},
	}
	for name, v := range vectors {
		v.Client = vectorClient
		vectors[name] = v
	}
	return vectors
}

// readAccessToken is a reader of the contract, written from mapping.json alone: the Principal an
// access token states for v's Client at v's time, or why it is refused.
func readAccessToken(v tokenVector, keys jose.JSONWebKeySet) (*stated, string) {
	jws, err := jose.ParseSigned(v.Token, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return nil, "bad-signature"
	}
	h := jws.Signatures[0].Protected
	if h.ExtraHeaders[jose.HeaderType] != "JWT" {
		return nil, "wrong-typ"
	}
	k := keys.Key(h.KeyID)
	if len(k) == 0 {
		return nil, "unknown-key"
	}
	payload, err := jws.Verify(k[0])
	if err != nil {
		return nil, "bad-signature"
	}
	var c struct {
		Issuer   string       `json:"iss"`
		Subject  string       `json:"sub"`
		Audience jwt.Audience `json:"aud"`
		Expiry   int64        `json:"exp"`
		principalclaims.Claims
	}
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, "bad-signature"
	}
	switch {
	case !strings.HasPrefix(c.Version, "1."):
		return nil, "unknown-major"
	case c.Issuer != v.Issuer:
		return nil, "wrong-issuer"
	case !c.Audience.Contains(v.Client):
		return nil, "wrong-audience"
	case !v.Now.Before(time.Unix(c.Expiry, 0)):
		return nil, "expired"
	}
	return statedFromClaims(c.Subject, c.Claims), ""
}

// readIntrospection is a reader of the contract, written from mapping.json alone: the Principal an
// introspection answer states, or why it is refused.
func readIntrospection(v introspectionVector) (*stated, string) {
	var c struct {
		Active  bool   `json:"active"`
		Subject string `json:"sub"`
		principalclaims.Claims
	}
	if err := json.Unmarshal(v.Answer, &c); err != nil || !c.Active {
		return nil, "inactive"
	}
	if !strings.HasPrefix(c.Version, "1.") {
		return nil, "unknown-major"
	}
	return statedFromClaims(c.Subject, c.Claims), ""
}

func statedFromClaims(subject string, c principalclaims.Claims) *stated {
	s := &stated{
		Subject: subject, Kind: c.Kind, Email: c.Email, Name: c.Name, Roles: c.Roles, Via: c.Via,
		TwoFactor: c.TwoFactor, SID: c.SessionID, Ver: c.Version,
	}
	if c.Actor != nil {
		s.Actor = &statedActor{ID: c.Actor.Subject, Name: c.Actor.Name}
	}
	if c.TwoFactorAt != 0 {
		s.TwoFactorAt = time.Unix(c.TwoFactorAt, 0).UTC().Format(time.RFC3339)
	}
	return s
}

// committedChannel is every committed vector under vectors/<channel>, by name.
func committedChannel[V any](t *testing.T, channel string) map[string]V {
	t.Helper()
	committedFiles := os.DirFS(contract)
	paths, _ := fs.Glob(committedFiles, "vectors/"+channel+"/*.json")
	out := map[string]V{}
	for _, path := range paths {
		raw, _ := fs.ReadFile(committedFiles, path)
		var v V
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		out[strings.TrimSuffix(filepath.Base(path), ".json")] = v
	}
	if len(out) == 0 {
		t.Fatalf("no %s vectors", channel)
	}
	return out
}

func TestEveryAccessTokenVectorIsWhatAReaderOfTheContractConcludes(t *testing.T) {
	keys := jwks(t)
	for name, v := range committedChannel[tokenVector](t, "access-token") {
		t.Run(name, func(t *testing.T) {
			p, refusal := readAccessToken(v, keys)
			if refusal != v.Refusal || string(encoded(t, p)) != string(encoded(t, v.Principal)) {
				t.Fatalf("read %+v, refused %q; the vector says %+v, refused %q", p, refusal, v.Principal, v.Refusal)
			}
		})
	}
}

func TestEveryIntrospectionVectorIsWhatAReaderOfTheContractConcludes(t *testing.T) {
	for name, v := range committedChannel[introspectionVector](t, "introspection") {
		t.Run(name, func(t *testing.T) {
			p, refusal := readIntrospection(v)
			if refusal != v.Refusal || string(encoded(t, p)) != string(encoded(t, v.Principal)) {
				t.Fatalf("read %+v, refused %q; the vector says %+v, refused %q", p, refusal, v.Principal, v.Refusal)
			}
		})
	}
}

func TestAMachinesAccessTokenVectorCarriesNoEmailOrName(t *testing.T) {
	v := committedChannel[tokenVector](t, "access-token")["machine"]
	payload, _ := base64.RawURLEncoding.DecodeString(strings.Split(v.Token, ".")[1])
	var c map[string]any
	if err := json.Unmarshal(payload, &c); err != nil {
		t.Fatal(err)
	}
	if _, has := c["email"]; has || c["name"] != nil || c["kind"] != "machine" {
		t.Fatalf("claims = %v", c)
	}
}

func TestEveryChannelsVectorsRefuseEveryWayTheMappingNamesForIt(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(contract, "mapping.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mapping struct {
		AccessToken   struct{ Refusals map[string]string } `json:"accessToken"`
		Introspection struct{ Refusals map[string]string } `json:"introspection"`
	}
	if err := json.Unmarshal(raw, &mapping); err != nil {
		t.Fatal(err)
	}
	refusals := func(named map[string]string) []string {
		out := make([]string, 0, len(named))
		for refusal := range named {
			out = append(out, refusal)
		}
		return slices.Sorted(slices.Values(out))
	}
	var tokens, answers []string
	for _, v := range committedChannel[tokenVector](t, "access-token") {
		if v.Refusal != "" {
			tokens = append(tokens, v.Refusal)
		}
	}
	for _, v := range committedChannel[introspectionVector](t, "introspection") {
		if v.Refusal != "" {
			answers = append(answers, v.Refusal)
		}
	}

	if !slices.Equal(slices.Sorted(slices.Values(tokens)), refusals(mapping.AccessToken.Refusals)) ||
		!slices.Equal(slices.Sorted(slices.Values(answers)), refusals(mapping.Introspection.Refusals)) {
		t.Fatalf("access tokens refuse %v, introspection %v; the mapping names %v and %v",
			tokens, answers, mapping.AccessToken.Refusals, mapping.Introspection.Refusals)
	}
}
