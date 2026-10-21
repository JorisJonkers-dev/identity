//go:build integration

package itest

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Logouts is a native Client's back-channel logout endpoint: it keeps the claims of each logout
// token identity posts to it.
type Logouts struct {
	srv *httptest.Server
	got chan map[string]any
}

// NewLogouts serves a back-channel logout endpoint for t's length.
func NewLogouts(t *testing.T) *Logouts {
	t.Helper()
	l := &Logouts{got: make(chan map[string]any, 16)}
	l.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.PostFormValue("logout_token"), ".")
		if len(parts) != 3 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
		var claims map[string]any
		_ = json.Unmarshal(payload, &claims)
		l.got <- claims
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(l.srv.Close)
	return l
}

// URI is where identity posts logout tokens.
func (l *Logouts) URI() string { return l.srv.URL + "/back-channel-logout" }

// Next is the claims of the next logout token identity posts, failing t when none comes within
// five seconds.
func (l *Logouts) Next(t *testing.T) map[string]any {
	t.Helper()
	select {
	case claims := <-l.got:
		return claims
	case <-time.After(5 * time.Second):
		t.Fatal("no logout token came")
		return nil
	}
}

// None fails t when identity posts a logout token within wait.
func (l *Logouts) None(t *testing.T, wait time.Duration) {
	t.Helper()
	select {
	case claims := <-l.got:
		t.Fatalf("a logout token came: %v", claims)
	case <-time.After(wait):
	}
}
