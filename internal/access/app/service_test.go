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
)

var errBoom = errors.New("boom")

// store is an in-memory app.Store; fail makes the named method return its error.
type store struct {
	groups      []domain.Group
	members     []domain.Membership
	assignments []domain.Assignment
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

func (s *store) DeleteGroup(_ context.Context, id uuid.UUID) error {
	s.groups = slices.DeleteFunc(s.groups, func(g domain.Group) bool { return g.ID == id && !g.Admins })
	return s.err("DeleteGroup")
}

func (s *store) Memberships(context.Context) ([]domain.Membership, error) {
	return slices.Clone(s.members), s.err("Memberships")
}

func (s *store) AddMember(_ context.Context, m domain.Membership) error {
	if err := s.err("AddMember"); err != nil {
		return err
	}
	s.members = append(s.members, m)
	return nil
}

func (s *store) RemoveMember(_ context.Context, m domain.Membership) error {
	if err := s.err("RemoveMember"); err != nil {
		return err
	}
	s.members = slices.DeleteFunc(s.members, func(o domain.Membership) bool { return o == m })
	return nil
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

func (s *store) Assignments(context.Context) ([]domain.Assignment, error) {
	return slices.Clone(s.assignments), s.err("Assignments")
}

func (s *store) Assign(_ context.Context, a domain.Assignment) error {
	if err := s.err("Assign"); err != nil {
		return err
	}
	s.assignments = append(s.assignments, a)
	return nil
}

func (s *store) Unassign(_ context.Context, a domain.Assignment) error {
	s.assignments = slices.DeleteFunc(s.assignments, func(o domain.Assignment) bool { return o == a })
	return s.err("Unassign")
}

func (s *store) RolesOf(_ context.Context, userID uuid.UUID, clientID string) ([]string, error) {
	var roles []string
	for _, m := range s.members {
		for _, a := range s.assignments {
			if m.UserID == userID && a.GroupID == m.GroupID && a.ClientID == clientID && !slices.Contains(roles, a.Role) {
				roles = append(roles, a.Role)
			}
		}
	}
	return roles, s.err("RolesOf")
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
	list []domain.Person
	err  error
}

func (p *people) People(context.Context) ([]domain.Person, error) { return p.list, p.err }

var (
	admin   = uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
	ada     = uuid.MustParse("00000000-0000-4000-8000-0000000000a2")
	admins  = domain.Group{ID: uuid.MustParse("00000000-0000-4000-8000-0000000000b1"), Name: "admins", Admins: true}
	family  = domain.Group{ID: uuid.MustParse("00000000-0000-4000-8000-0000000000b2"), Name: "Family"}
	friends = domain.Group{ID: uuid.MustParse("00000000-0000-4000-8000-0000000000b3"), Name: "Friends"}
	grafana = domain.Client{ID: "grafana", Label: "Grafana", Roles: []string{"user", "viewer", "editor"}}
)

type fixture struct {
	store  *store
	people *people
	svc    *app.Service
}

func newFixture() *fixture {
	s := &store{
		groups:  []domain.Group{admins, family, friends},
		members: []domain.Membership{{GroupID: admins.ID, UserID: admin}},
		fail:    map[string]error{},
	}
	p := &people{list: []domain.Person{{ID: ada, Email: "ada@example.org", Name: "Ada"}}}
	return &fixture{store: s, people: p, svc: app.New(s, catalog{grafana}, p, bytes.NewReader(bytes.Repeat([]byte{7}, 64)))}
}

var ctx = context.Background()

func TestOnlyAnAdminManagesAccess(t *testing.T) {
	f := newFixture()
	a := domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "viewer"}
	m := domain.Membership{GroupID: family.ID, UserID: ada}
	for name, call := range map[string]func(uuid.UUID) error{
		"Overview":     func(u uuid.UUID) error { _, err := f.svc.Overview(ctx, u); return err },
		"CreateGroup":  func(u uuid.UUID) error { _, err := f.svc.CreateGroup(ctx, u, "Cousins"); return err },
		"RenameGroup":  func(u uuid.UUID) error { _, err := f.svc.RenameGroup(ctx, u, family.ID, "Kin"); return err },
		"DeleteGroup":  func(u uuid.UUID) error { return f.svc.DeleteGroup(ctx, u, friends.ID) },
		"AddMember":    func(u uuid.UUID) error { return f.svc.AddMember(ctx, u, m) },
		"RemoveMember": func(u uuid.UUID) error { return f.svc.RemoveMember(ctx, u, m) },
		"Assign":       func(u uuid.UUID) error { return f.svc.Assign(ctx, u, a) },
		"Unassign":     func(u uuid.UUID) error { return f.svc.Unassign(ctx, u, a) },
	} {
		before := *f.store
		if err := call(ada); !errors.Is(err, app.ErrNotAdmin) {
			t.Errorf("%s by a non-Admin = %v, want ErrNotAdmin", name, err)
		}
		if len(f.store.groups) != len(before.groups) || len(f.store.members) != len(before.members) || len(f.store.assignments) != len(before.assignments) {
			t.Errorf("%s by a non-Admin changed the store", name)
		}
		if err := call(admin); errors.Is(err, app.ErrNotAdmin) {
			t.Errorf("%s by an Admin = ErrNotAdmin", name)
		}
	}
	f.store.fail["IsAdmin"] = errBoom
	if _, err := f.svc.Overview(ctx, admin); !errors.Is(err, errBoom) {
		t.Fatalf("Overview when the admin check fails = %v", err)
	}
}

func TestOverview(t *testing.T) {
	f := newFixture()
	f.store.assignments = []domain.Assignment{{GroupID: family.ID, ClientID: "grafana", Role: "viewer"}}
	got, err := f.svc.Overview(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 3 || len(got.Memberships) != 1 || len(got.Assignments) != 1 ||
		len(got.Clients) != 1 || got.People[0].ID != ada {
		t.Fatalf("Overview = %+v", got)
	}
	for _, name := range []string{"Groups", "Memberships", "Assignments"} {
		f.store.fail = map[string]error{name: errBoom}
		if _, err := f.svc.Overview(ctx, admin); !errors.Is(err, errBoom) {
			t.Errorf("Overview when %s fails = %v", name, err)
		}
	}
	f.store.fail = map[string]error{}
	f.people.err = errBoom
	if _, err := f.svc.Overview(ctx, admin); !errors.Is(err, errBoom) {
		t.Errorf("Overview when People fails = %v", err)
	}
}

func TestCreateGroup(t *testing.T) {
	f := newFixture()
	g, err := f.svc.CreateGroup(ctx, admin, "  Cousins ")
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "Cousins" || g.Admins || g.ID == uuid.Nil || f.store.groups[3] != g {
		t.Fatalf("CreateGroup = %+v, stored %+v", g, f.store.groups)
	}
	if _, err := f.svc.CreateGroup(ctx, admin, " "); !errors.Is(err, domain.ErrInvalidGroupName) {
		t.Errorf("an empty name = %v", err)
	}
	f.store.fail["CreateGroup"] = app.ErrNameTaken
	if _, err := f.svc.CreateGroup(ctx, admin, "Family"); !errors.Is(err, app.ErrNameTaken) {
		t.Errorf("a taken name = %v", err)
	}
	broken := app.New(f.store, catalog{}, f.people, bytes.NewReader(nil))
	if _, err := broken.CreateGroup(ctx, admin, "Cousins"); err == nil {
		t.Error("no randomness made a Group")
	}
}

func TestRenameGroup(t *testing.T) {
	f := newFixture()
	g, err := f.svc.RenameGroup(ctx, admin, family.ID, " Kin ")
	if err != nil || g.Name != "Kin" || g.ID != family.ID {
		t.Fatalf("RenameGroup = %+v, %v", g, err)
	}
	if _, err := f.svc.RenameGroup(ctx, admin, family.ID, ""); !errors.Is(err, domain.ErrInvalidGroupName) {
		t.Errorf("an empty name = %v", err)
	}
	if _, err := f.svc.RenameGroup(ctx, admin, uuid.New(), "Kin"); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("an unknown Group = %v", err)
	}
	f.store.fail["Group"] = errBoom
	if _, err := f.svc.RenameGroup(ctx, admin, family.ID, "Kin"); !errors.Is(err, errBoom) {
		t.Errorf("when reading back fails = %v", err)
	}
}

func TestDeleteGroup(t *testing.T) {
	f := newFixture()
	if err := f.svc.DeleteGroup(ctx, admin, family.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Group(ctx, family.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("the Group is still there")
	}
	if err := f.svc.DeleteGroup(ctx, admin, admins.ID); !errors.Is(err, app.ErrAdminsGroup) {
		t.Errorf("deleting the admins Group = %v", err)
	}
	if err := f.svc.DeleteGroup(ctx, admin, family.ID); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("deleting it twice = %v", err)
	}
}

func TestMembers(t *testing.T) {
	f := newFixture()
	m := domain.Membership{GroupID: family.ID, UserID: ada}
	if err := f.svc.AddMember(ctx, admin, m); err != nil || f.store.members[1] != m {
		t.Fatalf("AddMember = %v, %+v", err, f.store.members)
	}
	if err := f.svc.RemoveMember(ctx, admin, m); err != nil || len(f.store.members) != 1 {
		t.Fatalf("RemoveMember = %v, %+v", err, f.store.members)
	}
	f.store.fail["AddMember"] = app.ErrNotFound
	if err := f.svc.AddMember(ctx, admin, m); !errors.Is(err, app.ErrNotFound) {
		t.Errorf("adding to nothing = %v", err)
	}
	f.store.fail["RemoveMember"] = app.ErrLastAdmin
	if err := f.svc.RemoveMember(ctx, admin, domain.Membership{GroupID: admins.ID, UserID: admin}); !errors.Is(err, app.ErrLastAdmin) {
		t.Errorf("removing the last Admin = %v", err)
	}
}

func TestAssign(t *testing.T) {
	f := newFixture()
	a := domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "viewer"}
	if err := f.svc.Assign(ctx, admin, a); err != nil || f.store.assignments[0] != a {
		t.Fatalf("Assign = %v, %+v", err, f.store.assignments)
	}
	for _, tc := range []struct {
		name string
		a    domain.Assignment
		want error
	}{
		{"the admins Group", domain.Assignment{GroupID: admins.ID, ClientID: "grafana", Role: "user"}, app.ErrAdminsGroup},
		{"an unknown Group", domain.Assignment{GroupID: uuid.New(), ClientID: "grafana", Role: "user"}, app.ErrNotFound},
		{"a Removed Client", domain.Assignment{GroupID: family.ID, ClientID: "jellyfin", Role: "user"}, app.ErrUnknownClient},
		{"an undeclared role", domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "admin"}, app.ErrUnknownRole},
	} {
		if err := f.svc.Assign(ctx, admin, tc.a); !errors.Is(err, tc.want) {
			t.Errorf("assigning %s = %v, want %v", tc.name, err, tc.want)
		}
	}
	if len(f.store.assignments) != 1 {
		t.Fatalf("a refused Assignment was kept: %+v", f.store.assignments)
	}
	if err := f.svc.Unassign(ctx, admin, a); err != nil || len(f.store.assignments) != 0 {
		t.Fatalf("Unassign = %v, %+v", err, f.store.assignments)
	}
}

