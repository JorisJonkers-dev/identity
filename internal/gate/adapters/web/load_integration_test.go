//go:build integration

package web_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/adapters/catalog"
	"github.com/JorisJonkers-dev/identity/internal/access/adapters/people"
	accesspersistence "github.com/JorisJonkers-dev/identity/internal/access/adapters/persistence"
	accessapp "github.com/JorisJonkers-dev/identity/internal/access/app"
	auditdomain "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/gate/adapters/contexts"
	gatepersistence "github.com/JorisJonkers-dev/identity/internal/gate/adapters/persistence"
	"github.com/JorisJonkers-dev/identity/internal/gate/adapters/web"
	gateapp "github.com/JorisJonkers-dev/identity/internal/gate/app"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/accounts"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	signinpersistence "github.com/JorisJonkers-dev/identity/internal/signin/adapters/persistence"
	signinapp "github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
	userspersistence "github.com/JorisJonkers-dev/identity/internal/users/adapters/persistence"
	usersapp "github.com/JorisJonkers-dev/identity/internal/users/app"
)

// The check's target (docs/adr/0009), and how many requests the test sends.
const (
	p99Target = 5 * time.Millisecond
	requests  = 5000
)

func sum(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

// TestTheGateCheckHoldsItsP99 runs the real check over Postgres behind a real HTTP server, with
// a warm cache, and fails when its p99 reaches the target. It times the server's handling, not the
// client's round trip, and keeps one request in flight per CPU, so a busy CI runner's queueing in
// the test's own client is not counted against the check.
func TestTheGateCheckHoldsItsP99(t *testing.T) {
	ctx := context.Background()
	store, err := pg.Open(ctx, pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	q := store.Queries()

	userID, sessionID, groupID := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()
	must(t, q.CreateUser(ctx, queries.CreateUserParams{ID: userID, Email: "ada@example.org", Name: "Ada", PasswordHash: "h", CreatedAt: now}))
	must(t, q.CreateSession(ctx, queries.CreateSessionParams{TokenHash: sum("sso"), ID: sessionID, UserID: userID, CreatedAt: now, ExpiresAt: now.Add(time.Hour), Methods: []string{"pwd"}}))
	must(t, q.CreateGroup(ctx, queries.CreateGroupParams{ID: groupID, Name: "Readers"}))
	_, err = q.AddMember(ctx, queries.AddMemberParams{GroupID: groupID, UserID: userID})
	must(t, err)
	_, err = q.Assign(ctx, queries.AssignParams{GroupID: groupID, ClientID: "stats", Role: "user"})
	must(t, err)
	must(t, q.CreateGateSession(ctx, queries.CreateGateSessionParams{TokenHash: sum("host-cookie"), Host: "stats.jorisjonkers.dev", SessionID: sessionID, CreatedAt: now}))

	index := clients.NewIndex()
	index.Put(domain.Client{ID: "stats", SignIn: domain.Gated, Host: "stats.jorisjonkers.dev", Label: "Stats", Roles: []string{"user"}})
	users := usersapp.New(userspersistence.New(q), nil, nil, time.Now, rand.Reader, "")
	access := accessapp.New(accesspersistence.New(q, store), catalog.New(index), people.New(users), discard{}, rand.Reader)
	signIn := signinapp.New(signinpersistence.New(q), index, accounts.New(users), access, nil, discard{}, time.Now, rand.Reader)
	others := contexts.New(signIn, users)
	gate := gateapp.New(gatepersistence.New(q), others, others, others, discard{}, time.Now, rand.Reader, 30*time.Second)
	h, err := web.New(gate, web.Config{Issuer: "https://auth.test", SessionCookie: "__Host-identity_session"}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	times := make([]time.Duration, 0, requests)
	timed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		began := time.Now()
		h.ServeHTTP(w, r)
		took := time.Since(began)
		if r.Header.Get("X-Warm") == "" {
			mu.Lock()
			times = append(times, took)
			mu.Unlock()
		}
	})
	srv := httptest.NewServer(timed)
	t.Cleanup(srv.Close)
	workers := runtime.GOMAXPROCS(0)
	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: workers}}

	check := func(warm bool) error {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/gate/check", nil)
		req.Header.Set("X-Forwarded-Host", "stats.jorisjonkers.dev")
		req.Header.Set("X-Forwarded-Uri", "/")
		req.AddCookie(&http.Cookie{Name: "__Host-identity_gate", Value: "host-cookie"}) //nolint:gosec // a request's cookie carries no attributes
		if warm {
			req.Header.Set("X-Warm", "1")
		}
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("check = %d", res.StatusCode)
		}
		return nil
	}
	for range workers { // warm the cache and the connections
		if err := check(true); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Go(func() {
			for range requests / workers {
				if err := check(false); err != nil {
					errs <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	slices.Sort(times)
	p50, p99 := times[len(times)/2], times[len(times)*99/100]
	t.Logf("Gate check over %d requests: p50 %v, p99 %v", len(times), p50, p99)
	if p99 >= p99Target {
		t.Fatalf("the Gate check's p99 is %v, at or above its %v target (p50 %v)", p99, p99Target, p50)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// discard records no Audit Events.
type discard struct{}

func (discard) Record(context.Context, auditdomain.Event) {}
