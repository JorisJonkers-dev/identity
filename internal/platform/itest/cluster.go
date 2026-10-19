//go:build integration

package itest

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/kubernetes"
)

// ClusterIssuer is the issuer of the test cluster's ServiceAccount tokens.
const ClusterIssuer = "https://kubernetes.default.svc.cluster.local"

// The test binary's one cluster: the key it signs ServiceAccount tokens with, published as a JWKS.
var (
	clusterOnce sync.Once
	cluster     struct {
		key  *rsa.PrivateKey
		jwks *httptest.Server
	}
)

func startCluster(t *testing.T) {
	t.Helper()
	clusterOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "cluster", Algorithm: "RS256", Use: "sig"}}}
		cluster.key = key
		cluster.jwks = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(set)
		}))
	})
}

// serviceAccounts checks tokens as identity in the test cluster does.
func serviceAccounts(t *testing.T) *kubernetes.ServiceAccounts {
	t.Helper()
	startCluster(t)
	return kubernetes.New(cluster.jwks.Client(), ClusterIssuer, cluster.jwks.URL, "https://"+Host)
}

// ServiceAccountToken is the projected token a pod running as namespace/name is given for
// audience; identity accepts only https://identity.test.
func ServiceAccountToken(t *testing.T, namespace, name, audience string) string {
	t.Helper()
	startCluster(t)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: cluster.key}, (&jose.SignerOptions{}).WithHeader("kid", "cluster").WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	token, err := jwt.Signed(signer).Claims(jwt.Claims{
		Issuer: ClusterIssuer, Subject: "system:serviceaccount:" + namespace + ":" + name, Audience: jwt.Audience{audience},
		IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), Expiry: jwt.NewNumericDate(now.Add(10 * time.Minute)),
	}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// Token posts form to /oidc/token, with clientID and secret as basic auth when clientID is set,
// and answers the status and the JSON body.
func (e *Env) Token(t *testing.T, form url.Values, clientID, secret string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://"+Host+"/oidc/token", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if clientID != "" {
		req.SetBasicAuth(url.QueryEscape(clientID), url.QueryEscape(secret))
	}
	res, err := e.Browser().client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("token = %d %s", res.StatusCode, raw)
	}
	return res.StatusCode, out
}
