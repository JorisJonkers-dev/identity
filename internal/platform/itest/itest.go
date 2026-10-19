//go:build integration

// Package itest starts identity for an integration test as it runs in the cluster: the routes
// assembly builds, over HTTPS, with a database of the test's own, and a Vault and a Mailpit shared
// by the test binary. A Browser drives it the way a person's browser does: JSON over HTTP, with
// the session in a cookie (docs/testing-standards.md).
package itest

import (
	"bytes"
	"context"
	"crypto/rand"
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

	"github.com/google/uuid"
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
	srv   *httptest.Server
	db    *pg.Store
	logs  *logBuffer
	prune func(context.Context) (int64, error)
	clock *clock
}

// clock is identity's clock in a test: the real time, plus however far the test moved it on.
type clock struct {
	mu    sync.Mutex
	ahead time.Duration
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Now().Add(c.ahead)
}

// Now is the time as identity's use cases see it, for codes an authenticator would show then.
func (e *Env) Now() time.Time { return e.clock.now() }

// Pass moves identity's clock on by d, for what happens after a while.
func (e *Env) Pass(d time.Duration) {
	e.clock.mu.Lock()
	defer e.clock.mu.Unlock()
	e.clock.ahead += d
}

// logBuffer keeps what identity logs, for tests to read; requests log from many goroutines.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// Start runs identity for t with the Clients given, and stops it when t ends. Each Secret Client
// has its secret before Start returns: the one at its SecretPath, or a new one written there.
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
	logs := &logBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	clk := &clock{}
	v := vault.New(shared.vaultAddr, vault.Token(vaultToken), &http.Client{Timeout: 5 * time.Second})
	transit := v.Transit("transit")
	built, err := assembly.New(t.Context(), assembly.Config{
		Store:     store,
		PublicURL: "https://" + Host,
		Mailer:    mail.SMTP{Addr: shared.smtpAddr, From: "identity@" + Host},
		Sealer:    transit.Key("identity-totp"),
		KeySealer: transit.Key("identity-signing"),
		KV:        v.KV(),
		Index:     index,
		Synced:    func() bool { return true },
		Logger:    logger,
		Version:   "itest",
		Now:       clk.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := built.ProvisionSecrets(t.Context()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(server.New(logger, "itest", built.Routes).Handler())
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return &Env{srv: srv, db: store, logs: logs, prune: built.Prune, clock: clk}
}

// Logs is every line identity logged so far, each decoded from its JSON.
func (e *Env) Logs(t *testing.T) []map[string]any {
	t.Helper()
	e.logs.mu.Lock()
	raw := slices.Clone(e.logs.buf.Bytes())
	e.logs.mu.Unlock()
	var lines []map[string]any
	for line := range bytes.Lines(raw) {
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("a log line that is not JSON: %s", line)
		}
		lines = append(lines, m)
	}
	return lines
}

// Introspect asks /oidc/introspect about token as the Client clientID, authenticating with its
// secret, the way a Client's API checks a bearer token it was given (docs/adr/0017).
func (e *Env) Introspect(t *testing.T, clientID, secret, token string) map[string]any {
	t.Helper()
	form := url.Values{"token": {token}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://"+Host+"/oidc/introspect", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(url.QueryEscape(clientID), url.QueryEscape(secret))
	res, err := e.Browser().client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("introspect = %d %s", res.StatusCode, raw)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// ClientAuth is the status /oidc/introspect answers a Client authenticating with secret: 200 when
// the token endpoint accepts the secret, 401 when it does not.
func (e *Env) ClientAuth(t *testing.T, clientID, secret string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://"+Host+"/oidc/introspect", strings.NewReader("token=x"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(url.QueryEscape(clientID), url.QueryEscape(secret))
	res, err := e.Browser().client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	return res.StatusCode
}

// GateAnswer is what the Gate told Traefik: a status and the headers it set.
type GateAnswer struct {
	StatusCode int
	Header     http.Header
}

// GateCheck is Traefik's forwardAuth call to the Gate for a request to host, carrying
// authorization as its Authorization header when it is not empty.
func (e *Env) GateCheck(t *testing.T, host, authorization string) GateAnswer {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+Host+"/gate/check", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Forwarded-Host", host)
	req.Header.Set("X-Forwarded-Uri", "/")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	res, err := e.Browser().client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	return GateAnswer{StatusCode: res.StatusCode, Header: res.Header}
}

// Userinfo is the status /oidc/userinfo answers for a bearer token.
func (e *Env) Userinfo(t *testing.T, token string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+Host+"/oidc/userinfo", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := e.Browser().client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	return res.StatusCode
}

// Queries reaches the test's database directly, to set up what no request can, such as an event
// a year old.
func (e *Env) Queries() *queries.Queries { return e.db.Queries() }

// Prune runs the scheduled job that removes Audit Events older than a year.
func (e *Env) Prune(t *testing.T) {
	t.Helper()
	if _, err := e.prune(t.Context()); err != nil {
		t.Fatal(err)
	}
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
// signed in. The address gets a tag of its own (ada+3f2a@example.org), so tests that run at once
// never read each other's mail; Browser.Email is the address as signed up.
func (e *Env) SignedUp(t *testing.T, email string) *Browser {
	t.Helper()
	b := e.Browser()
	local, domain, _ := strings.Cut(email, "@")
	email = local + "+" + strings.ToLower(rand.Text()[:8]) + "@" + domain
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
	e.Promote(t, b.Email)
	secret := b.AddAuthenticator(t)
	// Signing in again as an Admin is what caps the session at 12 hours; the code that added the app
	// is spent, so the next step's code proves it.
	b.Call(t, http.MethodDelete, "/api/v1/session", nil, http.StatusNoContent, nil)
	b.Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": b.Email, "password": Password}, http.StatusAccepted, nil)
	next, err := totp.GenerateCode(secret, e.Now().Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	b.Call(t, http.MethodPost, "/api/v1/session/second-factor", map[string]string{"code": next}, http.StatusNoContent, nil)
	return b
}

// AddAuthenticator adds an authenticator app to the signed-in User, confirmed with its first code,
// and returns its secret, for the codes it shows later.
func (b *Browser) AddAuthenticator(t *testing.T) string {
	t.Helper()
	var setup struct{ Secret string }
	b.Call(t, http.MethodPost, "/api/v1/profile/second-factors/totp", nil, http.StatusOK, &setup)
	code, err := totp.GenerateCode(setup.Secret, b.env.Now())
	if err != nil {
		t.Fatal(err)
	}
	b.Call(t, http.MethodPost, "/api/v1/profile/second-factors/totp/confirmation",
		map[string]string{"label": "Phone", "code": code, "password": Password}, http.StatusNoContent, nil)
	return setup.Secret
}

// Promote puts the User signed up as email in the admins Group, as `identity admin promote` does.
// Their session proves no Second Factor until they add one and sign in again (Admin does both).
func (e *Env) Promote(t *testing.T, email string) {
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
	if _, err := q.AddMember(t.Context(), queries.AddMemberParams{GroupID: admins.ID, UserID: u.ID}); err != nil {
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

// Authorize starts a sign-in to the native Client c at its first redirect URI, as the app's sign-in
// button does, and returns the login step identity sends the browser to.
func (b *Browser) Authorize(t *testing.T, c signindomain.Client) string {
	t.Helper()
	q := url.Values{
		"client_id": {c.ID}, "redirect_uri": {c.RedirectURIs[0]}, "response_type": {"code"}, "scope": {"openid"},
		"state": {"st"}, "code_challenge": {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}, "code_challenge_method": {"S256"},
	}
	return b.Follow(t, "/oidc/authorize?"+q.Encode())
}

// Follow gets target, a path or a URL on identity, and returns where the redirect it answers goes.
func (b *Browser) Follow(t *testing.T, target string) string {
	t.Helper()
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+Host+u.RequestURI(), nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 300 || res.StatusCode > 399 {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("GET %s = %d %s, want a redirect", target, res.StatusCode, raw)
	}
	return res.Header.Get("Location")
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

// Vault is the dev-mode Vault the test binary shares, with the transit keys identity uses, and its
// root token, for a test that starts identity from its environment.
func Vault(t *testing.T) (addr, token string) {
	t.Helper()
	started(t)
	return shared.vaultAddr, vaultToken
}

// SecretPath is a Vault KV path of t's own, where a Secret Client's Application would read its
// secret; the test binary's tests share one Vault.
func SecretPath(t *testing.T) string {
	t.Helper()
	return "secret/data/itest/" + uuid.NewString() + "/client"
}

// PutSecret writes a Client's id and secret at path, as one already there before identity starts.
func PutSecret(t *testing.T, path, clientID, secret string) {
	t.Helper()
	started(t)
	kv := vault.New(shared.vaultAddr, vault.Token(vaultToken), http.DefaultClient).KV()
	if err := kv.Write(t.Context(), path, map[string]string{"id": clientID, "secret": secret}); err != nil {
		t.Fatal(err)
	}
}

// ReadSecret is what an Application reads at path: its Client's id and secret.
func ReadSecret(t *testing.T, path string) map[string]string {
	t.Helper()
	started(t)
	kv := vault.New(shared.vaultAddr, vault.Token(vaultToken), http.DefaultClient).KV()
	data, err := kv.Read(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	return data
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
		{"/v1/transit/keys/identity-signing", `{}`},
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
