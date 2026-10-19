package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/app"
	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
)

// MachineUseCases is what the Machines tab needs. Each refuses anyone but an Admin.
type MachineUseCases interface {
	Machines(ctx context.Context, actor app.Actor) (app.MachinesOverview, error)
	CreateMachine(ctx context.Context, actor app.Actor, name, namespace, serviceAccount string) (domain.Machine, error)
	DeleteMachine(ctx context.Context, actor app.Actor, id uuid.UUID) error
	AssignMachine(ctx context.Context, actor app.Actor, a domain.MachineAssignment) error
	UnassignMachine(ctx context.Context, actor app.Actor, a domain.MachineAssignment) error
}

// ListMachines implements listMachines.
func (h *Access) ListMachines(ctx context.Context) (oas.ListMachinesRes, error) {
	o, err := h.uc.Machines(ctx, actor(ctx))
	if err != nil {
		return machineRefusal(err)
	}
	res := &oas.MachinesOverview{
		Machines:    make([]oas.Machine, 0, len(o.Machines)),
		Assignments: make([]oas.MachineAssignment, 0, len(o.Assignments)),
		Clients:     clients(o.Clients),
	}
	for _, m := range o.Machines {
		res.Machines = append(res.Machines, machine(m))
	}
	for _, a := range o.Assignments {
		res.Assignments = append(res.Assignments, oas.MachineAssignment{
			MachineId: oas.ID(a.MachineID), ClientId: oas.ClientId(a.ClientID), Role: oas.AppRole(a.Role),
		})
	}
	return res, nil
}

// CreateMachine implements createMachine.
func (h *Access) CreateMachine(ctx context.Context, req *oas.MachineDraft) (oas.CreateMachineRes, error) {
	m, err := h.uc.CreateMachine(ctx, actor(ctx), req.Name, req.Namespace, req.ServiceAccount)
	if err != nil {
		return machineRefusal(err)
	}
	created := machine(m)
	return &created, nil
}

// DeleteMachine implements deleteMachine.
func (h *Access) DeleteMachine(ctx context.Context, p oas.DeleteMachineParams) (oas.DeleteMachineRes, error) {
	if err := h.uc.DeleteMachine(ctx, actor(ctx), uuid.UUID(p.MachineId)); err != nil {
		return machineRefusal(err)
	}
	return &oas.DeleteMachineNoContent{}, nil
}

// AssignMachineRole implements assignMachineRole.
func (h *Access) AssignMachineRole(ctx context.Context, p oas.AssignMachineRoleParams) (oas.AssignMachineRoleRes, error) {
	a := domain.MachineAssignment{MachineID: uuid.UUID(p.MachineId), ClientID: string(p.ClientId), Role: string(p.Role)}
	if err := h.uc.AssignMachine(ctx, actor(ctx), a); err != nil {
		return machineRefusal(err)
	}
	return &oas.AssignMachineRoleNoContent{}, nil
}

// UnassignMachineRole implements unassignMachineRole.
func (h *Access) UnassignMachineRole(ctx context.Context, p oas.UnassignMachineRoleParams) (oas.UnassignMachineRoleRes, error) {
	a := domain.MachineAssignment{MachineID: uuid.UUID(p.MachineId), ClientID: string(p.ClientId), Role: string(p.Role)}
	if err := h.uc.UnassignMachine(ctx, actor(ctx), a); err != nil {
		return machineRefusal(err)
	}
	return &oas.UnassignMachineRoleNoContent{}, nil
}

func machine(m domain.Machine) oas.Machine {
	return oas.Machine{ID: oas.ID(m.ID), Name: m.Name, Namespace: m.Namespace, ServiceAccount: m.ServiceAccount, CreatedAt: m.CreatedAt}
}

// machineRefusal answers the Machines tab's own refusals, and leaves the rest to refusal.
func machineRefusal(err error) (*oas.ProblemStatusCode, error) {
	switch {
	case errors.Is(err, app.ErrNotFound):
		return problem(http.StatusNotFound, "That Machine Identity no longer exists. Reload the page."), nil
	case errors.Is(err, app.ErrMachineNameTaken):
		return problem(http.StatusConflict, "A Machine Identity with that name already exists. Choose another name."), nil
	case errors.Is(err, app.ErrBindingTaken):
		return problem(http.StatusConflict, "Another Machine Identity already uses that ServiceAccount."), nil
	case errors.Is(err, domain.ErrInvalidMachineName):
		return problem(http.StatusUnprocessableEntity, "Enter a name of 1 to 64 characters."), nil
	case errors.Is(err, domain.ErrInvalidBinding):
		return problem(http.StatusUnprocessableEntity, "Use only lower-case letters, digits and hyphens in the namespace and ServiceAccount."), nil
	default:
		return refusal(err)
	}
}
