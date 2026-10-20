//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/connected/adapters/persistence"
	"github.com/JorisJonkers-dev/identity/internal/connected/app"
	"github.com/JorisJonkers-dev/identity/internal/connected/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
)

var t0 = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

type fixture struct {
	s   *persistence.Store
	q   *queries.Queries
	ctx context.Context
}

func open(t *testing.T) fixture {
	t.Helper()
	store, err := pg.Open(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return fixture{s: persistence.New(store.Queries(), store), q: store.Queries(), ctx: t.Context()}
}

func (f fixture) user(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := f.q.CreateUser(f.ctx, queries.CreateUserParams{ID: id, Email: id.String() + "@example.org", Name: "Ada", PasswordHash: "h", CreatedAt: t0}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f fixture) registration(t *testing.T, name string, at time.Time) string {
	t.Helper()
	id := "mcp_" + uuid.NewString()
	if err := f.s.CreateRegistration(f.ctx, domain.Registration{ClientID: id, Name: name, RedirectURIs: []string{"http://localhost:1/cb"}, CreatedAt: at}); err != nil {
		t.Fatal(err)
	}
	return id
}

// connected is a new User's Connected App on client, from a new registration called Claude Code.
func (f fixture) connected(t *testing.T, client string) domain.ConnectedApp {
	t.Helper()
	reg := f.registration(t, "Claude Code", t0)
	a, created, err := f.s.Consent(f.ctx, domain.ConnectedApp{ID: uuid.New(), UserID: f.user(t), ClientID: client, CreatedAt: t0}, reg)
	if err != nil || !created {
		t.Fatalf("Consent = %v, %v", created, err)
	}
	return a
}

func (f fixture) access(t *testing.T, appID uuid.UUID, expires time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := f.s.CreateAccess(f.ctx, id, appID, t0, expires); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestARegistrationIsKeptAndReadBack(t *testing.T) {
	t.Parallel()
	f := open(t)
	id := f.registration(t, "Claude Code", t0)
	r, err := f.s.Registration(f.ctx, id)
	if err != nil || r.Name != "Claude Code" || !slices.Equal(r.RedirectURIs, []string{"http://localhost:1/cb"}) || !r.CreatedAt.Equal(t0) {
		t.Fatalf("Registration = %+v, %v", r, err)
	}
}

func TestAMissingRegistrationConnectedAppOrTokenIsNotFound(t *testing.T) {
	t.Parallel()
	f := open(t)
	if _, err := f.s.Registration(f.ctx, "mcp_none"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Registration = %v", err)
	}
	if _, err := f.s.ConnectedApp(f.ctx, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("ConnectedApp = %v", err)
	}
	if _, err := f.s.Consented(f.ctx, "mcp_none", uuid.New(), "kb"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Consented = %v", err)
	}
	if _, err := f.s.Access(f.ctx, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Access = %v", err)
	}
	if _, err := f.s.Refresh(f.ctx, []byte("none")); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Refresh = %v", err)
	}
}

func TestPruningRemovesOnlyOldRegistrationsNobodyConsentedTo(t *testing.T) {
	t.Parallel()
	f := open(t)
	old := f.registration(t, "Forgotten", t0.Add(-48*time.Hour))
	fresh := f.registration(t, "Waiting", t0)
	used := f.connected(t, "kb-prune")

	if _, err := f.s.PruneRegistrations(f.ctx, t0.Add(-24*time.Hour), t0.Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	if _, err := f.s.Registration(f.ctx, old); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("the old registration is kept: %v", err)
	}
	if _, err := f.s.Registration(f.ctx, fresh); err != nil {
		t.Fatalf("the fresh registration is gone: %v", err)
	}
	if _, err := f.s.ConnectedApp(f.ctx, used.ID); err != nil {
		t.Fatalf("the consented registration is gone: %v", err)
	}
}

func TestPruningRemovesARegistrationUnusedSinceTheIdleTimeOnly(t *testing.T) {
	t.Parallel()
	f := open(t)
	idle := f.registration(t, "Idle", t0.Add(-90*24*time.Hour))
	recent := f.registration(t, "Recent", t0.Add(-90*24*time.Hour))
	for id, at := range map[string]time.Time{idle: t0.Add(-31 * 24 * time.Hour), recent: t0.Add(-29 * 24 * time.Hour)} {
		if err := f.s.UseRegistration(f.ctx, id, at); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := f.s.PruneRegistrations(f.ctx, t0.Add(-24*time.Hour), t0.Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	if _, err := f.s.Registration(f.ctx, idle); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("the idle registration is kept: %v", err)
	}
	if r, err := f.s.Registration(f.ctx, recent); err != nil || r.LastUsedAt == nil || !r.LastUsedAt.Equal(t0.Add(-29*24*time.Hour)) {
		t.Fatalf("the recent registration = %+v, %v", r, err)
	}
}

func TestAClientIsMarkedAndUnmarkedAsAnMCPResource(t *testing.T) {
	t.Parallel()
	f := open(t)
	client := "mark-" + uuid.NewString()
	for range 2 {
		if err := f.s.SetMCPResource(f.ctx, client, true, t0); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := f.s.MCPResources(f.ctx)
	if on, err := f.s.IsMCPResource(f.ctx, client); err != nil || !on || !slices.Contains(all, client) {
		t.Fatalf("marked = %v, %v; all %v", on, err, all)
	}
	_ = f.s.SetMCPResource(f.ctx, client, false, t0)
	if on, _ := f.s.IsMCPResource(f.ctx, client); on {
		t.Fatal("still marked")
	}
}

func TestConsentingAgainAnswersTheConnectedAppAlreadyThere(t *testing.T) {
	t.Parallel()
	f := open(t)
	reg := f.registration(t, "Claude Code", t0)
	user := f.user(t)
	first, _, _ := f.s.Consent(f.ctx, domain.ConnectedApp{ID: uuid.New(), UserID: user, ClientID: "kb", CreatedAt: t0}, reg)

	again, created, err := f.s.Consent(f.ctx, domain.ConnectedApp{ID: uuid.New(), UserID: user, ClientID: "kb", CreatedAt: t0.Add(time.Hour)}, reg)

	if err != nil || created || again.ID != first.ID || again.Name != "Claude Code" || !again.CreatedAt.Equal(t0) {
		t.Fatalf("again = %+v, %v, %v", again, created, err)
	}
	if id, err := f.s.Consented(f.ctx, reg, user, "kb"); err != nil || id != first.ID {
		t.Fatalf("Consented = %v, %v", id, err)
	}
}

func TestAUsersConnectedAppsCarryTheirRegisteredNameAndLastUse(t *testing.T) {
	t.Parallel()
	f := open(t)
	a := f.connected(t, "kb")
	used := t0.Add(time.Minute)
	if err := f.s.Touch(f.ctx, a.ID, used); err != nil {
		t.Fatal(err)
	}
	apps, err := f.s.ConnectedApps(f.ctx, a.UserID)
	if err != nil || len(apps) != 1 || apps[0].Name != "Claude Code" || apps[0].ClientID != "kb" || apps[0].LastUsedAt == nil || !apps[0].LastUsedAt.Equal(used) {
		t.Fatalf("ConnectedApps = %+v, %v", apps, err)
	}
}

func TestAnAccessTokenIsFoundWithItsConnectedApp(t *testing.T) {
	t.Parallel()
	f := open(t)
	a := f.connected(t, "kb")
	id := f.access(t, a.ID, t0.Add(time.Hour))
	got, err := f.s.Access(f.ctx, id)
	want := app.AccessToken{ID: id, AppID: a.ID, UserID: a.UserID, ClientID: "kb", ExpiresAt: t0.Add(time.Hour)}
	if err != nil || got.ID != want.ID || got.AppID != want.AppID || got.UserID != want.UserID || got.ClientID != want.ClientID || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Fatalf("Access = %+v, %v", got, err)
	}
}

func TestARefreshTokenIsFoundByItsHashAndSpentOnce(t *testing.T) {
	t.Parallel()
	f := open(t)
	a := f.connected(t, "kb")
	hash := []byte(uuid.NewString())
	if err := f.s.CreateRefresh(f.ctx, uuid.New(), a.ID, hash, t0, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	r, err := f.s.Refresh(f.ctx, hash)
	if err != nil || r.AppID != a.ID || r.UserID != a.UserID || r.ClientID != "kb" || !strings.HasPrefix(r.RegistrationID, "mcp_") {
		t.Fatalf("Refresh = %+v, %v", r, err)
	}
	first, _ := f.s.Spend(f.ctx, r.ID, t0)
	second, err := f.s.Spend(f.ctx, r.ID, t0)
	if !first || second || err != nil {
		t.Fatalf("spent %v then %v, %v", first, second, err)
	}
}

func TestAnAccessTokenIsNeverFoundAsARefreshToken(t *testing.T) {
	t.Parallel()
	f := open(t)
	a := f.connected(t, "kb")
	hash := []byte(uuid.NewString())
	if err := f.s.CreateRefresh(f.ctx, uuid.New(), a.ID, hash, t0, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	r, _ := f.s.Refresh(f.ctx, hash)
	if _, err := f.s.Access(f.ctx, r.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Access = %v", err)
	}
}

func TestDeletingAConnectedAppAnswersItsAccessTokensAndRemovesThem(t *testing.T) {
	t.Parallel()
	f := open(t)
	a := f.connected(t, "kb")
	access := f.access(t, a.ID, t0.Add(time.Hour))
	_ = f.s.CreateRefresh(f.ctx, uuid.New(), a.ID, []byte(uuid.NewString()), t0, t0.Add(time.Hour))

	tokens, err := f.s.DeleteConnectedApp(f.ctx, a.ID)

	if err != nil || !slices.Equal(tokens, []uuid.UUID{access}) {
		t.Fatalf("DeleteConnectedApp = %v, %v", tokens, err)
	}
	if _, err := f.s.Access(f.ctx, access); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("the access token outlived its Connected App: %v", err)
	}
}

func TestDeletingAClientsConnectedAppsAnswersThemAndTheirAccessTokens(t *testing.T) {
	t.Parallel()
	f := open(t)
	client := "step-" + uuid.NewString()
	a, b := f.connected(t, client), f.connected(t, client)
	other := f.connected(t, "kb")
	ta, tb := f.access(t, a.ID, t0.Add(time.Hour)), f.access(t, b.ID, t0.Add(time.Hour))
	if n, _ := f.s.CountForClient(f.ctx, client); n != 2 {
		t.Fatalf("count = %d", n)
	}

	apps, tokens, err := f.s.DeleteForClient(f.ctx, client)

	ids := []uuid.UUID{}
	for _, x := range apps {
		ids = append(ids, x.ID)
	}
	if err != nil || len(apps) != 2 || !slices.Contains(ids, a.ID) || !slices.Contains(ids, b.ID) || apps[0].Name != "Claude Code" {
		t.Fatalf("apps = %+v, %v", apps, err)
	}
	if len(tokens) != 2 || !slices.Contains(tokens, ta) || !slices.Contains(tokens, tb) {
		t.Fatalf("tokens = %v", tokens)
	}
	if n, _ := f.s.CountForClient(f.ctx, client); n != 0 {
		t.Fatalf("count after = %d", n)
	}
	if _, err := f.s.ConnectedApp(f.ctx, other.ID); err != nil {
		t.Fatalf("another Client's Connected App went too: %v", err)
	}
}

func TestPruningRemovesExpiredTokensOnly(t *testing.T) {
	t.Parallel()
	f := open(t)
	a := f.connected(t, "kb")
	expired, live := f.access(t, a.ID, t0.Add(-time.Minute)), f.access(t, a.ID, t0.Add(time.Hour))

	if _, err := f.s.PruneTokens(f.ctx, t0); err != nil {
		t.Fatal(err)
	}

	if _, err := f.s.Access(f.ctx, expired); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("the expired token is kept: %v", err)
	}
	if _, err := f.s.Access(f.ctx, live); err != nil {
		t.Fatalf("the live token is gone: %v", err)
	}
}

func TestDeletingAUserRemovesTheirConnectedApps(t *testing.T) {
	t.Parallel()
	f := open(t)
	a := f.connected(t, "kb")
	if _, err := f.q.DeleteUser(f.ctx, a.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ConnectedApp(f.ctx, a.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("ConnectedApp = %v", err)
	}
}
