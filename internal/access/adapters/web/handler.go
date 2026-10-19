// Package web is the access context's inbound HTTP adapter: it implements the access operations
// of the generated ogen Handler over the use cases, for the signed-in User.
package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/app"
	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/httpx"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
)

// UseCases is what the adapter needs from the application layer. Each refuses anyone but an Admin.
type UseCases interface {
	Overview(ctx context.Context, actor app.Actor) (app.Overview, error)
	CreateGroup(ctx context.Context, actor app.Actor, name string) (domain.Group, error)
	RenameGroup(ctx context.Context, actor app.Actor, id uuid.UUID, name string) (domain.Group, error)
	DeleteGroup(ctx context.Context, actor app.Actor, id uuid.UUID) error
	AddMember(ctx context.Context, actor app.Actor, m domain.Membership) error
	RemoveMember(ctx context.Context, actor app.Actor, m domain.Membership) error
	Assign(ctx context.Context, actor app.Actor, a domain.Assignment) error
	Unassign(ctx context.Context, actor app.Actor, a domain.Assignment) error
	AssignUser(ctx context.Context, actor app.Actor, a domain.UserAssignment) error
	UnassignUser(ctx context.Context, actor app.Actor, a domain.UserAssignment) error
	SignOutUser(ctx context.Context, actor app.Actor, userID uuid.UUID) error
	SetTwoFactor(ctx context.Context, actor app.Actor, id uuid.UUID, required bool) error
	MachineUseCases
}

// Access implements the access operations.
type Access struct{ uc UseCases }

// New returns an Access calling uc.
func New(uc UseCases) *Access { return &Access{uc: uc} }

// GetAccess implements getAccess.
func (h *Access) GetAccess(ctx context.Context) (oas.GetAccessRes, error) {
	o, err := h.uc.Overview(ctx, actor(ctx))
	if err != nil {
		return refusal(err)
	}
	members := map[uuid.UUID][]oas.ID{}
	for _, m := range o.Memberships {
		members[m.GroupID] = append(members[m.GroupID], oas.ID(m.UserID))
	}
	res := &oas.AccessOverview{
		Groups:          make([]oas.Group, 0, len(o.Groups)),
		Users:           make([]oas.Person, 0, len(o.People)),
		Clients:         clients(o.Clients),
		Assignments:     make([]oas.Assignment, 0, len(o.Assignments)),
		UserAssignments: make([]oas.UserAssignment, 0, len(o.UserAssignments)),
	}
	for _, g := range o.Groups {
		res.Groups = append(res.Groups, group(g, members[g.ID]))
	}
	for _, p := range o.People {
		res.Users = append(res.Users, oas.Person{ID: oas.ID(p.ID), Email: oas.Email(p.Email), Name: p.Name, TwoFactor: p.TwoFactor})
	}
	for _, a := range o.Assignments {
		res.Assignments = append(res.Assignments, oas.Assignment{
			GroupId: oas.ID(a.GroupID), ClientId: oas.ClientId(a.ClientID), Role: oas.AppRole(a.Role),
		})
	}
	for _, a := range o.UserAssignments {
		res.UserAssignments = append(res.UserAssignments, oas.UserAssignment{
			UserId: oas.ID(a.UserID), ClientId: oas.ClientId(a.ClientID), Role: oas.AppRole(a.Role),
		})
	}
	return res, nil
}

// CreateGroup implements createGroup.
func (h *Access) CreateGroup(ctx context.Context, req *oas.GroupDraft) (oas.CreateGroupRes, error) {
	g, err := h.uc.CreateGroup(ctx, actor(ctx), req.Name)
	if err != nil {
		return refusal(err)
	}
	return new(group(g, nil)), nil
}

// RenameGroup implements renameGroup.
func (h *Access) RenameGroup(ctx context.Context, req *oas.GroupDraft, p oas.RenameGroupParams) (oas.RenameGroupRes, error) {
	if _, err := h.uc.RenameGroup(ctx, actor(ctx), uuid.UUID(p.GroupId), req.Name); err != nil {
		return refusal(err)
	}
	return &oas.RenameGroupNoContent{}, nil
}

// DeleteGroup implements deleteGroup.
func (h *Access) DeleteGroup(ctx context.Context, p oas.DeleteGroupParams) (oas.DeleteGroupRes, error) {
	if err := h.uc.DeleteGroup(ctx, actor(ctx), uuid.UUID(p.GroupId)); err != nil {
		return refusal(err)
	}
	return &oas.DeleteGroupNoContent{}, nil
}

// AddMember implements addMember.
func (h *Access) AddMember(ctx context.Context, p oas.AddMemberParams) (oas.AddMemberRes, error) {
	m := domain.Membership{GroupID: uuid.UUID(p.GroupId), UserID: uuid.UUID(p.UserId)}
	if err := h.uc.AddMember(ctx, actor(ctx), m); err != nil {
		return refusal(err)
	}
	return &oas.AddMemberNoContent{}, nil
}

// RemoveMember implements removeMember.
func (h *Access) RemoveMember(ctx context.Context, p oas.RemoveMemberParams) (oas.RemoveMemberRes, error) {
	m := domain.Membership{GroupID: uuid.UUID(p.GroupId), UserID: uuid.UUID(p.UserId)}
	if err := h.uc.RemoveMember(ctx, actor(ctx), m); err != nil {
		return refusal(err)
	}
	return &oas.RemoveMemberNoContent{}, nil
}

// SignOutUser implements signOutUser.
func (h *Access) SignOutUser(ctx context.Context, p oas.SignOutUserParams) (oas.SignOutUserRes, error) {
	if err := h.uc.SignOutUser(ctx, actor(ctx), uuid.UUID(p.UserId)); err != nil {
		return refusal(err)
	}
	return &oas.SignOutUserNoContent{}, nil
}

