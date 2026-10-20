package web_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/gate/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/gate/app"
)

var errBoom = errors.New("boom")

// useCases answers with what a test sets and records what it was asked.
type useCases struct {
	checkErr, bearerErr, startErr, finishErr, clientErr, stateErr error
	asked                                                         []string
	// mcp is whether the host's Client takes Connected Apps, and mcpAsked how often it was asked.
	mcp      bool
	mcpAsked int
	// fresh is the proof window the last Start was asked for.
	fresh time.Duration
}

func (u *useCases) MCP(context.Context, string) bool {
	u.mcpAsked++
	return u.mcp
}

func (u *useCases) Client(_ context.Context, host string) (app.Client, error) {
	return app.Client{ID: "stats", Label: "Tribelt stats", Host: host}, u.clientErr
}

func (u *useCases) Check(_ context.Context, host, cookie string) (app.Verdict, error) {
	u.asked = append(u.asked, host, cookie)
	return app.Verdict{UserID: uuid.MustParse("00000000-0000-4000-8000-0000000000a1"), Email: "ada@example.org", Roles: []string{"user", "viewer"}, Kind: app.KindUser, Via: app.ViaSession, Signed: "signed-principal"}, u.checkErr
}

func (u *useCases) CheckBearer(_ context.Context, host, secret string) (app.Verdict, error) {
	u.asked = append(u.asked, host, secret)
	return app.Verdict{UserID: uuid.MustParse("00000000-0000-4000-8000-0000000000a1"), Email: "ada@example.org", Roles: []string{"viewer"}}, u.bearerErr
}

func (u *useCases) Start(_ context.Context, sso, host, state, back string, fresh time.Duration) (string, error) {
	u.asked = append(u.asked, sso, host, state, back)
	u.fresh = fresh
	return "the-code", u.startErr
}

func (u *useCases) Finish(_ context.Context, host, code, state string) (string, string, error) {
	u.asked = append(u.asked, host, code, state)
	return "host-cookie", "/stats?week=42", u.finishErr
}

func (u *useCases) NewState() (string, error) { return "state-1", u.stateErr }

