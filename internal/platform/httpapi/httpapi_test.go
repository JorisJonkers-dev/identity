package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	accessweb "github.com/JorisJonkers-dev/identity/internal/access/adapters/web"
	accessapp "github.com/JorisJonkers-dev/identity/internal/access/app"
	launcherdomain "github.com/JorisJonkers-dev/identity/internal/launcher/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/httpapi"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// users accepts one session token, "good", for one User.
type users struct{}

func (users) SignUp(context.Context, string, string, string) error { return nil }
func (users) Confirm(context.Context, string, string) error        { return nil }
func (users) SignIn(context.Context, string, string) (string, error) {
	return "good", nil
}
func (users) SignOut(context.Context, string) error { return nil }
func (users) SignedIn(_ context.Context, token string) (domain.User, app.Session, error) {
	if token != "good" {
		return domain.User{}, app.Session{}, app.ErrNoSession
	}
	e, _ := domain.NewEmail("ada@example.org")
	n, _ := domain.NewName("Ada")
	return domain.User{ID: uuid.New(), Email: e, Name: n, CreatedAt: time.Unix(0, 0).UTC()}, app.Session{Methods: []string{"pwd"}}, nil
}
func (users) CompleteSignIn(context.Context, string, string) (string, error) { return "good", nil }
func (users) SecondFactors(context.Context, uuid.UUID) ([]app.Factor, error) {
	return nil, nil
}
func (users) ChallengeKinds(context.Context, string) ([]string, error) { return nil, nil }
func (users) PasskeyOptions(context.Context, string) ([]byte, error)   { return nil, nil }
func (users) CompleteSignInWithPasskey(context.Context, string, []byte) (string, error) {
	return "good", nil
}
func (users) StartPasskey(context.Context, uuid.UUID) ([]byte, error) { return nil, nil }
func (users) ConfirmPasskey(context.Context, uuid.UUID, string, string, []byte, string) error {
	return nil
}

func (users) StartTOTP(context.Context, uuid.UUID) (app.TOTPSetup, error) {
	return app.TOTPSetup{}, nil
}

func (users) ConfirmTOTP(context.Context, uuid.UUID, string, string, string, string) error {
	return nil
}

func (users) RemoveFactor(context.Context, uuid.UUID, uuid.UUID, string, string) error { return nil }

func (users) Rename(context.Context, uuid.UUID, string) error { return nil }

func (users) ChangePassword(context.Context, uuid.UUID, string, string, string) error { return nil }

func (u users) User(ctx context.Context, _ uuid.UUID) (domain.User, error) {
	user, _, err := u.SignedIn(ctx, "good")
	return user, err
}

// access knows no Admin; only the operations the tests reach are answered.
type access struct{ accessweb.UseCases }

func (access) IsAdmin(context.Context, uuid.UUID) (bool, error) { return false, nil }

func (access) Overview(_ context.Context, actor accessapp.Actor) (accessapp.Overview, error) {
	if actor.ID == uuid.Nil {
		return accessapp.Overview{}, errors.New("no principal reached the access adapter")
	}
	return accessapp.Overview{}, accessapp.ErrNotAdmin
}

// launcher opens Grafana for whoever is signed in.
type launcher struct{}

func (launcher) Apps(context.Context, uuid.UUID) ([]launcherdomain.App, error) {
	return []launcherdomain.App{{Client: launcherdomain.Client{ID: "grafana", Label: "Grafana", Host: "grafana.test"}, Roles: []string{"user"}}}, nil
}

func (launcher) Icon(context.Context, string) (launcherdomain.Icon, error) {
	return launcherdomain.Icon{}, nil
}

func serve(t *testing.T) *httptest.Server {
	t.Helper()
	h, err := httpapi.New(slog.New(slog.DiscardHandler), "1.2.3", users{}, access{}, launcher{})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, srv *httptest.Server, method, path string) (int, http.Header, map[string]any) {
	t.Helper()
	return send(t, srv, method, path, "", "", "")
}

