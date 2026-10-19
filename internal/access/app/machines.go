package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
)

var (
	// ErrMachineNameTaken is a Machine Identity with another's name, in any case.
	ErrMachineNameTaken = errors.New("a Machine Identity has that name")
	// ErrBindingTaken is a ServiceAccount another Machine Identity is bound to.
	ErrBindingTaken = errors.New("a Machine Identity is bound to that ServiceAccount")
)

// MachineStore keeps Machine Identities and their Assignments (identity#22).
type MachineStore interface {
	Machines(ctx context.Context) ([]domain.Machine, error)
	// Machine is ErrNotFound when there is none.
	Machine(ctx context.Context, id uuid.UUID) (domain.Machine, error)
	// MachineByBinding is the Machine Identity bound to a ServiceAccount, or ErrNotFound.
	MachineByBinding(ctx context.Context, namespace, serviceAccount string) (domain.Machine, error)
	// CreateMachine is ErrMachineNameTaken or ErrBindingTaken.
	CreateMachine(ctx context.Context, m domain.Machine) error
	// DeleteMachine deletes it, its Assignments and its client secret, and reports whether it was there.
	DeleteMachine(ctx context.Context, id uuid.UUID) (bool, error)
	MachineAssignments(ctx context.Context) ([]domain.MachineAssignment, error)
	// AssignMachine is ErrNotFound when the Machine Identity is not there, and reports whether it is new.
	AssignMachine(ctx context.Context, a domain.MachineAssignment) (bool, error)
	// UnassignMachine reports whether the Assignment was there.
	UnassignMachine(ctx context.Context, a domain.MachineAssignment) (bool, error)
	// MachineRolesOf is the App Roles a Machine Identity holds on a Client.
	MachineRolesOf(ctx context.Context, id uuid.UUID, clientID string) ([]string, error)
}

// MachinesOverview is everything the Machines tab shows.
type MachinesOverview struct {
	Machines []domain.Machine
	// Assignments includes those on Removed Clients, which give no access while it is away.
	Assignments []domain.MachineAssignment
	Clients     []domain.Client
}

// SetMachines lets the Service manage Machine Identities; until it is called, it manages none.
func (s *Service) SetMachines(m MachineStore, now func() time.Time) {
	s.machines, s.now = m, now
}

// Machines is every Machine Identity, its Assignments and the Clients it may be given roles on.
func (s *Service) Machines(ctx context.Context, actor Actor) (MachinesOverview, error) {
	if err := s.admin(ctx, actor); err != nil {
		return MachinesOverview{}, err
	}
	var o MachinesOverview
	var err error
	if o.Machines, err = s.machines.Machines(ctx); err != nil {
		return MachinesOverview{}, err
	}
	if o.Assignments, err = s.machines.MachineAssignments(ctx); err != nil {
		return MachinesOverview{}, err
	}
	o.Clients = s.clients.Clients()
	return o, nil
}

// CreateMachine adds a Machine Identity bound to a ServiceAccount, holding no role yet.
func (s *Service) CreateMachine(ctx context.Context, actor Actor, name, namespace, serviceAccount string) (domain.Machine, error) {
	if err := s.admin(ctx, actor); err != nil {
		return domain.Machine{}, err
	}
	id, err := uuid.NewRandomFromReader(s.random)
	if err != nil {
		return domain.Machine{}, err
	}
	m, err := domain.NewMachine(id, name, namespace, serviceAccount, s.now())
	if err != nil {
		return domain.Machine{}, err
	}
	if err := s.machines.CreateMachine(ctx, m); err != nil {
		return domain.Machine{}, err
	}
	s.audit.Record(ctx, audit.Event{Action: audit.MachineCreated, Actor: actor.ID, Machine: m.ID, MachineName: m.Name})
	return m, nil
}

// DeleteMachine removes a Machine Identity with its roles and secret; its tokens end when they expire.
func (s *Service) DeleteMachine(ctx context.Context, actor Actor, id uuid.UUID) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	m, err := s.machines.Machine(ctx, id)
	if err != nil {
		return err
	}
	deleted, err := s.machines.DeleteMachine(ctx, id)
	if deleted {
		s.audit.Record(ctx, audit.Event{Action: audit.MachineDeleted, Actor: actor.ID, Machine: m.ID, MachineName: m.Name})
	}
	return err
}

// AssignMachine gives a Machine Identity an App Role the Client declares.
func (s *Service) AssignMachine(ctx context.Context, actor Actor, a domain.MachineAssignment) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	m, err := s.machines.Machine(ctx, a.MachineID)
	if err != nil {
		return err
	}
	c, ok := s.clients.Client(ctx, a.ClientID)
	if !ok {
		return ErrUnknownClient
	}
	if !c.Declares(a.Role) {
		return ErrUnknownRole
	}
	assigned, err := s.machines.AssignMachine(ctx, a)
	if assigned {
		s.audit.Record(ctx, audit.Event{Action: audit.RoleAssigned, Actor: actor.ID, Machine: m.ID, MachineName: m.Name, Client: a.ClientID, Role: a.Role})
	}
	return err
}

// UnassignMachine takes an App Role from a Machine Identity, also on a Removed Client.
func (s *Service) UnassignMachine(ctx context.Context, actor Actor, a domain.MachineAssignment) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	m, err := s.machines.Machine(ctx, a.MachineID)
	if err != nil {
		return err
	}
	removed, err := s.machines.UnassignMachine(ctx, a)
	if removed {
		s.audit.Record(ctx, audit.Event{Action: audit.RoleRemoved, Actor: actor.ID, Machine: m.ID, MachineName: m.Name, Client: a.ClientID, Role: a.Role})
	}
	return err
}

// Machine is one Machine Identity, for sign-in to know who a credential belongs to.
func (s *Service) Machine(ctx context.Context, id uuid.UUID) (domain.Machine, error) {
	return s.machines.Machine(ctx, id)
}

// MachineByBinding is the Machine Identity a ServiceAccount signs in as, or ErrNotFound.
func (s *Service) MachineByBinding(ctx context.Context, namespace, serviceAccount string) (domain.Machine, error) {
	return s.machines.MachineByBinding(ctx, namespace, serviceAccount)
}

// MachineRoles is the App Roles a Machine Identity holds on a Client.
func (s *Service) MachineRoles(ctx context.Context, id uuid.UUID, clientID string) ([]string, error) {
	return s.machines.MachineRolesOf(ctx, id, clientID)
}
