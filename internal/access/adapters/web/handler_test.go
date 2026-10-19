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
	machines app.MachinesOverview
	machine  domain.Machine
	actor    app.Actor
	asked    []any
}

func (u *useCases) Overview(_ context.Context, actor app.Actor) (app.Overview, error) {
	u.actor = actor
	return u.overview, u.err
}

func (u *useCases) CreateGroup(_ context.Context, actor app.Actor, name string) (domain.Group, error) {
	u.actor, u.asked = actor, append(u.asked, name)
	return u.group, u.err
}

func (u *useCases) RenameGroup(_ context.Context, actor app.Actor, id uuid.UUID, name string) (domain.Group, error) {
	u.actor, u.asked = actor, append(u.asked, id, name)
	return u.group, u.err
}

func (u *useCases) DeleteGroup(_ context.Context, actor app.Actor, id uuid.UUID) error {
	u.actor, u.asked = actor, append(u.asked, id)
	return u.err
}

func (u *useCases) AddMember(_ context.Context, actor app.Actor, m domain.Membership) error {
	u.actor, u.asked = actor, append(u.asked, m)
	return u.err
}

func (u *useCases) RemoveMember(_ context.Context, actor app.Actor, m domain.Membership) error {
	u.actor, u.asked = actor, append(u.asked, m)
	return u.err
}

func (u *useCases) Assign(_ context.Context, actor app.Actor, a domain.Assignment) error {
	u.actor, u.asked = actor, append(u.asked, a)
	return u.err
}

func (u *useCases) Unassign(_ context.Context, actor app.Actor, a domain.Assignment) error {
	u.actor, u.asked = actor, append(u.asked, a)
	return u.err
}

func (u *useCases) AssignUser(_ context.Context, actor app.Actor, a domain.UserAssignment) error {
	u.actor, u.asked = actor, append(u.asked, a)
	return u.err
}

func (u *useCases) UnassignUser(_ context.Context, actor app.Actor, a domain.UserAssignment) error {
	u.actor, u.asked = actor, append(u.asked, a)
	return u.err
}

func (u *useCases) SetTwoFactor(_ context.Context, actor app.Actor, id uuid.UUID, required bool) error {
	u.actor, u.asked = actor, append(u.asked, id, required)
	return u.err
}

func (u *useCases) SignOutUser(_ context.Context, actor app.Actor, userID uuid.UUID) error {
	u.actor, u.asked = actor, append(u.asked, userID)
	return u.err
}

var (
	admin  = uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
	ada    = uuid.MustParse("00000000-0000-4000-8000-0000000000a2")
	family = uuid.MustParse("00000000-0000-4000-8000-0000000000b2")
)

func signedIn() context.Context {
	return principal.With(context.Background(), principal.Principal{UserID: admin, SecondFactor: true})
}

func wantProblem(t *testing.T, res any, err error, status int, detail string) {
	t.Helper()
	p, ok := res.(*oas.ProblemStatusCode)
	if err != nil || !ok || p.StatusCode != status || p.Response.Detail.Value != detail {
		t.Fatalf("got %#v, %v; want %d %q", res, err, status, detail)
	}
}

var asAdmin = app.Actor{ID: admin, SecondFactor: true}

