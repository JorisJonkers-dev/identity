package app_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/connected/app"
	"github.com/JorisJonkers-dev/identity/internal/connected/domain"
)

var (
	t0      = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	ada     = uuid.MustParse("00000000-0000-4000-8000-0000000000a2")
	root    = uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
	errBoom = errors.New("boom")
	kb      = app.Client{ID: "kb", Label: "Knowledge", Roles: []string{"user", "editor"}}
	stats   = app.Client{ID: "stats", Label: "Stats", Roles: []string{"user"}, StepUp: true}
	wiki    = app.Client{ID: "wiki", Label: "Wiki", Roles: []string{"user"}}
)

type token struct {
	app.AccessToken
	refresh bool
	hash    []byte
	spent   bool
}

// store keeps the connected context in memory, as Postgres does.
type store struct {
	regs   map[string]domain.Registration
	mcp    map[string]bool
	apps   map[uuid.UUID]domain.ConnectedApp
	regOf  map[uuid.UUID]string
	tokens map[uuid.UUID]*token
	fail   map[string]error
}

func newStore() *store {
	return &store{
		regs: map[string]domain.Registration{}, mcp: map[string]bool{"kb": true, "stats": true},
		apps: map[uuid.UUID]domain.ConnectedApp{}, regOf: map[uuid.UUID]string{}, tokens: map[uuid.UUID]*token{}, fail: map[string]error{},
	}
}

func (s *store) CreateRegistration(_ context.Context, r domain.Registration) error {
	s.regs[r.ClientID] = r
	return s.fail["CreateRegistration"]
}

func (s *store) Registration(_ context.Context, id string) (domain.Registration, error) {
	if err := s.fail["Registration"]; err != nil {
		return domain.Registration{}, err
	}
	r, ok := s.regs[id]
	if !ok {
		return domain.Registration{}, app.ErrNotFound
	}
	return r, nil
}

func (s *store) PruneRegistrations(_ context.Context, before time.Time) (int64, error) {
	var n int64
	for id, r := range s.regs {
		used := slices.Contains(slices.Collect(func(yield func(string) bool) {
			for _, v := range s.regOf {
				if !yield(v) {
					return
				}
			}
		}), id)
		if r.CreatedAt.Before(before) && !used {
			delete(s.regs, id)
			n++
		}
	}
	return n, s.fail["PruneRegistrations"]
}

