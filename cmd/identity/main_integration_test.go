//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
)

// reachable is every required variable, with the test binary's Vault and a database of its own.
func reachable(t *testing.T, url string) map[string]string {
	t.Helper()
	vars := complete()
	vars["DATABASE_URL"] = url
	vars["VAULT_ADDR"], vars["VAULT_TOKEN"] = itest.Vault(t)
	return vars
}

func TestStartWithADatabase(t *testing.T) {
	url := pgtest.URL(t)
	t.Run("invalid address", func(t *testing.T) {
		vars := reachable(t, url)
		vars["ADDR"] = "not-an-address"
		if code := start(t.Context(), env(vars)); code != 1 {
			t.Fatalf("start with an invalid ADDR = %d, want 1", code)
		}
	})
	t.Run("invalid metrics address", func(t *testing.T) {
		vars := reachable(t, url)
		vars["ADDR"], vars["METRICS_ADDR"] = "127.0.0.1:0", "not-an-address"
		if code := start(t.Context(), env(vars)); code != 1 {
			t.Fatalf("start with an invalid METRICS_ADDR = %d, want 1", code)
		}
	})
	t.Run("a Clients file it cannot read", func(t *testing.T) {
		vars := reachable(t, url)
		vars["ADDR"], vars["METRICS_ADDR"], vars["CLIENTS_FILE"] = "127.0.0.1:0", "127.0.0.1:0", "/no/such/clients.yaml"
		if code := start(t.Context(), env(vars)); code != 1 {
			t.Fatalf("start with a missing CLIENTS_FILE = %d, want 1", code)
		}
	})
	t.Run("in a cluster it cannot read", func(t *testing.T) {
		vars := reachable(t, url)
		vars["ADDR"], vars["METRICS_ADDR"] = "127.0.0.1:0", "127.0.0.1:0"
		vars["KUBERNETES_SERVICE_HOST"], vars["KUBERNETES_SERVICE_PORT"] = "10.0.0.1", "443"
		if code := start(t.Context(), env(vars)); code != 1 {
			t.Fatalf("start without a service account = %d, want 1", code)
		}
	})
	t.Run("a Vault it cannot reach", func(t *testing.T) {
		vars := reachable(t, url)
		vars["VAULT_ADDR"], vars["ADDR"], vars["METRICS_ADDR"] = "http://127.0.0.1:1", "127.0.0.1:0", "127.0.0.1:0"
		if code := start(t.Context(), env(vars)); code != 1 {
			t.Fatalf("start without its signing keys = %d, want 1", code)
		}
	})
	t.Run("serves until cancelled", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		vars := reachable(t, url)
		vars["ADDR"], vars["METRICS_ADDR"] = "127.0.0.1:0", "127.0.0.1:0"
		if code := start(ctx, env(vars)); code != 0 {
			t.Fatalf("start after a clean shutdown = %d, want 0", code)
		}
	})
}
