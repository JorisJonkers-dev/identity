package httpx_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/httpx"
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

func TestDevIdentityFillsOnlyAMissingIdentity(t *testing.T) {
	var seen string
	h := httpx.DevIdentity("dev", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(httpx.IdentityHeader)
	}))
	for given, want := range map[string]string{"": "dev", "someone": "someone"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		if given != "" {
			req.Header.Set(httpx.IdentityHeader, given)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		if seen != want {
			t.Fatalf("identity %q became %q, want %q", given, seen, want)
		}
	}
}
