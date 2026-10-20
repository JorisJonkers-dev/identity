package principal

import (
	"bytes"
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

	"github.com/JorisJonkers-dev/identity/internal/gate/app"
)

// go test ./internal/gate/adapters/principal -run TestTheCommittedVectorsAreTheOnesIdentityStates -update
var update = flag.Bool("update", false, "write the conformance vectors to contract/principal")

const (
	contract     = "../../../../contract/principal"
	vectorIssuer = "https://auth.example.test"
	vectorClient = "wiki"
	signedHeader = "X-Identity-Principal"
)

// vector is one signed-header conformance vector: a request the Gate answered, read by an app of
// vectorClient at Now, and the Principal or refusal any adapter must conclude from it.
type vector struct {
	Description string            `json:"description"`
	Issuer      string            `json:"issuer"`
	Client      string            `json:"client"`
	Now         time.Time         `json:"now"`
	Headers     map[string]string `json:"headers"`
	Principal   *stated           `json:"principal,omitempty"`
	Refusal     string            `json:"refusal,omitempty"`
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

// statedFrom is what a reader of the contract must conclude from a Verdict the Gate signed.
func statedFrom(v app.Verdict, ver string) *stated {
	s := &stated{
		Subject: v.UserID.String(), Kind: v.Kind, Email: v.Email, Name: v.Name, Roles: v.Roles, Via: v.Via, Ver: ver,
	}
	if v.Via == app.ViaTool {
		s.Actor = &statedActor{ID: v.Actor.ID, Name: v.Actor.Name}
	}
	if !v.TwoFactorAt.IsZero() {
		s.TwoFactor, s.TwoFactorAt = true, v.TwoFactorAt.UTC().Format(time.RFC3339)
	}
	if v.Via == app.ViaSession {
		s.SID = v.SessionID.String()
	}
	return s
}

// generated is every vector, by name, signed with the committed keys at signedAt.
func generated(t *testing.T) map[string]vector {
	t.Helper()
	issuer, stranger := vectorKeys(t)
	signer := New(vectorIssuer, ring{issuer})
	readAt := signedAt.Add(30 * time.Second)
	signedBy := func(k key, typ string, c any) string {
		token, err := sign(k, typ, c)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	answer := func(v app.Verdict, token string) map[string]string {
		h := v.Headers()
		if token != "" {
			h[signedHeader] = token
		}
		return h
	}
	allowed := func(description string, v app.Verdict) vector {
		token, err := signer.Sign(v, vectorClient, signedAt)
		if err != nil {
			t.Fatal(err)
		}
		return vector{Description: description, Headers: answer(v, token), Principal: statedFrom(v, Version)}
	}
	refused := func(description, refusal string, headers map[string]string) vector {
		return vector{Description: description, Headers: headers, Refusal: refusal}
	}
	with := func(edit func(c *claims)) claims {
		c := claimsOf(adaSays, vectorIssuer, vectorClient, signedAt)
		edit(&c)
		return c
	}

	twoFactor := adaSays
	twoFactor.TwoFactorAt = signedAt.Add(-15 * time.Minute)
	personal := adaSays
	personal.Via, personal.SessionID = app.ViaPersonalToken, uuid.Nil
	tool := personal
	tool.Via, tool.Actor = app.ViaTool, app.Actor{ID: "mcp_6f1d0c2e", Name: "Claude Code"}
	machine := app.Verdict{
		UserID: uuid.MustParse("00000000-0000-4000-8000-0000000000b7"), Roles: []string{"backup"},
		Kind: "machine", Via: "machine",
	}
	spoofed := allowed("A caller's own plain headers never stand in for the signed Principal.", adaSays)
	spoofed.Headers["X-Auth-Request-User"] = "00000000-0000-4000-8000-0000000000e1"
	spoofed.Headers["X-Auth-Request-Roles"] = "admin"
	future := struct {
		claims
		Future string `json:"future"`
	}{with(func(c *claims) { c.Version = "1.2" }), "a field a later minor version adds"}
	minor := vector{
		Description: "A later minor version's Principal is read, and the field the reader does not know is ignored.",
		Headers:     answer(adaSays, signedBy(issuer, Type, future)),
		Principal:   statedFrom(adaSays, "1.2"),
	}
	valid := signedBy(issuer, Type, claimsOf(adaSays, vectorIssuer, vectorClient, signedAt))
	parts := strings.Split(valid, ".")
	forged, _ := json.Marshal(with(func(c *claims) { c.Roles = []string{"admin"} }))
	parts[1] = base64.RawURLEncoding.EncodeToString(forged)

	vectors := map[string]vector{
		"session":               allowed("A User signed in with a Single sign-on session that never proved two-factor.", adaSays),
		"session-two-factor":    allowed("A User whose session proved two-factor fifteen minutes before the Gate signed.", twoFactor),
		"personal-token":        allowed("A User calling with a Personal Token: no session, and no two-factor.", personal),
		"tool":                  allowed("A tool acting for a User, named as the actor.", tool),
		"machine":               allowed("A Machine Identity calling with its own credential.", machine),
		"spoofed-plain-headers": spoofed,
		"minor-version":         minor,
		"missing": refused("Plain headers without the signed Principal are refused, whatever they say.",
			"missing", answer(adaSays, "")),
		"wrong-typ": refused("A JWT typed as anything but identity-principal+jwt, such as an access token, is refused.",
			"wrong-typ", answer(adaSays, signedBy(issuer, "at+jwt", claimsOf(adaSays, vectorIssuer, vectorClient, signedAt)))),
		"wrong-issuer": refused("A Principal another issuer states is refused.",
			"wrong-issuer", answer(adaSays, signedBy(issuer, Type, with(func(c *claims) { c.Issuer = "https://auth.example.org" })))),
		"wrong-audience": refused("A Principal signed for another Client is refused.",
			"wrong-audience", answer(adaSays, signedBy(issuer, Type, with(func(c *claims) { c.Audience = "grafana" })))),
		"unknown-key": refused("A Principal signed by a key the Issuer's JWKS does not publish is refused.",
			"unknown-key", answer(adaSays, signedBy(stranger, Type, claimsOf(adaSays, vectorIssuer, vectorClient, signedAt)))),
		"bad-signature": refused("A Principal whose claims were changed after signing is refused.",
			"bad-signature", answer(adaSays, strings.Join(parts, "."))),
		"unknown-major": refused("A Principal stated in a major version the reader does not know is refused.",
			"unknown-major", answer(adaSays, signedBy(issuer, Type, with(func(c *claims) { c.Version = "2.0" })))),
	}
	expired := refused("A Principal read at the moment it expires is refused.",
		"expired", answer(adaSays, valid))
	expired.Now = signedAt.Add(app.PrincipalLifetime)
	vectors["expired"] = expired
	for name, v := range vectors {
		v.Issuer, v.Client = vectorIssuer, vectorClient
		if v.Now.IsZero() {
			v.Now = readAt
		}
		vectors[name] = v
	}
	return vectors
}

// jwks is the Issuer's published keys the vectors are read with.
func jwks(t *testing.T) jose.JSONWebKeySet {
	t.Helper()
	issuer, _ := vectorKeys(t)
	return jose.JSONWebKeySet{Keys: []jose.JSONWebKey{issuer.jwk.Public()}}
}

func encoded(t *testing.T, v any) []byte {
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

// files is every vector file the contract holds, by its path under the contract.
func files(t *testing.T) map[string][]byte {
	t.Helper()
	out := map[string][]byte{"vectors/jwks.json": encoded(t, jwks(t))}
	for name, v := range generated(t) {
		out["vectors/signed-header/"+name+".json"] = encoded(t, v)
	}
	for name, v := range accessTokenVectors(t) {
		out["vectors/access-token/"+name+".json"] = encoded(t, v)
	}
	for name, v := range introspectionVectors(t) {
		out["vectors/introspection/"+name+".json"] = encoded(t, v)
	}
	return out
}

func TestTheCommittedVectorsAreTheOnesIdentityStates(t *testing.T) {
	want := files(t)
	if *update {
		_ = os.RemoveAll(filepath.Join(contract, "vectors"))
		for path, b := range want {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(contract, path)), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(contract, path), b, 0o644); err != nil { //nolint:gosec // a committed fixture
				t.Fatal(err)
			}
		}
	}

	got := map[string][]byte{}
	committedFiles := os.DirFS(contract)
	_ = fs.WalkDir(committedFiles, "vectors", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			got[path], _ = fs.ReadFile(committedFiles, path)
		}
		return err
	})

	if !maps.EqualFunc(got, want, bytes.Equal) {
		t.Fatal("contract/principal/vectors is stale; run this test with -update")
	}
}

