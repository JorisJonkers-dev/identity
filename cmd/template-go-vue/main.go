// Command template-go-vue serves the API and the embedded web app, migrates its database at
// startup, and drains on SIGTERM.
//
// Environment: DATABASE_URL (required), ADDR (default :8080), and DEV_USER, which stands in for
// the platform's forward-auth on a local run and must never be set in a deployment.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/JorisJonkers-dev/template-go-vue/internal/notes/adapters/persistence"
	"github.com/JorisJonkers-dev/template-go-vue/internal/notes/app"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/httpapi"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/httpx"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/pg"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/webui"
	"github.com/JorisJonkers-dev/template-go-vue/internal/server"
	"github.com/JorisJonkers-dev/template-go-vue/web"
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
		logger.Error("template-go-vue stopped", "error", err)
		return 1
	}
	return 0
}

// run is the composition root: it reads the environment and builds the store, the use cases and
// the server. httpapi assembles the contexts' web adapters into the one generated API.
func run(ctx context.Context, logger *slog.Logger, getenv func(string) string) error {
	dbURL := getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set")
	}
	if err := pg.Migrate(ctx, dbURL); err != nil {
		return err
	}
	store, err := pg.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer store.Close()

	api, err := httpapi.New(logger, app.New(persistence.New(store.Queries())))
	if err != nil {
		return err
	}
	if subject := getenv("DEV_USER"); subject != "" {
		logger.Warn("DEV_USER is set: every request without an identity runs as it", "subject", subject)
		api = httpx.DevIdentity(subject, api)
	}

	addr := getenv("ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return server.New(logger, version, server.Routes{
		API:   api,
		Web:   webui.Handler(web.Dist()),
		Ready: store.Ping,
	}).Serve(ctx, ln)
}
