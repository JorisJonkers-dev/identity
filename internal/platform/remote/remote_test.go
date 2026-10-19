package remote_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/remote"
)

func TestTheClientIsTheAddressTraefikAddedLast(t *testing.T) {
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.7")
	if got := remote.Address(r); got != "198.51.100.7" {
		t.Fatalf("Address = %q", got)
	}
}

func TestWithoutAProxyTheClientIsTheConnection(t *testing.T) {
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.4:51234"
	if got := remote.Address(r); got != "192.0.2.4" {
		t.Fatalf("Address = %q", got)
	}
}

func TestAConnectionAddressWithoutAPortIsTakenWhole(t *testing.T) {
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.RemoteAddr = "pipe"
	if got := remote.Address(r); got != "pipe" {
		t.Fatalf("Address = %q", got)
	}
}

func TestTheMiddlewareNamesTheRequestsClient(t *testing.T) {
	var got string
	h := remote.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = remote.From(r.Context()) }))
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.4:51234"
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got != "192.0.2.4" {
		t.Fatalf("From = %q", got)
	}
}

func TestAContextWithoutAClientNamesNone(t *testing.T) {
	if got := remote.From(context.Background()); got != "" {
		t.Fatalf("From = %q", got)
	}
}
