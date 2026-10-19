package app_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/app"
	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
)

var errBoom = errors.New("boom")

// store is an in-memory app.Store; fail makes the named method return its error.
type store struct {
	groups      []domain.Group
	members     []domain.Membership
	assignments []domain.Assignment
	direct      []domain.UserAssignment
	fail        map[string]error
}

func (s *store) err(name string) error { return s.fail[name] }

func (s *store) Groups(context.Context) ([]domain.Group, error) {
	return slices.Clone(s.groups), s.err("Groups")
}

func (s *store) Group(_ context.Context, id uuid.UUID) (domain.Group, error) {
	if err := s.err("Group"); err != nil {
		return domain.Group{}, err
	}
	for _, g := range s.groups {
		if g.ID == id {
			return g, nil
		}
	}
	return domain.Group{}, app.ErrNotFound
}

func (s *store) CreateGroup(_ context.Context, g domain.Group) error {
	if err := s.err("CreateGroup"); err != nil {
		return err
	}
	s.groups = append(s.groups, g)
	return nil
}

func (s *store) RenameGroup(_ context.Context, id uuid.UUID, name domain.GroupName) error {
	if err := s.err("RenameGroup"); err != nil {
		return err
	}
	for i, g := range s.groups {
		if g.ID == id {
			s.groups[i].Name = name.String()
			return nil
		}
	}
	return app.ErrNotFound
}

func (s *store) SetGroupTwoFactor(_ context.Context, id uuid.UUID, required bool) error {
	if err := s.err("SetGroupTwoFactor"); err != nil {
		return err
	}
	for i, g := range s.groups {
		if g.ID == id {
			s.groups[i].RequiresTwoFactor = required
			return nil
		}
	}
	return app.ErrNotFound
}

func (s *store) DeleteGroup(_ context.Context, id uuid.UUID) error {
	s.groups = slices.DeleteFunc(s.groups, func(g domain.Group) bool { return g.ID == id && !g.Admins })
	return s.err("DeleteGroup")
}

func (s *store) Memberships(context.Context) ([]domain.Membership, error) {
	return slices.Clone(s.members), s.err("Memberships")
}

func (s *store) AddMember(_ context.Context, m domain.Membership) (bool, error) {
	if err := s.err("AddMember"); err != nil {
		return false, err
	}
	if slices.Contains(s.members, m) {
		return false, nil
	}
	s.members = append(s.members, m)
	return true, nil
}

func (s *store) RemoveMember(_ context.Context, m domain.Membership) (bool, error) {
	if err := s.err("RemoveMember"); err != nil {
		return false, err
	}
	n := len(s.members)
	s.members = slices.DeleteFunc(s.members, func(o domain.Membership) bool { return o == m })
	return len(s.members) < n, nil
}

func (s *store) IsAdmin(_ context.Context, userID uuid.UUID) (bool, error) {
	for _, m := range s.members {
		for _, g := range s.groups {
			if g.Admins && g.ID == m.GroupID && m.UserID == userID {
				return true, s.err("IsAdmin")
			}
		}
	}
	return false, s.err("IsAdmin")
}

func (s *store) AdminsGroup(context.Context) (domain.Group, error) {
	for _, g := range s.groups {
		if g.Admins {
			return g, s.err("AdminsGroup")
		}
	}
	return domain.Group{}, app.ErrNotFound
}

func (s *store) CountAdmins(ctx context.Context) (int, error) {
	g, _ := s.AdminsGroup(ctx)
	n := 0
	for _, m := range s.members {
		if m.GroupID == g.ID {
			n++
		}
	}
	return n, s.err("CountAdmins")
}

func (s *store) CountUnassigned(context.Context) (int, error) {
	return 7, s.err("CountUnassigned")
}

func (s *store) Unassigned(context.Context) ([]uuid.UUID, error) {
	return []uuid.UUID{ada}, s.err("Unassigned")
}

func (s *store) Assignments(context.Context) ([]domain.Assignment, error) {
	return slices.Clone(s.assignments), s.err("Assignments")
}

