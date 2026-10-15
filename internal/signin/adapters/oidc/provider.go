package oidc

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/JorisJonkers-dev/identity/internal/platform/httpx"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

const (
	loginPath = "/oidc/login"
	// bindingCookie ties an authorization request to the browser that started it.
	bindingCookie = "__Host-identity_authorize"
)

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
	mux.Handle("/oidc/authorize", bind(provider))
	mux.Handle("GET /oidc/authorize/callback", bound(provider))
	mux.Handle("GET "+loginPath, op.NewIssuerInterceptor(provider.IssuerFromRequest).Handler(l))
	mux.Handle("/", provider)
	return mux, nil
}

// bind sets the binding cookie when /authorize sends the browser to log in, so only that browser
// can complete the request and collect its code. Without it, someone could start a request, lure a
// signed-in person to its login link, and collect a code for them.
func bind(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(binder{w}, r)
	})
}

type binder struct{ http.ResponseWriter }

func (b binder) WriteHeader(code int) {
	if id, ok := strings.CutPrefix(b.Header().Get("Location"), loginPath+"?authRequestID="); ok {
		http.SetCookie(b.ResponseWriter, binding(id, int(bindingLifetime/time.Second)))
	}
	b.ResponseWriter.WriteHeader(code)
}

// bound lets only the browser that started a request collect its code, and clears the binding.
func bound(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !startedHere(r, r.URL.Query().Get("id")) {
			httpx.WriteProblem(w, http.StatusBadRequest, startedElsewhere)
			return
		}
		http.SetCookie(w, binding("", expireNow))
		next.ServeHTTP(w, r)
	})
}

// bindingLifetime outlasts the sign-in context's ten minutes for a request.
const bindingLifetime = 15 * time.Minute

const startedElsewhere = "This sign-in was started in another browser. Start again from the app."

// expireNow is net/http's MaxAge for "Max-Age=0".
const expireNow = -1

func startedHere(r *http.Request, id string) bool {
	c, err := r.Cookie(bindingCookie)
	return err == nil && id != "" && c.Value == id
}

func binding(id string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: bindingCookie, Value: id, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	}
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
	if !startedHere(r, id.String()) {
		httpx.WriteProblem(w, http.StatusBadRequest, startedElsewhere)
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
