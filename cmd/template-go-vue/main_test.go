package main

import (
	"context"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/pg/pgtest"
)

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

const unreachable = "postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"

func TestStartRefusesABadEnvironment(t *testing.T) {
	cases := map[string]map[string]string{
		"no database":          {},
		"unreachable database": {"DATABASE_URL": unreachable},
	}
	for name, vars := range cases {
		t.Run(name, func(t *testing.T) {
			if code := start(t.Context(), env(vars)); code != 1 {
				t.Fatalf("start = %d, want 1", code)
			}
		})
	}
}

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
