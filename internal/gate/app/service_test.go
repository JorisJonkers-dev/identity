package app_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/gate/app"
)

var (
	errBoom = errors.New("boom")
	t0      = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	ada     = app.Person{UserID: uuid.MustParse("00000000-0000-4000-8000-0000000000a1"), Email: "ada@example.org", Name: "Ada", SessionID: uuid.MustParse("00000000-0000-4000-8000-0000000000c1")}
	stats   = app.Client{ID: "tribelt-stats", Label: "Tribelt stats", Host: "tribelt.jorisjonkers.dev"}
)

func hashOf(s string) string {
	h := sha256.Sum256([]byte(s))
	return string(h[:])
}

type store struct {
	codes    map[string]app.Code
	sessions map[string]app.Session
	fail     map[string]error
	reads    int
}

func (s *store) CreateCode(_ context.Context, c app.Code) error {
	if err := s.fail["CreateCode"]; err != nil {
		return err
	}
	s.codes[string(c.Hash)] = c
	return nil
}

func (s *store) TakeCode(_ context.Context, hash []byte) (app.Code, error) {
	if err := s.fail["TakeCode"]; err != nil {
		return app.Code{}, err
	}
	c, ok := s.codes[string(hash)]
	if !ok {
		return app.Code{}, app.ErrNotFound
	}
	delete(s.codes, string(hash))
	return c, nil
}

func (s *store) CreateSession(_ context.Context, hash []byte, gs app.Session, _ time.Time) error {
	if err := s.fail["CreateSession"]; err != nil {
		return err
	}
	s.sessions[string(hash)] = gs
	return nil
}

func (s *store) SessionByHash(_ context.Context, hash []byte) (app.Session, error) {
	s.reads++
	if err := s.fail["SessionByHash"]; err != nil {
		return app.Session{}, err
	}
	gs, ok := s.sessions[string(hash)]
	if !ok {
		return app.Session{}, app.ErrNotFound
	}
	return gs, nil
}

type clients map[string]app.Client

func (c clients) GatedClient(_ context.Context, host string) (app.Client, error) {
	if host == "broken.jorisjonkers.dev" {
		return app.Client{}, errBoom
	}
	cl, ok := c[host]
	if !ok {
		return app.Client{}, app.ErrUnknownHost
	}
	return cl, nil
}

type roles struct {
	held map[string][]string
	err  error
}

func (r *roles) Roles(_ context.Context, _ uuid.UUID, clientID string) ([]string, error) {
	return r.held[clientID], r.err
}

// accounts knows one Single sign-on session, "sso", for Ada.
type accounts struct {
	signedOut bool
	err       error
}

func (a *accounts) SignedIn(_ context.Context, token string) (app.Person, error) {
	if token != "sso" || a.signedOut {
		return app.Person{}, app.ErrNoSignIn
	}
	return ada, a.err
}

func (a *accounts) Vouch(_ context.Context, id uuid.UUID) (app.Person, error) {
	if id != ada.SessionID || a.signedOut {
		return app.Person{}, app.ErrNoSignIn
	}
	return ada, a.err
}

type counter struct{ n byte }

func (c *counter) Read(p []byte) (int, error) {
	c.n++
	for i := range p {
		p[i] = c.n
	}
	return len(p), nil
}

type fixture struct {
	*app.Service
	store    *store
	roles    *roles
	accounts *accounts
	now      time.Time
}

func newFixture() *fixture {
	f := &fixture{
		store:    &store{codes: map[string]app.Code{}, sessions: map[string]app.Session{}, fail: map[string]error{}},
		roles:    &roles{held: map[string][]string{"tribelt-stats": {"user"}}},
		accounts: &accounts{},
		now:      t0,
	}
	f.Service = app.New(f.store, clients{stats.Host: stats}, f.roles, f.accounts, func() time.Time { return f.now }, &counter{}, 30*time.Second)
	return f
}

// signedIn runs a sign-in through the Gate and returns the host cookie.
func (f *fixture) signedIn(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	code, err := f.Start(ctx, "sso", stats.Host, "state-1", "/stats?week=42")
	if err != nil {
		t.Fatal(err)
	}
	cookie, back, err := f.Finish(ctx, stats.Host, code, "state-1")
	if err != nil || back != "/stats?week=42" {
		t.Fatalf("Finish = %q, %q, %v", cookie, back, err)
	}
	return cookie
}

