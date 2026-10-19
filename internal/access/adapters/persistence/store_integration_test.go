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

func TestGroupsAreListedAdminsFirstThenByName(t *testing.T) {
	f := open(t)
	f.admins(t)
	zoo, family := f.group(t, "zoo"), f.group(t, "Family")
	groups, err := f.s.Groups(f.ctx)
	if err != nil || len(groups) != 3 || groups[1] != family || groups[2] != zoo {
		t.Fatalf("Groups = %+v, %v; want admins, Family, zoo", groups, err)
	}
}

func TestAGroupNameIsTakenWhateverItsCase(t *testing.T) {
	f := open(t)
	f.group(t, "Family")
	if err := f.s.CreateGroup(f.ctx, domain.Group{ID: uuid.New(), Name: "FAMILY"}); !errors.Is(err, app.ErrNameTaken) {
		t.Fatalf("err = %v, want ErrNameTaken", err)
	}
}

func TestARenamedGroupReadsBackWithItsNewName(t *testing.T) {
	f := open(t)
	family := f.group(t, "Family")
	kin, _ := domain.NewGroupName("Kin")
	if err := f.s.RenameGroup(f.ctx, family.ID, kin); err != nil {
		t.Fatal(err)
	}
	if g, err := f.s.Group(f.ctx, family.ID); err != nil || g.Name != "Kin" {
		t.Fatalf("Group = %+v, %v", g, err)
	}
}

func TestAGroupCannotBeRenamedToATakenName(t *testing.T) {
	f := open(t)
	family := f.group(t, "Family")
	f.group(t, "zoo")
	taken, _ := domain.NewGroupName("ZOO")
	if err := f.s.RenameGroup(f.ctx, family.ID, taken); !errors.Is(err, app.ErrNameTaken) {
		t.Fatalf("err = %v, want ErrNameTaken", err)
	}
}