func (s *store) Assign(_ context.Context, a domain.Assignment) (bool, error) {
	if err := s.err("Assign"); err != nil {
		return false, err
	}
	if slices.Contains(s.assignments, a) {
		return false, nil
	}
	s.assignments = append(s.assignments, a)
	return true, nil
}

func (s *store) Unassign(_ context.Context, a domain.Assignment) (bool, error) {
	if err := s.err("Unassign"); err != nil {
		return false, err
	}
	n := len(s.assignments)
	s.assignments = slices.DeleteFunc(s.assignments, func(o domain.Assignment) bool { return o == a })
	return len(s.assignments) < n, nil
}

func (s *store) UserAssignments(context.Context) ([]domain.UserAssignment, error) {
	return slices.Clone(s.direct), s.err("UserAssignments")
}

func (s *store) AssignUser(_ context.Context, a domain.UserAssignment) (bool, error) {
	if err := s.err("AssignUser"); err != nil {
		return false, err
	}
	if slices.Contains(s.direct, a) {
		return false, nil
	}
	s.direct = append(s.direct, a)
	return true, nil
}

func (s *store) UnassignUser(_ context.Context, a domain.UserAssignment) (bool, error) {
	if err := s.err("UnassignUser"); err != nil {
		return false, err
	}
	n := len(s.direct)
	s.direct = slices.DeleteFunc(s.direct, func(o domain.UserAssignment) bool { return o == a })
	return len(s.direct) < n, nil
}

func (s *store) RolesOf(_ context.Context, userID uuid.UUID, clientID string) ([]string, error) {
	var roles []string
	for _, a := range s.direct {
		if a.UserID == userID && a.ClientID == clientID {
			roles = append(roles, a.Role)
		}
	}
	for _, m := range s.members {
		for _, a := range s.assignments {
			if m.UserID == userID && a.GroupID == m.GroupID && a.ClientID == clientID && !slices.Contains(roles, a.Role) {
				roles = append(roles, a.Role)
			}
		}
	}
	return roles, s.err("RolesOf")
}

func (s *store) RolesByClient(_ context.Context, userID uuid.UUID) (map[string][]string, error) {
	held := map[string][]string{}
	for _, a := range s.assignments {
		roles, _ := s.RolesOf(context.Background(), userID, a.ClientID)
		if len(roles) > 0 {
			held[a.ClientID] = roles
		}
	}
	return held, s.err("RolesByClient")
}

type catalog []domain.Client

func (c catalog) Clients() []domain.Client { return c }

func (c catalog) Client(_ context.Context, id string) (domain.Client, bool) {
	i := slices.IndexFunc(c, func(x domain.Client) bool { return x.ID == id })
	if i < 0 {
		return domain.Client{}, false
	}
	return c[i], true
}

type people struct {
	list        []domain.Person
	err         error
	signedOut   []uuid.UUID
	signedOutBy []uuid.UUID
}

func (p *people) People(context.Context) ([]domain.Person, error) { return p.list, p.err }

func (p *people) SignOut(_ context.Context, actor, userID uuid.UUID) error {
	p.signedOut = append(p.signedOut, userID)
	p.signedOutBy = append(p.signedOutBy, actor)
	return p.err
}

// recorded is the Audit Events a Service recorded.
type recorded []audit.Event

func (r *recorded) Record(_ context.Context, e audit.Event) { *r = append(*r, e) }

var (
	admin   = uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
	ada     = uuid.MustParse("00000000-0000-4000-8000-0000000000a2")
	admins  = domain.Group{ID: uuid.MustParse("00000000-0000-4000-8000-0000000000b1"), Name: "admins", Admins: true}
	family  = domain.Group{ID: uuid.MustParse("00000000-0000-4000-8000-0000000000b2"), Name: "Family"}
	friends = domain.Group{ID: uuid.MustParse("00000000-0000-4000-8000-0000000000b3"), Name: "Friends"}
	asAdmin = app.Actor{ID: admin, SecondFactor: true}
	grafana = domain.Client{ID: "grafana", Label: "Grafana", Roles: []string{"user", "viewer", "editor"}}
)

