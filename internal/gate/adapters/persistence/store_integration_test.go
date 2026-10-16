//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JorisJonkers-dev/identity/internal/gate/adapters/persistence"
	"github.com/JorisJonkers-dev/identity/internal/gate/app"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
)

var t0 = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

// session makes a User and a Single sign-on session, and returns the session's id.
func session(ctx context.Context, t *testing.T, q *queries.Queries) uuid.UUID {
	t.Helper()
	userID, id := uuid.New(), uuid.New()
	if err := q.CreateUser(ctx, queries.CreateUserParams{ID: userID, Email: "ada@example.org", Name: "Ada", PasswordHash: "h", CreatedAt: t0}); err != nil {
		t.Fatal(err)
	}
	if err := q.CreateSession(ctx, queries.CreateSessionParams{TokenHash: []byte("sso"), ID: id, UserID: userID, CreatedAt: t0, ExpiresAt: t0.Add(time.Hour), Methods: []string{"pwd"}}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCodesAndHostSessions(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := queries.New(pool)
	s := persistence.New(q)
	sso := session(ctx, t, q)

	c := app.Code{Hash: []byte("code"), Host: "stats.jorisjonkers.dev", SessionID: sso, StateHash: []byte("state"), ReturnPath: "/stats", ExpiresAt: t0}
	if err := s.CreateCode(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, err := s.TakeCode(ctx, c.Hash)
	if err != nil || got.Host != c.Host || got.SessionID != sso || string(got.StateHash) != "state" || got.ReturnPath != "/stats" || !got.ExpiresAt.Equal(t0) {
		t.Fatalf("TakeCode = %+v, %v", got, err)
	}
	if _, err := s.TakeCode(ctx, c.Hash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a code taken twice = %v", err)
	}

	if err := s.CreateSession(ctx, []byte("cookie"), app.Session{Host: c.Host, SessionID: sso}, t0); err != nil {
		t.Fatal(err)
	}
	if gs, err := s.SessionByHash(ctx, []byte("cookie")); err != nil || gs.Host != c.Host || gs.SessionID != sso {
		t.Fatalf("SessionByHash = %+v, %v", gs, err)
	}
	if _, err := s.SessionByHash(ctx, []byte("other")); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown cookie = %v", err)
	}

	// Signing out of the Single sign-on session ends the host sessions and codes made from it.
	_ = s.CreateCode(ctx, c)
	if _, err := pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sso); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash(ctx, []byte("cookie")); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a host session outlived its Single sign-on session: %v", err)
	}
	if _, err := s.TakeCode(ctx, c.Hash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a code outlived its Single sign-on session: %v", err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.SessionByHash(cancelled, []byte("x")); err == nil || errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a cancelled lookup = %v", err)
	}
}
