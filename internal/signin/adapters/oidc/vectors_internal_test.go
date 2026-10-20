package oidc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/zitadel/oidc/v3/pkg/crypto"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/JorisJonkers-dev/identity/internal/platform/principalclaims"
)

// go test ./internal/signin/adapters/oidc -run TestTheCommittedTokenVectorsAreTheOnesIdentityMakes -update
var update = flag.Bool("update", false, "write the access-token and introspection vectors to contract/principal")

const (
	contract     = "../../../../contract/principal"
	vectorIssuer = "https://auth.example.test"
	vectorClient = "web"
)

var (
	vectorSigned = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	vectorAda    = uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
	vectorRobot  = uuid.MustParse("00000000-0000-4000-8000-0000000000b7")
	vectorSID    = uuid.MustParse("00000000-0000-4000-8000-0000000000c1")
)

// vectorKey is an op.SigningKey over one of the committed vector keys.
type vectorKey struct{ jwk jose.JSONWebKey }

func (k vectorKey) SignatureAlgorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (k vectorKey) Key() any                                    { return k.jwk.Key }
func (k vectorKey) ID() string                                  { return k.jwk.KeyID }

func vectorKeys(t *testing.T) (issuer, stranger vectorKey) {
	t.Helper()
	raw, err := os.ReadFile("../../../platform/principalclaims/testdata/vector-keys.test.json")
	if err != nil {
		t.Fatal(err)
	}
	var set jose.JSONWebKeySet
	if err := json.Unmarshal(raw, &set); err != nil {
		t.Fatal(err)
	}
	return vectorKey{set.Key("vector-issuer")[0]}, vectorKey{set.Key("vector-stranger")[0]}
}

// stated is a Principal as the contract's schema states it.
type stated struct {
	Subject     string       `json:"subject"`
	Kind        string       `json:"kind"`
	Email       string       `json:"email,omitempty"`
	Name        string       `json:"name,omitempty"`
	Roles       []string     `json:"roles"`
	Via         string       `json:"via"`
	Actor       *statedActor `json:"actor,omitempty"`
	TwoFactor   bool         `json:"twoFactor"`
	TwoFactorAt string       `json:"twoFactorAt,omitempty"`
	SID         string       `json:"sid,omitempty"`
	Ver         string       `json:"ver"`
}

type statedActor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// tokenVector is an access-token vector: a bearer token an app of Client receives at Now.
type tokenVector struct {
	Description string    `json:"description"`
	Issuer      string    `json:"issuer"`
	Client      string    `json:"client"`
	Now         time.Time `json:"now"`
	Token       string    `json:"token"`
	Principal   *stated   `json:"principal,omitempty"`
	Refusal     string    `json:"refusal,omitempty"`
}

// introspectionVector is an introspection vector: identity's answer to Client about a token.
type introspectionVector struct {
	Description string          `json:"description"`
	Client      string          `json:"client"`
	Answer      json.RawMessage `json:"answer"`
	Principal   *stated         `json:"principal,omitempty"`
	Refusal     string          `json:"refusal,omitempty"`
}

// readClaims is the contract's claims, as a reader decodes them from a token or an answer.
type readClaims struct {
	Active   *bool         `json:"active"`
	Issuer   string        `json:"iss"`
	Subject  string        `json:"sub"`
	Audience oidc.Audience `json:"aud"`
	ClientID string        `json:"client_id"`
	Expiry   int64         `json:"exp"`
	Version  string        `json:"ver"`
	Kind     string        `json:"kind"`
	Email    string        `json:"email"`
	Name     string        `json:"name"`
	Roles    []string      `json:"roles"`
	Via      string        `json:"via"`
	Actor    *struct {
		Subject string `json:"sub"`
		Name    string `json:"name"`
	} `json:"act"`
	TwoFactor   bool   `json:"two_factor"`
	TwoFactorAt int64  `json:"two_factor_at"`
	SessionID   string `json:"sid"`
}

