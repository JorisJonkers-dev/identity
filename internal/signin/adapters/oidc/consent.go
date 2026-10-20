package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"github.com/JorisJonkers-dev/identity/internal/platform/httpx"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

// consent serves the consent page's two calls (identity#23): what a tool asks to do, and the
// User's answer. Like the login check, both read the Single sign-on session and answer only the
// browser that started the request.
type consent struct {
	svc      *app.Service
	cookie   string
	issuer   func(context.Context) string
	callback func(context.Context, string) string
	logger   *slog.Logger
}

// consentPrompt is what the consent page shows.
type consentPrompt struct {
	App          string   `json:"app"`
	Client       string   `json:"client"`
	Roles        []string `json:"roles"`
	RedirectHost string   `json:"redirectHost"`
}

// consentAnswer is the User's answer; redirect is where the browser goes next.
type consentAnswer struct {
	AuthRequestID string `json:"authRequestID"`
	Allow         bool   `json:"allow"`
}

type consentNext struct {
	Redirect string `json:"redirect"`
}

const maxConsentAnswer = 4 << 10

func (c *consent) prompt(w http.ResponseWriter, r *http.Request) {
	id, token, ok := c.request(w, r, r.URL.Query().Get("authRequestID"))
	if !ok {
		return
	}
	p, err := c.svc.Prompt(r.Context(), id, token)
	if err != nil {
		c.refuse(w, r, err)
		return
	}
	u, _ := url.Parse(p.RedirectURI) // checked against the registration at /authorize
	writeJSON(w, http.StatusOK, consentPrompt{App: p.App, Client: p.ClientLabel, Roles: p.Roles, RedirectHost: u.Host})
}

func (c *consent) answer(w http.ResponseWriter, r *http.Request) {
	var a consentAnswer
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxConsentAnswer)).Decode(&a); err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "This answer could not be read. Start again from the app.")
		return
	}
	id, token, ok := c.request(w, r, a.AuthRequestID)
	if !ok {
		return
	}
	if a.Allow {
		if err := c.svc.Approve(r.Context(), id, token); err != nil {
			c.refuse(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, consentNext{Redirect: c.callback(r.Context(), id.String())})
		return
	}
	req, err := c.svc.Deny(r.Context(), id, token)
	if err != nil {
		c.refuse(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, consentNext{Redirect: denied(req.RedirectURI, req.State, c.issuer(r.Context()))})
}

// denied tells the tool the User said no (RFC 6749 §4.1.2.1), naming the issuer (RFC 9207).
func denied(redirectURI, state, issuer string) string {
	u, _ := url.Parse(redirectURI) // checked against the registration at /authorize
	q := u.Query()
	q.Set("error", string(oidc.AccessDenied))
	q.Set("error_description", "The person declined.")
	if state != "" {
		q.Set("state", state)
	}
	q.Set("iss", issuer)
	u.RawQuery = q.Encode()
	return u.String()
}

// request is the request id and session token a consent call carries, from the browser that
// started the request; false once it has answered the refusal.
func (c *consent) request(w http.ResponseWriter, r *http.Request, raw string) (uuid.UUID, string, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "This link is broken. Start again from the app.")
		return uuid.Nil, "", false
	}
	if !startedHere(r, id.String()) {
		httpx.WriteProblem(w, http.StatusBadRequest, startedElsewhere)
		return uuid.Nil, "", false
	}
	var token string
	if ck, err := r.Cookie(c.cookie); err == nil {
		token = ck.Value
	}
	return id, token, true
}

func (c *consent) refuse(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrNoSession):
		httpx.WriteProblem(w, http.StatusUnauthorized, "Sign in first.")
	case errors.Is(err, app.ErrNotFound):
		httpx.WriteProblem(w, http.StatusNotFound, "This request has expired or was already answered. Start again from the tool.")
	case errors.Is(err, app.ErrNoAccess):
		httpx.WriteProblem(w, http.StatusForbidden, "You have no access to that app.")
	case errors.Is(err, app.ErrNotConnectable):
		httpx.WriteProblem(w, http.StatusForbidden, "This tool cannot connect to that app.")
	default:
		c.logger.ErrorContext(r.Context(), "consenting to a Connected App", "error", err)
		httpx.WriteProblem(w, http.StatusInternalServerError, "")
	}
}
