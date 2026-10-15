package httpx_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/httpx"
)

func TestWriteProblem(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.WriteProblem(rec, http.StatusTeapot, "Longer")
	if rec.Code != http.StatusTeapot || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["type"] != "about:blank" || got["title"] != "I'm a teapot" || got["status"] != float64(418) || got["detail"] != "Longer" {
		t.Fatalf("problem = %v", got)
	}
}

func TestProblemOmitsAnEmptyDetail(t *testing.T) {
	if p := httpx.Problem(http.StatusNotFound, ""); p.Detail.Set || p.Title != "Not Found" || p.Status != 404 {
		t.Fatalf("problem = %+v", p)
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.SecurityHeaders(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	for _, h := range []string{"Content-Security-Policy", "X-Content-Type-Options", "Referrer-Policy", "X-Frame-Options", "Strict-Transport-Security"} {
		if rec.Header().Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
}

func TestLimitBodyRefusesADeclaredOversizeAndCapsTheRest(t *testing.T) {
	var read int
	h := httpx.LimitBody(10, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		read = len(b)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))

	declared := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 11)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, declared)
	if rec.Code != http.StatusRequestEntityTooLarge || read != 0 {
		t.Fatalf("a declared 11 bytes = %d, read %d; want 413 before reading", rec.Code, read)
	}

	chunked := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 11)))
	chunked.ContentLength = -1
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, chunked)
	if rec.Code != http.StatusBadRequest || read > 10 {
		t.Fatalf("an undeclared 11 bytes = %d, read %d; want the read stopped at 10", rec.Code, read)
	}

	small := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("ok"))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, small)
	if rec.Code != http.StatusOK || read != 2 {
		t.Fatalf("a small body = %d, read %d", rec.Code, read)
	}
}
