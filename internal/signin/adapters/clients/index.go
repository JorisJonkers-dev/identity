// Package clients is where the sign-in context finds Clients: an in-memory Index by id and by host,
// kept current from the cluster's Client objects (docs/adr/0002, deploy-kit chapter 17).
package clients

import (
	"context"
	"sync"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// Index is the app.Clients: every Client the cluster declares, by id and by host. A Client removed
// from the cluster is a Removed Client: it is found by neither, and found again once it returns.
// Its Assignments are kept elsewhere, by id, so returning restores its access.
type Index struct {
	mu     sync.RWMutex
	byID   map[string]domain.Client
	byHost map[string]string
}

// NewIndex returns an empty Index.
func NewIndex() *Index {
	return &Index{byID: map[string]domain.Client{}, byHost: map[string]string{}}
}

var _ app.Clients = (*Index)(nil)

// Put adds or replaces a Client.
func (x *Index) Put(c domain.Client) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.remove(c.ID)
	x.byID[c.ID] = c
	x.byHost[c.Host] = c.ID
}

// Remove makes a Client a Removed Client.
func (x *Index) Remove(id string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.remove(id)
}

func (x *Index) remove(id string) {
	old, ok := x.byID[id]
	if !ok {
		return
	}
	delete(x.byID, id)
	if x.byHost[old.Host] == id {
		delete(x.byHost, old.Host)
	}
}

func (x *Index) Client(_ context.Context, id string) (domain.Client, error) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	c, ok := x.byID[id]
	if !ok {
		return domain.Client{}, app.ErrNotFound
	}
	return c, nil
}

func (x *Index) ClientForHost(ctx context.Context, host string) (domain.Client, error) {
	x.mu.RLock()
	id, ok := x.byHost[host]
	x.mu.RUnlock()
	if !ok {
		return domain.Client{}, app.ErrNotFound
	}
	return x.Client(ctx, id)
}
