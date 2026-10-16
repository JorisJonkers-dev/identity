package metrics_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/metrics"
)

func scrape(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	return string(body)
}

func TestTheUnassignedGaugeIsReadAtScrapeTime(t *testing.T) {
	n := 0
	h := metrics.Handler(func(ctx context.Context) (int, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("the count has no deadline")
		}
		n++
		return n, nil
	}, slog.New(slog.DiscardHandler))
	if got := scrape(t, h); !strings.Contains(got, "\nidentity_unassigned_users 1\n") {
		t.Fatalf("first scrape:\n%s", got)
	}
	if got := scrape(t, h); !strings.Contains(got, "\nidentity_unassigned_users 2\n") || !strings.Contains(got, "go_goroutines") {
		t.Fatalf("second scrape:\n%s", got)
	}
}

func TestAFailedCountLeavesTheGaugeOutAndIsLogged(t *testing.T) {
	var logs strings.Builder
	h := metrics.Handler(func(context.Context) (int, error) { return 0, errors.New("db down") }, slog.New(slog.NewTextHandler(&logs, nil)))
	if got := scrape(t, h); strings.Contains(got, "\nidentity_unassigned_users ") {
		t.Fatalf("a failed count was reported:\n%s", got)
	}
	if !strings.Contains(logs.String(), "db down") {
		t.Fatalf("logs = %q", logs.String())
	}
}
