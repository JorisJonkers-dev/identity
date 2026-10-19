package kubernetes_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/kubernetes"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

const (
	issuer   = "https://kubernetes.default.svc.cluster.local"
	audience = "https://id.example.org"
)

// cluster is an API server's ServiceAccount signing key, published at its JWKS URL.
type cluster struct {
	key  *rsa.PrivateKey
	jwks *httptest.Server
}

func newCluster(t *testing.T) *cluster {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(set)
	}))
	t.Cleanup(jwks.Close)
	return &cluster{key: key, jwks: jwks}
}

func (c *cluster) verifier() *kubernetes.ServiceAccounts {
	return kubernetes.New(c.jwks.Client(), issuer, c.jwks.URL, audience)
}

// claims is a projected token's claims, for a test to change one of.
type claims struct {
	jwt.Claims
}

func projected(subject string) claims {
	now := time.Now()
	return claims{jwt.Claims{
		Issuer: issuer, Subject: subject, Audience: jwt.Audience{audience},
		IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), Expiry: jwt.NewNumericDate(now.Add(10 * time.Minute)),
	}}
}

func sign(t *testing.T, key *rsa.PrivateKey, c claims) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "k1").WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.Signed(signer).Claims(c.Claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestAProjectedTokenNamesItsNamespaceAndServiceAccount(t *testing.T) {
	c := newCluster(t)
	token := sign(t, c.key, projected("system:serviceaccount:media:backup"))

	namespace, name, err := c.verifier().Verify(t.Context(), token)

	if err != nil || namespace != "media" || name != "backup" {
		t.Fatalf("Verify = %q, %q, %v", namespace, name, err)
	}
}

func TestAProjectedTokenIsRefused(t *testing.T) {
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		token func(c *cluster) string
	}{
		{"from another issuer", func(c *cluster) string {
			cl := projected("system:serviceaccount:media:backup")
			cl.Issuer = "https://elsewhere.example.org"
			return sign(t, c.key, cl)
		}},
		{"for another audience", func(c *cluster) string {
			cl := projected("system:serviceaccount:media:backup")
			cl.Audience = jwt.Audience{"vault"}
			return sign(t, c.key, cl)
		}},
		{"once it has expired", func(c *cluster) string {
			cl := projected("system:serviceaccount:media:backup")
			cl.Expiry = jwt.NewNumericDate(time.Now().Add(-time.Minute))
			return sign(t, c.key, cl)
		}},
		{"signed by another key", func(*cluster) string { return sign(t, other, projected("system:serviceaccount:media:backup")) }},
		{"for a subject that is no ServiceAccount", func(c *cluster) string { return sign(t, c.key, projected("system:node:worker-1")) }},
		{"for a ServiceAccount with no namespace", func(c *cluster) string { return sign(t, c.key, projected("system:serviceaccount:backup")) }},
		{"that is no JWT", func(*cluster) string { return "not-a-token" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			cl := newCluster(t)
			if _, _, err := cl.verifier().Verify(t.Context(), c.token(cl)); !errors.Is(err, app.ErrInvalidServiceAccountToken) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestOutsideAClusterEveryTokenIsRefused(t *testing.T) {
	if _, _, err := (kubernetes.Disabled{}).Verify(t.Context(), "anything"); !errors.Is(err, app.ErrInvalidServiceAccountToken) {
		t.Fatalf("err = %v", err)
	}
}

// apiServer is a TLS server that answers with the Authorization header it was sent.
func apiServer(t *testing.T) (srv *httptest.Server, caFile, tokenFile string) {
	t.Helper()
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("Authorization"))) //nolint:gosec // a test server echoing to its own client
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	caFile, tokenFile = filepath.Join(dir, "ca.crt"), filepath.Join(dir, "token")
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(caFile, ca, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenFile, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return srv, caFile, tokenFile
}

func get(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var body [64]byte
	n, _ := res.Body.Read(body[:])
	return string(body[:n])
}

func TestTheInClusterClientTrustsTheClusterCAAndSendsTheCurrentToken(t *testing.T) {
	srv, caFile, tokenFile := apiServer(t)
	client, err := kubernetes.InCluster(caFile, tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := get(t, client, srv.URL); got != "Bearer first" {
		t.Fatalf("first request sent %q", got)
	}
	if err := os.WriteFile(tokenFile, []byte("rotated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := get(t, client, srv.URL); got != "Bearer rotated" {
		t.Fatalf("after rotation sent %q", got)
	}
}

func TestTheInClusterClientNeedsTheClusterCA(t *testing.T) {
	dir := t.TempDir()
	notPEM := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(notPEM, []byte("no certificate here"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, caFile string }{
		{"missing", filepath.Join(dir, "absent.crt")},
		{"holding no certificate", notPEM},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := kubernetes.InCluster(c.caFile, filepath.Join(dir, "token")); err == nil {
				t.Fatal("a client without the cluster CA")
			}
		})
	}
}

func TestTheInClusterClientFailsWithoutItsToken(t *testing.T) {
	srv, caFile, tokenFile := apiServer(t)
	client, err := kubernetes.InCluster(caFile, tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(tokenFile); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if res, err := client.Do(req); err == nil {
		_ = res.Body.Close()
		t.Fatal("a request went out without a token")
	}
}