// AssignRole implements assignRole.
func (h *Access) AssignRole(ctx context.Context, p oas.AssignRoleParams) (oas.AssignRoleRes, error) {
	a := domain.Assignment{GroupID: uuid.UUID(p.GroupId), ClientID: string(p.ClientId), Role: string(p.Role)}
	if err := h.uc.Assign(ctx, actor(ctx), a); err != nil {
		return refusal(err)
	}
	return &oas.AssignRoleNoContent{}, nil
}

// UnassignRole implements unassignRole.
func (h *Access) UnassignRole(ctx context.Context, p oas.UnassignRoleParams) (oas.UnassignRoleRes, error) {
	a := domain.Assignment{GroupID: uuid.UUID(p.GroupId), ClientID: string(p.ClientId), Role: string(p.Role)}
	if err := h.uc.Unassign(ctx, actor(ctx), a); err != nil {
		return refusal(err)
	}
	return &oas.UnassignRoleNoContent{}, nil
}

// AssignUserRole implements assignUserRole.
func (h *Access) AssignUserRole(ctx context.Context, p oas.AssignUserRoleParams) (oas.AssignUserRoleRes, error) {
	a := domain.UserAssignment{UserID: uuid.UUID(p.UserId), ClientID: string(p.ClientId), Role: string(p.Role)}
	if err := h.uc.AssignUser(ctx, actor(ctx), a); err != nil {
		return refusal(err)
	}
	return &oas.AssignUserRoleNoContent{}, nil
}

// UnassignUserRole implements unassignUserRole.
func (h *Access) UnassignUserRole(ctx context.Context, p oas.UnassignUserRoleParams) (oas.UnassignUserRoleRes, error) {
	a := domain.UserAssignment{UserID: uuid.UUID(p.UserId), ClientID: string(p.ClientId), Role: string(p.Role)}
	if err := h.uc.UnassignUser(ctx, actor(ctx), a); err != nil {
		return refusal(err)
	}
	return &oas.UnassignUserRoleNoContent{}, nil
}

// SetGroupTwoFactor implements setGroupTwoFactor.
func (h *Access) SetGroupTwoFactor(ctx context.Context, req *oas.TwoFactorRequirement, p oas.SetGroupTwoFactorParams) (oas.SetGroupTwoFactorRes, error) {
	if err := h.uc.SetTwoFactor(ctx, actor(ctx), uuid.UUID(p.GroupId), req.Required); err != nil {
		return refusal(err)
	}
	return &oas.SetGroupTwoFactorNoContent{}, nil
}

func clients(cs []domain.Client) []oas.AccessClient {
	out := make([]oas.AccessClient, 0, len(cs))
	for _, c := range cs {
		roles := make([]oas.AppRole, 0, len(c.Roles))
		for _, r := range c.Roles {
			roles = append(roles, oas.AppRole(r))
		}
		out = append(out, oas.AccessClient{ID: oas.ClientId(c.ID), Label: c.Label, Roles: roles})
	}
	return out
}

func actor(ctx context.Context) app.Actor {
	p := principal.From(ctx)
	return app.Actor{ID: p.UserID, SecondFactor: p.SecondFactor}
}

func group(g domain.Group, members []oas.ID) oas.Group {
	if members == nil {
		members = []oas.ID{}
	}
	return oas.Group{ID: oas.ID(g.ID), Name: g.Name, Admins: g.Admins, RequiresTwoFactor: g.RequiresTwoFactor, Members: members}
}

// refusal answers a rule the caller broke with a problem saying which, and passes anything else on
// to NewError as a 500. Its result type fits every operation's response union.
func refusal(err error) (*oas.ProblemStatusCode, error) {
	switch {
	case errors.Is(err, app.ErrNotAdmin):
		return problem(http.StatusForbidden, "Only Admins manage access."), nil
	case errors.Is(err, app.ErrSecondFactorNeeded):
		return problem(http.StatusForbidden, "Sign out, then sign in again with a passkey or authenticator app to manage access. If you have neither, add one on the Security page first."), nil
	case errors.Is(err, app.ErrNotFound):
		return problem(http.StatusNotFound, "That Group or person no longer exists. Reload the page."), nil
	case errors.Is(err, app.ErrNameTaken):
		return problem(http.StatusConflict, "A Group with that name already exists. Choose another name."), nil
	case errors.Is(err, domain.ErrInvalidGroupName):
		return problem(http.StatusUnprocessableEntity, "Enter a Group name of 1 to 64 characters."), nil
	case errors.Is(err, app.ErrAdminsGroup):
		return problem(http.StatusUnprocessableEntity, "The admins Group cannot be deleted or given a role."), nil
	case errors.Is(err, app.ErrLastAdmin):
		return problem(http.StatusUnprocessableEntity, "Add another Admin before you remove the last one."), nil
	case errors.Is(err, app.ErrUnknownClient):
		return problem(http.StatusUnprocessableEntity, "That app is not registered."), nil
	case errors.Is(err, app.ErrAdminsNeedTwoFactor):
		return problem(http.StatusUnprocessableEntity, "The admins Group always requires two-factor."), nil
	case errors.Is(err, app.ErrUnknownRole):
		return problem(http.StatusUnprocessableEntity, "That app has no such role."), nil
	default:
		return nil, err
	}
}

func problem(status int, detail string) *oas.ProblemStatusCode {
	return &oas.ProblemStatusCode{StatusCode: status, Response: httpx.Problem(status, detail)}
}