func serve(t *testing.T, uc *useCases) http.Handler {
	t.Helper()
	h, err := web.New(uc, web.Config{Issuer: "https://auth.test", SessionCookie: "__Host-identity_session"}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func get(t *testing.T, h http.Handler, target string, header map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func forwarded(uri string) map[string]string {
	return map[string]string{"X-Forwarded-Host": "tribelt.jorisjonkers.dev", "X-Forwarded-Uri": uri}
}

func setCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestACheckWithTheHostCookiePassesTheUserOn(t *testing.T) {
	uc := &useCases{}
	rec := get(t, serve(t, uc), "http://identity:8080/gate/check", forwarded("/stats"), &http.Cookie{Name: "__Host-identity_gate", Value: "c"}) //nolint:gosec // a request\'s cookie carries no attributes
	if rec.Code != http.StatusOK || rec.Header().Get("X-Auth-Request-User") != "00000000-0000-4000-8000-0000000000a1" ||
		rec.Header().Get("X-Auth-Request-Email") != "ada@example.org" || rec.Header().Get("X-Auth-Request-Roles") != "user,viewer" {
		t.Fatalf("check = %d %v", rec.Code, rec.Header())
	}
	if uc.asked[0] != "tribelt.jorisjonkers.dev" || uc.asked[1] != "c" {
		t.Fatalf("asked %v", uc.asked)
	}
}

// Traefik sends forwardAuth the upgrade request's headers; the check decides on the cookie alone.
func TestAWebSocketUpgradePassesWithTheHostCookie(t *testing.T) {
	headers := forwarded("/api/live")
	headers["Connection"], headers["Upgrade"], headers["Sec-WebSocket-Version"] = "Upgrade", "websocket", "13"
	headers["Sec-WebSocket-Key"] = "dGhlIHNhbXBsZSBub25jZQ=="
	rec := get(t, serve(t, &useCases{}), "http://identity:8080/gate/check", headers, &http.Cookie{Name: "__Host-identity_gate", Value: "c"}) //nolint:gosec // a request\'s cookie carries no attributes
	if rec.Code != http.StatusOK || rec.Header().Get("X-Auth-Request-User") == "" {
		t.Fatalf("an upgrade = %d %v", rec.Code, rec.Header())
	}
}

func TestAVisitorWithoutTheCookieGetsOneSignInButton(t *testing.T) {
	rec := get(t, serve(t, &useCases{checkErr: app.ErrNoSignIn}), "http://identity:8080/gate/check", forwarded("/stats?week=42"))
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "<h1>Sign in to Tribelt stats</h1>") {
		t.Fatalf("check = %d %s", rec.Code, rec.Body)
	}
	state := setCookie(rec, "__Host-identity_gate_state")
	if state == nil || state.Value != "state-1" || !state.Secure || !state.HttpOnly || state.Path != "/" || state.Domain != "" || state.MaxAge != 600 {
		t.Fatalf("state cookie = %+v", state)
	}
	start := url.Values{"host": {"tribelt.jorisjonkers.dev"}, "state": {"state-1"}, "rd": {"/stats?week=42"}}.Encode()
	if !strings.Contains(rec.Body.String(), `href="https://auth.test/gate/start?`+strings.ReplaceAll(start, "&", "&amp;")+`"`) {
		t.Fatalf("the button does not start this sign-in: %s", rec.Body)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.HasPrefix(csp, "default-src 'none'; style-src 'sha256-") || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v", rec.Header())
	}
}

func TestACheckRefuses(t *testing.T) {
	for _, tc := range []struct {
		uc     *useCases
		status int
		text   string
	}{
		{&useCases{checkErr: app.ErrNoAccess}, http.StatusForbidden, "You don't have access to Tribelt stats"},
		{&useCases{checkErr: app.ErrUnknownHost}, http.StatusForbidden, "No app is at this address"},
		{&useCases{checkErr: errBoom}, http.StatusInternalServerError, "Something went wrong"},
		{&useCases{checkErr: app.ErrNoSignIn, clientErr: errBoom}, http.StatusInternalServerError, "Something went wrong"},
		{&useCases{checkErr: app.ErrNoSignIn, stateErr: errBoom}, http.StatusInternalServerError, "Something went wrong"},
		{&useCases{checkErr: app.ErrNoAccess, clientErr: errBoom}, http.StatusInternalServerError, "Something went wrong"},
	} {
		rec := get(t, serve(t, tc.uc), "http://identity:8080/gate/check", forwarded("/"))
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.text) {
			t.Errorf("check = %d, want %d with %q: %s", rec.Code, tc.status, tc.text, rec.Body)
		}
	}
}

func TestStartSendsTheCodeBackToTheHost(t *testing.T) {
	uc := &useCases{}
	target := "https://auth.test/gate/start?host=tribelt.jorisjonkers.dev&state=state-1&rd=%2Fstats"
	rec := get(t, serve(t, uc), target, nil, &http.Cookie{Name: "__Host-identity_session", Value: "sso"}) //nolint:gosec // a request\'s cookie carries no attributes
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "https://tribelt.jorisjonkers.dev/.auth/callback?code=the-code" {
		t.Fatalf("start = %d %v", rec.Code, rec.Header())
	}
	if strings.Join(uc.asked, " ") != "sso tribelt.jorisjonkers.dev state-1 /stats" {
		t.Fatalf("asked %v", uc.asked)
	}

	rec = get(t, serve(t, &useCases{startErr: app.ErrNoSignIn}), target, nil)
	if want := "/signin?gate=" + url.QueryEscape("/gate/start?host=tribelt.jorisjonkers.dev&state=state-1&rd=%2Fstats"); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
		t.Fatalf("start without a session = %d %q, want %q", rec.Code, rec.Header().Get("Location"), want)
	}
	for _, tc := range []struct {
		err    error
		status int
	}{
		{app.ErrNoAccess, http.StatusForbidden},
		{app.ErrUnknownHost, http.StatusBadRequest},
		{app.ErrInvalidCode, http.StatusBadRequest},
		{errBoom, http.StatusInternalServerError},
	} {
		if rec := get(t, serve(t, &useCases{startErr: tc.err}), target, nil); rec.Code != tc.status {
			t.Errorf("start with %v = %d, want %d", tc.err, rec.Code, tc.status)
		}
	}
}

