// Command identity serves the API and the embedded web app, and drains on SIGTERM. It never
// migrates its database: the identity-migration image does, before it starts (docs/adr/0010).
//
// With arguments it runs an admin command instead; see command.
//
// Environment: DATABASE_URL, PUBLIC_URL (where mailed links point), SMTP_ADDR and MAIL_FROM are
// required; SMTP_USERNAME and SMTP_PASSWORD sign in to the mail server; SMTP_TLS is "required"
// (the default) or "none", which only a local run against Mailpit may use; ADDR defaults to :8080,
// and METRICS_ADDR, which serves only /metrics, to :9090. VAULT_ADDR lets Users add authenticator
// apps: identity signs in to Vault with VAULT_TOKEN, or as VAULT_ROLE (identity) through the
// Kubernetes auth method; VAULT_TRANSIT_MOUNT (transit) and VAULT_TOTP_KEY (identity-totp) name the
// key. Outside a cluster, CLIENTS_FILE names a YAML file of Client objects for a local run.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/JorisJonkers-dev/identity/internal/access/adapters/catalog"
	"github.com/JorisJonkers-dev/identity/internal/access/adapters/people"
	accesspersistence "github.com/JorisJonkers-dev/identity/internal/access/adapters/persistence"
	accessapp "github.com/JorisJonkers-dev/identity/internal/access/app"
	gatecontexts "github.com/JorisJonkers-dev/identity/internal/gate/adapters/contexts"
	gatepersistence "github.com/JorisJonkers-dev/identity/internal/gate/adapters/persistence"
	gateweb "github.com/JorisJonkers-dev/identity/internal/gate/adapters/web"
	gateapp "github.com/JorisJonkers-dev/identity/internal/gate/app"
	"github.com/JorisJonkers-dev/identity/internal/platform/httpapi"
	"github.com/JorisJonkers-dev/identity/internal/platform/mail"
	"github.com/JorisJonkers-dev/identity/internal/platform/metrics"
	"github.com/JorisJonkers-dev/identity/internal/platform/passwords"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/vault"
	"github.com/JorisJonkers-dev/identity/internal/platform/webui"
	"github.com/JorisJonkers-dev/identity/internal/server"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/accounts"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	signinoidc "github.com/JorisJonkers-dev/identity/internal/signin/adapters/oidc"
	signinpersistence "github.com/JorisJonkers-dev/identity/internal/signin/adapters/persistence"
	signinapp "github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/users/adapters/passkeys"
	"github.com/JorisJonkers-dev/identity/internal/users/adapters/persistence"
	usersweb "github.com/JorisJonkers-dev/identity/internal/users/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/web"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

const (
	defaultAddr        = ":8080"
	defaultMetricsAddr = ":9090"
)

func main() {
	if len(os.Args) > 1 {
		os.Exit(command(context.Background(), os.Getenv, os.Args[1:], os.Stdout, os.Stderr))
	}
	os.Exit(start(context.Background(), os.Getenv))
}

// start is main minus os.Exit, so tests can drive it.
func start(parent context.Context, getenv func(string) string) int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger, getenv); err != nil {
		logger.Error("identity stopped", "error", err)
		return 1
	}
	return 0
}

// run is the composition root: it reads the environment and builds the store, the use cases and
// the server. httpapi assembles the contexts' web adapters into the one generated API.
// argon2id takes 46 MiB a hash: four at once stays under 200 MiB, and a check that cannot start
// within hashWait answers 503 rather than queueing.
const (
	maxConcurrentHashes = 4
	hashWait            = 3 * time.Second
	// A Gate check is answered from memory for this long; a sign-out or a role taken away reaches a
	// gated host within it (docs/adr/0009).
	gateCacheTTL = 30 * time.Second
)

