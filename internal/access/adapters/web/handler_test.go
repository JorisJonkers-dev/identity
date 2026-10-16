package web_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/access/app"
	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
)

var errBoom = errors.New("boom")

// useCases answers with what a test sets and records who asked for what.
type useCases struct {
	err      error
	overview app.Overview
	group    domain.Group
	actor    uuid.UUID
	asked    []any
}

func (u *useCases) Overview(_ context.Context, actor uuid.UUID) (app.Overview, error) {
	u.actor = actor
	return u.overview, u.err
}

func (u *useCases) CreateGroup(_ context.Context, actor uuid.UUID, name string) (domain.Group, error) {
	u.actor, u.asked = actor, append(u.asked, name)
	return u.group, u.err
}

func (u *useCases) RenameGroup(_ context.Context, actor, id uuid.UUID, name string) (domain.Group, error) {
	u.actor, u.asked = actor, append(u.asked, id, name)
	return u.group, u.err
}

func (u *useCases) DeleteGroup(_ context.Context, actor, id uuid.UUID) error {
	u.actor, u.asked = actor, append(u.asked, id)
	return u.err
}

func (u *useCases) AddMember(_ context.Context, actor uuid.UUID, m domain.Membership) error {
	u.actor, u.asked = actor, append(u.asked, m)
	return u.err
}

func (u *useCases) RemoveMember(_ context.Context, actor uuid.UUID, m domain.Membership) error {
	u.actor, u.asked = actor, append(u.asked, m)
	return u.err
}

func (u *useCases) Assign(_ context.Context, actor uuid.UUID, a domain.Assignment) error {
	u.actor, u.asked = actor, append(u.asked, a)
	return u.err
}

func (u *useCases) Unassign(_ context.Context, actor uuid.UUID, a domain.Assignment) error {
	u.actor, u.asked = actor, append(u.asked, a)
	return u.err
}

var (
	admin  = uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
	ada    = uuid.MustParse("00000000-0000-4000-8000-0000000000a2")
	family = uuid.MustParse("00000000-0000-4000-8000-0000000000b2")
)

func signedIn() context.Context { return principal.With(context.Background(), admin) }

func wantProblem(t *testing.T, res any, err error, status int, detail string) {
	t.Helper()
	p, ok := res.(*oas.ProblemStatusCode)
	if err != nil || !ok || p.StatusCode != status || p.Response.Detail.Value != detail {
		t.Fatalf("got %#v, %v; want %d %q", res, err, status, detail)
	}
}

func TestGetAccess(t *testing.T) {
	uc := &useCases{overview: app.Overview{
		Groups:      []domain.Group{{ID: family, Name: "Family"}, {ID: uuid.Nil, Name: "Empty"}},
		Memberships: []domain.Membership{{GroupID: family, UserID: ada}},
		Assignments: []domain.Assignment{{GroupID: family, ClientID: "grafana", Role: "viewer"}},
		Clients:     []domain.Client{{ID: "grafana", Label: "Grafana", Roles: []string{"user", "viewer"}}},
		People:      []domain.Person{{ID: ada, Email: "ada@example.org", Name: "Ada"}},
	}}
	res, err := web.New(uc).GetAccess(signedIn())
	o, ok := res.(*oas.AccessOverview)
	if err != nil || !ok || uc.actor != admin {
		t.Fatalf("GetAccess = %#v, %v (actor %v)", res, err, uc.actor)
	}
	if len(o.Groups) != 2 || o.Groups[0].Name != "Family" || len(o.Groups[0].Members) != 1 || uuid.UUID(o.Groups[0].Members[0]) != ada ||
		o.Groups[1].Members == nil || len(o.Groups[1].Members) != 0 {
		t.Fatalf("groups = %+v", o.Groups)
	}
	if len(o.Users) != 1 || o.Users[0].Email != "ada@example.org" || o.Users[0].Name != "Ada" || uuid.UUID(o.Users[0].ID) != ada {
		t.Fatalf("users = %+v", o.Users)
	}
	if len(o.Clients) != 1 || o.Clients[0].Label != "Grafana" || len(o.Clients[0].Roles) != 2 || o.Clients[0].Roles[1] != "viewer" {
		t.Fatalf("clients = %+v", o.Clients)
	}
	if len(o.Assignments) != 1 || o.Assignments[0] != (oas.Assignment{GroupId: oas.ID(family), ClientId: "grafana", Role: "viewer"}) {
		t.Fatalf("assignments = %+v", o.Assignments)
	}
}

