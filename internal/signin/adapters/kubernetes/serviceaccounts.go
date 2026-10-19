// Package kubernetes verifies projected ServiceAccount tokens against the cluster's own signing
// keys, so a pod signs in as its Machine Identity with no stored secret (docs/adr/0008).
package kubernetes

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

// ServiceAccounts is the app.ServiceAccountTokens of a cluster: a token must be signed by one of
// the issuer's keys, name the issuer, be meant for identity and not have expired.
type ServiceAccounts struct {
	issuer   string
	audience string
	keys     oidc.KeySet
}

var _ app.ServiceAccountTokens = (*ServiceAccounts)(nil)

// New returns ServiceAccounts reading the issuer's keys from jwksURL through client. audience is
// what a pod asks its projected token to be for: identity's issuer URL.
func New(client *http.Client, issuer, jwksURL, audience string) *ServiceAccounts {
	return &ServiceAccounts{issuer: issuer, audience: audience, keys: rp.NewRemoteKeySet(client, jwksURL)}
}

// algorithms are those the API server signs ServiceAccount tokens with.
func algorithms() []string { return []string{"RS256", "ES256"} }

const subjectPrefix = "system:serviceaccount:"

func (s *ServiceAccounts) Verify(ctx context.Context, token string) (string, string, error) {
	var claims oidc.TokenClaims
	payload, err := oidc.ParseToken(token, &claims)
	if err != nil {
		return "", "", app.ErrInvalidServiceAccountToken
	}
	if oidc.CheckIssuerWithISSVerifier(&claims, s.issuer, oidc.DefaultISSVerifier(s.issuer)) != nil || oidc.CheckAudience(&claims, s.audience) != nil ||
		oidc.CheckExpiration(&claims, 0) != nil || oidc.CheckSignature(ctx, token, payload, &claims, algorithms(), s.keys) != nil {
		return "", "", app.ErrInvalidServiceAccountToken
	}
	namespace, name, found := strings.Cut(strings.TrimPrefix(claims.Subject, subjectPrefix), ":")
	if !strings.HasPrefix(claims.Subject, subjectPrefix) || !found {
		return "", "", app.ErrInvalidServiceAccountToken
	}
	return namespace, name, nil
}

// Disabled is the app.ServiceAccountTokens outside a cluster, where no token can be checked.
type Disabled struct{}

func (Disabled) Verify(context.Context, string) (string, string, error) {
	return "", "", app.ErrInvalidServiceAccountToken
}

// InCluster is an HTTP client for the API server: it trusts the cluster's CA and sends identity's
// own ServiceAccount token, read again for each request because the kubelet rotates it.
func InCluster(caFile, tokenFile string) (*http.Client, error) {
	ca, err := os.ReadFile(caFile) //nolint:gosec // the path is configuration, not input
	if err != nil {
		return nil, fmt.Errorf("read the cluster CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("no certificate in %s", caFile)
	}
	base := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	return &http.Client{Timeout: 5 * time.Second, Transport: bearer{base: base, tokenFile: tokenFile}}, nil
}

type bearer struct {
	base      http.RoundTripper
	tokenFile string
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	token, err := os.ReadFile(b.tokenFile)
	if err != nil {
		return nil, fmt.Errorf("read the ServiceAccount token: %w", err)
	}
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	return b.base.RoundTrip(r)
}