func run(ctx context.Context, logger *slog.Logger, getenv func(string) string) error {
	cfg, err := readConfig(getenv)
	if err != nil {
		return err
	}
	store, err := pg.Open(ctx, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	// users caps an Admin's session at 12 hours, and access lists users' Users: each needs the other.
	var access *accessapp.Service
	admins := adminsFunc(func(ctx context.Context, id uuid.UUID) (bool, error) { return access.IsAdmin(ctx, id) })
	// Set once the Provider and the Gate exist, before the server serves anything.
	var signedOut app.SignedOut
	opts := []app.Option{app.WithAdmins(admins), app.WithSignedOut(func(ctx context.Context, userID uuid.UUID, sessionIDs []uuid.UUID) {
		signedOut(ctx, userID, sessionIDs)
	})}
	passkeyCeremonies, err := passkeyCeremonies(cfg.publicURL)
	if err != nil {
		return err
	}
	opts = append(opts, app.WithWebAuthn(passkeyCeremonies))
	if sealer, ok := cfg.vault.sealer(); ok {
		opts = append(opts, app.WithSealer(sealer))
	} else {
		logger.Warn("VAULT_ADDR is not set: no authenticator apps")
	}
	users := app.New(persistence.New(store.Queries()), passwords.NewArgon2id(maxConcurrentHashes, hashWait), cfg.mailer, time.Now, rand.Reader, cfg.publicURL, opts...)
	index, synced, err := watchClients(ctx, cfg, logger)
	if err != nil {
		return err
	}
	access = accessapp.New(accesspersistence.New(store.Queries(), store), catalog.New(index), people.New(users), rand.Reader)
	api, err := httpapi.New(logger, version, users, access)
	if err != nil {
		return err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	ring := signinoidc.NewRing(key)
	logouts := signinoidc.NewLogouts(cfg.publicURL, ring, &http.Client{}, logger, time.Now)
	signIn := signinapp.New(signinpersistence.New(store.Queries()), index, accounts.New(users), access, logouts, time.Now, rand.Reader)
	provider, err := oidcProvider(cfg, signIn, ring, logger)
	if err != nil {
		return err
	}
	others := gatecontexts.New(signIn, users)
	gate := gateapp.New(gatepersistence.New(store.Queries()), others, others, others, time.Now, rand.Reader, gateCacheTTL)
	signedOut = endSessions(gate, signIn, logger)
	gateHandler, err := gateweb.New(gate, gateweb.Config{Issuer: cfg.publicURL, SessionCookie: usersweb.CookieName}, logger)
	if err != nil {
		return err
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.addr)
	if err != nil {
		return err
	}
	mln, err := lc.Listen(ctx, "tcp", cfg.metricsAddr)
	if err != nil {
		_ = ln.Close()
		return err
	}
	stopMetrics := serveMetrics(mln, metrics.Handler(access.Unassigned, logger), logger)
	defer stopMetrics()
	return server.New(logger, version, server.Routes{
		API:  api,
		OIDC: provider,
		Gate: gateHandler,
		Web:  webui.Handler(web.Dist()),
		Ready: func(ctx context.Context) error {
			if !synced() {
				return errClientsNotListed
			}
			return store.Ping(ctx)
		},
	}).Serve(ctx, ln)
}

var errClientsNotListed = errors.New("the Client objects are not listed yet")

// passkeyCeremonies runs WebAuthn for the Identity Provider: its host is the relying party, and
// the Issuer the one origin a passkey is made or used on.
func passkeyCeremonies(publicURL string) (*passkeys.Ceremonies, error) {
	u, err := url.Parse(publicURL)
	if err != nil {
		return nil, err
	}
	return passkeys.New(u.Hostname(), publicURL)
}

// adminsFunc is app.Admins as a function.
type adminsFunc func(ctx context.Context, id uuid.UUID) (bool, error)

func (f adminsFunc) IsAdmin(ctx context.Context, id uuid.UUID) (bool, error) { return f(ctx, id) }

// vaultConfig reaches Vault's transit engine, which seals TOTP secrets (docs/adr/0013).
type vaultConfig struct {
	addr, token, role, authMount, jwtPath, transitMount, totpKey string
}

// sealer is the TOTP key, or false when no Vault is configured. A token signs in directly, for a
// local dev-mode Vault; without one identity signs in with its service account.
func (v vaultConfig) sealer() (vault.Key, bool) {
	if v.addr == "" {
		return vault.Key{}, false
	}
	login := vault.Kubernetes(v.authMount, v.role, v.jwtPath)
	if v.token != "" {
		login = vault.Token(v.token)
	}
	client := &http.Client{Timeout: vaultTimeout}
	return vault.NewTransit(v.addr, v.transitMount, login, client).Key(v.totpKey), true
}

const vaultTimeout = 5 * time.Second

// serveMetrics serves h on ln until the returned stop is called.
func serveMetrics(ln net.Listener, h http.Handler, logger *slog.Logger) (stop func()) {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics stopped", "error", err)
		}
	}()
	return func() { _ = srv.Close() }
}

