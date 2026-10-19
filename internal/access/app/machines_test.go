package app_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/app"
	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
)

var (
	madeAt = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	backup = domain.Machine{ID: uuid.MustParse("00000000-0000-4000-8000-0000000000c1"), Name: "Backup", Namespace: "media", ServiceAccount: "backup", CreatedAt: madeAt}
)

// machineStore keeps Machine Identities the way the Postgres adapter does.
type machineStore struct {
	machines    []domain.Machine
	assignments []domain.MachineAssignment
	fail        map[string]error
}

func (s *machineStore) err(name string) error { return s.fail[name] }

func (s *machineStore) Machines(context.Context) ([]domain.Machine, error) {
	return slices.Clone(s.machines), s.err("Machines")
}

func (s *machineStore) find(match func(domain.Machine) bool) (domain.Machine, error) {
	i := slices.IndexFunc(s.machines, match)
	if i < 0 {
		return domain.Machine{}, app.ErrNotFound
	}
	return s.machines[i], nil
}

func (s *machineStore) Machine(_ context.Context, id uuid.UUID) (domain.Machine, error) {
	if err := s.err("Machine"); err != nil {
		return domain.Machine{}, err
	}
	return s.find(func(m domain.Machine) bool { return m.ID == id })
}

func (s *machineStore) MachineByBinding(_ context.Context, namespace, serviceAccount string) (domain.Machine, error) {
	return s.find(func(m domain.Machine) bool { return m.Namespace == namespace && m.ServiceAccount == serviceAccount })
}

func (s *machineStore) CreateMachine(_ context.Context, m domain.Machine) error {
	if err := s.err("CreateMachine"); err != nil {
		return err
	}
	s.machines = append(s.machines, m)
	return nil
}

func (s *machineStore) DeleteMachine(_ context.Context, id uuid.UUID) (bool, error) {
	if err := s.err("DeleteMachine"); err != nil {
		return false, err
	}
	n := len(s.machines)
	s.machines = slices.DeleteFunc(s.machines, func(m domain.Machine) bool { return m.ID == id })
	s.assignments = slices.DeleteFunc(s.assignments, func(a domain.MachineAssignment) bool { return a.MachineID == id })
	return len(s.machines) < n, nil
}

func (s *machineStore) MachineAssignments(context.Context) ([]domain.MachineAssignment, error) {
	return slices.Clone(s.assignments), s.err("MachineAssignments")
}

func (s *machineStore) AssignMachine(_ context.Context, a domain.MachineAssignment) (bool, error) {
	if err := s.err("AssignMachine"); err != nil {
		return false, err
	}
	if slices.Contains(s.assignments, a) {
		return false, nil
	}
	s.assignments = append(s.assignments, a)
	return true, nil
}

func (s *machineStore) UnassignMachine(_ context.Context, a domain.MachineAssignment) (bool, error) {
	if err := s.err("UnassignMachine"); err != nil {
		return false, err
	}
	n := len(s.assignments)
	s.assignments = slices.DeleteFunc(s.assignments, func(b domain.MachineAssignment) bool { return b == a })
	return len(s.assignments) < n, nil
}

func (s *machineStore) MachineRolesOf(_ context.Context, id uuid.UUID, clientID string) ([]string, error) {
	var roles []string
	for _, a := range s.assignments {
		if a.MachineID == id && a.ClientID == clientID {
			roles = append(roles, a.Role)
		}
	}
	return roles, s.err("MachineRolesOf")
}

type machineFixture struct {
	*fixture
	machines *machineStore
}

// newMachineFixture is the access fixture holding the Backup Machine Identity.
func newMachineFixture() *machineFixture {
	f := &machineFixture{fixture: newFixture(), machines: &machineStore{machines: []domain.Machine{backup}, fail: map[string]error{}}}
	f.svc.SetMachines(f.machines, func() time.Time { return madeAt })
	return f
}

var viewerOnGrafana = domain.MachineAssignment{MachineID: backup.ID, ClientID: "grafana", Role: "viewer"}

var machineOperations = []struct {
	name string
	call func(f *machineFixture, actor app.Actor) error
}{
	{"listing Machine Identities", func(f *machineFixture, u app.Actor) error { _, err := f.svc.Machines(ctx, u); return err }},
	{"creating one", func(f *machineFixture, u app.Actor) error {
		_, err := f.svc.CreateMachine(ctx, u, "Sync", "media", "sync")
		return err
	}},
	{"deleting one", func(f *machineFixture, u app.Actor) error { return f.svc.DeleteMachine(ctx, u, backup.ID) }},
	{"giving one an App Role", func(f *machineFixture, u app.Actor) error { return f.svc.AssignMachine(ctx, u, viewerOnGrafana) }},
	{"taking one's App Role", func(f *machineFixture, u app.Actor) error { return f.svc.UnassignMachine(ctx, u, viewerOnGrafana) }},
}

