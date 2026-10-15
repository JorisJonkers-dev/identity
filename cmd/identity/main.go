// Command identity serves the API and the embedded web app, and drains on SIGTERM. It never
// migrates its database: the identity-migration image does, before it starts (docs/adr/0010).
//
// Environment: DATABASE_URL, PUBLIC_URL (where mailed links point), SMTP_ADDR and MAIL_FROM are
// required; SMTP_USERNAME and SMTP_PASSWORD sign in to the mail server; SMTP_TLS is "required"
// (the default) or "none", which only a local run against Mailpit may use; ADDR defaults to :8080.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/httpapi"
	"github.com/JorisJonkers-dev/identity/internal/platform/mail"
	"github.com/JorisJonkers-dev/identity/internal/platform/passwords"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/webui"
	"github.com/JorisJonkers-dev/identity/internal/server"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/accounts"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	signinoidc "github.com/JorisJonkers-dev/identity/internal/signin/adapters/oidc"
	signinpersistence "github.com/JorisJonkers-dev/identity/internal/signin/adapters/persistence"
	signinapp "github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/users/adapters/persistence"
	usersweb "github.com/JorisJonkers-dev/identity/internal/users/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/web"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

const defaultAddr = ":8080"

func main() {
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

	users := app.New(persistence.New(store.Queries()), passwords.NewArgon2id(maxConcurrentHashes, hashWait), cfg.mailer, time.Now, rand.Reader, cfg.publicURL)
	api, err := httpapi.New(logger, version, users)
	if err != nil {
		return err
	}
	provider, err := oidcProvider(cfg, store, users, logger)
	if err != nil {
		return err
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.addr)
	if err != nil {
		return err
	}
	return server.New(logger, version, server.Routes{
		API:   api,
		OIDC:  provider,
		Web:   webui.Handler(web.Dist()),
		Ready: store.Ping,
	}).Serve(ctx, ln)
}

// oidcProvider is the OIDC Provider over the sign-in context. It knows no Clients until they come
// from Kubernetes (identity#13), and signs with a key made at start-up until keys are kept
// (identity#30).
func oidcProvider(cfg config, store *pg.Store, users *app.Service, logger *slog.Logger) (http.Handler, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	svc := signinapp.New(signinpersistence.New(store.Queries()), clients.Static{}, accounts.New(users), time.Now, rand.Reader)
	return signinoidc.New(signinoidc.Config{
		Issuer:        cfg.publicURL,
		Insecure:      strings.HasPrefix(cfg.publicURL, "http://"),
		SessionCookie: usersweb.CookieName,
	}, svc, signinoidc.NewRing(key), logger, time.Now)
}

type config struct {
	databaseURL string
	publicURL   string
	addr        string
	mailer      mail.SMTP
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
	switch tlsMode := getenv("SMTP_TLS"); tlsMode {
	case "", "required":
		cfg.mailer.RequireTLS = true
	case "none":
	default:
		return config{}, fmt.Errorf("SMTP_TLS %q is not \"required\" or \"none\"", tlsMode)
	}
	return cfg, nil
}
