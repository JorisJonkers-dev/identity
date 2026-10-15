package httpapi_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/httpapi"
)

func serve(t *testing.T) *httptest.Server {
	t.Helper()
	h, err := httpapi.New(slog.New(slog.DiscardHandler), "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, srv *httptest.Server, method, path string) (int, http.Header, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("%s %s: body is not JSON: %s", method, path, raw)
		}
	}
	return resp.StatusCode, resp.Header, body
}

func TestVersionNeedsNoSignIn(t *testing.T) {
	code, _, body := call(t, serve(t), http.MethodGet, "/api/v1/version")
	if code != http.StatusOK || body["version"] != "1.2.3" {
		t.Fatalf("GET /api/v1/version = %d %v, want 200 with the version", code, body)
	}
}

func TestProblems(t *testing.T) {
	srv := serve(t)
	cases := []struct {
		name, method, path string
		want               int
	}{
		{"unknown path", http.MethodGet, "/api/v1/nope", http.StatusNotFound},
		{"wrong method", http.MethodDelete, "/api/v1/version", http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, header, body := call(t, srv, tc.method, tc.path)
			if code != tc.want || body["status"] != float64(tc.want) {
				t.Fatalf("%s %s = %d %v, want a %d problem", tc.method, tc.path, code, body, tc.want)
			}
			if ct := header.Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("Content-Type = %q", ct)
			}
		})
	}
}
