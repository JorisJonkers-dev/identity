//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/adapters/persistence"
	"github.com/JorisJonkers-dev/identity/internal/access/app"
	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
)

type fixture struct {
	s   *persistence.Store
	q   *queries.Queries
	ctx context.Context
}

func open(t *testing.T) fixture {
	t.Helper()
	store, err := pg.Open(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return fixture{s: persistence.New(store.Queries(), store), q: store.Queries(), ctx: t.Context()}
}

func (f fixture) user(t *testing.T, email string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := f.q.CreateUser(f.ctx, queries.CreateUserParams{
		ID: id, Email: email, Name: email, PasswordHash: "stored-hash", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f fixture) group(t *testing.T, name string) domain.Group {
	t.Helper()
	g := domain.Group{ID: uuid.New(), Name: name}
	if err := f.s.CreateGroup(f.ctx, g); err != nil {
		t.Fatal(err)
	}
	return g
}

func (f fixture) admins(t *testing.T) domain.Group {
	t.Helper()
	groups, err := f.s.Groups(f.ctx)
	if err != nil || !groups[0].Admins || groups[0].Name != "admins" {
		t.Fatalf("Groups = %+v, %v; want the admins Group first", groups, err)
	}
	return groups[0]
}

func TestGroups(t *testing.T) {
	f := open(t)
	f.admins(t)
	zoo, family := f.group(t, "zoo"), f.group(t, "Family")
	groups, err := f.s.Groups(f.ctx)
	if err != nil || len(groups) != 3 || groups[1] != family || groups[2] != zoo {
		t.Fatalf("Groups = %+v, %v; want admins, Family, zoo", groups, err)
	}
	if err := f.s.CreateGroup(f.ctx, domain.Group{ID: uuid.New(), Name: "FAMILY"}); !errors.Is(err, app.ErrNameTaken) {
		t.Fatalf("a name taken in another case: %v", err)
	}

	kin, _ := domain.NewGroupName("Kin")
	if err := f.s.RenameGroup(f.ctx, family.ID, kin); err != nil {
		t.Fatal(err)
	}
	if g, err := f.s.Group(f.ctx, family.ID); err != nil || g.Name != "Kin" {
		t.Fatalf("Group after rename = %+v, %v", g, err)
	}
	taken, _ := domain.NewGroupName("ZOO")
	if err := f.s.RenameGroup(f.ctx, family.ID, taken); !errors.Is(err, app.ErrNameTaken) {
		t.Fatalf("renaming to a taken name: %v", err)
	}
	if err := f.s.RenameGroup(f.ctx, uuid.New(), kin); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("renaming nothing: %v", err)
	}
	if _, err := f.s.Group(f.ctx, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Group of nothing: %v", err)
	}
}

func TestDeletingAGroupTakesItsMembersAndAssignments(t *testing.T) {
	f := open(t)
	admins, family := f.admins(t), f.group(t, "Family")
	ada := f.user(t, "ada@example.org")
	_ = f.s.AddMember(f.ctx, domain.Membership{GroupID: family.ID, UserID: ada})
	_ = f.s.Assign(f.ctx, domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "user"})

	if err := f.s.DeleteGroup(f.ctx, family.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DeleteGroup(f.ctx, admins.ID); err != nil {
		t.Fatal(err)
	}
	groups, _ := f.s.Groups(f.ctx)
	ms, _ := f.s.Memberships(f.ctx)
	as, _ := f.s.Assignments(f.ctx)
	if len(groups) != 1 || !groups[0].Admins || len(ms) != 0 || len(as) != 0 {
		t.Fatalf("after deleting: groups %+v, memberships %+v, assignments %+v", groups, ms, as)
	}
}

func TestMembers(t *testing.T) {
	f := open(t)
	admins, family := f.admins(t), f.group(t, "Family")
	ada, bob := f.user(t, "ada@example.org"), f.user(t, "bob@example.org")
	m := domain.Membership{GroupID: family.ID, UserID: ada}
	for range 2 {
		if err := f.s.AddMember(f.ctx, m); err != nil {
			t.Fatalf("adding a member twice: %v", err)
		}
	}
	if ms, err := f.s.Memberships(f.ctx); err != nil || len(ms) != 1 || ms[0] != m {
		t.Fatalf("Memberships = %+v, %v", ms, err)
	}
	for _, bad := range []domain.Membership{{GroupID: uuid.New(), UserID: ada}, {GroupID: family.ID, UserID: uuid.New()}} {
		if err := f.s.AddMember(f.ctx, bad); !errors.Is(err, app.ErrNotFound) {
			t.Fatalf("AddMember(%+v) = %v", bad, err)
		}
	}
	if err := f.s.RemoveMember(f.ctx, m); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RemoveMember(f.ctx, domain.Membership{GroupID: uuid.New(), UserID: ada}); err != nil {
		t.Fatalf("removing from no Group: %v", err)
	}

	if yes, _ := f.s.IsAdmin(f.ctx, bob); yes {
		t.Fatal("bob is an Admin before joining")
	}
	_ = f.s.AddMember(f.ctx, domain.Membership{GroupID: admins.ID, UserID: bob})
	if yes, err := f.s.IsAdmin(f.ctx, bob); !yes || err != nil {
		t.Fatalf("IsAdmin(bob) = %v, %v", yes, err)
	}
	last := domain.Membership{GroupID: admins.ID, UserID: bob}
	if err := f.s.RemoveMember(f.ctx, last); !errors.Is(err, app.ErrLastAdmin) {
		t.Fatalf("removing the last Admin: %v", err)
	}
	if yes, _ := f.s.IsAdmin(f.ctx, bob); !yes {
		t.Fatal("the refused removal still removed the last Admin")
	}
}

// Two Admins removing each other at once must leave one Admin, whichever wins.
func TestRacingRemovalsKeepAnAdmin(t *testing.T) {
	f := open(t)
	admins := f.admins(t)
	for i := range 10 {
		a, b := f.user(t, uuid.NewString()+"@example.org"), f.user(t, uuid.NewString()+"@example.org")
		for _, u := range []uuid.UUID{a, b} {
			_ = f.s.AddMember(f.ctx, domain.Membership{GroupID: admins.ID, UserID: u})
		}
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for j, u := range []uuid.UUID{a, b} {
			wg.Go(func() { errs[j] = f.s.RemoveMember(f.ctx, domain.Membership{GroupID: admins.ID, UserID: u}) })
		}
		wg.Wait()
		n, err := f.q.CountMembers(f.ctx, admins.ID)
		if err != nil || n != 1 {
			t.Fatalf("round %d: %d Admins left (%v), errors %v", i, n, err, errs)
		}
		if !errors.Is(errors.Join(errs...), app.ErrLastAdmin) {
			t.Fatalf("round %d: neither removal was refused: %v", i, errs)
		}
		ms, _ := f.s.Memberships(f.ctx)
		for _, m := range ms {
			_ = f.q.RemoveMember(f.ctx, queries.RemoveMemberParams{GroupID: m.GroupID, UserID: m.UserID})
		}
	}
}

func TestRolesAddUpAcrossGroups(t *testing.T) {
	f := open(t)
	viewers, editors := f.group(t, "Viewers"), f.group(t, "Editors")
	ada := f.user(t, "ada@example.org")
	for _, g := range []domain.Group{viewers, editors} {
		_ = f.s.AddMember(f.ctx, domain.Membership{GroupID: g.ID, UserID: ada})
	}
	for _, a := range []domain.Assignment{
		{GroupID: viewers.ID, ClientID: "grafana", Role: "viewer"},
		{GroupID: viewers.ID, ClientID: "grafana", Role: "viewer"},
		{GroupID: editors.ID, ClientID: "grafana", Role: "viewer"},
		{GroupID: editors.ID, ClientID: "grafana", Role: "editor"},
		{GroupID: editors.ID, ClientID: "jellyfin", Role: "user"},
	} {
		if err := f.s.Assign(f.ctx, a); err != nil {
			t.Fatalf("Assign(%+v) = %v", a, err)
		}
	}
	if err := f.s.Assign(f.ctx, domain.Assignment{GroupID: uuid.New(), ClientID: "grafana", Role: "user"}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("assigning to no Group: %v", err)
	}
	roles, err := f.s.RolesOf(f.ctx, ada, "grafana")
	if err != nil || !slices.Equal(roles, []string{"editor", "viewer"}) {
		t.Fatalf("RolesOf = %v, %v", roles, err)
	}
	if as, _ := f.s.Assignments(f.ctx); len(as) != 4 {
		t.Fatalf("Assignments = %+v, want 4", as)
	}
	if err := f.s.Unassign(f.ctx, domain.Assignment{GroupID: editors.ID, ClientID: "grafana", Role: "editor"}); err != nil {
		t.Fatal(err)
	}
	if roles, _ := f.s.RolesOf(f.ctx, ada, "grafana"); !slices.Equal(roles, []string{"viewer"}) {
		t.Fatalf("RolesOf after Unassign = %v", roles)
	}
}

func TestAUsersRolesAreListedPerClientAcrossGroups(t *testing.T) {
	f := open(t)
	viewers, editors := f.group(t, "Viewers"), f.group(t, "Editors")
	ada, grace := f.user(t, "ada@example.org"), f.user(t, "grace@example.org")
	for _, g := range []domain.Group{viewers, editors} {
		_ = f.s.AddMember(f.ctx, domain.Membership{GroupID: g.ID, UserID: ada})
	}
	for _, a := range []domain.Assignment{
		{GroupID: viewers.ID, ClientID: "grafana", Role: "viewer"},
		{GroupID: editors.ID, ClientID: "grafana", Role: "viewer"},
		{GroupID: editors.ID, ClientID: "grafana", Role: "editor"},
		{GroupID: editors.ID, ClientID: "jellyfin", Role: "user"},
	} {
		_ = f.s.Assign(f.ctx, a)
	}
	held, err := f.s.RolesByClient(f.ctx, ada)
	if err != nil || len(held) != 2 || !slices.Equal(held["grafana"], []string{"editor", "viewer"}) || !slices.Equal(held["jellyfin"], []string{"user"}) {
		t.Fatalf("RolesByClient(ada) = %v, %v", held, err)
	}
	if none, err := f.s.RolesByClient(f.ctx, grace); err != nil || len(none) != 0 {
		t.Fatalf("RolesByClient(grace) = %v, %v", none, err)
	}
}

func TestAdminsAndTheUnassigned(t *testing.T) {
	f := open(t)
	admins, family := f.admins(t), f.group(t, "Family")
	if g, err := f.s.AdminsGroup(f.ctx); err != nil || g != admins {
		t.Fatalf("AdminsGroup = %+v, %v", g, err)
	}
	ada, bob, cy := f.user(t, "ada@example.org"), f.user(t, "bob@example.org"), f.user(t, "cy@example.org")
	if n, err := f.s.CountAdmins(f.ctx); n != 0 || err != nil {
		t.Fatalf("CountAdmins on a fresh install = %d, %v", n, err)
	}
	if n, err := f.s.CountUnassigned(f.ctx); n != 3 || err != nil {
		t.Fatalf("CountUnassigned = %d, %v; want all three", n, err)
	}
	_ = f.s.AddMember(f.ctx, domain.Membership{GroupID: admins.ID, UserID: ada})
	_ = f.s.AddMember(f.ctx, domain.Membership{GroupID: family.ID, UserID: bob})
	if n, _ := f.s.CountAdmins(f.ctx); n != 1 {
		t.Fatalf("CountAdmins = %d, want 1", n)
	}
	if n, _ := f.s.CountUnassigned(f.ctx); n != 1 {
		t.Fatalf("CountUnassigned = %d, want only %v", n, cy)
	}
}
