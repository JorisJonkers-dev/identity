package web_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/launcher/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/launcher/app"
	"github.com/JorisJonkers-dev/identity/internal/launcher/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
)

var (
	errBoom = errors.New("boom")
	ada     = uuid.MustParse("00000000-0000-4000-8000-0000000000a2")
	grafana = domain.App{
		Client: domain.Client{ID: "grafana", Label: "Grafana", Description: "Metrics", Host: "grafana.test", IconURL: "https://grafana.test/icon.png"},
		Roles:  []string{"editor", "viewer"},
	}
	jellyfin = domain.App{Client: domain.Client{ID: "jellyfin", Label: "Jellyfin", Host: "jellyfin.test", LaunchURL: "https://jellyfin.test/web"}, Roles: []string{"user"}}
)

type useCases struct {
	apps  []domain.App
	icon  domain.Icon
	err   error
	asked []string
}

func (u *useCases) Apps(_ context.Context, userID uuid.UUID) ([]domain.App, error) {
	u.asked = append(u.asked, userID.String())
	return u.apps, u.err
}

func (u *useCases) Icon(_ context.Context, clientID string) (domain.Icon, error) {
	u.asked = append(u.asked, clientID)
	return u.icon, u.err
}

func signedIn() context.Context {
	return principal.With(context.Background(), principal.Principal{UserID: ada})
}

func launcher(t *testing.T, uc *useCases) *oas.Launcher {
	t.Helper()
	res, err := web.NewLauncher(uc).GetLauncher(signedIn())
	if err != nil {
		t.Fatal(err)
	}
	l, ok := res.(*oas.Launcher)
	if !ok {
		t.Fatalf("res = %#v", res)
	}
	return l
}

func TestTheLauncherListsTheSignedInUsersApps(t *testing.T) {
	uc := &useCases{apps: []domain.App{grafana, jellyfin}}
	l := launcher(t, uc)
	if len(l.Apps) != 2 || l.Apps[0].ID != "grafana" || l.Apps[1].ID != "jellyfin" || uc.asked[0] != ada.String() {
		t.Fatalf("apps = %+v, asked %v", l.Apps, uc.asked)
	}
}

func TestAnAppCarriesItsLabelDescriptionAndRoles(t *testing.T) {
	app := launcher(t, &useCases{apps: []domain.App{grafana}}).Apps[0]
	if app.Label != "Grafana" || app.Description.Value != "Metrics" || len(app.Roles) != 2 || app.Roles[0] != "editor" {
		t.Fatalf("app = %+v", app)
	}
}

func TestAnAppWithoutADescriptionSendsNone(t *testing.T) {
	if app := launcher(t, &useCases{apps: []domain.App{jellyfin}}).Apps[0]; app.Description.Set {
		t.Fatalf("description = %+v", app.Description)
	}
}

func TestAnAppOpensAtItsLaunchURL(t *testing.T) {
	apps := launcher(t, &useCases{apps: []domain.App{grafana, jellyfin}}).Apps
	if apps[0].LaunchUrl.String() != "https://grafana.test/" || apps[1].LaunchUrl.String() != "https://jellyfin.test/web" {
		t.Fatalf("launch = %s, %s", apps[0].LaunchUrl.String(), apps[1].LaunchUrl.String())
	}
}

func TestAnAppsIconIsOnIdentitysOriginAndChangesWithItsSource(t *testing.T) {
	first := launcher(t, &useCases{apps: []domain.App{grafana}}).Apps[0].IconUrl.Value
	moved := grafana
	moved.IconURL = "https://grafana.test/new.png"
	second := launcher(t, &useCases{apps: []domain.App{moved}}).Apps[0].IconUrl.Value
	if !strings.HasPrefix(first, "/icons/grafana?v=") || first == second {
		t.Fatalf("icon URLs = %q, %q", first, second)
	}
}

func TestAnAppWithoutAnIconSendsNoIconURL(t *testing.T) {
	if app := launcher(t, &useCases{apps: []domain.App{jellyfin}}).Apps[0]; app.IconUrl.Set {
		t.Fatalf("icon = %+v", app.IconUrl)
	}
}

func TestAnAppWhoseLaunchURLCannotBeReadIsLeftOut(t *testing.T) {
	broken := jellyfin
	broken.LaunchURL = "https://jellyfin.test/%zz"
	if apps := launcher(t, &useCases{apps: []domain.App{broken, grafana}}).Apps; len(apps) != 1 || apps[0].ID != "grafana" {
		t.Fatalf("apps = %+v", apps)
	}
}

func TestAUserWithNoAppsGetsAnEmptyList(t *testing.T) {
	if l := launcher(t, &useCases{}); l.Apps == nil || len(l.Apps) != 0 {
		t.Fatalf("apps = %#v, want an empty list", l.Apps)
	}
}

func TestTheLauncherPassesAFailureOn(t *testing.T) {
	if _, err := web.NewLauncher(&useCases{err: errBoom}).GetLauncher(signedIn()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func icon(t *testing.T, uc *useCases, path string) (*httptest.ResponseRecorder, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	h := web.NewIcons(uc, slog.New(slog.NewTextHandler(&logs, nil)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec, &logs
}

func TestAnIconIsServedWithItsTypeAndCachedForADay(t *testing.T) {
	uc := &useCases{icon: domain.Icon{ContentType: "image/png", Body: []byte("png")}}
	rec, _ := icon(t, uc, "/icons/grafana?v=abc")
	if rec.Code != http.StatusOK || rec.Body.String() != "png" || rec.Header().Get("Content-Type") != "image/png" ||
		rec.Header().Get("Cache-Control") != "public, max-age=86400" || uc.asked[0] != "grafana" {
		t.Fatalf("icon = %d %v %q, asked %v", rec.Code, rec.Header(), rec.Body, uc.asked)
	}
}

func TestAnIconCanRunNothing(t *testing.T) {
	rec, _ := icon(t, &useCases{icon: domain.Icon{ContentType: "image/svg+xml", Body: []byte("<svg/>")}}, "/icons/vault")
	if csp := rec.Header().Get("Content-Security-Policy"); csp != "default-src 'none'; style-src 'unsafe-inline'; sandbox" {
		t.Fatalf("CSP = %q", csp)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers = %v", rec.Header())
	}
}

func TestAClientWithNoIconAnswers404Quietly(t *testing.T) {
	rec, logs := icon(t, &useCases{err: app.ErrNoIcon}, "/icons/jellyfin")
	if rec.Code != http.StatusNotFound || logs.Len() != 0 {
		t.Fatalf("icon = %d, logged %s", rec.Code, logs)
	}
}

func TestAnIconThatFailedToFetchAnswers404AndLogsWhy(t *testing.T) {
	rec, logs := icon(t, &useCases{err: errors.Join(app.ErrNoIcon, errBoom)}, "/icons/grafana")
	if rec.Code != http.StatusNotFound || !strings.Contains(logs.String(), "boom") || strings.Contains(rec.Body.String(), "boom") {
		t.Fatalf("icon = %d %q, logged %s", rec.Code, rec.Body, logs)
	}
}

func TestOnlyGETServesAnIcon(t *testing.T) {
	h := web.NewIcons(&useCases{}, slog.New(slog.DiscardHandler))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/icons/grafana", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %d", rec.Code)
	}
}
