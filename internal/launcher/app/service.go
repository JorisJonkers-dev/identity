// Package app holds the Launcher's use cases: the apps a User can open, and the icons identity
// serves for them from its own origin.
package app

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/launcher/domain"
)

// How long a fetched icon is served before it is fetched again, and how long a failed fetch is
// remembered so a broken Client is not asked on every page load.
const (
	iconTTL   = 24 * time.Hour
	failedTTL = 10 * time.Minute
)

// ErrNoIcon is a Client with no icon to serve: unknown, without an icon URL, or failing to fetch.
var ErrNoIcon = errors.New("no icon")

// Roles is the App Roles a User holds, per Client.
type Roles interface {
	RolesByClient(ctx context.Context, userID uuid.UUID) (map[string][]string, error)
}

// Clients is the Clients a person can open, as the cluster declares them now.
type Clients interface {
	Launchable() []domain.Client
	Client(id string) (domain.Client, bool)
}

// Fetcher fetches an icon from a Client's host.
type Fetcher interface {
	Fetch(ctx context.Context, url string) (domain.Icon, error)
}

type cached struct {
	url   string
	icon  domain.Icon
	err   error
	until time.Time
}

// Service is the Launcher's use cases.
type Service struct {
	roles   Roles
	clients Clients
	fetcher Fetcher
	now     func() time.Time

	mu    sync.Mutex
	icons map[string]cached
}

// New returns a Service.
func New(roles Roles, clients Clients, fetcher Fetcher, now func() time.Time) *Service {
	return &Service{roles: roles, clients: clients, fetcher: fetcher, now: now, icons: map[string]cached{}}
}

// Apps is the Clients a User holds an App Role on, by label.
func (s *Service) Apps(ctx context.Context, userID uuid.UUID) ([]domain.App, error) {
	held, err := s.roles.RolesByClient(ctx, userID)
	if err != nil {
		return nil, err
	}
	var apps []domain.App
	for _, c := range s.clients.Launchable() {
		if roles := held[c.ID]; len(roles) > 0 {
			apps = append(apps, domain.App{Client: c, Roles: roles})
		}
	}
	slices.SortFunc(apps, func(a, b domain.App) int { return cmp.Compare(a.Label, b.Label) })
	return apps, nil
}

// Icon is a Client's icon, fetched from its icon URL and kept for a day. A failed fetch is
// ErrNoIcon joined with its cause.
func (s *Service) Icon(ctx context.Context, clientID string) (domain.Icon, error) {
	c, ok := s.clients.Client(clientID)
	if !ok || c.IconURL == "" {
		return domain.Icon{}, ErrNoIcon
	}
	now := s.now()
	s.mu.Lock()
	hit, found := s.icons[clientID]
	s.mu.Unlock()
	if found && hit.url == c.IconURL && now.Before(hit.until) {
		return hit.icon, hit.err
	}
	icon, err := s.fetcher.Fetch(ctx, c.IconURL)
	entry := cached{url: c.IconURL, icon: icon, until: now.Add(iconTTL)}
	if err != nil {
		entry = cached{url: c.IconURL, err: errors.Join(ErrNoIcon, err), until: now.Add(failedTTL)}
	}
	s.mu.Lock()
	s.icons[clientID] = entry
	s.mu.Unlock()
	return entry.icon, entry.err
}
