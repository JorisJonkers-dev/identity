//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
)

func TestStartWithADatabase(t *testing.T) {
	url := pgtest.URL(t)
	t.Run("invalid address", func(t *testing.T) {
		vars := complete()
		vars["DATABASE_URL"], vars["ADDR"] = url, "not-an-address"
		if code := start(t.Context(), env(vars)); code != 1 {
			t.Fatalf("start with an invalid ADDR = %d, want 1", code)
		}
	})
	t.Run("serves until cancelled", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		vars := complete()
		vars["DATABASE_URL"], vars["ADDR"] = url, "127.0.0.1:0"
		if code := start(ctx, env(vars)); code != 0 {
			t.Fatalf("start after a clean shutdown = %d, want 0", code)
		}
	})
}
