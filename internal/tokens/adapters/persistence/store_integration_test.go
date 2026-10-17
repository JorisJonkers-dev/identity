//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/identity/internal/tokens/adapters/persistence"
	"github.com/JorisJonkers-dev/identity/internal/tokens/app"
	"github.com/JorisJonkers-dev/identity/internal/tokens/domain"
)

var t0 = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

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
	return fixture{s: persistence.New(store.Queries()), q: store.Queries(), ctx: t.Context()}
}

// token keeps a token of a new User, expiring at expires, under hash.
func (f fixture) token(t *testing.T, hash string, expires time.Time) domain.Token {
	t.Helper()
	user := uuid.New()
	if err := f.q.CreateUser(f.ctx, queries.CreateUserParams{ID: user, Email: user.String() + "@example.org", Name: "Ada", PasswordHash: "h", CreatedAt: t0}); err != nil {
		t.Fatal(err)
	}
	tk := domain.Token{ID: uuid.New(), UserID: user, ClientID: "grafana", Name: "cli", CreatedAt: t0, ExpiresAt: expires}
	if err := f.s.Create(f.ctx, tk, []byte(hash)); err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestATokenIsFoundByItsHashAndItsID(t *testing.T) {
	t.Parallel()
	f := open(t)
	tk := f.token(t, "hash-1", t0.Add(time.Hour))
	byHash, err := f.s.ByHash(f.ctx, []byte("hash-1"))
	if err != nil || byHash.ID != tk.ID || byHash.UserID != tk.UserID {
		t.Fatalf("ByHash = %+v, %v", byHash, err)
	}
	byID, err := f.s.ByID(f.ctx, tk.ID)
	if err != nil || byID.Name != "cli" || !byID.ExpiresAt.Equal(tk.ExpiresAt) || byID.RevokedAt != nil {
		t.Fatalf("ByID = %+v, %v", byID, err)
	}
}

func TestAMissingTokenIsNotFound(t *testing.T) {
	t.Parallel()
	f := open(t)
	if _, err := f.s.ByHash(f.ctx, []byte("none")); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("ByHash = %v", err)
	}
	if _, err := f.s.ByID(f.ctx, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("ByID = %v", err)
	}
}

func TestATokenIsRevokedOnceAndOnlyByItsUser(t *testing.T) {
	t.Parallel()
	f := open(t)
	tk := f.token(t, "hash-2", t0.Add(time.Hour))
	other, _ := f.s.Revoke(f.ctx, tk.ID, uuid.New(), t0)
	first, _ := f.s.Revoke(f.ctx, tk.ID, tk.UserID, t0)
	again, err := f.s.Revoke(f.ctx, tk.ID, tk.UserID, t0.Add(time.Minute))
	got, _ := f.s.ByID(f.ctx, tk.ID)
	if other || !first || again || err != nil || got.RevokedAt == nil || !got.RevokedAt.Equal(t0) {
		t.Fatalf("by another %v, first %v, again %v, %v; revoked at %v", other, first, again, err, got.RevokedAt)
	}
}

func TestALastUseIsWrittenAtMostOnceAMinute(t *testing.T) {
	t.Parallel()
	f := open(t)
	tk := f.token(t, "hash-3", t0.Add(time.Hour))
	for _, at := range []time.Time{t0, t0.Add(30 * time.Second), t0.Add(2 * time.Minute)} {
		if err := f.s.Touch(f.ctx, tk.ID, at); err != nil {
			t.Fatal(err)
		}
		got, _ := f.s.ByID(f.ctx, tk.ID)
		if want := map[bool]time.Time{true: t0, false: t0.Add(2 * time.Minute)}[at.Before(t0.Add(time.Minute))]; !got.LastUsedAt.Equal(want) {
			t.Fatalf("after a use at %v, last use %v, want %v", at, got.LastUsedAt, want)
		}
	}
}

func TestAUsersTokensAreListedNewestFirst(t *testing.T) {
	t.Parallel()
	f := open(t)
	older := f.token(t, "hash-4", t0.Add(time.Hour))
	newer := domain.Token{ID: uuid.New(), UserID: older.UserID, ClientID: "wiki", Name: "wiki", CreatedAt: t0.Add(time.Minute), ExpiresAt: t0.Add(time.Hour)}
	if err := f.s.Create(f.ctx, newer, []byte("hash-5")); err != nil {
		t.Fatal(err)
	}
	got, err := f.s.OfUser(f.ctx, older.UserID)
	if err != nil || len(got) != 2 || got[0].ID != newer.ID || got[1].ID != older.ID {
		t.Fatalf("OfUser = %+v, %v", got, err)
	}
}

func TestOnlyLiveTokensEndingInTheWindowCount(t *testing.T) {
	t.Parallel()
	f := open(t)
	before, after := t0.Add(7*24*time.Hour), t0
	f.token(t, "in-window", t0.Add(24*time.Hour))
	f.token(t, "at-the-edge", before)
	f.token(t, "already-ended", t0)
	f.token(t, "later", before.Add(time.Second))
	revoked := f.token(t, "revoked", t0.Add(24*time.Hour))
	_, _ = f.s.Revoke(f.ctx, revoked.ID, revoked.UserID, t0)
	if n, err := f.s.CountExpiring(f.ctx, after, before); err != nil || n != 2 {
		t.Fatalf("CountExpiring = %d, %v", n, err)
	}
}