func TestRenamingAGroupThatIsGoneIsNotFound(t *testing.T) {
	f := open(t)
	kin, _ := domain.NewGroupName("Kin")
	if err := f.s.RenameGroup(f.ctx, uuid.New(), kin); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestReadingAGroupThatIsGoneIsNotFound(t *testing.T) {
	f := open(t)
	if _, err := f.s.Group(f.ctx, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeletingAGroupTakesItsMembersAndAssignments(t *testing.T) {
	f := open(t)
	family := f.group(t, "Family")
	_, _ = f.s.AddMember(f.ctx, domain.Membership{GroupID: family.ID, UserID: f.user(t, "ada@example.org")})
	_, _ = f.s.Assign(f.ctx, domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "user"})
	if err := f.s.DeleteGroup(f.ctx, family.ID); err != nil {
		t.Fatal(err)
	}
	ms, _ := f.s.Memberships(f.ctx)
	as, _ := f.s.Assignments(f.ctx)
	if len(ms) != 0 || len(as) != 0 {
		t.Fatalf("left behind: memberships %+v, assignments %+v", ms, as)
	}
}

func TestTheAdminsGroupIsNeverDeleted(t *testing.T) {
	f := open(t)
	admins := f.admins(t)
	if err := f.s.DeleteGroup(f.ctx, admins.ID); err != nil {
		t.Fatal(err)
	}
	if groups, _ := f.s.Groups(f.ctx); len(groups) != 1 || !groups[0].Admins {
		t.Fatalf("Groups = %+v, want the admins Group still there", groups)
	}
}

func TestAddingAMemberTwiceKeepsOneMembership(t *testing.T) {
	f := open(t)
	m := domain.Membership{GroupID: f.group(t, "Family").ID, UserID: f.user(t, "ada@example.org")}
	for range 2 {
		if _, err := f.s.AddMember(f.ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	if ms, err := f.s.Memberships(f.ctx); err != nil || len(ms) != 1 || ms[0] != m {
		t.Fatalf("Memberships = %+v, %v", ms, err)
	}
}

func TestAddingAMemberToAGroupOrUserThatIsGoneIsNotFound(t *testing.T) {
	f := open(t)
	family, ada := f.group(t, "Family"), f.user(t, "ada@example.org")
	for name, m := range map[string]domain.Membership{
		"a Group that is gone": {GroupID: uuid.New(), UserID: ada},
		"a User who is gone":   {GroupID: family.ID, UserID: uuid.New()},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := f.s.AddMember(f.ctx, m); !errors.Is(err, app.ErrNotFound) {
				t.Fatalf("err = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestARemovedMemberIsNoLongerListed(t *testing.T) {
	f := open(t)
	m := domain.Membership{GroupID: f.group(t, "Family").ID, UserID: f.user(t, "ada@example.org")}
	_, _ = f.s.AddMember(f.ctx, m)
	if _, err := f.s.RemoveMember(f.ctx, m); err != nil {
		t.Fatal(err)
	}
	if ms, _ := f.s.Memberships(f.ctx); len(ms) != 0 {
		t.Fatalf("Memberships = %+v", ms)
	}
}

func TestRemovingAMembershipThatDoesNotExistChangesNothing(t *testing.T) {
	f := open(t)
	if _, err := f.s.RemoveMember(f.ctx, domain.Membership{GroupID: uuid.New(), UserID: f.user(t, "ada@example.org")}); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestAUserIsAnAdminOnlyOnceInTheAdminsGroup(t *testing.T) {
	f := open(t)
	admins, bob := f.admins(t), f.user(t, "bob@example.org")
	if yes, _ := f.s.IsAdmin(f.ctx, bob); yes {
		t.Fatal("an Admin before joining")
	}
	_, _ = f.s.AddMember(f.ctx, domain.Membership{GroupID: admins.ID, UserID: bob})
	if yes, err := f.s.IsAdmin(f.ctx, bob); !yes || err != nil {
		t.Fatalf("IsAdmin = %v, %v", yes, err)
	}
}

func TestTheLastAdminIsNotRemoved(t *testing.T) {
	f := open(t)
	admins, bob := f.admins(t), f.user(t, "bob@example.org")
	last := domain.Membership{GroupID: admins.ID, UserID: bob}
	_, _ = f.s.AddMember(f.ctx, last)
	if _, err := f.s.RemoveMember(f.ctx, last); !errors.Is(err, app.ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
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
			_, _ = f.s.AddMember(f.ctx, domain.Membership{GroupID: admins.ID, UserID: u})
		}
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for j, u := range []uuid.UUID{a, b} {
			wg.Go(func() { _, errs[j] = f.s.RemoveMember(f.ctx, domain.Membership{GroupID: admins.ID, UserID: u}) })
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
			_, _ = f.q.RemoveMember(f.ctx, queries.RemoveMemberParams{GroupID: m.GroupID, UserID: m.UserID})
		}
	}
}

func TestAUsersAppRolesOnAClientAddUpAcrossGroups(t *testing.T) {
	f := open(t)
	viewers, editors := f.group(t, "Viewers"), f.group(t, "Editors")
	ada := f.user(t, "ada@example.org")
	for _, g := range []domain.Group{viewers, editors} {
		_, _ = f.s.AddMember(f.ctx, domain.Membership{GroupID: g.ID, UserID: ada})
	}
	for _, a := range []domain.Assignment{
		{GroupID: viewers.ID, ClientID: "grafana", Role: "viewer"},
		{GroupID: editors.ID, ClientID: "grafana", Role: "viewer"},
		{GroupID: editors.ID, ClientID: "grafana", Role: "editor"},
		{GroupID: editors.ID, ClientID: "jellyfin", Role: "user"},
	} {
		_, _ = f.s.Assign(f.ctx, a)
	}
	roles, err := f.s.RolesOf(f.ctx, ada, "grafana")
	if err != nil || !slices.Equal(roles, []string{"editor", "viewer"}) {
		t.Fatalf("RolesOf = %v, %v", roles, err)
	}
}

func TestGivingAnAppRoleTwiceKeepsOneAssignment(t *testing.T) {
	f := open(t)
	a := domain.Assignment{GroupID: f.group(t, "Viewers").ID, ClientID: "grafana", Role: "viewer"}
	for range 2 {
		if _, err := f.s.Assign(f.ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if as, _ := f.s.Assignments(f.ctx); len(as) != 1 || as[0] != a {
		t.Fatalf("Assignments = %+v", as)
	}
}

func TestGivingAnAppRoleToAGroupThatIsGoneIsNotFound(t *testing.T) {
	f := open(t)
	if _, err := f.s.Assign(f.ctx, domain.Assignment{GroupID: uuid.New(), ClientID: "grafana", Role: "user"}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestAnAppRoleTakenAwayNoLongerCounts(t *testing.T) {
	f := open(t)
	editors, ada := f.group(t, "Editors"), f.user(t, "ada@example.org")
	_, _ = f.s.AddMember(f.ctx, domain.Membership{GroupID: editors.ID, UserID: ada})
	a := domain.Assignment{GroupID: editors.ID, ClientID: "grafana", Role: "editor"}
	_, _ = f.s.Assign(f.ctx, a)
	if _, err := f.s.Unassign(f.ctx, a); err != nil {
		t.Fatal(err)
	}
	if roles, _ := f.s.RolesOf(f.ctx, ada, "grafana"); len(roles) != 0 {
		t.Fatalf("RolesOf = %v", roles)
	}
}

func TestAUsersAppRolesAreListedPerClientAcrossGroups(t *testing.T) {
	f := open(t)
	viewers, editors := f.group(t, "Viewers"), f.group(t, "Editors")
	ada := f.user(t, "ada@example.org")
	for _, g := range []domain.Group{viewers, editors} {
		_, _ = f.s.AddMember(f.ctx, domain.Membership{GroupID: g.ID, UserID: ada})
	}
	for _, a := range []domain.Assignment{
		{GroupID: viewers.ID, ClientID: "grafana", Role: "viewer"},
		{GroupID: editors.ID, ClientID: "grafana", Role: "viewer"},
		{GroupID: editors.ID, ClientID: "grafana", Role: "editor"},
		{GroupID: editors.ID, ClientID: "jellyfin", Role: "user"},
	} {
		_, _ = f.s.Assign(f.ctx, a)
	}
	held, err := f.s.RolesByClient(f.ctx, ada)
	if err != nil || len(held) != 2 || !slices.Equal(held["grafana"], []string{"editor", "viewer"}) || !slices.Equal(held["jellyfin"], []string{"user"}) {
		t.Fatalf("RolesByClient = %v, %v", held, err)
	}
}

func TestAUserInNoGroupHoldsNoAppRoles(t *testing.T) {
	f := open(t)
	if none, err := f.s.RolesByClient(f.ctx, f.user(t, "grace@example.org")); err != nil || len(none) != 0 {
		t.Fatalf("RolesByClient = %v, %v", none, err)
	}
}

func TestTheAdminsGroupIsFoundByItsFlag(t *testing.T) {
	f := open(t)
	admins := f.admins(t)
	if g, err := f.s.AdminsGroup(f.ctx); err != nil || g != admins {
		t.Fatalf("AdminsGroup = %+v, %v", g, err)
	}
}

func TestAFreshInstallHasNoAdmins(t *testing.T) {
	f := open(t)
	if n, err := f.s.CountAdmins(f.ctx); n != 0 || err != nil {
		t.Fatalf("CountAdmins = %d, %v", n, err)
	}
}

func TestTheAdminsAreCounted(t *testing.T) {
	f := open(t)
	_, _ = f.s.AddMember(f.ctx, domain.Membership{GroupID: f.admins(t).ID, UserID: f.user(t, "ada@example.org")})
	if n, err := f.s.CountAdmins(f.ctx); n != 1 || err != nil {
		t.Fatalf("CountAdmins = %d, %v", n, err)
	}
}

func TestTheUnassignedAreUsersWhoHoldNoAppRoleAndAreNotAdmins(t *testing.T) {
	f := open(t)
	admins, family, empty := f.admins(t), f.group(t, "Family"), f.group(t, "Empty")
	ada, bob, cy, dee := f.user(t, "ada@example.org"), f.user(t, "bob@example.org"), f.user(t, "cy@example.org"), f.user(t, "dee@example.org")
	_, _ = f.s.AddMember(f.ctx, domain.Membership{GroupID: admins.ID, UserID: ada})
	_, _ = f.s.AddMember(f.ctx, domain.Membership{GroupID: family.ID, UserID: bob})
	_, _ = f.s.Assign(f.ctx, domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "viewer"})
	_, _ = f.s.AssignUser(f.ctx, domain.UserAssignment{UserID: cy, ClientID: "grafana", Role: "viewer"})
	_, _ = f.s.AddMember(f.ctx, domain.Membership{GroupID: empty.ID, UserID: dee})
	eve := f.user(t, "eve@example.org")
	if n, err := f.s.CountUnassigned(f.ctx); n != 2 || err != nil {
		t.Fatalf("CountUnassigned = %d, %v; want dee and eve", n, err)
	}
	if ids, err := f.s.Unassigned(f.ctx); err != nil || len(ids) != 2 || !slices.Contains(ids, dee) || !slices.Contains(ids, eve) {
		t.Fatalf("Unassigned = %v, %v; want dee and eve", ids, err)
	}
}

func TestAUsersDirectAndGroupAppRolesAddUp(t *testing.T) {
	f := open(t)
	viewers := f.group(t, "Viewers")
	ada := f.user(t, "ada@example.org")
	_, _ = f.s.AddMember(f.ctx, domain.Membership{GroupID: viewers.ID, UserID: ada})
	_, _ = f.s.Assign(f.ctx, domain.Assignment{GroupID: viewers.ID, ClientID: "grafana", Role: "viewer"})
	_, _ = f.s.AssignUser(f.ctx, domain.UserAssignment{UserID: ada, ClientID: "grafana", Role: "editor"})
	_, _ = f.s.AssignUser(f.ctx, domain.UserAssignment{UserID: ada, ClientID: "grafana", Role: "viewer"})
	_, _ = f.s.AssignUser(f.ctx, domain.UserAssignment{UserID: ada, ClientID: "jellyfin", Role: "user"})
	if roles, err := f.s.RolesOf(f.ctx, ada, "grafana"); err != nil || !slices.Equal(roles, []string{"editor", "viewer"}) {
		t.Fatalf("RolesOf = %v, %v", roles, err)
	}
	held, err := f.s.RolesByClient(f.ctx, ada)
	if err != nil || len(held) != 2 || !slices.Equal(held["grafana"], []string{"editor", "viewer"}) || !slices.Equal(held["jellyfin"], []string{"user"}) {
		t.Fatalf("RolesByClient = %v, %v", held, err)
	}
}

func TestSomeoneElsesDirectAppRoleIsNotTheirs(t *testing.T) {
	f := open(t)
	ada, bob := f.user(t, "ada@example.org"), f.user(t, "bob@example.org")
	_, _ = f.s.AssignUser(f.ctx, domain.UserAssignment{UserID: bob, ClientID: "grafana", Role: "viewer"})
	if roles, err := f.s.RolesOf(f.ctx, ada, "grafana"); err != nil || len(roles) != 0 {
		t.Fatalf("RolesOf = %v, %v", roles, err)
	}
}

func TestDirectAssignmentsAreListedApartFromGroupOnes(t *testing.T) {
	f := open(t)
	family := f.group(t, "Family")
	ada := f.user(t, "ada@example.org")
	_, _ = f.s.Assign(f.ctx, domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "viewer"})
	direct := domain.UserAssignment{UserID: ada, ClientID: "grafana", Role: "editor"}
	_, _ = f.s.AssignUser(f.ctx, direct)
	if as, err := f.s.Assignments(f.ctx); err != nil || len(as) != 1 || as[0].GroupID != family.ID {
		t.Fatalf("Assignments = %+v, %v", as, err)
	}
	if as, err := f.s.UserAssignments(f.ctx); err != nil || !slices.Equal(as, []domain.UserAssignment{direct}) {
		t.Fatalf("UserAssignments = %+v, %v", as, err)
	}
}

func TestGivingAPersonAnAppRoleSaysWhetherItIsNew(t *testing.T) {
	f := open(t)
	a := domain.UserAssignment{UserID: f.user(t, "ada@example.org"), ClientID: "grafana", Role: "viewer"}
	first, _ := f.s.AssignUser(f.ctx, a)
	again, err := f.s.AssignUser(f.ctx, a)
	if !first || again || err != nil {
		t.Fatalf("first %v, again %v, %v", first, again, err)
	}
}

func TestGivingAnAppRoleToAUserWhoIsGoneIsNotFound(t *testing.T) {
	f := open(t)
	if _, err := f.s.AssignUser(f.ctx, domain.UserAssignment{UserID: uuid.New(), ClientID: "grafana", Role: "viewer"}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("AssignUser = %v", err)
	}
}

func TestTakingAPersonsAppRoleSaysWhetherItWasThere(t *testing.T) {
	f := open(t)
	a := domain.UserAssignment{UserID: f.user(t, "ada@example.org"), ClientID: "grafana", Role: "viewer"}
	_, _ = f.s.AssignUser(f.ctx, a)
	first, _ := f.s.UnassignUser(f.ctx, a)
	again, err := f.s.UnassignUser(f.ctx, a)
	if !first || again || err != nil {
		t.Fatalf("first %v, again %v, %v", first, again, err)
	}
}

func TestAGroupAndAPersonHoldingTheSameRoleAreTwoAssignments(t *testing.T) {
	f := open(t)
	family := f.group(t, "Family")
	ada := f.user(t, "ada@example.org")
	_, _ = f.s.Assign(f.ctx, domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "viewer"})
	if added, err := f.s.AssignUser(f.ctx, domain.UserAssignment{UserID: ada, ClientID: "grafana", Role: "viewer"}); !added || err != nil {
		t.Fatalf("AssignUser = %v, %v", added, err)
	}
}

func TestAddingAMemberSaysWhetherTheyWereNew(t *testing.T) {
	f := open(t)
	m := domain.Membership{GroupID: f.group(t, "Family").ID, UserID: f.user(t, "ada@example.org")}
	first, _ := f.s.AddMember(f.ctx, m)
	again, err := f.s.AddMember(f.ctx, m)
	if !first || again || err != nil {
		t.Fatalf("first %v, again %v, %v", first, again, err)
	}
}

func TestRemovingAMemberSaysWhetherTheyWereOne(t *testing.T) {
	f := open(t)
	family := f.group(t, "Family")
	m := domain.Membership{GroupID: family.ID, UserID: f.user(t, "ada@example.org")}
	_, _ = f.s.AddMember(f.ctx, m)
	first, _ := f.s.RemoveMember(f.ctx, m)
	again, err := f.s.RemoveMember(f.ctx, m)
	gone, _ := f.s.RemoveMember(f.ctx, domain.Membership{GroupID: uuid.New(), UserID: m.UserID})
	if !first || again || gone || err != nil {
		t.Fatalf("first %v, again %v, in no Group %v, %v", first, again, gone, err)
	}
}

func TestTheLastAdminsRefusedRemovalSaysNothingWasRemoved(t *testing.T) {
	f := open(t)
	last := domain.Membership{GroupID: f.admins(t).ID, UserID: f.user(t, "ada@example.org")}
	_, _ = f.s.AddMember(f.ctx, last)
	if removed, err := f.s.RemoveMember(f.ctx, last); removed || !errors.Is(err, app.ErrLastAdmin) {
		t.Fatalf("removed %v, %v", removed, err)
	}
}

func TestAssigningSaysWhetherTheAssignmentIsNew(t *testing.T) {
	f := open(t)
	a := domain.Assignment{GroupID: f.group(t, "Family").ID, ClientID: "grafana", Role: "viewer"}
	first, _ := f.s.Assign(f.ctx, a)
	again, err := f.s.Assign(f.ctx, a)
	if !first || again || err != nil {
		t.Fatalf("first %v, again %v, %v", first, again, err)
	}
}

func TestUnassigningSaysWhetherTheAssignmentWasThere(t *testing.T) {
	f := open(t)
	a := domain.Assignment{GroupID: f.group(t, "Family").ID, ClientID: "grafana", Role: "viewer"}
	_, _ = f.s.Assign(f.ctx, a)
	first, _ := f.s.Unassign(f.ctx, a)
	again, err := f.s.Unassign(f.ctx, a)
	if !first || again || err != nil {
		t.Fatalf("first %v, again %v, %v", first, again, err)
	}
}
