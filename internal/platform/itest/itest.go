//go:build integration

// Package itest starts identity for an integration test as it runs in the cluster: the routes
// assembly builds, over HTTPS, with a database of the test's own, and a Vault and a Mailpit shared
// by the test binary. A Browser drives it the way a person's browser does: JSON over HTTP, with
// the session in a cookie (docs/testing-standards.md).
package itest

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/JorisJonkers-dev/identity/internal/assembly"
	"github.com/JorisJonkers-dev/identity/internal/platform/mail"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/identity/internal/platform/vault"
	"github.com/JorisJonkers-dev/identity/internal/server"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// Password is every test User's password.
const Password = "correct horse battery"

const (
	// Host is the Issuer's host in tests; every Browser reaches the test server under it.
	Host         = "identity.test"
	vaultImage   = "hashicorp/vault:1.19.0"
	vaultToken   = "itest-root"
	mailpitImage = "axllent/mailpit:v1.27.10"
	mailWait     = 10 * time.Second
)

var confirmLink = regexp.MustCompile(`/confirm\?token=([^\s"<>]+)`)

// The containers every test in a binary shares, started on first use.
var (
	once     sync.Once
	shared   struct{ vaultAddr, smtpAddr, mailAPI string }
	errStart error
)

// Env is one running identity.
type Env struct {
	srv *httptest.Server
	db  *pg.Store
}