func TestASignInCarriesToTheHostAndIsCheckedFromMemory(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	cookie := f.signedIn(t)
	if _, ok := f.store.sessions[hashOf(cookie)]; !ok {
		t.Fatal("the host session is not stored by its cookie's hash")
	}

	v, err := f.Check(ctx, stats.Host, cookie)
	if err != nil || v.UserID != ada.UserID || v.Email != "ada@example.org" || v.Name != "Ada" || len(v.Roles) != 1 || v.Roles[0] != "user" {
		t.Fatalf("Check = %+v, %v", v, err)
	}
	for range 100 {
		_, _ = f.Check(ctx, stats.Host, cookie)
	}
	if f.store.reads != 1 {
		t.Fatalf("%d store reads for 101 checks within the cache's time", f.store.reads)
	}

	// Signing out reaches the host once the cached answer runs out.
	f.accounts.signedOut = true
	if _, err := f.Check(ctx, stats.Host, cookie); err != nil {
		t.Fatalf("a cached check = %v", err)
	}
	f.now = t0.Add(30 * time.Second)
	if _, err := f.Check(ctx, stats.Host, cookie); !errors.Is(err, app.ErrNoSignIn) {
		t.Fatalf("after signing out = %v", err)
	}
}

func TestASignOutReachesTheCacheAtOnce(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	cookie := f.signedIn(t)
	other := f.signedIn(t)
	f.store.sessions[hashOf(other)] = app.Session{Host: stats.Host, SessionID: uuid.New()}
	_, _ = f.Check(ctx, stats.Host, cookie)
	_, _ = f.Check(ctx, stats.Host, other)
	f.accounts.signedOut = true
	f.Forget([]uuid.UUID{ada.SessionID})
	if _, err := f.Check(ctx, stats.Host, cookie); !errors.Is(err, app.ErrNoSignIn) {
		t.Fatalf("a check after the session was forgotten = %v", err)
	}
	reads := f.store.reads
	_, _ = f.Check(ctx, stats.Host, other)
	if f.store.reads != reads {
		t.Fatal("another session's cached check was dropped too")
	}
}

func TestACheckRefuses(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	cookie := f.signedIn(t)
	if _, err := f.Check(ctx, stats.Host, ""); !errors.Is(err, app.ErrNoSignIn) {
		t.Errorf("no cookie = %v", err)
	}
	if _, err := f.Check(ctx, "unknown.jorisjonkers.dev", ""); !errors.Is(err, app.ErrUnknownHost) {
		t.Errorf("an unknown host without a cookie = %v", err)
	}
	if _, err := f.Check(ctx, "unknown.jorisjonkers.dev", cookie); !errors.Is(err, app.ErrUnknownHost) {
		t.Errorf("an unknown host = %v", err)
	}
	if _, err := f.Check(ctx, stats.Host, "made-up"); !errors.Is(err, app.ErrNoSignIn) {
		t.Errorf("an unknown cookie = %v", err)
	}

	// A host's cookie is no good on another gated host.
	f2 := newFixture()
	other := app.Client{ID: "other", Label: "Other", Host: "other.jorisjonkers.dev"}
	f2.Service = app.New(f2.store, clients{stats.Host: stats, other.Host: other}, f2.roles, f2.accounts, time.Now, &counter{}, time.Minute)
	theirs := f2.signedIn(t)
	f2.roles.held["other"] = []string{"user"}
	if _, err := f2.Check(ctx, other.Host, theirs); !errors.Is(err, app.ErrNoSignIn) {
		t.Errorf("a cookie on another host = %v", err)
	}

	f.roles.held["tribelt-stats"] = nil
	f.now = t0.Add(time.Hour)
	if _, err := f.Check(ctx, stats.Host, cookie); !errors.Is(err, app.ErrNoAccess) {
		t.Errorf("a role taken away = %v", err)
	}
}

func TestFailuresAreNotRemembered(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	cookie := f.signedIn(t)
	f.store.fail["SessionByHash"] = errBoom
	if _, err := f.Check(ctx, stats.Host, cookie); !errors.Is(err, errBoom) {
		t.Fatalf("a failing store = %v", err)
	}
	f.store.fail = map[string]error{}
	if _, err := f.Check(ctx, stats.Host, cookie); err != nil {
		t.Fatalf("the next check after a failure = %v", err)
	}
	f.now = t0.Add(time.Hour)
	f.accounts.err = errBoom
	if _, err := f.Check(ctx, stats.Host, cookie); !errors.Is(err, errBoom) {
		t.Errorf("failing accounts = %v", err)
	}
	f.accounts.err = nil
	f.roles.err = errBoom
	if _, err := f.Check(ctx, stats.Host, cookie); !errors.Is(err, errBoom) {
		t.Errorf("failing roles = %v", err)
	}
	if _, err := f.Check(ctx, "broken.jorisjonkers.dev", cookie); !errors.Is(err, errBoom) {
		t.Errorf("a failing Client lookup = %v", err)
	}
}

func TestTheCacheStartsAgainWhenFull(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	cookie := f.signedIn(t)
	for i := range 50_001 {
		_, _ = f.Check(ctx, stats.Host, fmt.Sprint("nobody-", i))
	}
	reads := f.store.reads
	_, _ = f.Check(ctx, stats.Host, cookie)
	_, _ = f.Check(ctx, stats.Host, cookie)
	if f.store.reads != reads+1 {
		t.Fatalf("checks after the cache started again read the store %d times", f.store.reads-reads)
	}
}

func TestAGateSignInIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name, sso, host, state string
		noRole                 bool
		want                   error
	}{
		{name: "for a host with no gated Client", sso: "sso", host: "unknown.jorisjonkers.dev", state: "s", want: app.ErrUnknownHost},
		{name: "without the page's state", sso: "sso", host: stats.Host, want: app.ErrInvalidCode},
		{name: "without a Single sign-on session", sso: "nobody", host: stats.Host, state: "s", want: app.ErrNoSignIn},
		{name: "for a User with no role on the app", sso: "sso", host: stats.Host, state: "s", noRole: true, want: app.ErrNoAccess},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			if tc.noRole {
				f.roles.held["tribelt-stats"] = nil
			}
			if _, err := f.Start(context.Background(), tc.sso, tc.host, tc.state, "/"); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAGateSignInFailsWhenTheStoreDoes(t *testing.T) {
	f := newFixture()
	f.store.fail["CreateCode"] = errBoom
	if _, err := f.Start(context.Background(), "sso", stats.Host, "s", "/"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestNoGateCodeOrStateIsMadeWithoutRandomness(t *testing.T) {
	f := newFixture()
	dry := app.New(f.store, clients{stats.Host: stats}, f.roles, f.accounts, time.Now, bytes.NewReader(nil), time.Minute)
	if _, err := dry.Start(context.Background(), "sso", stats.Host, "s", "/"); err == nil {
		t.Error("a code was made")
	}
	if _, err := dry.NewState(); err == nil {
		t.Error("a state was made")
	}
}

func TestACodeWorksOnceInTheBrowserThatStartedIt(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	code, _ := f.Start(ctx, "sso", stats.Host, "state-1", "/")
	if c := f.store.codes[hashOf(code)]; c.Host != stats.Host || c.SessionID != ada.SessionID || !c.ExpiresAt.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("code = %+v", c)
	}
	if _, _, err := f.Finish(ctx, stats.Host, code, "another browser's state"); !errors.Is(err, app.ErrInvalidCode) {
		t.Fatalf("a code in another browser = %v", err)
	}
	if _, _, err := f.Finish(ctx, stats.Host, code, "state-1"); !errors.Is(err, app.ErrInvalidCode) {
		t.Fatalf("a code tried once already = %v", err)
	}
}

func TestACodeIsRefusedOnAnotherHost(t *testing.T) {
	f := newFixture()
	c, _ := f.Start(context.Background(), "sso", stats.Host, "s", "/")
	if _, _, err := f.Finish(context.Background(), "other.jorisjonkers.dev", c, "s"); !errors.Is(err, app.ErrInvalidCode) {
		t.Fatalf("err = %v", err)
	}
}

func TestACodeIsRefusedOnceItExpired(t *testing.T) {
	f := newFixture()
	f.now = t0.Add(time.Hour)
	c, _ := f.Start(context.Background(), "sso", stats.Host, "s", "/")
	f.now = t0.Add(time.Hour + 2*time.Minute)
	if _, _, err := f.Finish(context.Background(), stats.Host, c, "s"); !errors.Is(err, app.ErrInvalidCode) {
		t.Fatalf("err = %v", err)
	}
}

func TestFinishingAGateSignInFailsWhenTheStoreDoes(t *testing.T) {
	for _, op := range []string{"CreateSession", "TakeCode"} {
		t.Run(op, func(t *testing.T) {
			f := newFixture()
			c, _ := f.Start(context.Background(), "sso", stats.Host, "s", "/")
			f.store.fail[op] = errBoom
			if _, _, err := f.Finish(context.Background(), stats.Host, c, "s"); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestNoHostCookieIsMadeWithoutRandomness(t *testing.T) {
	f := newFixture()
	c, _ := f.Start(context.Background(), "sso", stats.Host, "s", "/")
	f.Service = app.New(f.store, clients{stats.Host: stats}, f.roles, f.accounts, func() time.Time { return t0 }, bytes.NewReader(nil), time.Minute)
	if _, _, err := f.Finish(context.Background(), stats.Host, c, "s"); err == nil {
		t.Fatal("a host cookie was made")
	}
}

func TestSafePathStaysOnTheHost(t *testing.T) {
	for in, want := range map[string]string{
		"/stats?week=42":         "/stats?week=42",
		"/":                      "/",
		"":                       "/",
		"https://evil.test/":     "/",
		"//evil.test/":           "/",
		"/\\evil.test":           "/",
		"/ok\r\nSet-Cookie: x=1": "/",
	} {
		if got := app.SafePath(in); got != want {
			t.Errorf("SafePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClientAndState(t *testing.T) {
	f := newFixture()
	if c, err := f.Client(context.Background(), stats.Host); err != nil || c != stats {
		t.Fatalf("Client = %+v, %v", c, err)
	}
	a, _ := f.NewState()
	b, _ := f.NewState()
	if a == "" || a == b {
		t.Fatalf("states %q and %q", a, b)
	}
}