func (c readClaims) stated() *stated {
	s := &stated{
		Subject: c.Subject, Kind: c.Kind, Email: c.Email, Name: c.Name, Roles: c.Roles, Via: c.Via,
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

// vectorStorage is the storage the vectors' Principals come from: Ada, fixed, holding user on web.
func vectorStorage() *storage {
	s, _, a := newStorage()
	a.ada = vectorAda
	return s
}

// accessToken is a JWT access token as zitadel/oidc signs one, typ JWT, with private as its
// Principal claims; edit changes the registered claims before signing.
func accessToken(t *testing.T, k vectorKey, typ string, private map[string]any, edit func(c *oidc.AccessTokenClaims)) string {
	t.Helper()
	sub := vectorAda.String()
	if private["kind"] == principalclaims.KindMachine {
		sub = vectorRobot.String()
	}
	c := oidc.NewAccessTokenClaims(vectorIssuer, sub, []string{vectorClient}, vectorSigned.Add(accessTokenLifetime), "at-1", vectorClient, 0)
	c.IssuedAt, c.NotBefore, c.Claims = oidc.FromTime(vectorSigned), oidc.FromTime(vectorSigned), private
	edit(c)
	signer, err := op.SignerFromKey(k)
	if typ != "JWT" {
		signer, err = jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: k.Key(), KeyID: k.ID()}}, (&jose.SignerOptions{}).WithType(jose.ContentType(typ)))
	}
	if err != nil {
		t.Fatal(err)
	}
	token, err := crypto.Sign(c, signer)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func unchanged(*oidc.AccessTokenClaims) {}

func expected(t *testing.T, private map[string]any, subject string) *stated {
	t.Helper()
	raw, _ := json.Marshal(private)
	var c readClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	c.Subject = subject
	return c.stated()
}