func TestRolesAddUp(t *testing.T) {
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
	if got, _ := f.svc.Roles(ctx, admin, "grafana"); len(got) != 0 {
		t.Fatalf("an Admin with no Assignment holds %v", got)
	}
}

func TestIsAdmin(t *testing.T) {
	f := newFixture()
	if yes, err := f.svc.IsAdmin(ctx, admin); !yes || err != nil {
		t.Fatalf("IsAdmin(admin) = %v, %v", yes, err)
	}
	if yes, _ := f.svc.IsAdmin(ctx, ada); yes {
		t.Fatal("IsAdmin(ada) = true")
	}
}

func TestPromoteMakesTheFirstAdminOnly(t *testing.T) {
	f := newFixture()
	f.store.members = nil
	if err := f.svc.Promote(ctx, ada, false); err != nil {
		t.Fatal(err)
	}
	if yes, _ := f.svc.IsAdmin(ctx, ada); !yes {
		t.Fatal("the first promote made no Admin")
	}
	if err := f.svc.Promote(ctx, admin, false); !errors.Is(err, app.ErrAdminExists) {
		t.Fatalf("a second promote without force = %v, want ErrAdminExists", err)
	}
	if yes, _ := f.svc.IsAdmin(ctx, admin); yes {
		t.Fatal("a refused promote made an Admin")
	}
	if err := f.svc.Promote(ctx, admin, true); err != nil {
		t.Fatalf("a forced promote = %v", err)
	}
	if yes, _ := f.svc.IsAdmin(ctx, admin); !yes {
		t.Fatal("a forced promote made no Admin")
	}
	for _, name := range []string{"CountAdmins", "AdminsGroup", "AddMember"} {
		f.store.fail = map[string]error{name: errBoom}
		if err := f.svc.Promote(ctx, uuid.New(), true); !errors.Is(err, errBoom) {
			t.Errorf("Promote when %s fails = %v", name, err)
		}
	}
}

func TestUnassignedCountsUsersInNoGroup(t *testing.T) {
	f := newFixture()
	if n, err := f.svc.Unassigned(ctx); n != 7 || err != nil {
		t.Fatalf("Unassigned = %d, %v", n, err)
	}
}
