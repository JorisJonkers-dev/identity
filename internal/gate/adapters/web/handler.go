// Package web is the Gate's HTTP adapter: the check Traefik's forwardAuth calls for every request
// to a gated host, the start of a sign-in on the Identity Provider's host, and the callback on the
// gated host's own /.auth/ path that sets its cookie (docs/adr/0003).
package web

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/gate/app"
)

// The host's own cookies. __Host- keeps each to the host that set it, over HTTPS, with Path=/.
const (
	sessionCookie = "__Host-identity_gate"
	stateCookie   = "__Host-identity_gate_state"
)

const (
	// The host cookie outlives no Single sign-on session: the session it names is checked.
	sessionMaxAge = int(30 * 24 * time.Hour / time.Second)
	stateMaxAge   = int(10 * time.Minute / time.Second)
	expireNow     = -1
)

// UseCases is what the adapter needs from the Gate.
type UseCases interface {
	Client(ctx context.Context, host string) (app.Client, error)
	Check(ctx context.Context, host, cookie string) (app.Verdict, error)
	CheckBearer(ctx context.Context, host, secret string) (app.Verdict, error)
	MCP(ctx context.Context, host string) bool
	Start(ctx context.Context, ssoToken, host, state, returnPath string) (string, error)
	Finish(ctx context.Context, host, code, state string) (string, string, error)
	NewState() (string, error)
}

// Config is where the Gate's pages send the browser.
type Config struct {
	// Issuer is the Identity Provider's URL, where a sign-in starts.
	Issuer string
	// SessionCookie is the Single sign-on session cookie on the Issuer's host.
	SessionCookie string
}

//go:embed pages.html
var pagesFS embed.FS

// policy is the pages' own: they are served on other apps' hosts, so nothing loads, and the one
// inline style is allowed by its hash.
func policy(fsys embed.FS) string {
	raw, _ := fsys.ReadFile("pages.html")
	_, rest, _ := strings.Cut(string(raw), "<style>")
	style, _, _ := strings.Cut(rest, "</style>")
	sum := sha256.Sum256([]byte(style))
	return "default-src 'none'; style-src 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'; " +
		"form-action 'none'; base-uri 'none'; frame-ancestors 'none'"
}

// Handler serves /gate/check, /gate/start and /.auth/callback.
type Handler struct {
	uc     UseCases
	cfg    Config
	scheme string
	logger *slog.Logger
	pages  *template.Template
	policy string
}

// New returns a Handler. Gated hosts are reached with the Issuer's scheme.
func New(uc UseCases, cfg Config, logger *slog.Logger) (http.Handler, error) {
	issuer, err := url.Parse(cfg.Issuer)
	if err != nil {
		return nil, err
	}
	h := &Handler{
		uc: uc, cfg: cfg, scheme: issuer.Scheme, logger: logger,
		pages: template.Must(template.ParseFS(pagesFS, "pages.html")), policy: policy(pagesFS),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gate/check", h.check)
	mux.HandleFunc("GET /gate/start", h.start)
	mux.HandleFunc("GET /.auth/callback", h.callback)
	return mux, nil
}

// check answers forwardAuth: 200 with the User's headers, or the page the browser should see.
func (h *Handler) check(w http.ResponseWriter, r *http.Request) {
	host := r.Header.Get("X-Forwarded-Host")
	if secret, ok := bearerToken(r); ok {
		h.bearer(w, r, host, secret)
		return
	}
	v, err := h.uc.Check(r.Context(), host, cookie(r, sessionCookie))
	switch {
	case err == nil:
		allow(w, v)
	case errors.Is(err, app.ErrBearerNeeded):
		unauthorized(w, h.challenge(r, host, ""))
	case errors.Is(err, app.ErrNoSignIn):
		h.signInPage(w, r, host)
	case errors.Is(err, app.ErrNoAccess):
		h.noAccess(w, r, host)
	case errors.Is(err, app.ErrUnknownHost):
		h.page(w, http.StatusForbidden, "unknown", nil)
	default:
		h.fail(w, r, err)
	}
}

// bearer answers forwardAuth for a request carrying a Personal Token. A script reads the answer,
// not a person, so a refusal is a bare 401 that says the token is not good, never a sign-in page.
func (h *Handler) bearer(w http.ResponseWriter, r *http.Request, host, secret string) {
	v, err := h.uc.CheckBearer(r.Context(), host, secret)
	switch {
	case err == nil:
		allow(w, v)
	case errors.Is(err, app.ErrNoSignIn), errors.Is(err, app.ErrNoAccess):
		unauthorized(w, h.challenge(r, host, `, error="invalid_token"`))
	case errors.Is(err, app.ErrUnknownHost):
		h.page(w, http.StatusForbidden, "unknown", nil)
	default:
		h.fail(w, r, err)
	}
}

func allow(w http.ResponseWriter, v app.Verdict) {
	w.Header().Set("X-Auth-Request-User", v.UserID.String())
	w.Header().Set("X-Auth-Request-Email", v.Email)
	w.Header().Set("X-Auth-Request-Roles", strings.Join(v.Roles, ","))
	w.WriteHeader(http.StatusOK)
}

// challenge is the Bearer challenge for a host; an MCP resource's points the tool to its metadata
// (RFC 9728), where it learns which authorization server issues its tokens.
func (h *Handler) challenge(r *http.Request, host, errorParam string) string {
	c := `Bearer realm="` + host + `"` + errorParam
	if h.uc.MCP(r.Context(), host) {
		c += `, resource_metadata="` + h.cfg.Issuer + `/.well-known/oauth-protected-resource/` + host + `"`
	}
	return c
}

func unauthorized(w http.ResponseWriter, challenge string) {
	w.Header().Set("WWW-Authenticate", challenge)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
}

// bearerToken is the Personal Token or Connected App's token a request carries as its bearer
// token. Any other bearer token is the app's own and is left alone (docs/adr/0017).
func bearerToken(r *http.Request) (string, bool) {
	scheme, secret, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || !strings.HasPrefix(secret, "idpat_") && !strings.HasPrefix(secret, "idapp_") {
		return "", false
	}
	return secret, true
}

// signInPage is the one-button page a visitor without the host's cookie sees. It sets the state
// that the sign-in must bring back, so a code carried to this browser from another is refused.
func (h *Handler) signInPage(w http.ResponseWriter, r *http.Request, host string) {
	client, err := h.uc.Client(r.Context(), host)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	state, err := h.uc.NewState()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	back := r.Header.Get("X-Forwarded-Uri")
	start := h.cfg.Issuer + "/gate/start?" + url.Values{"host": {host}, "state": {state}, "rd": {back}}.Encode()
	http.SetCookie(w, hostCookie(stateCookie, state, stateMaxAge))
	if h.uc.MCP(r.Context(), host) {
		w.Header().Set("WWW-Authenticate", h.challenge(r, host, ""))
	}
	h.page(w, http.StatusUnauthorized, "signin", map[string]string{"App": client.Label, "Start": start})
}

func (h *Handler) noAccess(w http.ResponseWriter, r *http.Request, host string) {
	client, err := h.uc.Client(r.Context(), host)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, http.StatusForbidden, "noaccess", map[string]string{"App": client.Label})
}