func tokenVectors(t *testing.T) map[string]tokenVector {
	t.Helper()
	issuer, stranger := vectorKeys(t)
	ctx := context.Background()
	s := vectorStorage()
	session, err := s.GetPrivateClaimsFromRequest(ctx, signedIn(vectorAda, vectorClient, vectorSID, vectorSigned.Add(-15*time.Minute)), nil)
	if err != nil {
		t.Fatal(err)
	}
	machine := principalclaims.Claims(principalclaims.Principal{Kind: principalclaims.KindMachine, Via: principalclaims.ViaMachine, Roles: []string{"backup"}})
	with := func(edit func(c map[string]any)) map[string]any {
		c := maps.Clone(session)
		edit(c)
		return c
	}
	valid := accessToken(t, issuer, "JWT", session, unchanged)
	parts := strings.Split(valid, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	parts[1] = base64.RawURLEncoding.EncodeToString(bytes.Replace(payload, []byte(`"roles":["user"]`), []byte(`"roles":["admin"]`), 1))
	idToken := accessToken(t, issuer, "JWT", session, func(c *oidc.AccessTokenClaims) { c.ClientID = "" })

	vectors := map[string]tokenVector{
		"session": {
			Description: "A User's access token from a sign-in whose session proved two-factor fifteen minutes before.",
			Token:       valid, Principal: expected(t, session, vectorAda.String()),
		},
		"machine": {
			Description: "A Machine Identity's access token: kind machine, and no email or name.",
			Token:       accessToken(t, issuer, "JWT", machine, unchanged), Principal: expected(t, machine, vectorRobot.String()),
		},
		"minor-version": {
			Description: "A later minor version's token is read, and the claim the reader does not know is ignored.",
			Token:       accessToken(t, issuer, "JWT", with(func(c map[string]any) { c["ver"], c["future"] = "1.2", "a later claim" }), unchanged),
			Principal:   expected(t, with(func(c map[string]any) { c["ver"] = "1.2" }), vectorAda.String()),
		},
		"wrong-typ": {
			Description: "The Gate's signed Principal, or any JWT not typed JWT, is no access token.",
			Token:       accessToken(t, issuer, "identity-principal+jwt", session, unchanged), Refusal: "wrong-typ",
		},
		"not-an-access-token": {
			Description: "An ID token, which names no client_id, is no access token.",
			Token:       idToken, Refusal: "not-an-access-token",
		},
		"wrong-issuer": {
			Description: "A token another issuer signed is refused.",
			Token:       accessToken(t, issuer, "JWT", session, func(c *oidc.AccessTokenClaims) { c.Issuer = "https://auth.example.org" }), Refusal: "wrong-issuer",
		},
		"wrong-audience": {
			Description: "A token for another Client is refused.",
			Token:       accessToken(t, issuer, "JWT", session, func(c *oidc.AccessTokenClaims) { c.Audience = oidc.Audience{"grafana"} }), Refusal: "wrong-audience",
		},
		"unknown-key": {
			Description: "A token signed by a key the Issuer's JWKS does not publish is refused.",
			Token:       accessToken(t, stranger, "JWT", session, unchanged), Refusal: "unknown-key",
		},
		"bad-signature": {
			Description: "A token whose claims were changed after signing is refused.",
			Token:       strings.Join(parts, "."), Refusal: "bad-signature",
		},
		"unknown-major": {
			Description: "A token stated in a major version the reader does not know is refused.",
			Token:       accessToken(t, issuer, "JWT", with(func(c map[string]any) { c["ver"] = "2.0" }), unchanged), Refusal: "unknown-major",
		},
	}
	expired := tokenVector{
		Description: "A token read at the moment it expires is refused.", Token: valid, Refusal: "expired",
		Now: vectorSigned.Add(accessTokenLifetime),
	}
	vectors["expired"] = expired
	for name, v := range vectors {
		v.Issuer, v.Client = vectorIssuer, vectorClient
		if v.Now.IsZero() {
			v.Now = vectorSigned.Add(time.Minute)
		}
		vectors[name] = v
	}
	return vectors
}

func introspectionVectors(t *testing.T) map[string]introspectionVector {
	t.Helper()
	ctx := context.Background()
	s := vectorStorage()
	expires := vectorSigned.Add(30 * 24 * time.Hour)
	answered := func(p principalclaims.Principal, edit func(c map[string]any)) (json.RawMessage, map[string]any) {
		claims, err := s.userClaims(ctx, vectorAda.String(), []string{"user"}, p)
		if err != nil {
			t.Fatal(err)
		}
		edit(claims)
		resp := new(oidc.IntrospectionResponse)
		answer(resp, vectorAda.String(), vectorClient, expires, claims)
		resp.Active = true
		raw, err := json.Marshal(resp)
		if err != nil {
			t.Fatal(err)
		}
		return raw, claims
	}
	none := func(map[string]any) {}
	personal, personalClaims := answered(principalclaims.Principal{Via: principalclaims.ViaPersonalToken}, none)
	tool, toolClaims := answered(principalclaims.Principal{Via: principalclaims.ViaTool, Actor: &principalclaims.Actor{ID: "mcp_6f1d0c2e", Name: "Claude Code"}}, none)
	major, _ := answered(principalclaims.Principal{Via: principalclaims.ViaPersonalToken}, func(c map[string]any) { c["ver"] = "2.0" })
	var other map[string]any
	_ = json.Unmarshal(personal, &other)
	other["aud"], other["client_id"] = []string{"grafana"}, "grafana"
	otherClient, _ := json.Marshal(other)

	vectors := map[string]introspectionVector{
		"personal-token": {Description: "A Personal Token: no session, and no two-factor.", Answer: personal, Principal: expected(t, personalClaims, vectorAda.String())},
		"tool":           {Description: "A Connected App's token, naming the tool acting for the User as act.", Answer: tool, Principal: expected(t, toolClaims, vectorAda.String())},
		"inactive":       {Description: "A token identity answers inactive carries no Principal.", Answer: json.RawMessage(`{"active":false}`), Refusal: "inactive"},
		"wrong-audience": {Description: "An answer for another Client is refused.", Answer: otherClient, Refusal: "wrong-audience"},
		"unknown-major":  {Description: "An answer stated in a major version the reader does not know is refused.", Answer: major, Refusal: "unknown-major"},
	}
	for name, v := range vectors {
		v.Client = vectorClient
		vectors[name] = v
	}
	return vectors
}

func encodedVector(t *testing.T, v any) []byte {
	t.Helper()
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// tokenChannels is the vector directories this package owns.
var tokenChannels = []string{"vectors/access-token", "vectors/introspection"}

func generatedVectors(t *testing.T) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for name, v := range tokenVectors(t) {
		out["vectors/access-token/"+name+".json"] = encodedVector(t, v)
	}
	for name, v := range introspectionVectors(t) {
		out["vectors/introspection/"+name+".json"] = encodedVector(t, v)
	}
	return out
}

func TestTheCommittedTokenVectorsAreTheOnesIdentityMakes(t *testing.T) {
	want := generatedVectors(t)
	if *update {
		for _, dir := range tokenChannels {
			_ = os.RemoveAll(filepath.Join(contract, dir))
			if err := os.MkdirAll(filepath.Join(contract, dir), 0o750); err != nil {
				t.Fatal(err)
			}
		}
		for path, b := range want {
			if err := os.WriteFile(filepath.Join(contract, path), b, 0o644); err != nil { //nolint:gosec // a committed fixture
				t.Fatal(err)
			}
		}
	}

	got := map[string][]byte{}
	committedFiles := os.DirFS(contract)
	for _, dir := range tokenChannels {
		_ = fs.WalkDir(committedFiles, dir, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				got[path], _ = fs.ReadFile(committedFiles, path)
			}
			return err
		})
	}

	if !maps.EqualFunc(got, want, bytes.Equal) {
		t.Fatal("contract/principal/vectors is stale for the token channels; run this test with -update")
	}
}

// committedVectors is every committed vector of dir, by name, decoded into T.
func committedVectors[T any](t *testing.T, dir string) map[string]T {
	t.Helper()
	committedFiles := os.DirFS(contract)
	paths, _ := fs.Glob(committedFiles, dir+"/*.json")
	out := map[string]T{}
	for _, path := range paths {
		raw, _ := fs.ReadFile(committedFiles, path)
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		out[strings.TrimSuffix(filepath.Base(path), ".json")] = v
	}
	if len(out) == 0 {
		t.Fatalf("no vectors in %s", dir)
	}
	return out
}