func TestTheCallbackSetsTheHostsOwnCookie(t *testing.T) {
	uc := &useCases{}
	rec := get(t, serve(t, uc), "https://tribelt.jorisjonkers.dev/.auth/callback?code=the-code", nil, &http.Cookie{Name: "__Host-identity_gate_state", Value: "state-1"}) //nolint:gosec // a request\'s cookie carries no attributes
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/stats?week=42" {
		t.Fatalf("callback = %d %v", rec.Code, rec.Header())
	}
	if strings.Join(uc.asked, " ") != "tribelt.jorisjonkers.dev the-code state-1" {
		t.Fatalf("asked %v", uc.asked)
	}
	session := setCookie(rec, "__Host-identity_gate")
	if session == nil || session.Value != "host-cookie" || !session.Secure || !session.HttpOnly || session.Path != "/" || session.Domain != "" ||
		session.SameSite != http.SameSiteLaxMode {
		t.Fatalf("host cookie = %+v", session)
	}
	if state := setCookie(rec, "__Host-identity_gate_state"); state == nil || state.MaxAge != -1 {
		t.Fatalf("the state cookie was not cleared: %+v", state)
	}
	if rec := get(t, serve(t, &useCases{finishErr: app.ErrInvalidCode}), "https://tribelt.jorisjonkers.dev/.auth/callback?code=x", nil); rec.Code != http.StatusBadRequest ||
		!strings.Contains(rec.Body.String(), "opened in another browser") {
		t.Fatalf("a refused code = %d %s", rec.Code, rec.Body)
	}
	if rec := get(t, serve(t, &useCases{finishErr: errBoom}), "https://tribelt.jorisjonkers.dev/.auth/callback?code=x", nil); rec.Code != http.StatusInternalServerError {
		t.Fatalf("a failure = %d", rec.Code)
	}
}

func TestABadIssuerIsRefused(t *testing.T) {
	if _, err := web.New(&useCases{}, web.Config{Issuer: "::"}, slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("a bad Issuer was accepted")
	}
}

func TestStartSendsAStaleProofToProveTwoFactorAgainAndBack(t *testing.T) {
	target := "https://auth.test/gate/start?host=tribelt.jorisjonkers.dev&state=state-1&rd=%2Fstats"
	rec := get(t, serve(t, &useCases{startErr: app.ErrStepUpNeeded}), target, nil)
	want := "/signin/step-up?next=" + url.QueryEscape("/gate/start?host=tribelt.jorisjonkers.dev&state=state-1&rd=%2Fstats")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
		t.Fatalf("start = %d %q, want %q", rec.Code, rec.Header().Get("Location"), want)
	}
}