// committed is every committed signed-header vector, by name.
func committed(t *testing.T) map[string]vector {
	t.Helper()
	committedFiles := os.DirFS(contract)
	paths, _ := fs.Glob(committedFiles, "vectors/signed-header/*.json")
	out := map[string]vector{}
	for _, path := range paths {
		raw, _ := fs.ReadFile(committedFiles, path)
		var v vector
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		out[strings.TrimSuffix(filepath.Base(path), ".json")] = v
	}
	if len(out) == 0 {
		t.Fatal("no vectors")
	}
	return out
}

// read is a reader of the contract, written from mapping.json alone: the Principal a request
// carries for client at now, or why it is refused.
func read(v vector, keys jose.JSONWebKeySet) (*stated, string) {
	token := v.Headers[signedHeader]
	if token == "" {
		return nil, "missing"
	}
	jws, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return nil, "bad-signature"
	}
	h := jws.Signatures[0].Protected
	if h.ExtraHeaders[jose.HeaderType] != Type {
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
	var c claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, "bad-signature"
	}
	switch {
	case !strings.HasPrefix(c.Version, "1."):
		return nil, "unknown-major"
	case c.Issuer != v.Issuer:
		return nil, "wrong-issuer"
	case c.Audience != v.Client:
		return nil, "wrong-audience"
	case !v.Now.Before(time.Unix(c.Expiry, 0)):
		return nil, "expired"
	}
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
	return s, ""
}

