package web_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/access/app"
	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
)

func (u *useCases) Machines(_ context.Context, actor app.Actor) (app.MachinesOverview, error) {
	u.actor = actor
	return u.machines, u.err
}

func (u *useCases) CreateMachine(_ context.Context, actor app.Actor, name, namespace, serviceAccount string) (domain.Machine, error) {
	u.actor, u.asked = actor, append(u.asked, name, namespace, serviceAccount)
	return u.machine, u.err
}

func (u *useCases) DeleteMachine(_ context.Context, actor app.Actor, id uuid.UUID) error {
	u.actor, u.asked = actor, append(u.asked, id)
	return u.err
}

func (u *useCases) AssignMachine(_ context.Context, actor app.Actor, a domain.MachineAssignment) error {
	u.actor, u.asked = actor, append(u.asked, a)
	return u.err
}

func (u *useCases) UnassignMachine(_ context.Context, actor app.Actor, a domain.MachineAssignment) error {
	u.actor, u.asked = actor, append(u.asked, a)
	return u.err
}

var (
	backupID = uuid.MustParse("00000000-0000-4000-8000-0000000000c1")
	backup   = domain.Machine{ID: backupID, Name: "Backup", Namespace: "media", ServiceAccount: "backup", CreatedAt: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	viewer   = domain.MachineAssignment{MachineID: backupID, ClientID: "grafana", Role: "viewer"}
)

func TestTheMachinesTabCarriesMachineIdentitiesTheirRolesAndTheClients(t *testing.T) {
	uc := &useCases{machines: app.MachinesOverview{
		Machines:    []domain.Machine{backup},
		Assignments: []domain.MachineAssignment{viewer},
		Clients:     []domain.Client{{ID: "grafana", Label: "Grafana", Roles: []string{"user", "viewer"}}},
	}}
	res, err := web.New(uc).ListMachines(signedIn())
	o, ok := res.(*oas.MachinesOverview)
	if err != nil || !ok || uc.actor != asAdmin {
		t.Fatalf("ListMachines = %#v, %v", res, err)
	}
	want := oas.Machine{ID: oas.ID(backupID), Name: "Backup", Namespace: "media", ServiceAccount: "backup", CreatedAt: backup.CreatedAt}
	if len(o.Machines) != 1 || o.Machines[0] != want {
		t.Fatalf("machines = %+v", o.Machines)
	}
	if len(o.Assignments) != 1 || o.Assignments[0] != (oas.MachineAssignment{MachineId: oas.ID(backupID), ClientId: "grafana", Role: "viewer"}) {
		t.Fatalf("assignments = %+v", o.Assignments)
	}
	if len(o.Clients) != 1 || o.Clients[0].Label != "Grafana" || len(o.Clients[0].Roles) != 2 {
		t.Fatalf("clients = %+v", o.Clients)
	}
}

func TestCreatingAMachineIdentitySendsWhatWasTypedAndAnswersIt(t *testing.T) {
	uc := &useCases{machine: backup}
	res, err := web.New(uc).CreateMachine(signedIn(), &oas.MachineDraft{Name: " Backup ", Namespace: "media", ServiceAccount: "backup"})
	m, ok := res.(*oas.Machine)
	if err != nil || !ok || uuid.UUID(m.ID) != backupID || uc.asked[0] != " Backup " || uc.asked[1] != "media" || uc.asked[2] != "backup" {
		t.Fatalf("CreateMachine = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestDeletingAMachineIdentitySendsItsID(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).DeleteMachine(signedIn(), oas.DeleteMachineParams{MachineId: oas.ID(backupID)})
	if _, ok := res.(*oas.DeleteMachineNoContent); err != nil || !ok || uc.asked[0] != backupID {
		t.Fatalf("DeleteMachine = %#v, %v", res, err)
	}
}

func TestGivingAMachineIdentityARoleSendsTheAssignment(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).AssignMachineRole(signedIn(), oas.AssignMachineRoleParams{MachineId: oas.ID(backupID), ClientId: "grafana", Role: "viewer"})
	if _, ok := res.(*oas.AssignMachineRoleNoContent); err != nil || !ok || uc.asked[0] != viewer {
		t.Fatalf("AssignMachineRole = %#v, %v", res, err)
	}
}

func TestTakingAMachineIdentitysRoleSendsTheAssignment(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).UnassignMachineRole(signedIn(), oas.UnassignMachineRoleParams{MachineId: oas.ID(backupID), ClientId: "grafana", Role: "viewer"})
	if _, ok := res.(*oas.UnassignMachineRoleNoContent); err != nil || !ok || uc.asked[0] != viewer {
		t.Fatalf("UnassignMachineRole = %#v, %v", res, err)
	}
}

func TestEachMachineRefusalIsAProblemSayingWhatToDo(t *testing.T) {
	for _, c := range []struct {
		err    error
		status int
		detail string
	}{
		{app.ErrNotFound, http.StatusNotFound, "That Machine Identity no longer exists. Reload the page."},
		{app.ErrMachineNameTaken, http.StatusConflict, "A Machine Identity with that name already exists. Choose another name."},
		{app.ErrBindingTaken, http.StatusConflict, "Another Machine Identity already uses that ServiceAccount."},
		{domain.ErrInvalidMachineName, http.StatusUnprocessableEntity, "Enter a name of 1 to 64 characters."},
		{domain.ErrInvalidBinding, http.StatusUnprocessableEntity, "Use only lower-case letters, digits and hyphens in the namespace and ServiceAccount."},
		{app.ErrNotAdmin, http.StatusForbidden, "Only Admins manage access."},
	} {
		t.Run(c.err.Error(), func(t *testing.T) {
			res, err := web.New(&useCases{err: c.err}).CreateMachine(signedIn(), &oas.MachineDraft{Name: "Backup", Namespace: "media", ServiceAccount: "backup"})
			wantProblem(t, res, err, c.status, c.detail)
		})
	}
}

func TestEveryMachineOperationAnswersARefusalAsAProblem(t *testing.T) {
	h := web.New(&useCases{err: app.ErrNotAdmin})
	for _, c := range []struct {
		name string
		call func() (any, error)
	}{
		{"listing", func() (any, error) { return h.ListMachines(signedIn()) }},
		{"deleting", func() (any, error) { return h.DeleteMachine(signedIn(), oas.DeleteMachineParams{}) }},
		{"giving a role", func() (any, error) { return h.AssignMachineRole(signedIn(), oas.AssignMachineRoleParams{}) }},
		{"taking a role", func() (any, error) { return h.UnassignMachineRole(signedIn(), oas.UnassignMachineRoleParams{}) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			res, err := c.call()
			wantProblem(t, res, err, http.StatusForbidden, "Only Admins manage access.")
		})
	}
}

func TestAMachineFailureTheContractDoesNotNamePassesOn(t *testing.T) {
	if _, err := web.New(&useCases{err: errBoom}).ListMachines(signedIn()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}
