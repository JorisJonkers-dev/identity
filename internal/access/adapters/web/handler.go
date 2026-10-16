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
	Overview(ctx context.Context, actor uuid.UUID) (app.Overview, error)
	CreateGroup(ctx context.Context, actor uuid.UUID, name string) (domain.Group, error)
	RenameGroup(ctx context.Context, actor, id uuid.UUID, name string) (domain.Group, error)
	DeleteGroup(ctx context.Context, actor, id uuid.UUID) error
	AddMember(ctx context.Context, actor uuid.UUID, m domain.Membership) error
	RemoveMember(ctx context.Context, actor uuid.UUID, m domain.Membership) error
	Assign(ctx context.Context, actor uuid.UUID, a domain.Assignment) error
	Unassign(ctx context.Context, actor uuid.UUID, a domain.Assignment) error
}

// Access implements the access operations.
type Access struct{ uc UseCases }

// New returns an Access calling uc.
func New(uc UseCases) *Access { return &Access{uc: uc} }

// GetAccess implements getAccess.
func (h *Access) GetAccess(ctx context.Context) (oas.GetAccessRes, error) {
	o, err := h.uc.Overview(ctx, principal.From(ctx))
	if err != nil {
		return refusal(err)
	}
	members := map[uuid.UUID][]oas.ID{}
	for _, m := range o.Memberships {
		members[m.GroupID] = append(members[m.GroupID], oas.ID(m.UserID))
	}
	res := &oas.AccessOverview{
		Groups:      make([]oas.Group, 0, len(o.Groups)),
		Users:       make([]oas.Person, 0, len(o.People)),
		Clients:     make([]oas.AccessClient, 0, len(o.Clients)),
		Assignments: make([]oas.Assignment, 0, len(o.Assignments)),
	}
	for _, g := range o.Groups {
		res.Groups = append(res.Groups, group(g, members[g.ID]))
	}
	for _, p := range o.People {
		res.Users = append(res.Users, oas.Person{ID: oas.ID(p.ID), Email: oas.Email(p.Email), Name: p.Name})
	}
	for _, c := range o.Clients {
		roles := make([]oas.AppRole, 0, len(c.Roles))
		for _, r := range c.Roles {
			roles = append(roles, oas.AppRole(r))
		}
		res.Clients = append(res.Clients, oas.AccessClient{ID: oas.ClientId(c.ID), Label: c.Label, Roles: roles})
	}
	for _, a := range o.Assignments {
		res.Assignments = append(res.Assignments, oas.Assignment{
			GroupId: oas.ID(a.GroupID), ClientId: oas.ClientId(a.ClientID), Role: oas.AppRole(a.Role),
		})
	}
	return res, nil
}

// CreateGroup implements createGroup.
func (h *Access) CreateGroup(ctx context.Context, req *oas.GroupDraft) (oas.CreateGroupRes, error) {
	g, err := h.uc.CreateGroup(ctx, principal.From(ctx), req.Name)
	if err != nil {
		return refusal(err)
	}
	return new(group(g, nil)), nil
}

// RenameGroup implements renameGroup.
func (h *Access) RenameGroup(ctx context.Context, req *oas.GroupDraft, p oas.RenameGroupParams) (oas.RenameGroupRes, error) {
	if _, err := h.uc.RenameGroup(ctx, principal.From(ctx), uuid.UUID(p.GroupId), req.Name); err != nil {
		return refusal(err)
	}
	return &oas.RenameGroupNoContent{}, nil
}

// DeleteGroup implements deleteGroup.
func (h *Access) DeleteGroup(ctx context.Context, p oas.DeleteGroupParams) (oas.DeleteGroupRes, error) {
	if err := h.uc.DeleteGroup(ctx, principal.From(ctx), uuid.UUID(p.GroupId)); err != nil {
		return refusal(err)
	}
	return &oas.DeleteGroupNoContent{}, nil
}

// AddMember implements addMember.
func (h *Access) AddMember(ctx context.Context, p oas.AddMemberParams) (oas.AddMemberRes, error) {
	m := domain.Membership{GroupID: uuid.UUID(p.GroupId), UserID: uuid.UUID(p.UserId)}
	if err := h.uc.AddMember(ctx, principal.From(ctx), m); err != nil {
		return refusal(err)
	}
	return &oas.AddMemberNoContent{}, nil
}

// RemoveMember implements removeMember.
func (h *Access) RemoveMember(ctx context.Context, p oas.RemoveMemberParams) (oas.RemoveMemberRes, error) {
	m := domain.Membership{GroupID: uuid.UUID(p.GroupId), UserID: uuid.UUID(p.UserId)}
	if err := h.uc.RemoveMember(ctx, principal.From(ctx), m); err != nil {
		return refusal(err)
	}
	return &oas.RemoveMemberNoContent{}, nil
}

// AssignRole implements assignRole.
func (h *Access) AssignRole(ctx context.Context, p oas.AssignRoleParams) (oas.AssignRoleRes, error) {
	a := domain.Assignment{GroupID: uuid.UUID(p.GroupId), ClientID: string(p.ClientId), Role: string(p.Role)}
	if err := h.uc.Assign(ctx, principal.From(ctx), a); err != nil {
		return refusal(err)
	}
	return &oas.AssignRoleNoContent{}, nil
}

// UnassignRole implements unassignRole.
func (h *Access) UnassignRole(ctx context.Context, p oas.UnassignRoleParams) (oas.UnassignRoleRes, error) {
	a := domain.Assignment{GroupID: uuid.UUID(p.GroupId), ClientID: string(p.ClientId), Role: string(p.Role)}
	if err := h.uc.Unassign(ctx, principal.From(ctx), a); err != nil {
		return refusal(err)
	}
	return &oas.UnassignRoleNoContent{}, nil
}

func group(g domain.Group, members []oas.ID) oas.Group {
	if members == nil {
		members = []oas.ID{}
	}
	return oas.Group{ID: oas.ID(g.ID), Name: g.Name, Admins: g.Admins, Members: members}
}

// refusal answers a rule the caller broke with a problem saying which, and passes anything else on
// to NewError as a 500. Its result type fits every operation's response union.
func refusal(err error) (*oas.ProblemStatusCode, error) {
	switch {
	case errors.Is(err, app.ErrNotAdmin):
		return problem(http.StatusForbidden, "Only Admins manage access."), nil
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
	case errors.Is(err, app.ErrUnknownRole):
		return problem(http.StatusUnprocessableEntity, "That app has no such role."), nil
	default:
		return nil, err
	}
}

func problem(status int, detail string) *oas.ProblemStatusCode {
	return &oas.ProblemStatusCode{StatusCode: status, Response: httpx.Problem(status, detail)}
}
