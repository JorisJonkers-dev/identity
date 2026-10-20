package principal

import (
	"crypto/rsa"
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/JorisJonkers-dev/identity/internal/gate/app"
)

// key is an op.SigningKey over a JWK.
type key struct{ jwk jose.JSONWebKey }

func (k key) SignatureAlgorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (k key) Key() any                                    { return k.jwk.Key }
func (k key) ID() string                                  { return k.jwk.KeyID }

// ring is Keys signing with one key.
type ring struct{ key key }

func (r ring) Signing() op.SigningKey { return r.key }

// vectorKeys are the committed keys the vectors are signed with: the Issuer's, and a stranger's no
// JWKS publishes.
func vectorKeys(t *testing.T) (issuer, stranger key) {
	t.Helper()
	raw, err := os.ReadFile("testdata/vector-keys.test.json")
	if err != nil {
		t.Fatal(err)
	}
	var set jose.JSONWebKeySet
	if err := json.Unmarshal(raw, &set); err != nil {
		t.Fatal(err)
	}
	return key{set.Key("vector-issuer")[0]}, key{set.Key("vector-stranger")[0]}
}

var (
	signedAt = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	adaSays  = app.Verdict{
		UserID: uuid.MustParse("00000000-0000-4000-8000-0000000000a1"), Email: "ada@example.org", Name: "Ada Lovelace",
		Roles: []string{"user", "editor"}, Kind: app.KindUser, Via: app.ViaSession,
		SessionID: uuid.MustParse("00000000-0000-4000-8000-0000000000c1"),
	}
)

// verified is the JWT a Signer made, verified with k: its typ, its key id and its claims.
func verified(t *testing.T, token string, k key) (typ, kid string, c map[string]any) {
	t.Helper()
	jws, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := jws.Verify(&k.jwk.Key.(*rsa.PrivateKey).PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &c); err != nil {
		t.Fatal(err)
	}
	h := jws.Signatures[0].Protected
	return h.ExtraHeaders[jose.HeaderType].(string), h.KeyID, c
}

func TestASignedPrincipalIsTypedSoItNeverPassesAsAnAccessToken(t *testing.T) {
	k, _ := vectorKeys(t)

	token, err := New("https://auth.example.test", ring{k}).Sign(adaSays, "wiki", signedAt)

	if typ, kid, _ := verified(t, token, k); err != nil || typ != "identity-principal+jwt" || kid != "vector-issuer" {
		t.Fatalf("typ = %q, kid = %q, err = %v", typ, kid, err)
	}
}

func TestASignedPrincipalIsBoundToItsClientAndLastsSixtySeconds(t *testing.T) {
	k, _ := vectorKeys(t)

	token, _ := New("https://auth.example.test", ring{k}).Sign(adaSays, "wiki", signedAt)

	_, _, c := verified(t, token, k)
	if c["iss"] != "https://auth.example.test" || c["aud"] != "wiki" || c["iat"] != float64(signedAt.Unix()) || c["exp"] != float64(signedAt.Unix()+60) || c["ver"] != "1.0" {
		t.Fatalf("claims = %v", c)
	}
}

func TestASessionsPrincipalNamesItsSessionAndNoActor(t *testing.T) {
	k, _ := vectorKeys(t)

	token, _ := New("https://auth.example.test", ring{k}).Sign(adaSays, "wiki", signedAt)

	_, _, c := verified(t, token, k)
	if c["sid"] != adaSays.SessionID.String() || c["act"] != nil || c["two_factor"] != false || c["two_factor_at"] != nil {
		t.Fatalf("claims = %v", c)
	}
}

func TestAPrincipalWithoutRolesStatesAnEmptyList(t *testing.T) {
	k, _ := vectorKeys(t)
	v := adaSays
	v.Roles = nil

	token, _ := New("https://auth.example.test", ring{k}).Sign(v, "wiki", signedAt)

	if _, _, c := verified(t, token, k); !slices.Equal(c["roles"].([]any), []any{}) {
		t.Fatalf("roles = %v", c["roles"])
	}
}

func TestAKeyGoJoseCannotSignWithFailsTheSigning(t *testing.T) {
	_, err := New("https://auth.example.test", ring{key{jose.JSONWebKey{KeyID: "broken"}}}).Sign(adaSays, "wiki", signedAt)

	if err == nil {
		t.Fatal("a key that is no key signed")
	}
}
