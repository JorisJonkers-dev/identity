package oidc

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/JorisJonkers-dev/identity/internal/platform/httpx"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

const loginPath = "/oidc/login"

// Config is the Provider's setting.
type Config struct {
	// Issuer is the URL every Client trusts, https://auth.jorisjonkers.dev in production.
	Issuer string
	// Insecure allows an http Issuer, for local runs and tests only.
	Insecure bool
	// SessionCookie is the name of the Single sign-on session cookie.
	SessionCookie string
}

// New returns the Provider's handler: discovery at /.well-known/openid-configuration and every
// other endpoint under /oidc/.
func New(cfg Config, svc *app.Service, ring *Ring, logger *slog.Logger, now func() time.Time) (http.Handler, error) {
	var cryptoKey [32]byte // seals opaque tokens, which no Client is given; JWTs are signed by ring
	_, _ = rand.Read(cryptoKey[:])
	opts := []op.Option{
		op.WithLogger(logger),
		op.WithCustomEndpoints(
			op.NewEndpoint("/oidc/authorize"), op.NewEndpoint("/oidc/token"), op.NewEndpoint("/oidc/userinfo"),
			op.NewEndpoint("/oidc/revoke"), op.NewEndpoint("/oidc/end_session"), op.NewEndpoint("/oidc/keys"),
		),
		op.WithCustomIntrospectionEndpoint(op.NewEndpoint("/oidc/introspect")),
	}
	if cfg.Insecure {
		opts = append(opts, op.WithAllowInsecure())
	}
	provider, err := op.NewProvider(&op.Config{
		CryptoKey: cryptoKey, CodeMethodS256: true, AuthMethodPost: true,
	}, &storage{svc: svc, ring: ring, logger: logger, now: now}, op.StaticIssuer(cfg.Issuer), opts...)
	if err != nil {
		return nil, err
	}
	l := &login{svc: svc, cookie: cfg.SessionCookie, callback: op.AuthCallbackURL(provider), logger: logger}
	mux := http.NewServeMux()
	mux.Handle("GET "+loginPath, op.NewIssuerInterceptor(provider.IssuerFromRequest).Handler(l))
	mux.Handle("/", provider)
	return mux, nil
}

// login completes an authorization request from the Single sign-on session, or sends the browser
// to sign in first; the sign-in page comes back here when it is done.
type login struct {
	svc      *app.Service
	cookie   string
	callback func(context.Context, string) string
	logger   *slog.Logger
}

func (l *login) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.URL.Query().Get("authRequestID"))
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "This sign-in link is broken. Start again from the app.")
		return
	}
	var token string
	if c, err := r.Cookie(l.cookie); err == nil {
		token = c.Value
	}
	err = l.svc.Complete(r.Context(), id, token)
	switch {
	case err == nil:
		http.Redirect(w, r, l.callback(r.Context(), id.String()), http.StatusSeeOther)
	case errors.Is(err, app.ErrNoSession):
		http.Redirect(w, r, "/signin?authRequestID="+id.String(), http.StatusSeeOther) //nolint:gosec // a fixed path and a parsed UUID
	case errors.Is(err, app.ErrNotFound):
		httpx.WriteProblem(w, http.StatusBadRequest, "This sign-in has expired or was already used. Start again from the app.")
	default:
		l.logger.ErrorContext(r.Context(), "completing a sign-in", "error", err)
		httpx.WriteProblem(w, http.StatusInternalServerError, "")
	}
}
