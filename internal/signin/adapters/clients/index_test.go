package clients_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

func TestAClientIsFoundByIDAndHostUntilRemoved(t *testing.T) {
	ctx := context.Background()
	x := clients.NewIndex()
	x.Put(domain.Client{ID: "grafana", Host: "grafana.jorisjonkers.dev", Label: "Grafana"})
	if c, err := x.ClientForHost(ctx, "grafana.jorisjonkers.dev"); err != nil || c.ID != "grafana" {
		t.Fatalf("ClientForHost = %+v, %v", c, err)
	}

	// A changed host moves the Client; the old host has none.
	x.Put(domain.Client{ID: "grafana", Host: "metrics.jorisjonkers.dev", Label: "Grafana"})
	if _, err := x.ClientForHost(ctx, "grafana.jorisjonkers.dev"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("the old host: %v", err)
	}
	if c, err := x.ClientForHost(ctx, "metrics.jorisjonkers.dev"); err != nil || c.ID != "grafana" {
		t.Fatalf("the new host: %+v, %v", c, err)
	}

	x.Remove("grafana")
	x.Remove("grafana")
	if _, err := x.Client(ctx, "grafana"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a Removed Client by id: %v", err)
	}
	if _, err := x.ClientForHost(ctx, "metrics.jorisjonkers.dev"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a Removed Client by host: %v", err)
	}
	x.Put(domain.Client{ID: "grafana", Host: "metrics.jorisjonkers.dev"})
	if _, err := x.Client(ctx, "grafana"); err != nil {
		t.Fatalf("a returned Client: %v", err)
	}
}

func TestRemovingAClientLeavesAHostAnotherNowHolds(t *testing.T) {
	ctx := context.Background()
	x := clients.NewIndex()
	x.Put(domain.Client{ID: "old", Host: "app.jorisjonkers.dev"})
	x.Put(domain.Client{ID: "new", Host: "app.jorisjonkers.dev"})
	x.Remove("old")
	if c, err := x.ClientForHost(ctx, "app.jorisjonkers.dev"); err != nil || c.ID != "new" {
		t.Fatalf("ClientForHost = %+v, %v", c, err)
	}
}

func TestAllListsThePresentClientsByLabel(t *testing.T) {
	x := clients.NewIndex()
	x.Put(domain.Client{ID: "b", Host: "b.jorisjonkers.dev", Label: "Vault"})
	x.Put(domain.Client{ID: "a", Host: "a.jorisjonkers.dev", Label: "grafana"})
	x.Put(domain.Client{ID: "d", Host: "d.jorisjonkers.dev"})
	x.Put(domain.Client{ID: "c", Host: "c.jorisjonkers.dev"})
	x.Put(domain.Client{ID: "gone", Host: "gone.jorisjonkers.dev", Label: "Gone"})
	x.Remove("gone")
	var ids []string
	for _, c := range x.All() {
		ids = append(ids, c.ID)
	}
	if want := []string{"c", "d", "a", "b"}; !slices.Equal(ids, want) {
		t.Fatalf("All = %v, want %v", ids, want)
	}
}
