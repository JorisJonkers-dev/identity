package domain_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/connected/domain"
)

var at = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func TestARegistrationKeepsItsTrimmedNameAndRedirectURIs(t *testing.T) {
	r, err := domain.NewRegistration("mcp-1", "  Claude Code ", []string{"http://localhost:33418/callback"}, at)
	if err != nil {
		t.Fatal(err)
	}
	if r.ClientID != "mcp-1" || r.Name != "Claude Code" || !slices.Equal(r.RedirectURIs, []string{"http://localhost:33418/callback"}) || !r.CreatedAt.Equal(at) {
		t.Fatalf("registration %+v", r)
	}
}

func TestARegistrationWithNoNameIsCalledUnnamed(t *testing.T) {
	r, _ := domain.NewRegistration("mcp-1", " ", []string{"https://app.example.org/cb"}, at)
	if r.Name != "An unnamed app" {
		t.Fatalf("name %q", r.Name)
	}
}

func TestARegistrationRedirectsOnlyToLoopbackHTTPOrHTTPS(t *testing.T) {
	for _, c := range []struct {
		uri string
		ok  bool
	}{
		{"http://localhost:8080/cb", true},
		{"http://127.0.0.1:49152/callback", true},
		{"http://[::1]:3000/cb", true},
		{"https://app.example.org/oauth/callback", true},
		{"http://app.example.org/cb", false},
		{"http://10.0.0.5/cb", false},
		{"https://app.example.org/cb#fragment", false},
		{"https://user@app.example.org/cb", false},
		{"myapp://callback", false},
		{"https:///cb", false},
		{"://not a url", false},
	} {
		t.Run(c.uri, func(t *testing.T) {
			_, err := domain.NewRegistration("mcp-1", "App", []string{c.uri}, at)
			if (err == nil) != c.ok {
				t.Fatalf("err = %v, want accepted %v", err, c.ok)
			}
		})
	}
}

func TestARegistrationGivesOneToTenRedirectURIsAndAShortName(t *testing.T) {
	eleven := slices.Repeat([]string{"https://app.example.org/cb"}, 11)
	for _, c := range []struct {
		name string
		app  string
		uris []string
	}{
		{"no redirect URI", "App", nil},
		{"eleven redirect URIs", "App", eleven},
		{"a name of 101 characters", strings.Repeat("a", 101), []string{"https://app.example.org/cb"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := domain.NewRegistration("mcp-1", c.app, c.uris, at); !errors.Is(err, domain.ErrInvalidRegistration) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
