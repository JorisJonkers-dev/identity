package pg_test

import (
	"context"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
)

const unreachable = "postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"

func TestOpeningAnUnreachableOrMalformedDatabaseFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	for _, url := range []string{unreachable, "::not a url::"} {
		if _, err := pg.Open(ctx, url); err == nil {
			t.Fatalf("Open(%q) succeeded", url)
		}
	}
}