type fixture struct {
	store  *store
	people *people
	events *recorded
	svc    *app.Service
}

func newFixture() *fixture {
	s := &store{
		groups:  []domain.Group{admins, family, friends},
		members: []domain.Membership{{GroupID: admins.ID, UserID: admin}},
		fail:    map[string]error{},
	}
	p := &people{list: []domain.Person{{ID: ada, Email: "ada@example.org", Name: "Ada"}}}
	events := &recorded{}
	return &fixture{store: s, people: p, events: events, svc: app.New(s, catalog{grafana}, p, events, bytes.NewReader(bytes.Repeat([]byte{7}, 64)))}
}

var ctx = context.Background()

// asNonAdmin is a signed-in User who proved a Second Factor but is in no admins Group.
var asNonAdmin = app.Actor{ID: ada, SecondFactor: true}

// operation is one access use case, called as actor against a fixture.
type operation func(f *fixture, actor app.Actor) error

var operations = []struct {
	name string
	call operation
}{
	{"reading the overview", func(f *fixture, u app.Actor) error { _, err := f.svc.Overview(ctx, u); return err }},
	{"creating a Group", func(f *fixture, u app.Actor) error { _, err := f.svc.CreateGroup(ctx, u, "Cousins"); return err }},
	{"renaming a Group", func(f *fixture, u app.Actor) error { _, err := f.svc.RenameGroup(ctx, u, family.ID, "Kin"); return err }},
	{"deleting a Group", func(f *fixture, u app.Actor) error { return f.svc.DeleteGroup(ctx, u, friends.ID) }},
	{"adding a member", func(f *fixture, u app.Actor) error {
		return f.svc.AddMember(ctx, u, domain.Membership{GroupID: family.ID, UserID: ada})
	}},
	{"removing a member", func(f *fixture, u app.Actor) error {
		return f.svc.RemoveMember(ctx, u, domain.Membership{GroupID: family.ID, UserID: ada})
	}},
	{"giving an App Role", func(f *fixture, u app.Actor) error {
		return f.svc.Assign(ctx, u, domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "viewer"})
	}},
	{"taking an App Role", func(f *fixture, u app.Actor) error {
		return f.svc.Unassign(ctx, u, domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "viewer"})
	}},
	{"signing a User out", func(f *fixture, u app.Actor) error { return f.svc.SignOutUser(ctx, u, ada) }},
	{"making a Group require two-factor", func(f *fixture, u app.Actor) error { return f.svc.SetTwoFactor(ctx, u, family.ID, true) }},
	{"giving a person an App Role", func(f *fixture, u app.Actor) error {
		return f.svc.AssignUser(ctx, u, domain.UserAssignment{UserID: ada, ClientID: "grafana", Role: "viewer"})
	}},
	{"taking a person's App Role", func(f *fixture, u app.Actor) error {
		return f.svc.UnassignUser(ctx, u, domain.UserAssignment{UserID: ada, ClientID: "grafana", Role: "viewer"})
	}},
}

func TestEveryAccessOperationRefusesAUserWhoIsNotAnAdminAndChangesNothing(t *testing.T) {
	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			f := newFixture()
			if err := op.call(f, asNonAdmin); !errors.Is(err, app.ErrNotAdmin) {
				t.Fatalf("err = %v, want ErrNotAdmin", err)
			}
			if len(f.store.groups) != 3 || len(f.store.members) != 1 || len(f.store.assignments) != 0 || len(f.store.direct) != 0 || len(f.people.signedOut) != 0 {
				t.Fatalf("a refused call changed something: %+v, signed out %v", f.store, f.people.signedOut)
			}
		})
	}
}

func TestEveryAccessOperationLetsAnAdminThrough(t *testing.T) {
	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			if err := op.call(newFixture(), asAdmin); errors.Is(err, app.ErrNotAdmin) || errors.Is(err, app.ErrSecondFactorNeeded) {
				t.Fatalf("err = %v, want the Admin let through", err)
			}
		})
	}
}