func TestEveryMachineOperationRefusesAUserWhoIsNotAnAdminAndChangesNothing(t *testing.T) {
	for _, op := range machineOperations {
		t.Run(op.name, func(t *testing.T) {
			f := newMachineFixture()
			if err := op.call(f, asNonAdmin); !errors.Is(err, app.ErrNotAdmin) {
				t.Fatalf("err = %v, want ErrNotAdmin", err)
			}
			if len(f.machines.machines) != 1 || len(f.machines.assignments) != 0 || len(*f.events) != 0 {
				t.Fatalf("a refused call changed something: %+v, events %+v", f.machines, *f.events)
			}
		})
	}
}

func TestAnAdminCreatesAMachineIdentityAsAnAuditEvent(t *testing.T) {
	f := newMachineFixture()

	m, err := f.svc.CreateMachine(ctx, asAdmin, " Sync ", "media", "sync")
	if err != nil {
		t.Fatal(err)
	}

	if m.Name != "Sync" || m.Namespace != "media" || m.ServiceAccount != "sync" || !m.CreatedAt.Equal(madeAt) || len(f.machines.machines) != 2 {
		t.Fatalf("created %+v", m)
	}
	if want := (audit.Event{Action: audit.MachineCreated, Actor: admin, Machine: m.ID, MachineName: "Sync"}); len(*f.events) != 1 || (*f.events)[0] != want {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestAMachineIdentityIsNotCreatedWithABindingKubernetesWouldRefuse(t *testing.T) {
	f := newMachineFixture()
	if _, err := f.svc.CreateMachine(ctx, asAdmin, "Sync", "Media", "sync"); !errors.Is(err, domain.ErrInvalidBinding) || len(f.machines.machines) != 1 {
		t.Fatalf("err = %v", err)
	}
}

func TestAnAdminDeletesAMachineIdentityAsAnAuditEventNamingIt(t *testing.T) {
	f := newMachineFixture()
	f.machines.assignments = []domain.MachineAssignment{viewerOnGrafana}

	if err := f.svc.DeleteMachine(ctx, asAdmin, backup.ID); err != nil {
		t.Fatal(err)
	}

	if len(f.machines.machines) != 0 || len(f.machines.assignments) != 0 {
		t.Fatalf("left %+v", f.machines)
	}
	if want := (audit.Event{Action: audit.MachineDeleted, Actor: admin, Machine: backup.ID, MachineName: "Backup"}); len(*f.events) != 1 || (*f.events)[0] != want {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestAnAdminGivesAMachineIdentityAnAppRoleAsAnAuditEvent(t *testing.T) {
	f := newMachineFixture()

	if err := f.svc.AssignMachine(ctx, asAdmin, viewerOnGrafana); err != nil {
		t.Fatal(err)
	}

	if roles, _ := f.svc.MachineRoles(ctx, backup.ID, "grafana"); !slices.Equal(roles, []string{"viewer"}) {
		t.Fatalf("roles = %v", roles)
	}
	want := audit.Event{Action: audit.RoleAssigned, Actor: admin, Machine: backup.ID, MachineName: "Backup", Client: "grafana", Role: "viewer"}
	if len(*f.events) != 1 || (*f.events)[0] != want {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestGivingAMachineIdentityARoleItHoldsRecordsNothing(t *testing.T) {
	f := newMachineFixture()
	f.machines.assignments = []domain.MachineAssignment{viewerOnGrafana}

	if err := f.svc.AssignMachine(ctx, asAdmin, viewerOnGrafana); err != nil || len(*f.events) != 0 {
		t.Fatalf("err %v, events %+v", err, *f.events)
	}
}

func TestAMachineIdentityIsGivenOnlyARoleItsClientDeclares(t *testing.T) {
	for _, c := range []struct {
		name string
		a    domain.MachineAssignment
		want error
	}{
		{"an unknown Client", domain.MachineAssignment{MachineID: backup.ID, ClientID: "wiki", Role: "user"}, app.ErrUnknownClient},
		{"an undeclared role", domain.MachineAssignment{MachineID: backup.ID, ClientID: "grafana", Role: "owner"}, app.ErrUnknownRole},
		{"a Machine Identity that is gone", domain.MachineAssignment{MachineID: uuid.New(), ClientID: "grafana", Role: "viewer"}, app.ErrNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newMachineFixture()
			if err := f.svc.AssignMachine(ctx, asAdmin, c.a); !errors.Is(err, c.want) || len(f.machines.assignments) != 0 {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAnAdminTakesAMachineIdentitysAppRoleAsAnAuditEvent(t *testing.T) {
	f := newMachineFixture()
	f.machines.assignments = []domain.MachineAssignment{viewerOnGrafana}

	if err := f.svc.UnassignMachine(ctx, asAdmin, viewerOnGrafana); err != nil {
		t.Fatal(err)
	}

	want := audit.Event{Action: audit.RoleRemoved, Actor: admin, Machine: backup.ID, MachineName: "Backup", Client: "grafana", Role: "viewer"}
	if len(f.machines.assignments) != 0 || len(*f.events) != 1 || (*f.events)[0] != want {
		t.Fatalf("left %v, events %+v", f.machines.assignments, *f.events)
	}
}

func TestTakingARoleAMachineIdentityDoesNotHoldRecordsNothing(t *testing.T) {
	f := newMachineFixture()
	if err := f.svc.UnassignMachine(ctx, asAdmin, viewerOnGrafana); err != nil || len(*f.events) != 0 {
		t.Fatalf("err %v, events %+v", err, *f.events)
	}
}

func TestTheMachinesTabListsMachineIdentitiesTheirRolesAndTheClients(t *testing.T) {
	f := newMachineFixture()
	f.machines.assignments = []domain.MachineAssignment{viewerOnGrafana}

	o, err := f.svc.Machines(ctx, asAdmin)
	if err != nil {
		t.Fatal(err)
	}

	if len(o.Machines) != 1 || o.Machines[0] != backup || len(o.Assignments) != 1 || len(o.Clients) != 1 {
		t.Fatalf("overview = %+v", o)
	}
}

func TestSignInFindsAMachineIdentityByIdOrByItsServiceAccount(t *testing.T) {
	f := newMachineFixture()
	byID, err := f.svc.Machine(ctx, backup.ID)
	if err != nil || byID != backup {
		t.Fatalf("by id = %+v, %v", byID, err)
	}
	byBinding, err := f.svc.MachineByBinding(ctx, "media", "backup")
	if err != nil || byBinding != backup {
		t.Fatalf("by binding = %+v, %v", byBinding, err)
	}
}

func TestAMachineOperationFailsWhenTheStoreDoes(t *testing.T) {
	for _, c := range []struct {
		method string
		call   func(f *machineFixture) error
	}{
		{"Machines", func(f *machineFixture) error { _, err := f.svc.Machines(ctx, asAdmin); return err }},
		{"MachineAssignments", func(f *machineFixture) error { _, err := f.svc.Machines(ctx, asAdmin); return err }},
		{"CreateMachine", func(f *machineFixture) error {
			_, err := f.svc.CreateMachine(ctx, asAdmin, "Sync", "media", "sync")
			return err
		}},
		{"Machine", func(f *machineFixture) error { return f.svc.DeleteMachine(ctx, asAdmin, backup.ID) }},
		{"DeleteMachine", func(f *machineFixture) error { return f.svc.DeleteMachine(ctx, asAdmin, backup.ID) }},
		{"AssignMachine", func(f *machineFixture) error { return f.svc.AssignMachine(ctx, asAdmin, viewerOnGrafana) }},
		{"UnassignMachine", func(f *machineFixture) error { return f.svc.UnassignMachine(ctx, asAdmin, viewerOnGrafana) }},
	} {
		t.Run(c.method, func(t *testing.T) {
			f := newMachineFixture()
			f.machines.fail[c.method] = errBoom
			if err := c.call(f); !errors.Is(err, errBoom) || len(*f.events) != 0 {
				t.Fatalf("err = %v, events %+v", err, *f.events)
			}
		})
	}
}

func TestAMachineIdentityTakesItsIdFromTheServicesRandomness(t *testing.T) {
	f := newMachineFixture()
	f.svc = app.New(f.store, catalog{grafana}, f.people, f.events, failingReader{})
	f.svc.SetMachines(f.machines, func() time.Time { return madeAt })
	if _, err := f.svc.CreateMachine(ctx, asAdmin, "Sync", "media", "sync"); err == nil {
		t.Fatal("created a Machine Identity without an id")
	}
}

func TestChangingAMachineIdentitysRolesFailsWhenItCannotBeRead(t *testing.T) {
	for _, c := range []struct {
		name string
		call func(f *machineFixture) error
	}{
		{"giving", func(f *machineFixture) error { return f.svc.AssignMachine(ctx, asAdmin, viewerOnGrafana) }},
		{"taking", func(f *machineFixture) error { return f.svc.UnassignMachine(ctx, asAdmin, viewerOnGrafana) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newMachineFixture()
			f.machines.fail["Machine"] = errBoom
			if err := c.call(f); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no randomness") }