// start runs on the Issuer's host: with a Single sign-on session it sends the browser back to the
// gated host with a code, and without one it sends it to sign in first.
func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	host := q.Get("host")
	code, err := h.uc.Start(r.Context(), cookie(r, h.cfg.SessionCookie), host, q.Get("state"), q.Get("rd"))
	switch {
	case err == nil:
		target := h.scheme + "://" + host + "/.auth/callback?" + url.Values{"code": {code}}.Encode()
		http.Redirect(w, r, target, http.StatusSeeOther) //nolint:gosec // Start refused any host that is not a gated Client's

	case errors.Is(err, app.ErrNoSignIn):
		http.Redirect(w, r, "/signin?"+url.Values{"gate": {r.URL.RequestURI()}}.Encode(), http.StatusSeeOther)
	case errors.Is(err, app.ErrStepUpNeeded):
		http.Redirect(w, r, "/signin/step-up?"+url.Values{"next": {r.URL.RequestURI()}}.Encode(), http.StatusSeeOther)
	case errors.Is(err, app.ErrTwoFactorNeeded):
		http.Redirect(w, r, "/security/two-factor?for=step-up", http.StatusSeeOther)
	case errors.Is(err, app.ErrNoAccess):
		h.noAccess(w, r, host)
	case errors.Is(err, app.ErrUnknownHost), errors.Is(err, app.ErrInvalidCode):
		h.page(w, http.StatusBadRequest, "broken", nil)
	default:
		h.fail(w, r, err)
	}
}

// callback runs on the gated host: it redeems the code in the browser holding the page's state and
// sets the host's own cookie.
func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	token, back, err := h.uc.Finish(r.Context(), r.Host, r.URL.Query().Get("code"), cookie(r, stateCookie))
	switch {
	case err == nil:
		http.SetCookie(w, hostCookie(stateCookie, "", expireNow))
		http.SetCookie(w, hostCookie(sessionCookie, token, sessionMaxAge))
		http.Redirect(w, r, back, http.StatusSeeOther) //nolint:gosec // app.SafePath kept the path on this host
	case errors.Is(err, app.ErrInvalidCode):
		h.page(w, http.StatusBadRequest, "broken", nil)
	default:
		h.fail(w, r, err)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "gate", "error", err)
	h.page(w, http.StatusInternalServerError, "failed", nil)
}

func (h *Handler) page(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", h.policy)
	w.WriteHeader(status)
	_ = h.pages.ExecuteTemplate(w, name, data)
}

func cookie(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

func hostCookie(name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	}
}
