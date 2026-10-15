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
		if code := start(t.Context(), env(map[string]string{"DATABASE_URL": url, "ADDR": "not-an-address"})); code != 1 {
			t.Fatalf("start with an invalid ADDR = %d, want 1", code)
		}
	})
	t.Run("serves until cancelled", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		vars := map[string]string{"DATABASE_URL": url, "ADDR": "127.0.0.1:0", "DEV_USER": "dev"}
		if code := start(ctx, env(vars)); code != 0 {
			t.Fatalf("start after a clean shutdown = %d, want 0", code)
		}
	})
}
