package catalog_test

import (
	"slices"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/access/adapters/catalog"
	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

func TestCatalogReadsTheIndex(t *testing.T) {
	x := clients.NewIndex()
	x.Put(signindomain.Client{ID: "grafana", Host: "grafana.jorisjonkers.dev", Label: "Grafana", Roles: []string{"user", "editor"}})
	x.Put(signindomain.Client{ID: "backup", Host: "backup.jorisjonkers.dev", SignIn: signindomain.Token, Roles: []string{"user"}})
	c := catalog.New(x)

	got := c.Clients()
	want := []domain.Client{
		{ID: "backup", Label: "backup", Roles: []string{"user"}},
		{ID: "grafana", Label: "Grafana", Roles: []string{"user", "editor"}},
	}
	if !slices.EqualFunc(got, want, func(a, b domain.Client) bool {
		return a.ID == b.ID && a.Label == b.Label && slices.Equal(a.Roles, b.Roles)
	}) {
		t.Fatalf("Clients = %+v, want %+v", got, want)
	}
	if g, ok := c.Client(t.Context(), "grafana"); !ok || !g.Declares("editor") {
		t.Fatalf("Client(grafana) = %+v, %v", g, ok)
	}
	x.Remove("grafana")
	if _, ok := c.Client(t.Context(), "grafana"); ok {
		t.Fatal("a Removed Client is still found")
	}
}