func TestTheOverviewCarriesGroupsWithTheirMembersPeopleClientsAndAssignments(t *testing.T) {
	uc := &useCases{overview: app.Overview{
		Groups:          []domain.Group{{ID: family, Name: "Family"}},
		Memberships:     []domain.Membership{{GroupID: family, UserID: ada}},
		Assignments:     []domain.Assignment{{GroupID: family, ClientID: "grafana", Role: "viewer"}},
		UserAssignments: []domain.UserAssignment{{UserID: ada, ClientID: "grafana", Role: "user"}},
		Clients:         []domain.Client{{ID: "grafana", Label: "Grafana", Roles: []string{"user", "viewer"}}},
		People:          []domain.Person{{ID: ada, Email: "ada@example.org", Name: "Ada"}},
	}}
	res, err := web.New(uc).GetAccess(signedIn())
	o, ok := res.(*oas.AccessOverview)
	if err != nil || !ok || uc.actor != asAdmin {
		t.Fatalf("GetAccess = %#v, %v (actor %v)", res, err, uc.actor)
	}
	if len(o.Groups) != 1 || o.Groups[0].Name != "Family" || len(o.Groups[0].Members) != 1 || uuid.UUID(o.Groups[0].Members[0]) != ada {
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
	if len(o.UserAssignments) != 1 || o.UserAssignments[0] != (oas.UserAssignment{UserId: oas.ID(ada), ClientId: "grafana", Role: "user"}) {
		t.Fatalf("user assignments = %+v", o.UserAssignments)
	}
}

func TestAGroupWithNoMembersIsSentWithAnEmptyList(t *testing.T) {
	uc := &useCases{overview: app.Overview{Groups: []domain.Group{{ID: family, Name: "Empty"}}}}
	res, _ := web.New(uc).GetAccess(signedIn())
	if g := res.(*oas.AccessOverview).Groups[0]; g.Members == nil || len(g.Members) != 0 {
		t.Fatalf("members = %#v, want an empty list", g.Members)
	}
}

func TestCreatingAGroupSendsTheNameAsTypedAndAnswersTheGroup(t *testing.T) {
	uc := &useCases{group: domain.Group{ID: family, Name: "Family"}}
	res, err := web.New(uc).CreateGroup(signedIn(), &oas.GroupDraft{Name: " Family "})
	g, ok := res.(*oas.Group)
	if err != nil || !ok || g.Name != "Family" || uuid.UUID(g.ID) != family || g.Members == nil || uc.asked[0] != " Family " || uc.actor != asAdmin {
		t.Fatalf("CreateGroup = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestRenamingAGroupSendsItsIDAndTheNewName(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).RenameGroup(signedIn(), &oas.GroupDraft{Name: "Kin"}, oas.RenameGroupParams{GroupId: oas.ID(family)})
	if _, ok := res.(*oas.RenameGroupNoContent); err != nil || !ok || uc.asked[0] != family || uc.asked[1] != "Kin" || uc.actor != asAdmin {
		t.Fatalf("RenameGroup = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestDeletingAGroupSendsItsID(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).DeleteGroup(signedIn(), oas.DeleteGroupParams{GroupId: oas.ID(family)})
	if _, ok := res.(*oas.DeleteGroupNoContent); err != nil || !ok || uc.asked[0] != family || uc.actor != asAdmin {
		t.Fatalf("DeleteGroup = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestAddingAMemberSendsTheGroupAndThePerson(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).AddMember(signedIn(), oas.AddMemberParams{GroupId: oas.ID(family), UserId: oas.ID(ada)})
	if _, ok := res.(*oas.AddMemberNoContent); err != nil || !ok || uc.asked[0] != (domain.Membership{GroupID: family, UserID: ada}) || uc.actor != asAdmin {
		t.Fatalf("AddMember = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestRemovingAMemberSendsTheGroupAndThePerson(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).RemoveMember(signedIn(), oas.RemoveMemberParams{GroupId: oas.ID(family), UserId: oas.ID(ada)})
	if _, ok := res.(*oas.RemoveMemberNoContent); err != nil || !ok || uc.asked[0] != (domain.Membership{GroupID: family, UserID: ada}) || uc.actor != asAdmin {
		t.Fatalf("RemoveMember = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestGivingAnAppRoleSendsTheGroupClientAndRole(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).AssignRole(signedIn(), oas.AssignRoleParams{GroupId: oas.ID(family), ClientId: "grafana", Role: "viewer"})
	want := domain.Assignment{GroupID: family, ClientID: "grafana", Role: "viewer"}
	if _, ok := res.(*oas.AssignRoleNoContent); err != nil || !ok || uc.asked[0] != want || uc.actor != asAdmin {
		t.Fatalf("AssignRole = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestTakingAnAppRoleSendsTheGroupClientAndRole(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).UnassignRole(signedIn(), oas.UnassignRoleParams{GroupId: oas.ID(family), ClientId: "grafana", Role: "viewer"})
	want := domain.Assignment{GroupID: family, ClientID: "grafana", Role: "viewer"}
	if _, ok := res.(*oas.UnassignRoleNoContent); err != nil || !ok || uc.asked[0] != want || uc.actor != asAdmin {
		t.Fatalf("UnassignRole = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestGivingAPersonAnAppRoleSendsThePersonClientAndRole(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).AssignUserRole(signedIn(), oas.AssignUserRoleParams{UserId: oas.ID(ada), ClientId: "grafana", Role: "viewer"})
	want := domain.UserAssignment{UserID: ada, ClientID: "grafana", Role: "viewer"}
	if _, ok := res.(*oas.AssignUserRoleNoContent); err != nil || !ok || uc.asked[0] != want || uc.actor != asAdmin {
		t.Fatalf("AssignUserRole = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestTakingAPersonsAppRoleSendsThePersonClientAndRole(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).UnassignUserRole(signedIn(), oas.UnassignUserRoleParams{UserId: oas.ID(ada), ClientId: "grafana", Role: "viewer"})
	want := domain.UserAssignment{UserID: ada, ClientID: "grafana", Role: "viewer"}
	if _, ok := res.(*oas.UnassignUserRoleNoContent); err != nil || !ok || uc.asked[0] != want || uc.actor != asAdmin {
		t.Fatalf("UnassignUserRole = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestSigningAUserOutSendsWhoToSignOut(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).SignOutUser(signedIn(), oas.SignOutUserParams{UserId: oas.ID(ada)})
	if _, ok := res.(*oas.SignOutUserNoContent); err != nil || !ok || uc.asked[0] != ada || uc.actor != asAdmin {
		t.Fatalf("SignOutUser = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestEachRefusalBecomesAProblemSayingWhatToDo(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		detail string
	}{
		{app.ErrNotAdmin, http.StatusForbidden, "Only Admins manage access."},
		{app.ErrSecondFactorNeeded, http.StatusForbidden, "Sign out, then sign in again with a passkey or authenticator app to manage access. If you have neither, add one on the Security page first."},
		{app.ErrNotFound, http.StatusNotFound, "That Group or person no longer exists. Reload the page."},
		{app.ErrNameTaken, http.StatusConflict, "A Group with that name already exists. Choose another name."},
		{domain.ErrInvalidGroupName, http.StatusUnprocessableEntity, "Enter a Group name of 1 to 64 characters."},
		{app.ErrAdminsGroup, http.StatusUnprocessableEntity, "The admins Group cannot be deleted or given a role."},
		{app.ErrLastAdmin, http.StatusUnprocessableEntity, "Add another Admin before you remove the last one."},
		{app.ErrUnknownClient, http.StatusUnprocessableEntity, "That app is not registered."},
		{app.ErrUnknownRole, http.StatusUnprocessableEntity, "That app has no such role."},
		{app.ErrAdminsNeedTwoFactor, http.StatusUnprocessableEntity, "The admins Group always requires two-factor."},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			res, err := web.New(&useCases{err: tc.err}).GetAccess(signedIn())
			wantProblem(t, res, err, tc.status, tc.detail)
		})
	}
}

func TestEveryOperationAnswersARefusalAsAProblem(t *testing.T) {
	gid := oas.ID(family)
	for _, op := range []struct {
		name string
		call func(h *web.Access) (any, error)
	}{
		{"reading the overview", func(h *web.Access) (any, error) { return h.GetAccess(signedIn()) }},
		{"creating a Group", func(h *web.Access) (any, error) { return h.CreateGroup(signedIn(), &oas.GroupDraft{Name: "x"}) }},
		{"renaming a Group", func(h *web.Access) (any, error) {
			return h.RenameGroup(signedIn(), &oas.GroupDraft{Name: "x"}, oas.RenameGroupParams{GroupId: gid})
		}},
		{"deleting a Group", func(h *web.Access) (any, error) {
			return h.DeleteGroup(signedIn(), oas.DeleteGroupParams{GroupId: gid})
		}},
		{"adding a member", func(h *web.Access) (any, error) { return h.AddMember(signedIn(), oas.AddMemberParams{GroupId: gid}) }},
		{"removing a member", func(h *web.Access) (any, error) {
			return h.RemoveMember(signedIn(), oas.RemoveMemberParams{GroupId: gid})
		}},
		{"giving an App Role", func(h *web.Access) (any, error) { return h.AssignRole(signedIn(), oas.AssignRoleParams{GroupId: gid}) }},
		{"taking an App Role", func(h *web.Access) (any, error) {
			return h.UnassignRole(signedIn(), oas.UnassignRoleParams{GroupId: gid})
		}},
		{"signing a User out", func(h *web.Access) (any, error) { return h.SignOutUser(signedIn(), oas.SignOutUserParams{}) }},
		{"making a Group require two-factor", func(h *web.Access) (any, error) {
			return h.SetGroupTwoFactor(signedIn(), &oas.TwoFactorRequirement{}, oas.SetGroupTwoFactorParams{GroupId: gid})
		}},
		{"giving a person an App Role", func(h *web.Access) (any, error) {
			return h.AssignUserRole(signedIn(), oas.AssignUserRoleParams{})
		}},
		{"taking a person's App Role", func(h *web.Access) (any, error) {
			return h.UnassignUserRole(signedIn(), oas.UnassignUserRoleParams{})
		}},
	} {
		t.Run(op.name, func(t *testing.T) {
			res, err := op.call(web.New(&useCases{err: app.ErrNotAdmin}))
			wantProblem(t, res, err, http.StatusForbidden, "Only Admins manage access.")
		})
	}
}

func TestAnUnexpectedFailureIsPassedOnForA500(t *testing.T) {
	if _, err := web.New(&useCases{err: errBoom}).GetAccess(signedIn()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want it passed on", err)
	}
}

func TestMakingAGroupRequireTwoFactorSendsTheGroupAndTheChoice(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).SetGroupTwoFactor(signedIn(), &oas.TwoFactorRequirement{Required: true}, oas.SetGroupTwoFactorParams{GroupId: oas.ID(family)})
	if _, ok := res.(*oas.SetGroupTwoFactorNoContent); err != nil || !ok || uc.asked[0] != family || uc.asked[1] != true || uc.actor != asAdmin {
		t.Fatalf("SetGroupTwoFactor = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestTheOverviewSaysWhichGroupsRequireTwoFactorAndWhoHasIt(t *testing.T) {
	uc := &useCases{overview: app.Overview{
		Groups: []domain.Group{{ID: family, Name: "Family", RequiresTwoFactor: true}},
		People: []domain.Person{{ID: ada, Email: "ada@example.org", Name: "Ada", TwoFactor: true}},
	}}
	res, _ := web.New(uc).GetAccess(signedIn())
	o := res.(*oas.AccessOverview)
	if !o.Groups[0].RequiresTwoFactor || !o.Users[0].TwoFactor {
		t.Fatalf("overview = %+v", o)
	}
}