// Start runs identity for t with the Clients given, and stops it when t ends.
func Start(t *testing.T, cs ...signindomain.Client) *Env {
	t.Helper()
	started(t)
	store, err := pg.Open(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	index := clients.NewIndex()
	for _, c := range cs {
		index.Put(c)
	}
	logger := slog.New(slog.DiscardHandler)
	built, err := assembly.New(assembly.Config{
		Store:     store,
		PublicURL: "https://" + Host,
		Mailer:    mail.SMTP{Addr: shared.smtpAddr, From: "identity@" + Host},
		Sealer:    vault.NewTransit(shared.vaultAddr, "transit", vault.Token(vaultToken), &http.Client{Timeout: 5 * time.Second}).Key("identity-totp"),
		Index:     index,
		Synced:    func() bool { return true },
		Logger:    logger,
		Version:   "itest",
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(server.New(logger, "itest", built.Routes).Handler())
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return &Env{srv: srv, db: store}
}

// Browser is a fresh browser, signed in to nothing, that reaches identity at https://identity.test.
func (e *Env) Browser() *Browser {
	jar, _ := cookiejar.New(nil) // cookiejar.New never fails without options
	addr := e.srv.Listener.Addr().String()
	roots := x509.NewCertPool()
	roots.AddCert(e.srv.Certificate())
	transport := &http.Transport{
		// The Browser asks for identity.test, and checks the test server's own certificate under a
		// name it was issued for.
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}
	return &Browser{env: e, client: &http.Client{
		Transport: transport, Jar: jar, Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// SignedUp is a Browser of a User who signed up as email, confirmed through the mailed link, and
// signed in.
func (e *Env) SignedUp(t *testing.T, email string) *Browser {
	t.Helper()
	b := e.Browser()
	b.Email = email
	b.Call(t, http.MethodPost, "/api/v1/signup", map[string]string{"email": email, "name": "Grace Hopper", "password": Password}, http.StatusAccepted, nil)
	b.Call(t, http.MethodPost, "/api/v1/confirmations", map[string]string{"token": confirmationToken(t, email), "password": Password}, http.StatusNoContent, nil)
	b.Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": email, "password": Password}, http.StatusNoContent, nil)
	return b
}

// Admin is a Browser of an Admin whose session proved a Second Factor: a User signed up as email,
// put in the admins Group, who added an authenticator app and signed in again with its code.
func (e *Env) Admin(t *testing.T, email string) *Browser {
	t.Helper()
	b := e.SignedUp(t, email)
	e.promote(t, email)
	var setup struct{ Secret string }
	b.Call(t, http.MethodPost, "/api/v1/profile/second-factors/totp", nil, http.StatusOK, &setup)
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b.Call(t, http.MethodPost, "/api/v1/profile/second-factors/totp/confirmation",
		map[string]string{"label": "Phone", "code": code, "password": Password}, http.StatusNoContent, nil)
	// Signing in again as an Admin is what caps the session at 12 hours; the code that added the app
	// is spent, so the next step's code proves it.
	b.Call(t, http.MethodDelete, "/api/v1/session", nil, http.StatusNoContent, nil)
	b.Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": email, "password": Password}, http.StatusAccepted, nil)
	next, err := totp.GenerateCode(setup.Secret, time.Now().Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	b.Call(t, http.MethodPost, "/api/v1/session/second-factor", map[string]string{"code": next}, http.StatusNoContent, nil)
	return b
}

// promote puts the User signed up as email in the admins Group, as `identity admin promote` does.
func (e *Env) promote(t *testing.T, email string) {
	t.Helper()
	q := e.db.Queries()
	u, err := q.UserByEmail(t.Context(), email)
	if err != nil {
		t.Fatalf("itest: promote %s: %v", email, err)
	}
	admins, err := q.AdminsGroup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := q.AddMember(t.Context(), queries.AddMemberParams{GroupID: admins.ID, UserID: u.ID}); err != nil {
		t.Fatal(err)
	}
}

// Browser is one person's browser: its cookies are its sessions.
type Browser struct {
	env    *Env
	client *http.Client
	// Email is who signed up in it, if anyone.
	Email string
}

// Call sends body as JSON and fails t unless the answer has status want; out, when not nil, gets
// the answer's JSON.
func (b *Browser) Call(t *testing.T, method, path string, body any, want int, out any) {
	t.Helper()
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, "https://"+Host+path, payload)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := b.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		t.Fatalf("%s %s = %d %s, want %d", method, path, res.StatusCode, raw, want)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s %s: %v in %s", method, path, err, raw)
		}
	}
}

// UserID is the id of the User signed up as email, read from the access page as an Admin sees it.
func (b *Browser) UserID(t *testing.T, email string) string {
	t.Helper()
	var overview struct{ Users []struct{ ID, Email string } }
	b.Call(t, http.MethodGet, "/api/v1/access", nil, http.StatusOK, &overview)
	i := slices.IndexFunc(overview.Users, func(u struct{ ID, Email string }) bool { return u.Email == email })
	if i < 0 {
		t.Fatalf("no User %s among %+v", email, overview.Users)
	}
	return overview.Users[i].ID
}

// confirmationToken is the token in the confirmation mail Mailpit received for email.
func confirmationToken(t *testing.T, email string) string {
	t.Helper()
	deadline := time.Now().Add(mailWait)
	for time.Now().Before(deadline) {
		var found struct{ Messages []struct{ ID string } }
		get(t, shared.mailAPI+"/api/v1/search?query="+url.QueryEscape(fmt.Sprintf("to:%q", email)), &found)
		if len(found.Messages) > 0 {
			var message struct{ Text string }
			get(t, shared.mailAPI+"/api/v1/message/"+found.Messages[0].ID, &message)
			if m := confirmLink.FindStringSubmatch(message.Text); m != nil {
				return m[1]
			}
			t.Fatalf("no confirmation link in the mail to %s: %s", email, message.Text)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no mail reached %s within %s", email, mailWait)
	return ""
}

func get(t *testing.T, target string, out any) {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
}

// started starts Vault, with its transit key, and Mailpit, once for the test binary.
func started(t *testing.T) {
	t.Helper()
	once.Do(func() { errStart = start(context.Background()) })
	if errStart != nil {
		t.Fatalf("itest: start containers: %v", errStart)
	}
}

func start(ctx context.Context) error {
	v, err := testcontainers.Run(ctx, vaultImage,
		testcontainers.WithEnv(map[string]string{"VAULT_DEV_ROOT_TOKEN_ID": vaultToken, "VAULT_DEV_LISTEN_ADDRESS": "0.0.0.0:8200"}),
		testcontainers.WithExposedPorts("8200/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/v1/sys/health").WithPort("8200/tcp")),
	)
	if err != nil {
		return err
	}
	if shared.vaultAddr, err = v.PortEndpoint(ctx, "8200/tcp", "http"); err != nil {
		return err
	}
	for _, step := range []struct{ path, body string }{
		{"/v1/sys/mounts/transit", `{"type":"transit"}`},
		{"/v1/transit/keys/identity-totp", `{}`},
	} {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, shared.vaultAddr+step.path, strings.NewReader(step.body))
		req.Header.Set("X-Vault-Token", vaultToken)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		_ = res.Body.Close()
		if res.StatusCode >= http.StatusBadRequest {
			return fmt.Errorf("vault %s = %d", step.path, res.StatusCode)
		}
	}
	m, err := testcontainers.Run(ctx, mailpitImage,
		testcontainers.WithExposedPorts("1025/tcp", "8025/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/readyz").WithPort("8025/tcp")),
	)
	if err != nil {
		return err
	}
	if shared.mailAPI, err = m.PortEndpoint(ctx, "8025/tcp", "http"); err != nil {
		return err
	}
	shared.smtpAddr, err = m.PortEndpoint(ctx, "1025/tcp", "")
	return err
}
