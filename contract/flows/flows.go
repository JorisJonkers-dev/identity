// Package flows is the Principal contract's flow-conformance suite for native sign-in kits. It runs
// a real identity in containers, signs people in through the app under test, and holds the app to
// four scenarios: sign in, sign out everywhere, a role taken away, and step-up. The app serves the
// routes README.md names, whatever its language.
package flows

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"testing"
	"time"
)

// ClientID is the Client the app under test signs in as: a native, public PKCE Client holding the
// App Roles user and editor.
const ClientID = "flows-app"

// Env is what the app under test is told.
type Env struct {
	// Issuer is identity, as the app's OIDC library discovers it.
	Issuer string
	// ClientID is the app's Client.
	ClientID string
	// BaseURL is where the browser reaches the app, over HTTPS through the suite's own proxy; its
	// redirect URI is BaseURL + "/callback".
	BaseURL string
	// Port is the port on 127.0.0.1 the app listens on, in plain HTTP.
	Port int
}

// App starts the app under test with env, serving README.md's routes on env.Port by the time it
// returns, and stops it with t.Cleanup.
type App func(t *testing.T, env Env)

// Images names the identity images the suite runs: the server, and the migration that readies its
// database. Pin the release whose contract the kit pins.
type Images struct {
	Identity  string
	Migration string
}

// ImagesFromEnv is the images FLOWS_IDENTITY_IMAGE and FLOWS_MIGRATION_IMAGE name.
func ImagesFromEnv() Images {
	return Images{Identity: os.Getenv("FLOWS_IDENTITY_IMAGE"), Migration: os.Getenv("FLOWS_MIGRATION_IMAGE")}
}

// Run starts identity from images and the app, then runs every scenario against the app.
func Run(t *testing.T, images Images, app App) {
	t.Helper()
	if images.Identity == "" || images.Migration == "" {
		t.Fatal("flows: name the identity and migration images, for example through FLOWS_IDENTITY_IMAGE and FLOWS_MIGRATION_IMAGE")
	}
	appPort, proxyPort, identityPort := freePort(t), freePort(t), freePort(t)
	s := startStack(t, images, identityPort, proxyPort, appPort)
	env := Env{Issuer: s.issuer, ClientID: ClientID, BaseURL: fmt.Sprintf("https://localhost:%d", proxyPort), Port: appPort}
	app(t, env)
	waitServing(t, fmt.Sprintf("http://127.0.0.1:%d", appPort))
	s.trust = proxy(t, proxyPort, appPort)
	admin := s.newAdmin(t)
	w := world{stack: s, env: env, admin: admin}

	t.Run("a User signs in and the app reads their Principal", w.signsIn)
	t.Run("signing out of identity ends the app's session", w.signsOut)
	t.Run("a role taken away stops working in the app", w.losesARole)
	t.Run("an action asking for fresh two-factor sends the browser to prove it", w.stepsUp)
}

// world is one run's identity, app and Admin.
type world struct {
	stack *stack
	env   Env
	admin *person
}

// proxy serves HTTPS at localhost:port in front of the app's plain HTTP on appPort, as an edge
// does, since identity takes only https redirect URIs. It answers the certificates to trust.
func proxy(t *testing.T, port, appPort int) *x509.CertPool {
	t.Helper()
	target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", appPort))
	l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	cert, leaf := localhostCertificate(t)
	srv := &http.Server{ //nolint:gosec // the suite's own proxy, closed with t
		Handler:   httputil.NewSingleHostReverseProxy(target),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
	}
	go func() { _ = srv.ServeTLS(l, "", "") }()
	t.Cleanup(func() { _ = srv.Close() })
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return pool
}

// localhostCertificate is a self-signed certificate for localhost, valid for a day.
func localhostCertificate(t *testing.T) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, leaf
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// waitServing waits until something answers at base.
func waitServing(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(base + "/me") //nolint:noctx // a readiness probe
		if err == nil {
			_ = res.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("flows: the app did not answer at %s", base)
}
