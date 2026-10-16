//go:build integration

package persistence_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/identity/internal/users/adapters/persistence"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
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

func user(t *testing.T) domain.User {
	t.Helper()
	e, err := domain.NewEmail("ada@example.org")
	if err != nil {
		t.Fatal(err)
	}
	n, _ := domain.NewName("Ada")
	return domain.User{ID: uuid.New(), Email: e, Name: n, PasswordHash: "stored-hash-1", CreatedAt: t0}
}

func TestUsers(t *testing.T) {
	ctx := context.Background()
	s, _ := open(t)
	u := user(t)
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateUser(ctx, user(t)); !errors.Is(err, app.ErrEmailTaken) {
		t.Fatalf("a second user with the same email: %v", err)
	}

	byEmail, err := s.UserByEmail(ctx, u.Email)
	if err != nil || byEmail.ID != u.ID || !byEmail.CreatedAt.Equal(t0) {
		t.Fatalf("UserByEmail = %+v, %v", byEmail, err)
	}
	missing, _ := domain.NewEmail("nobody@example.org")
	if _, err := s.UserByEmail(ctx, missing); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown email: %v", err)
	}
	if _, err := s.UserByID(ctx, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown id: %v", err)
	}

	if all, err := s.Users(ctx); err != nil || len(all) != 1 || all[0].ID != u.ID {
		t.Fatalf("Users = %+v, %v", all, err)
	}

	newName, _ := domain.NewName("Ada King")
	if err := s.UpdateName(ctx, u.ID, newName); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePasswordHash(ctx, u.ID, "stored-hash-2"); err != nil {
		t.Fatal(err)
	}
	got, err := s.UserByID(ctx, u.ID)
	if err != nil || got.Name != newName || got.PasswordHash != "stored-hash-2" {
		t.Fatalf("after the changes: %+v, %v", got, err)
	}
}

func TestARowTheDomainRefusesIsAnError(t *testing.T) {
	ctx := context.Background()
	s, pool := open(t)
	// The database checks case and length, not the @; the domain refuses this when reading it.
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name, password_hash, created_at) VALUES ($1, 'not-an-email', 'Ada', 'h', now())`, uuid.New()); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email = 'not-an-email'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserByID(ctx, id); !errors.Is(err, domain.ErrInvalidEmail) {
		t.Fatalf("UserByID = %v, want ErrInvalidEmail", err)
	}
	if _, err := s.Users(ctx); !errors.Is(err, domain.ErrInvalidEmail) {
		t.Fatalf("Users = %v, want ErrInvalidEmail", err)
	}
}

func TestSignups(t *testing.T) {
	ctx := context.Background()
	s, pool := open(t)
	u := user(t)
	su := app.Signup{TokenHash: []byte("hash-of-a-token"), Email: u.Email, Name: u.Name, PasswordHash: "stored-hash-1", ExpiresAt: t0.Add(24 * time.Hour)}
	if err := s.CreateSignup(ctx, su); err != nil {
		t.Fatal(err)
	}
	got, err := s.SignupByHash(ctx, su.TokenHash)
	if err != nil || got.Email != su.Email || got.Name != su.Name || got.PasswordHash != su.PasswordHash || !got.ExpiresAt.Equal(su.ExpiresAt) {
		t.Fatalf("SignupByHash = %+v, %v", got, err)
	}
	if err := s.DeleteSignup(ctx, su.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SignupByHash(ctx, su.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("after deleting: %v", err)
	}

	for _, row := range [][2]string{{"not-an-email", "Ada"}, {"ada@example.org", ""}} {
		hash := []byte("bad-" + row[0] + row[1])
		if _, err := pool.Exec(ctx, `INSERT INTO signups (token_hash, email, name, password_hash, expires_at) VALUES ($1, $2, $3, 'h', now())`, hash, row[0], row[1]); err != nil {
			t.Fatal(err)
		}
		if _, err := s.SignupByHash(ctx, hash); err == nil {
			t.Fatalf("a sign-up row %v the domain refuses was read", row)
		}
	}
}

func TestSessions(t *testing.T) {
	ctx := context.Background()
	s, pool := open(t)
	u := user(t)
	_ = s.CreateUser(ctx, u)
	sess := app.Session{TokenHash: []byte("hash"), ID: uuid.New(), UserID: u.ID, CreatedAt: t0, LastSeenAt: t0, ExpiresAt: t0.Add(time.Hour)}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	if err := s.TouchSession(ctx, sess.TokenHash, t0.Add(time.Minute), t0.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := s.SessionByHash(ctx, sess.TokenHash)
	if err != nil || !bytes.Equal(got.TokenHash, sess.TokenHash) || got.ID != sess.ID || got.UserID != u.ID ||
		!got.CreatedAt.Equal(t0) || !got.LastSeenAt.Equal(t0.Add(time.Minute)) || !got.ExpiresAt.Equal(t0.Add(2*time.Hour)) {
		t.Fatalf("SessionByHash = %+v, %v", got, err)
	}
	if err := s.DeleteSession(ctx, sess.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash(ctx, sess.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("after deleting: %v", err)
	}
	if err := s.DeleteSession(ctx, sess.TokenHash); err != nil {
		t.Fatalf("deleting a session that does not exist: %v", err)
	}

	keep := app.Session{TokenHash: []byte("keep"), ID: uuid.New(), UserID: u.ID, CreatedAt: t0, LastSeenAt: t0, ExpiresAt: t0.Add(time.Hour)}
	other := keep
	other.TokenHash, other.ID = []byte("other"), uuid.New()
	_ = s.CreateSession(ctx, keep)
	_ = s.CreateSession(ctx, other)
	if err := s.DeleteOtherSessions(ctx, u.ID, keep.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash(ctx, other.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("the other session survived: %v", err)
	}
	if _, err := s.SessionByHash(ctx, keep.TokenHash); err != nil {
		t.Fatalf("the kept session ended: %v", err)
	}

	if err := s.DeleteSessionsOfUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash(ctx, keep.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a session survived signing out everywhere: %v", err)
	}

	// Deleting a User takes their sessions with them.
	_ = s.CreateSession(ctx, sess)
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash(ctx, sess.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a deleted User's session: %v", err)
	}
}

func TestFailuresOtherThanNotFoundPassThrough(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s, _ := open(t)
	cancel()
	e, _ := domain.NewEmail("ada@example.org")
	if _, err := s.UserByEmail(ctx, e); err == nil || errors.Is(err, app.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a cancelled query: %v", err)
	}
}
