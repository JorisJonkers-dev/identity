package machines_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	accessapp "github.com/JorisJonkers-dev/identity/internal/access/app"
	accessdomain "github.com/JorisJonkers-dev/identity/internal/access/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/machines"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

var backup = accessdomain.Machine{ID: uuid.New(), Name: "Backup", Namespace: "media", ServiceAccount: "backup"}

type access struct{ err error }

func (a access) Machine(context.Context, uuid.UUID) (accessdomain.Machine, error) {
	return backup, a.err
}

func (a access) MachineByBinding(context.Context, string, string) (accessdomain.Machine, error) {
	return backup, a.err
}

func (a access) MachineRoles(context.Context, uuid.UUID, string) ([]string, error) {
	return []string{"viewer"}, a.err
}

var want = app.MachineIdentity{ID: backup.ID, Name: "Backup", Namespace: "media", ServiceAccount: "backup"}

func TestAMachineIdentityIsReadByIdAsSignInSeesIt(t *testing.T) {
	m, err := machines.New(access{}).Machine(context.Background(), backup.ID)
	if err != nil || m != want {
		t.Fatalf("Machine = %+v, %v", m, err)
	}
}

func TestAMachineIdentityIsReadByItsServiceAccountAsSignInSeesIt(t *testing.T) {
	m, err := machines.New(access{}).MachineByBinding(context.Background(), "media", "backup")
	if err != nil || m != want {
		t.Fatalf("MachineByBinding = %+v, %v", m, err)
	}
}

func TestAMachineIdentitysRolesComeFromAccess(t *testing.T) {
	roles, err := machines.New(access{}).MachineRoles(context.Background(), backup.ID, "grafana")
	if err != nil || !slices.Equal(roles, []string{"viewer"}) {
		t.Fatalf("MachineRoles = %v, %v", roles, err)
	}
}

func TestAMissingMachineIdentityIsSignInsNotFound(t *testing.T) {
	if _, err := machines.New(access{err: accessapp.ErrNotFound}).Machine(context.Background(), backup.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnotherFailurePassesThrough(t *testing.T) {
	boom := errors.New("boom")
	if _, err := machines.New(access{err: boom}).Machine(context.Background(), backup.ID); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