func TestEveryVectorIsWhatAReaderOfTheContractConcludes(t *testing.T) {
	keys := jwks(t)
	for name, v := range committed(t) {
		t.Run(name, func(t *testing.T) {
			p, refusal := read(v, keys)
			if refusal != v.Refusal || string(encoded(t, p)) != string(encoded(t, v.Principal)) {
				t.Fatalf("read %+v, refused %q; the vector says %+v, refused %q", p, refusal, v.Principal, v.Refusal)
			}
		})
	}
}

func TestEveryVectorsPrincipalValidatesAgainstTheSchema(t *testing.T) {
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	schema, err := c.Compile(filepath.Join(contract, "principal.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	principals := map[string]*stated{}
	for name, v := range committed(t) {
		principals["signed-header/"+name] = v.Principal
	}
	for name, v := range committedChannel[tokenVector](t, "access-token") {
		principals["access-token/"+name] = v.Principal
	}
	for name, v := range committedChannel[introspectionVector](t, "introspection") {
		principals["introspection/"+name] = v.Principal
	}
	for name, p := range principals {
		if p == nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			doc, _ := jsonschema.UnmarshalJSON(bytes.NewReader(encoded(t, p)))
			if err := schema.Validate(doc); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTheVectorsRefuseEveryWayTheMappingNames(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(contract, "mapping.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mapping struct {
		Refusals map[string]string `json:"refusals"`
	}
	if err := json.Unmarshal(raw, &mapping); err != nil {
		t.Fatal(err)
	}
	var refused []string
	for _, v := range committed(t) {
		if v.Refusal != "" {
			refused = append(refused, v.Refusal)
		}
	}

	if want := slices.Sorted(maps.Keys(mapping.Refusals)); !slices.Equal(slices.Sorted(slices.Values(refused)), want) {
		t.Fatalf("vectors refuse %v; the mapping names %v", refused, want)
	}
}