func TestAnAdminWhoseSessionProvedNoSecondFactorIsSentToAddOne(t *testing.T) {
	if _, err := newFixture().svc.Overview(ctx, app.Actor{ID: admin}); !errors.Is(err, app.ErrSecondFactorNeeded) {
		t.Fatalf("err = %v, want ErrSecondFactorNeeded", err)
	}
}

func TestAnAccessOperationFailsWhenTheAdminCheckCannotBeMade(t *testing.T) {
	f := newFixture()
	f.store.fail["IsAdmin"] = errBoom
	if _, err := f.svc.Overview(ctx, asAdmin); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestTheOverviewShowsGroupsMembersAssignmentsClientsAndPeople(t *testing.T) {
	f := newFixture()
	f.store.assignments = []domain.Assignment{{GroupID: family.ID, ClientID: "grafana", Role: "viewer"}}
	f.store.direct = []domain.UserAssignment{{UserID: ada, ClientID: "grafana", Role: "editor"}}
	got, err := f.svc.Overview(ctx, asAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 3 || len(got.Memberships) != 1 || len(got.Assignments) != 1 || len(got.UserAssignments) != 1 || len(got.Clients) != 1 || got.People[0].ID != ada {
		t.Fatalf("Overview = %+v", got)
	}
}

func TestTheOverviewFailsWhenAnyPartCannotBeRead(t *testing.T) {
	for _, part := range []string{"Groups", "Memberships", "Assignments", "UserAssignments", "People"} {
		t.Run(part, func(t *testing.T) {
			f := newFixture()
			if part == "People" {
				f.people.err = errBoom
			} else {
				f.store.fail[part] = errBoom
			}
			if _, err := f.svc.Overview(ctx, asAdmin); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAnAdminCreatesAGroupWithItsNameTrimmed(t *testing.T) {
	f := newFixture()
	g, err := f.svc.CreateGroup(ctx, asAdmin, "  Cousins ")
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "Cousins" || g.Admins || g.ID == uuid.Nil || f.store.groups[3] != g {
		t.Fatalf("CreateGroup = %+v, stored %+v", g, f.store.groups)
	}
}

func TestAGroupNeedsAName(t *testing.T) {
	if _, err := newFixture().svc.CreateGroup(ctx, asAdmin, " "); !errors.Is(err, domain.ErrInvalidGroupName) {
		t.Fatalf("err = %v", err)
	}
}

func TestTwoGroupsCannotShareAName(t *testing.T) {
	f := newFixture()
	f.store.fail["CreateGroup"] = app.ErrNameTaken
	if _, err := f.svc.CreateGroup(ctx, asAdmin, "Family"); !errors.Is(err, app.ErrNameTaken) {
		t.Fatalf("err = %v", err)
	}
}

func TestNoGroupIsMadeWithoutRandomnessForItsID(t *testing.T) {
	f := newFixture()
	broken := app.New(f.store, catalog{}, f.people, f.events, bytes.NewReader(nil))
	if _, err := broken.CreateGroup(ctx, asAdmin, "Cousins"); err == nil {
		t.Fatal("a Group was made without randomness")
	}
}

func TestAnAdminRenamesAGroup(t *testing.T) {
	g, err := newFixture().svc.RenameGroup(ctx, asAdmin, family.ID, " Kin ")
	if err != nil || g.Name != "Kin" || g.ID != family.ID {
		t.Fatalf("RenameGroup = %+v, %v", g, err)
	}
}

func TestARenameNeedsAName(t *testing.T) {
	if _, err := newFixture().svc.RenameGroup(ctx, asAdmin, family.ID, ""); !errors.Is(err, domain.ErrInvalidGroupName) {
		t.Fatalf("err = %v", err)
	}
}

func TestRenamingAGroupThatIsGoneIsNotFound(t *testing.T) {
	if _, err := newFixture().svc.RenameGroup(ctx, asAdmin, uuid.New(), "Kin"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestARenameFailsWhenTheRenamedGroupCannotBeReadBack(t *testing.T) {
	f := newFixture()
	f.store.fail["Group"] = errBoom
	if _, err := f.svc.RenameGroup(ctx, asAdmin, family.ID, "Kin"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnAdminDeletesAGroup(t *testing.T) {
	f := newFixture()
	if err := f.svc.DeleteGroup(ctx, asAdmin, family.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Group(ctx, family.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("the Group is still there")
	}
}

func TestTheAdminsGroupCannotBeDeleted(t *testing.T) {
	if err := newFixture().svc.DeleteGroup(ctx, asAdmin, admins.ID); !errors.Is(err, app.ErrAdminsGroup) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeletingAGroupThatIsGoneIsNotFound(t *testing.T) {
	if err := newFixture().svc.DeleteGroup(ctx, asAdmin, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnAdminAddsAPersonToAGroup(t *testing.T) {
	f := newFixture()
	m := domain.Membership{GroupID: family.ID, UserID: ada}
	if err := f.svc.AddMember(ctx, asAdmin, m); err != nil || f.store.members[1] != m {
		t.Fatalf("AddMember = %v, members %+v", err, f.store.members)
	}
}

func TestAddingAPersonToAGroupThatIsGoneIsNotFound(t *testing.T) {
	f := newFixture()
	f.store.fail["AddMember"] = app.ErrNotFound
	if err := f.svc.AddMember(ctx, asAdmin, domain.Membership{GroupID: uuid.New(), UserID: ada}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnAdminTakesAPersonOutOfAGroup(t *testing.T) {
	f := newFixture()
	m := domain.Membership{GroupID: family.ID, UserID: ada}
	f.store.members = append(f.store.members, m)
	if err := f.svc.RemoveMember(ctx, asAdmin, m); err != nil || len(f.store.members) != 1 {
		t.Fatalf("RemoveMember = %v, members %+v", err, f.store.members)
	}
}

func TestTheLastAdminCannotLeaveTheAdminsGroup(t *testing.T) {
	f := newFixture()
	f.store.fail["RemoveMember"] = app.ErrLastAdmin
	if err := f.svc.RemoveMember(ctx, asAdmin, domain.Membership{GroupID: admins.ID, UserID: admin}); !errors.Is(err, app.ErrLastAdmin) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnAdminGivesAGroupAnAppRoleTheClientDeclares(t *testing.T) {
	f := newFixture()
	a := domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "viewer"}
	if err := f.svc.Assign(ctx, asAdmin, a); err != nil || len(f.store.assignments) != 1 || f.store.assignments[0] != a {
		t.Fatalf("Assign = %v, assignments %+v", err, f.store.assignments)
	}
}

func TestAnAppRoleIsRefusedAndNotKept(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    domain.Assignment
		want error
	}{
		{"to the admins Group", domain.Assignment{GroupID: admins.ID, ClientID: "grafana", Role: "user"}, app.ErrAdminsGroup},
		{"to a Group that is gone", domain.Assignment{GroupID: uuid.New(), ClientID: "grafana", Role: "user"}, app.ErrNotFound},
		{"on a Client the cluster no longer declares", domain.Assignment{GroupID: family.ID, ClientID: "jellyfin", Role: "user"}, app.ErrUnknownClient},
		{"the Client does not declare", domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "admin"}, app.ErrUnknownRole},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			if err := f.svc.Assign(ctx, asAdmin, tc.a); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if len(f.store.assignments) != 0 {
				t.Fatalf("a refused Assignment was kept: %+v", f.store.assignments)
			}
		})
	}
}

func TestAnAdminTakesAnAppRoleAway(t *testing.T) {
	f := newFixture()
	a := domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "viewer"}
	f.store.assignments = []domain.Assignment{a}
	if err := f.svc.Unassign(ctx, asAdmin, a); err != nil || len(f.store.assignments) != 0 {
		t.Fatalf("Unassign = %v, assignments %+v", err, f.store.assignments)
	}
}

func TestAUsersAppRolesOnAClientAreTheUnionOverTheirGroups(t *testing.T) {
	f := newFixture()
	f.store.members = append(f.store.members, domain.Membership{GroupID: family.ID, UserID: ada}, domain.Membership{GroupID: friends.ID, UserID: ada})
	f.store.assignments = []domain.Assignment{
		{GroupID: family.ID, ClientID: "grafana", Role: "viewer"},
		{GroupID: friends.ID, ClientID: "grafana", Role: "editor"},
		{GroupID: friends.ID, ClientID: "grafana", Role: "viewer"},
		{GroupID: friends.ID, ClientID: "jellyfin", Role: "user"},
	}
	got, err := f.svc.Roles(ctx, ada, "grafana")
	if err != nil || !slices.Equal(got, []string{"viewer", "editor"}) {
		t.Fatalf("Roles = %v, %v", got, err)
	}
}

func TestAnAdminGivesAPersonAnAppRoleTheClientDeclares(t *testing.T) {
	f := newFixture()
	a := domain.UserAssignment{UserID: ada, ClientID: "grafana", Role: "viewer"}
	if err := f.svc.AssignUser(ctx, asAdmin, a); err != nil || !slices.Equal(f.store.direct, []domain.UserAssignment{a}) {
		t.Fatalf("AssignUser = %v, direct %+v", err, f.store.direct)
	}
}

func TestAPersonsAppRoleIsRefusedAndNotKept(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    domain.UserAssignment
		fail error
		want error
	}{
		{"for someone who is gone", domain.UserAssignment{UserID: uuid.New(), ClientID: "grafana", Role: "user"}, app.ErrNotFound, app.ErrNotFound},
		{"on a Client the cluster no longer declares", domain.UserAssignment{UserID: ada, ClientID: "jellyfin", Role: "user"}, nil, app.ErrUnknownClient},
		{"the Client does not declare", domain.UserAssignment{UserID: ada, ClientID: "grafana", Role: "admin"}, nil, app.ErrUnknownRole},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			if tc.fail != nil {
				f.store.fail["AssignUser"] = tc.fail
			}
			if err := f.svc.AssignUser(ctx, asAdmin, tc.a); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if len(f.store.direct) != 0 {
				t.Fatalf("a refused Assignment was kept: %+v", f.store.direct)
			}
		})
	}
}

func TestAnAdminTakesAPersonsAppRoleAway(t *testing.T) {
	f := newFixture()
	a := domain.UserAssignment{UserID: ada, ClientID: "grafana", Role: "viewer"}
	f.store.direct = []domain.UserAssignment{a}
	if err := f.svc.UnassignUser(ctx, asAdmin, a); err != nil || len(f.store.direct) != 0 {
		t.Fatalf("UnassignUser = %v, direct %+v", err, f.store.direct)
	}
}

func TestBeingAnAdminGivesNoAppRole(t *testing.T) {
	if got, _ := newFixture().svc.Roles(ctx, admin, "grafana"); len(got) != 0 {
		t.Fatalf("an Admin with no Assignment holds %v", got)
	}
}

func TestAUsersAppRolesAreListedPerClient(t *testing.T) {
	f := newFixture()
	f.store.members = append(f.store.members, domain.Membership{GroupID: family.ID, UserID: ada})
	f.store.assignments = []domain.Assignment{
		{GroupID: family.ID, ClientID: "grafana", Role: "viewer"},
		{GroupID: family.ID, ClientID: "jellyfin", Role: "user"},
	}
	got, err := f.svc.RolesByClient(ctx, ada)
	if err != nil || len(got) != 2 || !slices.Equal(got["grafana"], []string{"viewer"}) || !slices.Equal(got["jellyfin"], []string{"user"}) {
		t.Fatalf("RolesByClient = %v, %v", got, err)
	}
}

func TestAMemberOfTheAdminsGroupIsAnAdmin(t *testing.T) {
	if yes, err := newFixture().svc.IsAdmin(ctx, admin); !yes || err != nil {
		t.Fatalf("IsAdmin = %v, %v", yes, err)
	}
}

func TestAUserOutsideTheAdminsGroupIsNoAdmin(t *testing.T) {
	if yes, _ := newFixture().svc.IsAdmin(ctx, ada); yes {
		t.Fatal("IsAdmin = true")
	}
}

func TestTheFirstPromoteMakesAnAdmin(t *testing.T) {
	f := newFixture()
	f.store.members = nil
	if err := f.svc.Promote(ctx, ada, false); err != nil {
		t.Fatal(err)
	}
	if yes, _ := f.svc.IsAdmin(ctx, ada); !yes {
		t.Fatal("no Admin was made")
	}
}

func TestAPromoteWithoutForceIsRefusedOnceAnAdminExists(t *testing.T) {
	f := newFixture()
	if err := f.svc.Promote(ctx, ada, false); !errors.Is(err, app.ErrAdminExists) {
		t.Fatalf("err = %v, want ErrAdminExists", err)
	}
	if yes, _ := f.svc.IsAdmin(ctx, ada); yes {
		t.Fatal("a refused promote made an Admin")
	}
}

func TestAForcedPromoteMakesAnotherAdmin(t *testing.T) {
	f := newFixture()
	if err := f.svc.Promote(ctx, ada, true); err != nil {
		t.Fatal(err)
	}
	if yes, _ := f.svc.IsAdmin(ctx, ada); !yes {
		t.Fatal("no Admin was made")
	}
}

func TestAPromoteFailsWhenTheStoreDoes(t *testing.T) {
	for _, step := range []string{"CountAdmins", "AdminsGroup", "AddMember"} {
		t.Run(step, func(t *testing.T) {
			f := newFixture()
			f.store.fail[step] = errBoom
			if err := f.svc.Promote(ctx, uuid.New(), true); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestTheUnassignedCountIsTheStores(t *testing.T) {
	if n, err := newFixture().svc.Unassigned(ctx); n != 7 || err != nil {
		t.Fatalf("Unassigned = %d, %v", n, err)
	}
}

func TestAnAdminSignsAUserOutEverywhere(t *testing.T) {
	f := newFixture()
	if err := f.svc.SignOutUser(ctx, asAdmin, ada); err != nil || len(f.people.signedOut) != 1 || f.people.signedOut[0] != ada {
		t.Fatalf("SignOutUser = %v, signed out %v", err, f.people.signedOut)
	}
}

func TestSigningAUserOutFailsWhenTheUsersContextDoes(t *testing.T) {
	f := newFixture()
	f.people.err = errBoom
	if err := f.svc.SignOutUser(ctx, asAdmin, ada); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestTheUnassignedUsersAreThoseInNoGroup(t *testing.T) {
	f := newFixture()
	if ids, err := f.svc.UnassignedUsers(context.Background()); err != nil || !slices.Equal(ids, []uuid.UUID{ada}) {
		t.Fatalf("UnassignedUsers = %v, %v", ids, err)
	}
}

func TestAnAdminMakesAGroupRequireTwoFactorAndStop(t *testing.T) {
	f := newFixture()
	if err := f.svc.SetTwoFactor(ctx, asAdmin, family.ID, true); err != nil || !f.store.groups[1].RequiresTwoFactor {
		t.Fatalf("SetTwoFactor = %v, %+v", err, f.store.groups[1])
	}
	if err := f.svc.SetTwoFactor(ctx, asAdmin, family.ID, false); err != nil || f.store.groups[1].RequiresTwoFactor {
		t.Fatalf("SetTwoFactor = %v, %+v", err, f.store.groups[1])
	}
}

func TestTheAdminsGroupCannotStopRequiringTwoFactor(t *testing.T) {
	if err := newFixture().svc.SetTwoFactor(ctx, asAdmin, admins.ID, false); !errors.Is(err, app.ErrAdminsNeedTwoFactor) {
		t.Fatalf("err = %v", err)
	}
}

func TestTheAdminsGroupMayBeToldToRequireTwoFactor(t *testing.T) {
	if err := newFixture().svc.SetTwoFactor(ctx, asAdmin, admins.ID, true); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestATwoFactorChangeFailsWhenTheStoreDoes(t *testing.T) {
	for _, method := range []string{"Group", "SetGroupTwoFactor"} {
		t.Run(method, func(t *testing.T) {
			f := newFixture()
			f.store.fail[method] = errBoom
			if err := f.svc.SetTwoFactor(ctx, asAdmin, family.ID, true); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