func TestStartSendsAUserWithNoTwoFactorToSetItUp(t *testing.T) {
	rec := get(t, serve(t, &useCases{startErr: app.ErrTwoFactorNeeded}), "https://auth.test/gate/start?host=tribelt.jorisjonkers.dev&state=s", nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/security/two-factor?for=step-up" {
		t.Fatalf("start = %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestAnAllowedCheckSendsTheSignedPrincipal(t *testing.T) {
	rec := get(t, serve(t, &useCases{}), "http://identity:8080/gate/check", forwarded("/stats"), &http.Cookie{Name: "__Host-identity_gate", Value: "c"}) //nolint:gosec // a request's cookie carries no attributes

	if got := rec.Header().Get("X-Identity-Principal"); got != "signed-principal" {
		t.Fatalf("X-Identity-Principal = %q", got)
	}
}

func TestAnAllowedCheckSetsEveryPlainHeaderEvenWhenItIsEmpty(t *testing.T) {
	rec := get(t, serve(t, &useCases{}), "http://identity:8080/gate/check", forwarded("/stats"), &http.Cookie{Name: "__Host-identity_gate", Value: "c"}) //nolint:gosec // a request's cookie carries no attributes

	for _, name := range []string{"X-Auth-Request-User", "X-Auth-Request-Email", "X-Auth-Request-Name", "X-Auth-Request-Roles", "X-Auth-Request-Kind", "X-Auth-Request-Via", "X-Auth-Request-Actor"} {
		if _, ok := rec.Header()[name]; !ok {
			t.Errorf("%s is not set", name)
		}
	}
	if rec.Header().Get("X-Auth-Request-Via") != "session" || rec.Header().Get("X-Auth-Request-Name") != "" {
		t.Fatalf("headers = %v", rec.Header())
	}
}

func TestAnAppsStepUpSendsTheBrowserToStartWithItsStateAndWindow(t *testing.T) {
	rec := get(t, serve(t, &useCases{}), "https://tribelt.jorisjonkers.dev/.auth/step-up?minutes=5&rd=%2Fpay", nil)

	want := "https://auth.test/gate/start?" + url.Values{"host": {"tribelt.jorisjonkers.dev"}, "state": {"state-1"}, "rd": {"/pay"}, "fresh": {"5"}}.Encode()
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want || !strings.Contains(rec.Header().Get("Set-Cookie"), "__Host-identity_gate_state=state-1") {
		t.Fatalf("step-up = %d %q %q", rec.Code, rec.Header().Get("Location"), rec.Header().Get("Set-Cookie"))
	}
}

func TestAnAppsStepUpKeepsTheReturnPathOnTheHost(t *testing.T) {
	rec := get(t, serve(t, &useCases{}), "https://tribelt.jorisjonkers.dev/.auth/step-up?minutes=5&rd=https%3A%2F%2Fevil.test%2F", nil)

	if loc, _ := url.Parse(rec.Header().Get("Location")); loc.Query().Get("rd") != "/" {
		t.Fatalf("rd = %q", loc.Query().Get("rd"))
	}
}

func TestAStepUpWindowOutsideOneToSixtyMinutesIsRefused(t *testing.T) {
	for _, minutes := range []string{"", "0", "61", "five"} {
		t.Run("minutes="+minutes, func(t *testing.T) {
			rec := get(t, serve(t, &useCases{}), "https://tribelt.jorisjonkers.dev/.auth/step-up?minutes="+minutes+"&rd=%2F", nil)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("step-up = %d", rec.Code)
			}
		})
	}
}

func TestAStepUpWhoseStateCannotBeMadeFails(t *testing.T) {
	rec := get(t, serve(t, &useCases{stateErr: errBoom}), "https://tribelt.jorisjonkers.dev/.auth/step-up?minutes=5&rd=%2F", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("step-up = %d", rec.Code)
	}
}

func TestStartAsksForTheWindowAnAppsStepUpCarries(t *testing.T) {
	uc := &useCases{}
	get(t, serve(t, uc), "https://auth.test/gate/start?host=tribelt.jorisjonkers.dev&state=s&rd=%2Fpay&fresh=15", nil)

	if uc.fresh != 15*time.Minute {
		t.Fatalf("fresh = %v", uc.fresh)
	}
}

func TestStartRefusesAWindowOutsideOneToSixtyMinutes(t *testing.T) {
	for _, fresh := range []string{"0", "61", "soon"} {
		t.Run("fresh="+fresh, func(t *testing.T) {
			uc := &useCases{}
			rec := get(t, serve(t, uc), "https://auth.test/gate/start?host=tribelt.jorisjonkers.dev&state=s&fresh="+fresh, nil)
			if rec.Code != http.StatusBadRequest || len(uc.asked) != 0 {
				t.Fatalf("start = %d, asked %v", rec.Code, uc.asked)
			}
		})
	}
}
