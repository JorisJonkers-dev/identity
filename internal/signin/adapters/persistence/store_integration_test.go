//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/persistence"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var t0 = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func open(t *testing.T) (*persistence.Store, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return persistence.New(queries.New(pool)), pool
}

// session inserts a User with one session and returns their ids.
func session(t *testing.T, pool *pgxpool.Pool) (userID, sessionID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	userID, sessionID = uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name, password_hash, created_at) VALUES ($1, $2, 'Ada', 'h', now())`,
		userID, userID.String()+"@example.org"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions (token_hash, id, user_id, created_at, last_seen_at, expires_at) VALUES ($1, $2, $3, now(), now(), now())`,
		sessionID[:], sessionID, userID); err != nil {
		t.Fatal(err)
	}
	return userID, sessionID
}

func request() domain.AuthRequest {
	return domain.AuthRequest{
		ID: uuid.New(), ClientID: "spa", RedirectURI: "https://spa.test/cb", Scopes: []string{"openid", "email"},
		State: "st", Nonce: "n", ResponseType: "code", ResponseMode: "query",
		CodeChallenge: "ch", CodeChallengeMethod: "S256", CreatedAt: t0, ExpiresAt: t0.Add(10 * time.Minute),
	}
}

func TestARequestFromStartToCode(t *testing.T) {
	ctx := context.Background()
	s, pool := open(t)
	r := request()
	if err := s.CreateRequest(ctx, r); err != nil {
		t.Fatal(err)
	}
	got, err := s.RequestByID(ctx, r.ID)
	if err != nil || got.ClientID != "spa" || len(got.Scopes) != 2 || got.CodeChallenge != "ch" || got.SignIn != nil || !got.ExpiresAt.Equal(r.ExpiresAt) {
		t.Fatalf("RequestByID = %+v, %v", got, err)
	}
	if saved, err := s.SaveCode(ctx, r.ID, []byte("h")); saved || err != nil {
		t.Fatalf("a code for a request nobody completed: %v, %v", saved, err)
	}

	userID, sessionID := session(t, pool)
	in := domain.SignIn{UserID: userID, SessionID: sessionID, At: t0}
	if done, err := s.CompleteRequest(ctx, r.ID, in); !done || err != nil {
		t.Fatalf("CompleteRequest = %v, %v", done, err)
	}
	if done, _ := s.CompleteRequest(ctx, r.ID, in); done {
		t.Fatal("a request was completed twice")
	}
	if saved, err := s.SaveCode(ctx, r.ID, []byte("code-hash")); !saved || err != nil {
		t.Fatalf("SaveCode = %v, %v", saved, err)
	}
	if saved, _ := s.SaveCode(ctx, r.ID, []byte("other")); saved {
		t.Fatal("a request got a second code")
	}
	got, err = s.TakeByCodeHash(ctx, []byte("code-hash"))
	if err != nil || got.ID != r.ID || got.SignIn == nil || got.SignIn.UserID != in.UserID ||
		got.SignIn.SessionID != in.SessionID || !got.SignIn.At.Equal(in.At) {
		t.Fatalf("RequestByCodeHash = %+v, %v", got, err)
	}

	if _, err := s.TakeByCodeHash(ctx, []byte("code-hash")); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a code taken twice: %v", err)
	}
	if _, err := s.RequestByID(ctx, r.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a taken request is still there: %v", err)
	}

	r = request()
	_ = s.CreateRequest(ctx, r)
	if err := s.DeleteRequest(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RequestByID(ctx, r.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("after deleting: %v", err)
	}
}

func TestEndingTheSessionEndsItsRequests(t *testing.T) {
	ctx := context.Background()
	s, pool := open(t)
	r := request()
	_ = s.CreateRequest(ctx, r)
	userID, sessionID := session(t, pool)
	_, _ = s.CompleteRequest(ctx, r.ID, domain.SignIn{UserID: userID, SessionID: sessionID, At: t0})
	if _, err := pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RequestByID(ctx, r.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a request survived its session: %v", err)
	}
}

func TestFailuresOtherThanNotFoundPassThrough(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s, _ := open(t)
	cancel()
	if _, err := s.RequestByID(ctx, uuid.New()); err == nil || errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a cancelled query: %v", err)
	}
}

func TestOnlyOneOfTwoRacingExchangesTakesACode(t *testing.T) {
	ctx := context.Background()
	s, pool := open(t)
	for range 20 {
		r := request()
		_ = s.CreateRequest(ctx, r)
		userID, sessionID := session(t, pool)
		_, _ = s.CompleteRequest(ctx, r.ID, domain.SignIn{UserID: userID, SessionID: sessionID, At: t0})
		hash := []byte("race-" + r.ID.String())
		_, _ = s.SaveCode(ctx, r.ID, hash)

		taken := make(chan error, 2)
		for range 2 {
			go func() {
				_, err := s.TakeByCodeHash(ctx, hash)
				taken <- err
			}()
		}
		first, second := <-taken, <-taken
		if (first == nil) == (second == nil) {
			t.Fatalf("two exchanges with one code: %v and %v", first, second)
		}
	}
}
