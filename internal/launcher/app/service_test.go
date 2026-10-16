package app_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/launcher/app"
	"github.com/JorisJonkers-dev/identity/internal/launcher/domain"
)

var (
	errBoom = errors.New("boom")
	ada     = uuid.MustParse("00000000-0000-4000-8000-0000000000a2")
	t0      = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

	grafana  = domain.Client{ID: "grafana", Label: "Grafana", Host: "grafana.test", IconURL: "https://grafana.test/icon.png"}
	jellyfin = domain.Client{ID: "jellyfin", Label: "Jellyfin", Host: "jellyfin.test"}
	vault    = domain.Client{ID: "vault", Label: "Vault", Host: "vault.test", IconURL: "https://vault.test/icon.svg"}
	png      = domain.Icon{ContentType: "image/png", Body: []byte("png")}
)

// roles gives Ada the listed App Roles per Client.
type roles struct {
	held map[string][]string
	err  error
}

func (r roles) RolesByClient(_ context.Context, userID uuid.UUID) (map[string][]string, error) {
	if userID != ada {
		return nil, r.err
	}
	return r.held, r.err
}

// clients is the Clients the cluster declares; a test may change them under a running Service.
type clients struct{ list []domain.Client }

func (c *clients) Launchable() []domain.Client { return c.list }

func (c *clients) Client(id string) (domain.Client, bool) {
	for _, x := range c.list {
		if x.ID == id {
			return x, true
		}
	}
	return domain.Client{}, false
}

// fetcher answers each URL with its icon or error, and counts the fetches.
type fetcher struct {
	icons   map[string]domain.Icon
	err     error
	fetched []string
}

func (f *fetcher) Fetch(_ context.Context, url string) (domain.Icon, error) {
	f.fetched = append(f.fetched, url)
	return f.icons[url], f.err
}

type fixture struct {
	*app.Service
	fetcher *fetcher
	clients *clients
	now     time.Time
}

func newFixture(held map[string][]string) *fixture {
	f := &fixture{fetcher: &fetcher{icons: map[string]domain.Icon{grafana.IconURL: png}}, clients: &clients{list: []domain.Client{vault, grafana, jellyfin}}, now: t0}
	f.Service = app.New(roles{held: held}, f.clients, f.fetcher, func() time.Time { return f.now })
	return f
}

func ids(apps []domain.App) []string {
	out := make([]string, 0, len(apps))
	for _, a := range apps {
		out = append(out, a.ID)
	}
	return out
}

func TestTheLauncherShowsOnlyTheAppsAUserHoldsARoleOn(t *testing.T) {
	f := newFixture(map[string][]string{"grafana": {"viewer"}, "vault": {"user"}})
	apps, err := f.Apps(context.Background(), ada)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(apps); !slices.Equal(got, []string{"grafana", "vault"}) {
		t.Fatalf("apps = %v, want grafana and vault, by label", got)
	}
}

func TestEachAppCarriesTheUsersRolesOnIt(t *testing.T) {
	f := newFixture(map[string][]string{"grafana": {"editor", "viewer"}})
	apps, _ := f.Apps(context.Background(), ada)
	if len(apps) != 1 || !slices.Equal(apps[0].Roles, []string{"editor", "viewer"}) || apps[0].Label != "Grafana" {
		t.Fatalf("apps = %+v", apps)
	}
}

func TestARoleOnAClientTheClusterNoLongerDeclaresOpensNothing(t *testing.T) {
	f := newFixture(map[string][]string{"old-wiki": {"user"}})
	apps, err := f.Apps(context.Background(), ada)
	if err != nil || len(apps) != 0 {
		t.Fatalf("apps = %v, %v", apps, err)
	}
}

func TestAUserWithNoRolesSeesNoApps(t *testing.T) {
	f := newFixture(nil)
	apps, err := f.Apps(context.Background(), ada)
	if err != nil || len(apps) != 0 {
		t.Fatalf("apps = %v, %v", apps, err)
	}
}

func TestTheLauncherFailsWhenRolesCannotBeRead(t *testing.T) {
	s := app.New(roles{err: errBoom}, &clients{list: []domain.Client{grafana}}, &fetcher{}, time.Now)
	if _, err := s.Apps(context.Background(), ada); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnIconIsFetchedFromItsClientOnce(t *testing.T) {
	f := newFixture(nil)
	for range 2 {
		icon, err := f.Icon(context.Background(), "grafana")
		if err != nil || icon.ContentType != "image/png" || string(icon.Body) != "png" {
			t.Fatalf("icon = %+v, %v", icon, err)
		}
	}
	if !slices.Equal(f.fetcher.fetched, []string{grafana.IconURL}) {
		t.Fatalf("fetched %v, want one fetch", f.fetcher.fetched)
	}
}

func TestAnIconIsFetchedAgainAfterADay(t *testing.T) {
	f := newFixture(nil)
	_, _ = f.Icon(context.Background(), "grafana")
	f.now = t0.Add(24 * time.Hour)
	_, _ = f.Icon(context.Background(), "grafana")
	if len(f.fetcher.fetched) != 2 {
		t.Fatalf("fetched %v, want a second fetch after a day", f.fetcher.fetched)
	}
}

func TestAClientWithoutAnIconURLHasNoIcon(t *testing.T) {
	f := newFixture(nil)
	if _, err := f.Icon(context.Background(), "jellyfin"); !errors.Is(err, app.ErrNoIcon) {
		t.Fatalf("err = %v", err)
	}
	if len(f.fetcher.fetched) != 0 {
		t.Fatalf("fetched %v", f.fetcher.fetched)
	}
}

func TestAnUnknownClientHasNoIcon(t *testing.T) {
	f := newFixture(nil)
	if _, err := f.Icon(context.Background(), "nobody"); !errors.Is(err, app.ErrNoIcon) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnIconThatFailsToFetchIsNotAskedForAgainForTenMinutes(t *testing.T) {
	f := newFixture(nil)
	f.fetcher.err = errBoom
	_, err := f.Icon(context.Background(), "grafana")
	if !errors.Is(err, app.ErrNoIcon) || !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want ErrNoIcon with its cause", err)
	}
	f.now = t0.Add(10*time.Minute - time.Second)
	_, _ = f.Icon(context.Background(), "grafana")
	if len(f.fetcher.fetched) != 1 {
		t.Fatalf("fetched %v within ten minutes", f.fetcher.fetched)
	}
	f.now = t0.Add(10 * time.Minute)
	f.fetcher.err = nil
	if _, err := f.Icon(context.Background(), "grafana"); err != nil || len(f.fetcher.fetched) != 2 {
		t.Fatalf("after ten minutes: %v, fetched %v", err, f.fetcher.fetched)
	}
}

func TestAnIconWhoseURLChangedIsFetchedFromTheNewURL(t *testing.T) {
	f := newFixture(nil)
	_, _ = f.Icon(context.Background(), "grafana")
	moved := grafana
	moved.IconURL = "https://grafana.test/new.png"
	f.clients.list = []domain.Client{moved}
	_, _ = f.Icon(context.Background(), "grafana")
	if !slices.Equal(f.fetcher.fetched, []string{grafana.IconURL, moved.IconURL}) {
		t.Fatalf("fetched %v", f.fetcher.fetched)
	}
}