func send(t *testing.T, srv *httptest.Server, method, path, contentType, body, session string) (int, http.Header, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if session != "" {
		req.AddCookie(&http.Cookie{Name: "__Host-identity_session", Value: session, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	decoded := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("%s %s: body is not JSON: %s", method, path, raw)
		}
	}
	return resp.StatusCode, resp.Header, decoded
}

func TestVersionNeedsNoSignIn(t *testing.T) {
	code, _, body := call(t, serve(t), http.MethodGet, "/api/v1/version")
	if code != http.StatusOK || body["version"] != "1.2.3" {
		t.Fatalf("GET /api/v1/version = %d %v, want 200 with the version", code, body)
	}
}

func TestProblems(t *testing.T) {
	srv := serve(t)
	cases := []struct {
		name, method, path string
		want               int
	}{
		{"unknown path", http.MethodGet, "/api/v1/nope", http.StatusNotFound},
		{"wrong method", http.MethodDelete, "/api/v1/version", http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, header, body := call(t, srv, tc.method, tc.path)
			if code != tc.want || body["status"] != float64(tc.want) {
				t.Fatalf("%s %s = %d %v, want a %d problem", tc.method, tc.path, code, body, tc.want)
			}
			if ct := header.Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("Content-Type = %q", ct)
			}
		})
	}
}

func TestTheSessionCookieGuardsTheProfile(t *testing.T) {
	srv := serve(t)
	for name, session := range map[string]string{"no cookie": "", "an unknown session": "stolen"} {
		code, _, body := send(t, srv, http.MethodGet, "/api/v1/profile", "", "", session)
		if code != http.StatusUnauthorized || body["status"] != float64(http.StatusUnauthorized) {
			t.Errorf("%s: GET /api/v1/profile = %d %v, want a 401 problem", name, code, body)
		}
	}
	code, _, body := send(t, srv, http.MethodGet, "/api/v1/profile", "", "", "good")
	if code != http.StatusOK || body["email"] != "ada@example.org" || body["admin"] != false {
		t.Fatalf("signed in: GET /api/v1/profile = %d %v", code, body)
	}
}

func TestOnlyAnAdminReadsAccess(t *testing.T) {
	srv := serve(t)
	if code, _, _ := send(t, srv, http.MethodGet, "/api/v1/access", "", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("signed out: GET /api/v1/access = %d, want 401", code)
	}
	code, _, body := send(t, srv, http.MethodGet, "/api/v1/access", "", "", "good")
	if code != http.StatusForbidden || body["detail"] != "Only Admins manage access." {
		t.Fatalf("a signed-in non-Admin: GET /api/v1/access = %d %v, want a 403 problem", code, body)
	}
}

func TestTheLauncherNeedsASignIn(t *testing.T) {
	if code, _, _ := send(t, serve(t), http.MethodGet, "/api/v1/launcher", "", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("signed out: GET /api/v1/launcher = %d, want 401", code)
	}
}

func TestTheLauncherListsTheSignedInUsersApps(t *testing.T) {
	code, _, body := send(t, serve(t), http.MethodGet, "/api/v1/launcher", "", "", "good")
	apps, _ := body["apps"].([]any)
	if code != http.StatusOK || len(apps) != 1 {
		t.Fatalf("GET /api/v1/launcher = %d %v", code, body)
	}
}

func TestSignInSetsTheCookieOverHTTP(t *testing.T) {
	code, header, _ := send(t, serve(t), http.MethodPost, "/api/v1/session", "application/json", `{"email":"a@b.c","password":"correct horse battery"}`, "")
	if code != http.StatusNoContent || !strings.HasPrefix(header.Get("Set-Cookie"), "__Host-identity_session=good;") {
		t.Fatalf("POST /api/v1/session = %d, Set-Cookie %q", code, header.Get("Set-Cookie"))
	}
}

// A cross-site form can post only urlencoded, multipart or plain text, never JSON, so refusing
// every other content type keeps a forged form from using the session cookie.
func TestAFormCannotPostToTheAPI(t *testing.T) {
	srv := serve(t)
	for _, ct := range []string{"application/x-www-form-urlencoded", "multipart/form-data; boundary=x", "text/plain"} {
		code, _, _ := send(t, srv, http.MethodPost, "/api/v1/profile/password", ct, "current=a&next=b", "good")
		if code < 400 || code >= 500 {
			t.Errorf("a %s post = %d, want a 4xx", ct, code)
		}
	}
}

func TestABodyOverTheCapIsRefusedUnread(t *testing.T) {
	big := `{"email":"a@b.c","password":"` + strings.Repeat("x", 64<<10) + `"}`
	code, _, body := send(t, serve(t), http.MethodPost, "/api/v1/session", "application/json", big, "")
	if code != http.StatusRequestEntityTooLarge || body["status"] != float64(http.StatusRequestEntityTooLarge) {
		t.Fatalf("a 64 KiB sign-in = %d %v, want a 413 problem", code, body)
	}
}
