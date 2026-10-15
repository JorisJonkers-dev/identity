package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/template-go-vue/internal/notes/app"
	"github.com/JorisJonkers-dev/template-go-vue/internal/notes/domain"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/httpapi"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/httpx"
)

// memory is an in-memory domain.Repository, so these tests exercise the contract, not Postgres.
type memory struct {
	notes []domain.Note
	err   error
}

func (m *memory) Create(_ context.Context, text domain.Text) (domain.Note, error) {
	if m.err != nil {
		return domain.Note{}, m.err
	}
	n := domain.Note{ID: uuid.New(), Text: text, CreatedAt: time.Now().UTC()}
	m.notes = append([]domain.Note{n}, m.notes...)
	return n, nil
}

func (m *memory) List(_ context.Context, limit int) ([]domain.Note, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.notes[:min(limit, len(m.notes))], nil
}

func serve(t *testing.T, repo *memory) *httptest.Server {
	t.Helper()
	h, err := httpapi.New(slog.New(slog.DiscardHandler), app.New(repo))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

type response struct {
	code   int
	header http.Header
	body   map[string]any
}

func call(t *testing.T, srv *httptest.Server, method, path, body string, identified bool) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if identified {
		req.Header.Set(httpx.IdentityHeader, "tester")
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
	out := response{code: resp.StatusCode, header: resp.Header, body: map[string]any{}}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out.body); err != nil {
			t.Fatalf("%s %s: body is not JSON: %s", method, path, raw)
		}
	}
	return out
}

func TestCreateThenList(t *testing.T) {
	srv := serve(t, &memory{})
	created := call(t, srv, http.MethodPost, "/api/v1/notes", `{"text":"  Buy milk "}`, true)
	if created.code != http.StatusCreated || created.body["text"] != "Buy milk" || created.body["id"] == "" {
		t.Fatalf("create = %d %v", created.code, created.body)
	}
	listed := call(t, srv, http.MethodGet, "/api/v1/notes?limit=10", "", true)
	items, _ := listed.body["items"].([]any)
	if listed.code != http.StatusOK || len(items) != 1 {
		t.Fatalf("list = %d %v", listed.code, listed.body)
	}
}

func TestProblems(t *testing.T) {
	srv := serve(t, &memory{})
	cases := []struct {
		name, method, path, body string
		identified               bool
		want                     int
	}{
		{"text the domain refuses", http.MethodPost, "/api/v1/notes", `{"text":"   "}`, true, http.StatusUnprocessableEntity},
		{"text the contract refuses", http.MethodPost, "/api/v1/notes", `{"text":"` + strings.Repeat("x", 501) + `"}`, true, http.StatusBadRequest},
		{"unknown field", http.MethodPost, "/api/v1/notes", `{"text":"x","extra":1}`, true, http.StatusBadRequest},
		{"limit out of range", http.MethodGet, "/api/v1/notes?limit=0", "", true, http.StatusBadRequest},
		{"no identity", http.MethodGet, "/api/v1/notes", "", false, http.StatusUnauthorized},
		{"unknown path", http.MethodGet, "/api/v1/nope", "", true, http.StatusNotFound},
		{"wrong method", http.MethodDelete, "/api/v1/notes", "", true, http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := call(t, srv, tc.method, tc.path, tc.body, tc.identified)
			if got.code != tc.want || got.body["status"] != float64(tc.want) {
				t.Fatalf("%s %s = %d %v, want a %d problem", tc.method, tc.path, got.code, got.body, tc.want)
			}
			if ct := got.header.Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("Content-Type = %q", ct)
			}
		})
	}
}

func TestFailureIsAnOpaque500(t *testing.T) {
	srv := serve(t, &memory{err: errors.New("connection refused to 10.0.0.7")})
	for _, req := range []struct{ method, body string }{{http.MethodGet, ""}, {http.MethodPost, `{"text":"x"}`}} {
		got := call(t, srv, req.method, "/api/v1/notes", req.body, true)
		if got.code != http.StatusInternalServerError || got.body["detail"] != nil {
			t.Fatalf("%s = %d %v, want a 500 without detail", req.method, got.code, got.body)
		}
	}
}
