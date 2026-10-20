// Command identity serves the API and the embedded web app, and drains on SIGTERM. It never
// migrates its database: the identity-migration image does, before it starts (docs/adr/0010).
//
// With arguments it runs an admin command or the import from auth-api instead; see command.
//
// Environment: DATABASE_URL, PUBLIC_URL (where mailed links point), SMTP_ADDR, MAIL_FROM and
// VAULT_ADDR are required; SMTP_USERNAME and SMTP_PASSWORD sign in to the mail server; SMTP_TLS is
// "required" (the default) or "none", which only a local run against Mailpit may use; ADDR defaults
// to :8080, and METRICS_ADDR, which serves only /metrics, to :9090. identity signs in to Vault with
// VAULT_TOKEN, or as VAULT_ROLE (identity) through the Kubernetes auth method; under
// VAULT_TRANSIT_MOUNT (transit), VAULT_SIGNING_KEY (identity-signing) seals the signing keys and
// VAULT_TOTP_KEY (identity-totp) the authenticator apps' secrets (docs/adr/0013). Secret Clients'
// secrets are written to Vault KV at the path each Client object names (identity#32). Outside a cluster, CLIENTS_FILE names a YAML file of Client objects for a local run.
// GRAFANA_URL, an https address, is where the Admin dashboard links for charts over time. In a
// cluster, Machine Identities exchange ServiceAccount tokens checked against SERVICE_ACCOUNT_ISSUER
// and the keys at SERVICE_ACCOUNT_JWKS_URL, fetched with identity's own token and the cluster CA at
// SERVICE_ACCOUNT_CA_FILE; the defaults are the API server's in-cluster addresses.
package main

import (
	"context"
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

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/JorisJonkers-dev/identity/internal/assembly"
	"github.com/JorisJonkers-dev/identity/internal/platform/mail"
	"github.com/JorisJonkers-dev/identity/internal/platform/metrics"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/schedule"
	"github.com/JorisJonkers-dev/identity/internal/platform/vault"
	"github.com/JorisJonkers-dev/identity/internal/server"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/kubernetes"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

const (
	defaultAddr        = ":8080"
	defaultMetricsAddr = ":9090"
	// Audit Events older than a year are removed this often, by every replica.
	pruneInterval = time.Hour
	// Each replica reads the signing keys this often, so a new key signs on time everywhere.
	keysInterval = time.Hour
	// Each replica gives new Secret Clients their secrets this often, and retries ones that failed.
	secretsInterval = time.Minute
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

// run is the composition root: it reads the environment, opens the store and the Clients, and
// serves what assembly builds from them.
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
	index, synced, err := watchClients(ctx, cfg, logger)
	if err != nil {
		return err
	}
	built := assembly.Config{Store: store, PublicURL: cfg.publicURL, Mailer: cfg.mailer, Index: index, Synced: synced, Grafana: cfg.grafana, Logger: logger, Version: version, TOTPMatch: totpMatch()}
	v := cfg.vault.vault()
	transit := v.Transit(cfg.vault.transitMount)
	built.Sealer, built.KeySealer, built.KV = transit.Key(cfg.vault.totpKey), transit.Key(cfg.vault.signingKey), v.KV()
	if cfg.inCluster {
		client, err := kubernetes.InCluster(cfg.serviceAccounts.caFile, cfg.vault.jwtPath)
		if err != nil {
			return err
		}
		built.ServiceAccounts = kubernetes.New(client, cfg.serviceAccounts.issuer, cfg.serviceAccounts.jwksURL, cfg.publicURL)
	}
	identity, err := assembly.New(ctx, built)
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
	stopMetrics := serveMetrics(mln, metrics.Handler(identity.Unassigned, logger), logger)
	defer stopMetrics()
	go schedule.Every(ctx, pruneInterval, prune(identity.Prune, logger, "audit events"), func(err error) {
		logger.ErrorContext(ctx, "audit events not pruned", "error", err)
	})
	go schedule.Every(ctx, pruneInterval, prune(identity.PruneConnected, logger, "connected app records"), func(err error) {
		logger.ErrorContext(ctx, "connected app records not pruned", "error", err)
	})
	go schedule.Every(ctx, keysInterval, identity.RefreshKeys, func(err error) {
		logger.ErrorContext(ctx, "signing keys not refreshed", "error", err)
	})
	go schedule.Every(ctx, secretsInterval, identity.ProvisionSecrets, func(err error) {
		logger.ErrorContext(ctx, "client secrets not provisioned", "error", err)
	})
	return server.New(logger, version, identity.Routes).Serve(ctx, ln)
}

// prune runs a job removing what has expired, logging how many of what it removed.
func prune(job func(context.Context) (int64, error), logger *slog.Logger, what string) func(context.Context) error {
	return func(ctx context.Context) error {
		n, err := job(ctx)
		if n > 0 {
			logger.InfoContext(ctx, what+" pruned", "count", n)
		}
		return err
	}
}

// vaultConfig reaches Vault: its transit engine, which seals the signing keys and TOTP secrets
// (docs/adr/0013), and its KV engine, where Secret Clients' secrets are kept (identity#32).
type vaultConfig struct {
	addr, token, role, authMount, jwtPath, transitMount, totpKey, signingKey string
}

// vault is Vault. A token signs in directly, for a local dev-mode Vault; without one identity signs
// in with its service account.
func (v vaultConfig) vault() *vault.Vault {
	login := vault.Kubernetes(v.authMount, v.role, v.jwtPath)
	if v.token != "" {
		login = vault.Token(v.token)
	}
	return vault.New(v.addr, login, &http.Client{Timeout: vaultTimeout})
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
	// grafana is where the dashboard links for charts over time; nil when unset.
	grafana *url.URL
	// serviceAccounts is where the cluster's ServiceAccount tokens are checked, in a cluster.
	serviceAccounts serviceAccountConfig
}

// serviceAccountConfig reaches the API server's keys for ServiceAccount tokens (identity#22).
type serviceAccountConfig struct {
	issuer, jwksURL, caFile string
}

// or is value, or fallback when value is empty.
func or(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// readVault reads how to reach Vault at addr, with its defaults.
func readVault(getenv func(string) string, addr string) vaultConfig {
	return vaultConfig{
		addr: addr, token: getenv("VAULT_TOKEN"), signingKey: or(getenv("VAULT_SIGNING_KEY"), "identity-signing"),
		role: or(getenv("VAULT_ROLE"), "identity"), authMount: or(getenv("VAULT_AUTH_MOUNT"), "kubernetes"),
		jwtPath:      or(getenv("VAULT_JWT_PATH"), "/var/run/secrets/kubernetes.io/serviceaccount/token"),
		transitMount: or(getenv("VAULT_TRANSIT_MOUNT"), "transit"), totpKey: or(getenv("VAULT_TOTP_KEY"), "identity-totp"),
	}
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
		serviceAccounts: serviceAccountConfig{
			issuer:  or(getenv("SERVICE_ACCOUNT_ISSUER"), "https://kubernetes.default.svc.cluster.local"),
			jwksURL: or(getenv("SERVICE_ACCOUNT_JWKS_URL"), "https://kubernetes.default.svc/openid/v1/jwks"),
			caFile:  or(getenv("SERVICE_ACCOUNT_CA_FILE"), "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"),
		},
		vault: readVault(getenv, need("VAULT_ADDR")),
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
	if raw := getenv("GRAFANA_URL"); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return config{}, fmt.Errorf("GRAFANA_URL %q is not an https address", raw)
		}
		cfg.grafana = u
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
