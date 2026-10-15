// Package server is the HTTP surface of the service: the probes, the API under /api, the OIDC
// Provider, the web app everywhere else, and a graceful drain when its context is cancelled.
package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/httpx"
)

const (
	readHeaderTimeout = 5 * time.Second
	shutdownGrace     = 10 * time.Second
	readyTimeout      = 2 * time.Second
)

// Routes are what the server serves besides its probes.
type Routes struct {
	// API serves every path under /api/.
	API http.Handler
	// OIDC serves the Provider: discovery and every path under /oidc/.
	OIDC http.Handler
	// Web serves every other path: the single-page app.
	Web http.Handler
	// Ready reports whether the service's dependencies answer; /readyz fails while it errors.
	Ready func(context.Context) error
}

// Server serves the probes and Routes.
type Server struct {
	logger  *slog.Logger
	version string
	routes  Routes
	grace   time.Duration
	serving atomic.Bool
}

// Option changes a Server's defaults.
type Option func(*Server)

// WithShutdownGrace bounds how long a drain waits for in-flight requests; the default is 10s.
func WithShutdownGrace(d time.Duration) Option {
	return func(s *Server) { s.grace = d }
}

// New returns a Server that logs to logger and reports version at startup.
func New(logger *slog.Logger, version string, routes Routes, opts ...Option) *Server {
	s := &Server{logger: logger, version: version, routes: routes, grace: shutdownGrace}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Handler returns the routes, behind the browser hardening headers. Liveness never depends on
// readiness, and readiness fails from the moment the drain starts.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		plain(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !s.serving.Load() {
			plain(w, http.StatusServiceUnavailable, "not ready")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
		defer cancel()
		if err := s.routes.Ready(ctx); err != nil {
			s.logger.WarnContext(ctx, "not ready", "error", err)
			plain(w, http.StatusServiceUnavailable, "not ready")
			return
		}
		plain(w, http.StatusOK, "ready")
	})
	mux.Handle("/api/", s.routes.API)
	mux.Handle("/.well-known/openid-configuration", s.routes.OIDC)
	mux.Handle("/oidc/", s.routes.OIDC)
	mux.Handle("/", s.routes.Web)
	return httpx.SecurityHeaders(mux)
}

// Serve accepts on ln until ctx is cancelled, then fails readiness and drains in-flight
// requests for up to the shutdown grace period.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: readHeaderTimeout}

	errs := make(chan error, 1)
	go func() { errs <- srv.Serve(ln) }()
	s.serving.Store(true)
	s.logger.Info("listening", "addr", ln.Addr().String(), "version", s.version)

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	s.serving.Store(false)
	drain, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.grace)
	defer cancel()
	if err := srv.Shutdown(drain); err != nil {
		return err
	}
	<-errs // http.ErrServerClosed: Serve returns nothing else once Shutdown has run
	s.logger.Info("stopped")
	return nil
}

func plain(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, body+"\n")
}
