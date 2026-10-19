// Package domain is the legacy context's model: a person as Legacy Auth keeps them, read once to
// import them into identity (docs/adr/0006).
package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Account is one person in Legacy Auth.
type Account struct {
	ID                  uuid.UUID
	Username, Email     string
	FirstName, LastName string
	PasswordHash        string
	// TOTPSecret is the base32 secret of an authenticator app they finished setting up, or empty.
	TOTPSecret string
	Admin      bool
	Confirmed  bool
	// Services are the SERVICE_* grants, such as GRAFANA.
	Services  []string
	CreatedAt time.Time
}

// FullName is the name Legacy Auth shows, or the username when no name was given.
func (a Account) FullName() string {
	if name := strings.TrimSpace(a.FirstName + " " + a.LastName); name != "" {
		return name
	}
	return a.Username
}

// ClientFor is the Client a service's grant becomes: the one named in clients, or by default the
// service in lower case with hyphens, so KNOWLEDGE_API becomes knowledge-api.
func ClientFor(service string, clients map[string]string) string {
	if id, ok := clients[service]; ok {
		return id
	}
	return strings.ReplaceAll(strings.ToLower(service), "_", "-")
}
