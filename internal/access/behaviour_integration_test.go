//go:build integration

// Package access_test checks the access context's behaviours the way a browser reaches them: over
// HTTP, against the real routes, Postgres, Vault and mail (docs/testing-standards.md).
package access_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
)

type group struct {
	ID      string
	Members []string
}

type overview struct{ Groups []group }

func TestAnAdminAddsAPersonToAGroup(t *testing.T) {
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	env.SignedUp(t, "ada@example.org")
	ada := admin.UserID(t, "ada@example.org")

	var family group
	admin.Call(t, http.MethodPost, "/api/v1/access/groups", map[string]string{"name": "Family"}, http.StatusCreated, &family)
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+family.ID+"/members/"+ada, nil, http.StatusNoContent, nil)

	var after overview
	admin.Call(t, http.MethodGet, "/api/v1/access", nil, http.StatusOK, &after)
	i := slices.IndexFunc(after.Groups, func(g group) bool { return g.ID == family.ID })
	if i < 0 || !slices.Contains(after.Groups[i].Members, ada) {
		t.Fatalf("Groups = %+v, want Ada in Family", after.Groups)
	}
}