// watchClients keeps an Index of the cluster's Client objects. Outside a cluster there are none, and
// the Index stays empty.
func watchClients(ctx context.Context, cfg config, logger *slog.Logger) (*clients.Index, func() bool, error) {
	index := clients.NewIndex()
	if !cfg.inCluster {
		if cfg.clientsFile == "" {
			logger.Warn("not in a cluster and CLIENTS_FILE is not set: no Clients")
			return index, func() bool { return true }, nil
		}
		return index, func() bool { return true }, clients.LoadFile(cfg.clientsFile, index)
	}
	rc, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, err
	}
	dyn, err := dynamic.NewForConfig(rc)
	if err != nil {
		return nil, nil, err
	}
	return index, clients.Watch(ctx, dyn, index, logger), nil
}

// oidcProvider is the OIDC Provider over the sign-in context. It signs with a key made at start-up
// until keys are kept (identity#30).
// endSessions ends what was signed in through ended sessions: the Gate's cached checks on this
// replica, and native Clients through back-channel logout (docs/adr/0007).
func endSessions(gate *gateapp.Service, signIn *signinapp.Service, logger *slog.Logger) app.SignedOut {
	return func(ctx context.Context, userID uuid.UUID, sessionIDs []uuid.UUID) {
		gate.Forget(sessionIDs)
		if err := signIn.SignedOut(ctx, userID, sessionIDs); err != nil {
			logger.ErrorContext(ctx, "back-channel logout", "error", err)
		}
	}
}

func oidcProvider(cfg config, svc *signinapp.Service, ring *signinoidc.Ring, logger *slog.Logger) (http.Handler, error) {
	return signinoidc.New(signinoidc.Config{
		Issuer:        cfg.publicURL,
		Insecure:      strings.HasPrefix(cfg.publicURL, "http://"),
		SessionCookie: usersweb.CookieName,
	}, svc, ring, logger, time.Now)
}

type config struct {
	databaseURL string
	publicURL   string
	addr        string
	// metricsAddr serves /metrics, on a port the edge never routes.
	metricsAddr string
	mailer      mail.SMTP
	// inCluster is set when running in Kubernetes, where Client objects are watched.
	inCluster bool
	// clientsFile holds Client objects for a run outside a cluster.
	clientsFile string
	vault       vaultConfig
}

// or is value, or fallback when value is empty.
func or(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// readConfig reads the environment, naming every required variable that is missing at once.
func readConfig(getenv func(string) string) (config, error) {
	var missing []string
	need := func(name string) string {
		v := getenv(name)
		if v == "" {
			missing = append(missing, name)
		}
		return v
	}
	cfg := config{
		databaseURL: need("DATABASE_URL"),
		publicURL:   strings.TrimRight(need("PUBLIC_URL"), "/"),
		addr:        getenv("ADDR"),
		metricsAddr: getenv("METRICS_ADDR"),
		inCluster:   getenv("KUBERNETES_SERVICE_HOST") != "",
		clientsFile: getenv("CLIENTS_FILE"),
		vault: vaultConfig{
			addr: getenv("VAULT_ADDR"), token: getenv("VAULT_TOKEN"),
			role: or(getenv("VAULT_ROLE"), "identity"), authMount: or(getenv("VAULT_AUTH_MOUNT"), "kubernetes"),
			jwtPath:      or(getenv("VAULT_JWT_PATH"), "/var/run/secrets/kubernetes.io/serviceaccount/token"),
			transitMount: or(getenv("VAULT_TRANSIT_MOUNT"), "transit"), totpKey: or(getenv("VAULT_TOTP_KEY"), "identity-totp"),
		},
		mailer: mail.SMTP{
			Addr: need("SMTP_ADDR"), From: need("MAIL_FROM"),
			Username: getenv("SMTP_USERNAME"), Password: getenv("SMTP_PASSWORD"),
		},
	}
	if len(missing) > 0 {
		return config{}, fmt.Errorf("not set: %s", strings.Join(missing, ", "))
	}
	if cfg.addr == "" {
		cfg.addr = defaultAddr
	}
	if cfg.metricsAddr == "" {
		cfg.metricsAddr = defaultMetricsAddr
	}
	switch tlsMode := getenv("SMTP_TLS"); tlsMode {
	case "", "required":
		cfg.mailer.RequireTLS = true
	case "none":
	default:
		return config{}, fmt.Errorf("SMTP_TLS %q is not \"required\" or \"none\"", tlsMode)
	}
	return cfg, nil
}