func TestGroups(t *testing.T) {
	uc := &useCases{group: domain.Group{ID: family, Name: "Family"}}
	h := web.New(uc)
	res, err := h.CreateGroup(signedIn(), &oas.GroupDraft{Name: " Family "})
	g, ok := res.(*oas.Group)
	if err != nil || !ok || g.Name != "Family" || uuid.UUID(g.ID) != family || g.Members == nil || uc.asked[0] != " Family " {
		t.Fatalf("CreateGroup = %#v, %v", res, err)
	}
	if res, err := h.RenameGroup(signedIn(), &oas.GroupDraft{Name: "Kin"}, oas.RenameGroupParams{GroupId: oas.ID(family)}); err != nil {
		t.Fatal(err)
	} else if _, ok := res.(*oas.RenameGroupNoContent); !ok || uc.asked[1] != family || uc.asked[2] != "Kin" {
		t.Fatalf("RenameGroup = %#v, asked %v", res, uc.asked)
	}
	if res, err := h.DeleteGroup(signedIn(), oas.DeleteGroupParams{GroupId: oas.ID(family)}); err != nil {
		t.Fatal(err)
	} else if _, ok := res.(*oas.DeleteGroupNoContent); !ok || uc.asked[3] != family || uc.actor != admin {
		t.Fatalf("DeleteGroup = %#v, asked %v", res, uc.asked)
	}
}

func TestMembersAndRoles(t *testing.T) {
	uc := &useCases{}
	h := web.New(uc)
	m := domain.Membership{GroupID: family, UserID: ada}
	a := domain.Assignment{GroupID: family, ClientID: "grafana", Role: "viewer"}
	for name, call := range map[string]func() (any, any, error){
		"AddMember": func() (any, any, error) {
			res, err := h.AddMember(signedIn(), oas.AddMemberParams{GroupId: oas.ID(family), UserId: oas.ID(ada)})
			_, ok := res.(*oas.AddMemberNoContent)
			return ok, m, err
		},
		"RemoveMember": func() (any, any, error) {
			res, err := h.RemoveMember(signedIn(), oas.RemoveMemberParams{GroupId: oas.ID(family), UserId: oas.ID(ada)})
			_, ok := res.(*oas.RemoveMemberNoContent)
			return ok, m, err
		},
		"AssignRole": func() (any, any, error) {
			res, err := h.AssignRole(signedIn(), oas.AssignRoleParams{GroupId: oas.ID(family), ClientId: "grafana", Role: "viewer"})
			_, ok := res.(*oas.AssignRoleNoContent)
			return ok, a, err
		},
		"UnassignRole": func() (any, any, error) {
			res, err := h.UnassignRole(signedIn(), oas.UnassignRoleParams{GroupId: oas.ID(family), ClientId: "grafana", Role: "viewer"})
			_, ok := res.(*oas.UnassignRoleNoContent)
			return ok, a, err
		},
	} {
		uc.asked, uc.actor = nil, uuid.Nil
		ok, want, err := call()
		if err != nil || ok != true || len(uc.asked) != 1 || uc.asked[0] != want || uc.actor != admin {
			t.Errorf("%s = %v, %v; asked %v by %v", name, ok, err, uc.asked, uc.actor)
		}
	}
}

func TestRefusals(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		detail string
	}{
		{app.ErrNotAdmin, http.StatusForbidden, "Only Admins manage access."},
		{app.ErrNotFound, http.StatusNotFound, "That Group or User is not there any more."},
		{app.ErrNameTaken, http.StatusConflict, "A Group with that name exists."},
		{domain.ErrInvalidGroupName, http.StatusUnprocessableEntity, "A Group name is 1 to 64 characters."},
		{app.ErrAdminsGroup, http.StatusUnprocessableEntity, "The admins Group cannot be deleted or given an App Role."},
		{app.ErrLastAdmin, http.StatusUnprocessableEntity, "The last Admin cannot leave the admins Group."},
		{app.ErrUnknownClient, http.StatusUnprocessableEntity, "That Client is not in the cluster."},
		{app.ErrUnknownRole, http.StatusUnprocessableEntity, "That Client declares no such App Role."},
	} {
		h := web.New(&useCases{err: tc.err})
		ctx := signedIn()
		gid := oas.ID(family)
		calls := map[string]func() (any, error){
			"GetAccess":   func() (any, error) { return h.GetAccess(ctx) },
			"CreateGroup": func() (any, error) { return h.CreateGroup(ctx, &oas.GroupDraft{Name: "x"}) },
			"RenameGroup": func() (any, error) {
				return h.RenameGroup(ctx, &oas.GroupDraft{Name: "x"}, oas.RenameGroupParams{GroupId: gid})
			},
			"DeleteGroup":  func() (any, error) { return h.DeleteGroup(ctx, oas.DeleteGroupParams{GroupId: gid}) },
			"AddMember":    func() (any, error) { return h.AddMember(ctx, oas.AddMemberParams{GroupId: gid}) },
			"RemoveMember": func() (any, error) { return h.RemoveMember(ctx, oas.RemoveMemberParams{GroupId: gid}) },
			"AssignRole":   func() (any, error) { return h.AssignRole(ctx, oas.AssignRoleParams{GroupId: gid}) },
			"UnassignRole": func() (any, error) { return h.UnassignRole(ctx, oas.UnassignRoleParams{GroupId: gid}) },
		}
		for name, call := range calls {
			t.Run(name, func(t *testing.T) {
				res, err := call()
				wantProblem(t, res, err, tc.status, tc.detail)
			})
		}
	}
	h := web.New(&useCases{err: errBoom})
	if _, err := h.GetAccess(signedIn()); !errors.Is(err, errBoom) {
		t.Fatalf("an unexpected failure = %v, want it passed on as a 500", err)
	}
}
