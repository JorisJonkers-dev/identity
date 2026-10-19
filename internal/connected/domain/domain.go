// Package domain is the connected context's model (docs/adr/0008, identity#23): a tool that
// registered itself at runtime, such as an MCP client, and the Connected App a User's consent
// makes of it, acting for that User on one Client.
package domain

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	// ErrInvalidRegistration is a registration whose name or redirect URIs break the rules.
	ErrInvalidRegistration = errors.New("a registration names itself in 1 to 100 characters and gives 1 to 10 redirect URIs, each loopback http or https")
)

const (
	maxName         = 100
	maxRedirectURIs = 10
	// unnamed is what a registration that gives no name is called.
	unnamed = "An unnamed app"
)

// Registration is a client that registered itself (RFC 7591). It holds nothing until a signed-in
// User consents.
type Registration struct {
	ClientID     string
	Name         string
	RedirectURIs []string
	CreatedAt    time.Time
}

// NewRegistration is a registration with a trimmed name of up to 100 characters, an unnamed one
// called so, and 1 to 10 redirect URIs. A redirect URI is https, or http to a loopback address
// on any port, which is where a tool on the User's own machine listens (RFC 8252).
func NewRegistration(clientID, name string, redirectURIs []string, at time.Time) (Registration, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = unnamed
	}
	if utf8.RuneCountInString(name) > maxName || len(redirectURIs) == 0 || len(redirectURIs) > maxRedirectURIs {
		return Registration{}, ErrInvalidRegistration
	}
	for _, raw := range redirectURIs {
		if !allowedRedirect(raw) {
			return Registration{}, ErrInvalidRegistration
		}
	}
	return Registration{ClientID: clientID, Name: name, RedirectURIs: redirectURIs, CreatedAt: at}, nil
}

func allowedRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.User != nil || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		host := u.Hostname()
		ip := net.ParseIP(host)
		return host == "localhost" || ip != nil && ip.IsLoopback()
	}
	return false
}

// ConnectedApp is a Registration a User consented to, acting for them on one Client: never beyond
// the App Roles they hold there.
type ConnectedApp struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	ClientID string
	// Name is what the app called itself when it registered.
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}