func committedJWKS(t *testing.T) jose.JSONWebKeySet {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(contract, "vectors/jwks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var keys jose.JSONWebKeySet
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	return keys
}

// readToken is a reader of the contract's access-token channel, written from mapping.json alone.
func readToken(v tokenVector, keys jose.JSONWebKeySet) (*stated, string) {
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
	var c readClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, "bad-signature"
	}
	switch {
	case c.ClientID == "":
		return nil, "not-an-access-token"
	case !strings.HasPrefix(c.Version, "1."):
		return nil, "unknown-major"
	case c.Issuer != v.Issuer:
		return nil, "wrong-issuer"
	case !slices.Equal(c.Audience, oidc.Audience{v.Client}):
		return nil, "wrong-audience"
	case !v.Now.Before(time.Unix(c.Expiry, 0)):
		return nil, "expired"
	}
	return c.stated(), ""
}

// readAnswer is a reader of the contract's introspection channel, written from mapping.json alone.
func readAnswer(v introspectionVector) (*stated, string) {
	var c readClaims
	if err := json.Unmarshal(v.Answer, &c); err != nil || c.Active == nil || !*c.Active {
		return nil, "inactive"
	}
	switch {
	case !strings.HasPrefix(c.Version, "1."):
		return nil, "unknown-major"
	case c.ClientID != v.Client || !slices.Equal(c.Audience, oidc.Audience{v.Client}):
		return nil, "wrong-audience"
	}
	return c.stated(), ""
}

func statesTheSame(t *testing.T, got, want *stated) bool {
	t.Helper()
	return bytes.Equal(encodedVector(t, got), encodedVector(t, want))
}

func TestEveryAccessTokenVectorIsWhatAReaderOfTheContractConcludes(t *testing.T) {
	keys := committedJWKS(t)
	for name, v := range committedVectors[tokenVector](t, "vectors/access-token") {
		t.Run(name, func(t *testing.T) {
			if p, refusal := readToken(v, keys); refusal != v.Refusal || !statesTheSame(t, p, v.Principal) {
				t.Fatalf("read %+v, refused %q; the vector says %+v, refused %q", p, refusal, v.Principal, v.Refusal)
			}
		})
	}
}

func TestEveryIntrospectionVectorIsWhatAReaderOfTheContractConcludes(t *testing.T) {
	for name, v := range committedVectors[introspectionVector](t, "vectors/introspection") {
		t.Run(name, func(t *testing.T) {
			if p, refusal := readAnswer(v); refusal != v.Refusal || !statesTheSame(t, p, v.Principal) {
				t.Fatalf("read %+v, refused %q; the vector says %+v, refused %q", p, refusal, v.Principal, v.Refusal)
			}
		})
	}
}

func TestEveryTokenChannelPrincipalValidatesAgainstTheSchema(t *testing.T) {
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	schema, err := c.Compile(filepath.Join(contract, "principal.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	principals := map[string]*stated{}
	for name, v := range committedVectors[tokenVector](t, "vectors/access-token") {
		principals["access-token/"+name] = v.Principal
	}
	for name, v := range committedVectors[introspectionVector](t, "vectors/introspection") {
		principals["introspection/"+name] = v.Principal
	}
	for name, p := range principals {
		if p == nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			doc, _ := jsonschema.UnmarshalJSON(bytes.NewReader(encodedVector(t, p)))
			if err := schema.Validate(doc); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTheTokenVectorsRefuseEveryWayTheMappingNames(t *testing.T) {
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
	for _, c := range []struct {
		name    string
		named   map[string]string
		refused func() []string
	}{
		{"access-token", mapping.AccessToken.Refusals, func() []string {
			var r []string
			for _, v := range committedVectors[tokenVector](t, "vectors/access-token") {
				if v.Refusal != "" {
					r = append(r, v.Refusal)
				}
			}
			return r
		}},
		{"introspection", mapping.Introspection.Refusals, func() []string {
			var r []string
			for _, v := range committedVectors[introspectionVector](t, "vectors/introspection") {
				if v.Refusal != "" {
					r = append(r, v.Refusal)
				}
			}
			return r
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			want := slices.Sorted(maps.Keys(c.named))
			if got := slices.Sorted(slices.Values(c.refused())); len(want) == 0 || !slices.Equal(got, want) {
				t.Fatalf("vectors refuse %v; the mapping names %v", got, want)
			}
		})
	}
}