func (s *store) MCPResources(context.Context) ([]string, error) {
	var out []string
	for id, on := range s.mcp {
		if on {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out, s.fail["MCPResources"]
}

func (s *store) IsMCPResource(_ context.Context, id string) (bool, error) {
	return s.mcp[id], s.fail["IsMCPResource"]
}

func (s *store) SetMCPResource(_ context.Context, id string, on bool, _ time.Time) error {
	if err := s.fail["SetMCPResource"]; err != nil {
		return err
	}
	s.mcp[id] = on
	return nil
}

func (s *store) Consent(_ context.Context, a domain.ConnectedApp, reg string) (domain.ConnectedApp, bool, error) {
	if err := s.fail["Consent"]; err != nil {
		return domain.ConnectedApp{}, false, err
	}
	for id, b := range s.apps {
		if s.regOf[id] == reg && b.UserID == a.UserID && b.ClientID == a.ClientID {
			return b, false, nil
		}
	}
	s.apps[a.ID], s.regOf[a.ID] = a, reg
	return a, true, nil
}

func (s *store) Consented(_ context.Context, reg string, userID uuid.UUID, clientID string) (uuid.UUID, error) {
	for id, b := range s.apps {
		if s.regOf[id] == reg && b.UserID == userID && b.ClientID == clientID {
			return id, nil
		}
	}
	return uuid.Nil, app.ErrNotFound
}

func (s *store) ConnectedApps(_ context.Context, userID uuid.UUID) ([]domain.ConnectedApp, error) {
	var out []domain.ConnectedApp
	for _, a := range s.apps {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return out, s.fail["ConnectedApps"]
}

func (s *store) ConnectedApp(_ context.Context, id uuid.UUID) (domain.ConnectedApp, error) {
	if err := s.fail["ConnectedApp"]; err != nil {
		return domain.ConnectedApp{}, err
	}
	a, ok := s.apps[id]
	if !ok {
		return domain.ConnectedApp{}, app.ErrNotFound
	}
	return a, nil
}

func (s *store) accessOf(appID uuid.UUID) []uuid.UUID {
	var ids []uuid.UUID
	for id, t := range s.tokens {
		if t.AppID == appID && !t.refresh {
			ids = append(ids, id)
		}
	}
	return ids
}

func (s *store) DeleteConnectedApp(_ context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	if err := s.fail["DeleteConnectedApp"]; err != nil {
		return nil, err
	}
	ids := s.accessOf(id)
	for tid, t := range s.tokens {
		if t.AppID == id {
			delete(s.tokens, tid)
		}
	}
	delete(s.apps, id)
	return ids, nil
}

func (s *store) DeleteForClient(ctx context.Context, clientID string) ([]domain.ConnectedApp, []uuid.UUID, error) {
	if err := s.fail["DeleteForClient"]; err != nil {
		return nil, nil, err
	}
	var gone []domain.ConnectedApp
	var tokens []uuid.UUID
	for id, a := range s.apps {
		if a.ClientID == clientID {
			ids, _ := s.DeleteConnectedApp(ctx, id)
			gone, tokens = append(gone, a), append(tokens, ids...)
		}
	}
	return gone, tokens, nil
}

func (s *store) CountForClient(_ context.Context, clientID string) (int, error) {
	n := 0
	for _, a := range s.apps {
		if a.ClientID == clientID {
			n++
		}
	}
	return n, s.fail["CountForClient"]
}

func (s *store) Touch(_ context.Context, appID uuid.UUID, at time.Time) error {
	a := s.apps[appID]
	a.LastUsedAt = &at
	s.apps[appID] = a
	return s.fail["Touch"]
}

func (s *store) create(id, appID uuid.UUID, refresh bool, hash []byte, expires time.Time) {
	a := s.apps[appID]
	s.tokens[id] = &token{AccessToken: app.AccessToken{ID: id, AppID: appID, UserID: a.UserID, ClientID: a.ClientID, ExpiresAt: expires}, refresh: refresh, hash: hash}
}

func (s *store) CreateAccess(_ context.Context, id, appID uuid.UUID, _, expires time.Time) error {
	s.create(id, appID, false, nil, expires)
	return s.fail["CreateAccess"]
}

func (s *store) CreateRefresh(_ context.Context, id, appID uuid.UUID, hash []byte, _, expires time.Time) error {
	s.create(id, appID, true, hash, expires)
	return s.fail["CreateRefresh"]
}

func (s *store) Access(_ context.Context, id uuid.UUID) (app.AccessToken, error) {
	if err := s.fail["Access"]; err != nil {
		return app.AccessToken{}, err
	}
	t, ok := s.tokens[id]
	if !ok || t.refresh {
		return app.AccessToken{}, app.ErrNotFound
	}
	return t.AccessToken, nil
}

func (s *store) Refresh(_ context.Context, hash []byte) (app.RefreshToken, error) {
	if err := s.fail["Refresh"]; err != nil {
		return app.RefreshToken{}, err
	}
	for _, t := range s.tokens {
		if t.refresh && bytes.Equal(t.hash, hash) {
			return app.RefreshToken{ID: t.ID, AppID: t.AppID, UserID: t.UserID, ClientID: t.ClientID, ExpiresAt: t.ExpiresAt}, nil
		}
	}
	return app.RefreshToken{}, app.ErrNotFound
}

func (s *store) Spend(_ context.Context, id uuid.UUID, _ time.Time) (bool, error) {
	if err := s.fail["Spend"]; err != nil {
		return false, err
	}
	t := s.tokens[id]
	if t.spent {
		return false, nil
	}
	t.spent = true
	return true, nil
}

func (s *store) PruneTokens(_ context.Context, before time.Time) (int64, error) {
	var n int64
	for id, t := range s.tokens {
		if t.ExpiresAt.Before(before) {
			delete(s.tokens, id)
			n++
		}
	}
	return n, s.fail["PruneTokens"]
}

type clients map[string]app.Client

func (c clients) Client(_ context.Context, id string) (app.Client, error) {
	cl, ok := c[id]
	if !ok {
		return app.Client{}, app.ErrNotFound
	}
	return cl, nil
}

func (c clients) ClientForHost(_ context.Context, host string) (app.Client, error) {
	if host == "boom.example.org" {
		return app.Client{}, errBoom
	}
	cl, ok := c[strings.TrimSuffix(host, ".example.org")]
	if !ok {
		return app.Client{}, app.ErrNotFound
	}
	return cl, nil
}

// roles gives Ada user and editor on kb, and user on stats and wiki.
type roles struct{ fail error }

func (r roles) Roles(_ context.Context, userID uuid.UUID, clientID string) ([]string, error) {
	if userID != ada {
		return nil, r.fail
	}
	if clientID == "kb" {
		return []string{"editor", "user", "owner"}, r.fail
	}
	return []string{"user"}, r.fail
}

type admins map[uuid.UUID]bool

func (a admins) IsAdmin(_ context.Context, id uuid.UUID) (bool, error) {
	if id == uuid.Nil {
		return false, errBoom
	}
	return a[id], nil
}

type recorded []audit.Event

func (r *recorded) Record(_ context.Context, e audit.Event) { *r = append(*r, e) }

// counter is randomness that counts up, so ids and secrets differ.
type counter struct{ n byte }

func (c *counter) Read(p []byte) (int, error) {
	for i := range p {
		c.n++
		p[i] = c.n
	}
	return len(p), nil
}

type dry struct{}

func (dry) Read([]byte) (int, error) { return 0, errBoom }

type fixture struct {
	*app.Service
	store     *store
	events    *recorded
	now       time.Time
	forgotten []uuid.UUID
}

func newFixture() *fixture {
	f := &fixture{store: newStore(), events: &recorded{}, now: t0}
	f.Service = app.New(f.store, clients{"kb": kb, "stats": stats, "wiki": wiki}, roles{}, admins{root: true}, f.events, func() time.Time { return f.now }, &counter{})
	f.OnForget(func(ids []uuid.UUID) { f.forgotten = append(f.forgotten, ids...) })
	return f
}

var ctx = context.Background()

// connected is a Connected App Ada consented to on kb, from the registration of Claude Code.
func (f *fixture) connected(t *testing.T) (domain.Registration, uuid.UUID) {
	t.Helper()
	r, err := f.Register(ctx, "Claude Code", []string{"http://localhost:33418/callback"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := f.Consent(ctx, ada, r.ClientID, "kb")
	if err != nil {
		t.Fatal(err)
	}
	*f.events = nil
	return r, id
}

func TestARegisteredToolGetsAnIDOfItsOwnKind(t *testing.T) {
	r, err := newFixture().Register(ctx, "Claude Code", []string{"http://localhost:33418/callback"})
	if err != nil || !strings.HasPrefix(r.ClientID, app.RegistrationPrefix) || r.Name != "Claude Code" {
		t.Fatalf("Register = %+v, %v", r, err)
	}
}

func TestARegistrationThatBreaksTheRulesIsRefused(t *testing.T) {
	if _, err := newFixture().Register(ctx, "App", []string{"http://evil.example.org/cb"}); !errors.Is(err, domain.ErrInvalidRegistration) {
		t.Fatalf("err = %v", err)
	}
}

func TestNoRegistrationIsMadeWithoutRandomness(t *testing.T) {
	f := newFixture()
	f.Service = app.New(f.store, clients{}, roles{}, admins{}, f.events, time.Now, dry{})
	if _, err := f.Register(ctx, "App", []string{"https://app.example.org/cb"}); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestARegistrationIsReadBack(t *testing.T) {
	f := newFixture()
	r, _ := f.Register(ctx, "App", []string{"https://app.example.org/cb"})
	got, err := f.Registration(ctx, r.ClientID)
	if err != nil || got.Name != "App" {
		t.Fatalf("Registration = %+v, %v", got, err)
	}
}

func TestAResourceURLNamesTheMCPClientItsHostServes(t *testing.T) {
	c, err := newFixture().Resource(ctx, "https://KB.example.org:443/mcp")
	if err != nil || c.ID != "kb" {
		t.Fatalf("Resource = %+v, %v", c, err)
	}
}

func TestAResourceIsRefused(t *testing.T) {
	for _, c := range []struct{ name, resource string }{
		{"over plain http", "http://kb.example.org/mcp"},
		{"for a host no Client serves", "https://nowhere.example.org/mcp"},
		{"for a Client not marked as an MCP resource", "https://wiki.example.org/mcp"},
		{"that is no URL", "kb"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := newFixture().Resource(ctx, c.resource); !errors.Is(err, app.ErrNotMCPResource) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAResourceFailsWhenItsClientOrMarkCannotBeRead(t *testing.T) {
	if _, err := newFixture().Resource(ctx, "https://boom.example.org/mcp"); !errors.Is(err, errBoom) {
		t.Fatalf("host lookup: err = %v", err)
	}
	f := newFixture()
	f.store.fail["IsMCPResource"] = errBoom
	if _, err := f.Resource(ctx, "https://kb.example.org/mcp"); !errors.Is(err, errBoom) {
		t.Fatalf("mark: err = %v", err)
	}
}

func TestConsentingMakesAConnectedAppAsAnAuditEvent(t *testing.T) {
	f := newFixture()
	r, _ := f.Register(ctx, "Claude Code", []string{"http://localhost:1/cb"})

	id, err := f.Consent(ctx, ada, r.ClientID, "kb")

	if err != nil || id == uuid.Nil {
		t.Fatalf("Consent = %v, %v", id, err)
	}
	want := audit.Event{Action: audit.ConnectedAppAuthorized, User: ada, Client: "kb", AppName: "Claude Code"}
	if len(*f.events) != 1 || (*f.events)[0] != want {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestConsentingAgainAnswersTheSameConnectedAppAndRecordsNothing(t *testing.T) {
	f := newFixture()
	r, first := f.connected(t)
	again, err := f.Consent(ctx, ada, r.ClientID, "kb")
	if err != nil || again != first || len(*f.events) != 0 {
		t.Fatalf("again = %v (first %v), %v, events %+v", again, first, err, *f.events)
	}
}

func TestConsentIsRefused(t *testing.T) {
	for _, c := range []struct {
		name   string
		user   uuid.UUID
		client string
		want   error
	}{
		{"for a Step-up Client", ada, "stats", app.ErrStepUpClient},
		{"for a Client not marked as an MCP resource", ada, "wiki", app.ErrNotMCPResource},
		{"for a User with no App Role there", root, "kb", app.ErrNoAccess},
		{"for a Client not in the cluster", ada, "gone", app.ErrNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture()
			r, _ := f.Register(ctx, "App", []string{"https://app.example.org/cb"})
			if _, err := f.Consent(ctx, c.user, r.ClientID, c.client); !errors.Is(err, c.want) || len(f.store.apps) != 0 {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestConsentFails(t *testing.T) {
	for _, c := range []struct {
		name  string
		spoil func(f *fixture)
	}{
		{"for a registration that is gone", func(f *fixture) { f.store.regs = map[string]domain.Registration{} }},
		{"when the registration cannot be read", func(f *fixture) { f.store.fail["Registration"] = errBoom }},
		{"when the mark cannot be read", func(f *fixture) { f.store.fail["IsMCPResource"] = errBoom }},
		{"when the roles cannot be read", func(f *fixture) {
			f.Service = app.New(f.store, clients{"kb": kb}, roles{fail: errBoom}, admins{}, f.events, time.Now, &counter{})
		}},
		{"when the consent cannot be kept", func(f *fixture) { f.store.fail["Consent"] = errBoom }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture()
			r, _ := f.Register(ctx, "App", []string{"https://app.example.org/cb"})
			c.spoil(f)
			if _, err := f.Consent(ctx, ada, r.ClientID, "kb"); err == nil {
				t.Fatal("consented")
			}
		})
	}
}

func TestNoConsentIsMadeWithoutRandomness(t *testing.T) {
	f := newFixture()
	r, _ := f.Register(ctx, "App", []string{"https://app.example.org/cb"})
	f.Service = app.New(f.store, clients{"kb": kb}, roles{}, admins{}, f.events, time.Now, dry{})
	if _, err := f.Consent(ctx, ada, r.ClientID, "kb"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAConsentGivenBeforeIsFoundAndAMissingOneIsNot(t *testing.T) {
	f := newFixture()
	r, id := f.connected(t)
	if got, err := f.Consented(ctx, ada, r.ClientID, "kb"); err != nil || got != id {
		t.Fatalf("Consented = %v, %v", got, err)
	}
	other, _ := f.Register(ctx, "Other", []string{"https://app.example.org/cb"})
	if _, err := f.Consented(ctx, ada, other.ClientID, "kb"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("missing: err = %v", err)
	}
}

func TestAConsentGivenBeforeNoLongerHoldsOnceTheClientAsksForStepUp(t *testing.T) {
	f := newFixture()
	r, _ := f.connected(t)
	f.Service = app.New(f.store, clients{"kb": {ID: "kb", Roles: []string{"user"}, StepUp: true}}, roles{}, admins{}, f.events, time.Now, &counter{})
	if _, err := f.Consented(ctx, ada, r.ClientID, "kb"); !errors.Is(err, app.ErrStepUpClient) {
		t.Fatalf("err = %v", err)
	}
}

func TestTheConsentScreenNamesTheClientAndTheRolesInItsOrder(t *testing.T) {
	c, roles, err := newFixture().RolesFor(ctx, ada, "kb")
	if err != nil || c.Label != "Knowledge" || !slices.Equal(roles, []string{"user", "editor"}) {
		t.Fatalf("RolesFor = %+v, %v, %v", c, roles, err)
	}
}

func TestTheConsentScreenFailsForAClientNotInTheCluster(t *testing.T) {
	if _, _, err := newFixture().RolesFor(ctx, ada, "gone"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnAccessTokenWorksForItsClientWithTheUsersRolesThere(t *testing.T) {
	f := newFixture()
	_, appID := f.connected(t)
	id, expires, err := f.IssueAccess(ctx, appID)
	if err != nil || !expires.Equal(t0.Add(time.Hour)) {
		t.Fatalf("IssueAccess = %v, %v", expires, err)
	}

	tok, roles, err := f.Check(ctx, id, "kb")

	if err != nil || tok.UserID != ada || !slices.Equal(roles, []string{"user", "editor"}) {
		t.Fatalf("Check = %+v, %v, %v", tok, roles, err)
	}
	if f.store.apps[appID].LastUsedAt == nil {
		t.Fatal("the Connected App was not marked used")
	}
}

func TestAnAccessTokenIsRefused(t *testing.T) {
	for _, c := range []struct {
		name   string
		client string
		later  time.Duration
		id     func(id uuid.UUID) uuid.UUID
	}{
		{"for another Client", "wiki", 0, func(id uuid.UUID) uuid.UUID { return id }},
		{"once it has expired", "kb", time.Hour, func(id uuid.UUID) uuid.UUID { return id }},
		{"that was never issued", "kb", 0, func(uuid.UUID) uuid.UUID { return uuid.New() }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture()
			_, appID := f.connected(t)
			id, _, _ := f.IssueAccess(ctx, appID)
			f.now = t0.Add(c.later)
			if _, _, err := f.Check(ctx, c.id(id), c.client); !errors.Is(err, app.ErrInvalidToken) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAnAccessTokenForAClientThatLeftTheClusterIsRefused(t *testing.T) {
	f := newFixture()
	_, appID := f.connected(t)
	id, _, _ := f.IssueAccess(ctx, appID)
	f.Service = app.New(f.store, clients{}, roles{}, admins{}, f.events, func() time.Time { return t0 }, &counter{})
	if _, _, err := f.Check(ctx, id, "kb"); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestACheckFails(t *testing.T) {
	for _, c := range []struct {
		name  string
		spoil func(f *fixture)
	}{
		{"when the token cannot be read", func(f *fixture) { f.store.fail["Access"] = errBoom }},
		{"when the roles cannot be read", func(f *fixture) {
			f.Service = app.New(f.store, clients{"kb": kb}, roles{fail: errBoom}, admins{}, f.events, func() time.Time { return t0 }, &counter{})
		}},
		{"when its use cannot be recorded", func(f *fixture) { f.store.fail["Touch"] = errBoom }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture()
			_, appID := f.connected(t)
			id, _, _ := f.IssueAccess(ctx, appID)
			c.spoil(f)
			if _, _, err := f.Check(ctx, id, "kb"); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// failingClients answers its one failure for every lookup.
type failingClients struct{}

func (failingClients) Client(context.Context, string) (app.Client, error)        { return app.Client{}, errBoom }
func (failingClients) ClientForHost(context.Context, string) (app.Client, error) { return app.Client{}, errBoom }

func TestACheckFailsWhenTheClientCannotBeRead(t *testing.T) {
	f := newFixture()
	_, appID := f.connected(t)
	id, _, _ := f.IssueAccess(ctx, appID)
	f.Service = app.New(f.store, failingClients{}, roles{}, admins{}, f.events, func() time.Time { return t0 }, &counter{})
	if _, _, err := f.Check(ctx, id, "kb"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestARefreshTokenIsSpentOnceForItsConnectedApp(t *testing.T) {
	f := newFixture()
	_, appID := f.connected(t)
	secret, err := f.IssueRefresh(ctx, appID)
	if err != nil {
		t.Fatal(err)
	}

	got, err := f.Refresh(ctx, secret)

	if err != nil || got.AppID != appID || got.UserID != ada || got.ClientID != "kb" {
		t.Fatalf("Refresh = %+v, %v", got, err)
	}
}

func TestARefreshTokenSpentTwiceRevokesItsConnectedApp(t *testing.T) {
	f := newFixture()
	_, appID := f.connected(t)
	access, _, _ := f.IssueAccess(ctx, appID)
	secret, _ := f.IssueRefresh(ctx, appID)
	_, _ = f.Refresh(ctx, secret)

	_, err := f.Refresh(ctx, secret)

	if !errors.Is(err, app.ErrInvalidToken) || len(f.store.apps) != 0 || !slices.Contains(f.forgotten, access) {
		t.Fatalf("err = %v, apps %d, forgotten %v", err, len(f.store.apps), f.forgotten)
	}
}

func TestARefreshTokenIsRefused(t *testing.T) {
	for _, c := range []struct {
		name   string
		later  time.Duration
		secret func(string) string
	}{
		{"once it has expired", 30 * 24 * time.Hour, func(s string) string { return s }},
		{"that was never issued", 0, func(string) string { return "forged" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture()
			_, appID := f.connected(t)
			secret, _ := f.IssueRefresh(ctx, appID)
			f.now = t0.Add(c.later)
			if _, err := f.Refresh(ctx, c.secret(secret)); !errors.Is(err, app.ErrInvalidToken) || len(f.store.apps) != 1 {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestARefreshFails(t *testing.T) {
	for _, c := range []struct {
		name   string
		spoil  func(f *fixture)
		second bool
	}{
		{"when the token cannot be read", func(f *fixture) { f.store.fail["Refresh"] = errBoom }, false},
		{"when it cannot be spent", func(f *fixture) { f.store.fail["Spend"] = errBoom }, false},
		{"when a reused one's Connected App cannot be revoked", func(f *fixture) { f.store.fail["DeleteConnectedApp"] = errBoom }, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture()
			_, appID := f.connected(t)
			secret, _ := f.IssueRefresh(ctx, appID)
			if c.second {
				_, _ = f.Refresh(ctx, secret)
			}
			c.spoil(f)
			if _, err := f.Refresh(ctx, secret); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestNoTokenIsMadeWithoutRandomness(t *testing.T) {
	for _, c := range []struct {
		name  string
		issue func(f *fixture, appID uuid.UUID) error
	}{
		{"access", func(f *fixture, appID uuid.UUID) error { _, _, err := f.IssueAccess(ctx, appID); return err }},
		{"refresh", func(f *fixture, appID uuid.UUID) error { _, err := f.IssueRefresh(ctx, appID); return err }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture()
			_, appID := f.connected(t)
			f.Service = app.New(f.store, clients{}, roles{}, admins{}, f.events, time.Now, dry{})
			if err := c.issue(f, appID); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// halfDry gives the first read and fails the second, for a refresh token whose secret is made but
// whose id is not.
type halfDry struct{ reads int }

func (h *halfDry) Read(p []byte) (int, error) {
	h.reads++
	if h.reads > 1 {
		return 0, errBoom
	}
	return len(p), nil
}

func TestNoRefreshTokenIsKeptWithoutAnID(t *testing.T) {
	f := newFixture()
	_, appID := f.connected(t)
	f.Service = app.New(f.store, clients{}, roles{}, admins{}, f.events, time.Now, &halfDry{})
	if _, err := f.IssueRefresh(ctx, appID); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAUserRevokesTheirConnectedAppAndItsTokensStopAtOnce(t *testing.T) {
	f := newFixture()
	_, appID := f.connected(t)
	access, _, _ := f.IssueAccess(ctx, appID)

	if err := f.Revoke(ctx, ada, appID); err != nil {
		t.Fatal(err)
	}

	if _, _, err := f.Check(ctx, access, "kb"); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("the access token still works: %v", err)
	}
	if !slices.Equal(f.forgotten, []uuid.UUID{access}) {
		t.Fatalf("forgotten %v", f.forgotten)
	}
	want := audit.Event{Action: audit.ConnectedAppRevoked, User: ada, Client: "kb", AppName: "Claude Code"}
	if len(*f.events) != 1 || (*f.events)[0] != want {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestAUserCannotRevokeSomeoneElsesConnectedApp(t *testing.T) {
	f := newFixture()
	_, appID := f.connected(t)
	if err := f.Revoke(ctx, root, appID); !errors.Is(err, app.ErrNotFound) || len(f.store.apps) != 1 {
		t.Fatalf("err = %v", err)
	}
}

func TestARevocationFails(t *testing.T) {
	for _, method := range []string{"ConnectedApp", "DeleteConnectedApp"} {
		t.Run(method, func(t *testing.T) {
			f := newFixture()
			_, appID := f.connected(t)
			f.store.fail[method] = errBoom
			if err := f.Revoke(ctx, ada, appID); !errors.Is(err, errBoom) || len(*f.events) != 0 {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAUserListsTheirConnectedApps(t *testing.T) {
	f := newFixture()
	_, appID := f.connected(t)
	apps, err := f.ConnectedApps(ctx, ada)
	if err != nil || len(apps) != 1 || apps[0].ID != appID || apps[0].Name != "Claude Code" {
		t.Fatalf("ConnectedApps = %+v, %v", apps, err)
	}
}

func TestTurningStepUpOnRevokesTheClientsConnectedAppsAsTheAdmin(t *testing.T) {
	f := newFixture()
	_, appID := f.connected(t)
	access, _, _ := f.IssueAccess(ctx, appID)
	if n, _ := f.CountForClient(ctx, "kb"); n != 1 {
		t.Fatalf("count = %d", n)
	}

	if err := f.RevokeForClient(ctx, root, "kb"); err != nil {
		t.Fatal(err)
	}

	want := audit.Event{Action: audit.ConnectedAppRevoked, Actor: root, User: ada, Client: "kb", AppName: "Claude Code"}
	if len(f.store.apps) != 0 || len(*f.events) != 1 || (*f.events)[0] != want || !slices.Contains(f.forgotten, access) {
		t.Fatalf("apps %d, events %+v, forgotten %v", len(f.store.apps), *f.events, f.forgotten)
	}
}

func TestRevokingAClientsConnectedAppsFailsWhenTheStoreDoes(t *testing.T) {
	f := newFixture()
	f.connected(t)
	f.store.fail["DeleteForClient"] = errBoom
	if err := f.RevokeForClient(ctx, root, "kb"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnAdminMarksAClientAsAnMCPResourceAsAnAuditEvent(t *testing.T) {
	f := newFixture()
	if err := f.SetMCPResource(ctx, app.Actor{ID: root, SecondFactor: true}, "wiki", true); err != nil {
		t.Fatal(err)
	}
	if on, _ := f.IsMCPResource(ctx, "wiki"); !on {
		t.Fatal("wiki is not marked")
	}
	if all, _ := f.MCPResources(ctx); !slices.Equal(all, []string{"kb", "stats", "wiki"}) {
		t.Fatalf("resources %v", all)
	}
	if (*f.events)[0] != (audit.Event{Action: audit.MCPResourceMarked, Actor: root, Client: "wiki"}) {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestAnAdminUnmarksAClientAsAnAuditEvent(t *testing.T) {
	f := newFixture()
	_ = f.SetMCPResource(ctx, app.Actor{ID: root, SecondFactor: true}, "kb", false)
	if on, _ := f.IsMCPResource(ctx, "kb"); on || (*f.events)[0].Action != audit.MCPResourceUnmarked {
		t.Fatalf("still marked, events %+v", *f.events)
	}
}

func TestMarkingAnMCPResourceIsRefused(t *testing.T) {
	for _, c := range []struct {
		name   string
		actor  app.Actor
		client string
		want   error
	}{
		{"for someone who is no Admin", app.Actor{ID: ada, SecondFactor: true}, "wiki", app.ErrNotAdmin},
		{"from a session with no second factor", app.Actor{ID: root}, "wiki", app.ErrSecondFactorNeeded},
		{"for a Client not in the cluster", app.Actor{ID: root, SecondFactor: true}, "gone", app.ErrNotFound},
		{"when the Admin check fails", app.Actor{}, "wiki", errBoom},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture()
			if err := f.SetMCPResource(ctx, c.actor, c.client, true); !errors.Is(err, c.want) || f.store.mcp["wiki"] {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestMarkingAnMCPResourceFailsWhenTheStoreDoes(t *testing.T) {
	f := newFixture()
	f.store.fail["SetMCPResource"] = errBoom
	if err := f.SetMCPResource(ctx, app.Actor{ID: root, SecondFactor: true}, "wiki", true); !errors.Is(err, errBoom) || len(*f.events) != 0 {
		t.Fatalf("err = %v", err)
	}
}

func TestPruningRemovesRegistrationsNobodyConsentedToAndExpiredTokens(t *testing.T) {
	f := newFixture()
	_, appID := f.connected(t)
	_, _ = f.Register(ctx, "Forgotten", []string{"https://app.example.org/cb"})
	_, _, _ = f.IssueAccess(ctx, appID)
	f.now = t0.Add(25 * time.Hour)

	n, err := f.Prune(ctx)

	if err != nil || n != 2 || len(f.store.regs) != 1 || len(f.store.tokens) != 0 {
		t.Fatalf("pruned %d, %v; registrations %d, tokens %d", n, err, len(f.store.regs), len(f.store.tokens))
	}
}

func TestPruningKeepsARegistrationForADayToBeConsentedTo(t *testing.T) {
	f := newFixture()
	_, _ = f.Register(ctx, "Waiting", []string{"https://app.example.org/cb"})
	f.now = t0.Add(23 * time.Hour)
	if n, err := f.Prune(ctx); err != nil || n != 0 || len(f.store.regs) != 1 {
		t.Fatalf("pruned %d, %v", n, err)
	}
}

func TestPruningFailsWhenTheStoreDoes(t *testing.T) {
	for _, method := range []string{"PruneRegistrations", "PruneTokens"} {
		t.Run(method, func(t *testing.T) {
			f := newFixture()
			f.store.fail[method] = errBoom
			if _, err := f.Prune(ctx); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
