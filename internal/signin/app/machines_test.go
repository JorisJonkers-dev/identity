package app_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var backupJob = app.MachineIdentity{ID: uuid.MustParse("00000000-0000-4000-8000-0000000000c1"), Name: "Backup", Namespace: "media", ServiceAccount: "backup"}

// machines is the access context holding the Backup Machine Identity and its roles.
type machines struct {
	roles map[string][]string
	fail  error
}

func (m *machines) Machine(_ context.Context, id uuid.UUID) (app.MachineIdentity, error) {
	if m.fail != nil {
		return app.MachineIdentity{}, m.fail
	}
	if id != backupJob.ID {
		return app.MachineIdentity{}, app.ErrNotFound
	}
	return backupJob, nil
}

func (m *machines) MachineByBinding(_ context.Context, namespace, serviceAccount string) (app.MachineIdentity, error) {
	if namespace != backupJob.Namespace || serviceAccount != backupJob.ServiceAccount {
		return app.MachineIdentity{}, app.ErrNotFound
	}
	return backupJob, m.fail
}

func (m *machines) MachineRoles(_ context.Context, _ uuid.UUID, clientID string) ([]string, error) {
	return m.roles[clientID], m.fail
}

// serviceAccounts verifies "sa:<namespace>:<name>" tokens, as the cluster's would.
type serviceAccounts struct{}

func (serviceAccounts) Verify(_ context.Context, token string) (string, string, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 3 || parts[0] != "sa" {
		return "", "", app.ErrInvalidServiceAccountToken
	}
	return parts[1], parts[2], nil
}

type machineFixture struct {
	*app.MachineSignIn
	machines *machines
	hashes   *hashes
	events   *recorded
}

func newMachineFixture() *machineFixture {
	f := &machineFixture{
		machines: &machines{roles: map[string][]string{"grafana": {"editor", "viewer"}}},
		hashes:   &hashes{current: map[string][]byte{}, previous: map[string][]byte{}, fail: map[string]error{}},
		events:   &recorded{},
	}
	cs := clients{"grafana": {ID: "grafana", SignIn: domain.Native, Roles: []string{"user", "viewer", "editor"}}}
	f.MachineSignIn = app.NewMachineSignIn(f.machines, serviceAccounts{}, f.hashes, cs, admins{root: true}, f.events, &counter{})
	return f
}

func TestAServiceAccountTokenSignsInAsTheMachineIdentityBoundToIt(t *testing.T) {
	m, err := newMachineFixture().Exchange(context.Background(), "sa:media:backup")
	if err != nil || m != backupJob {
		t.Fatalf("Exchange = %+v, %v", m, err)
	}
}

func TestAServiceAccountNoMachineIdentityIsBoundToIsRefused(t *testing.T) {
	if _, err := newMachineFixture().Exchange(context.Background(), "sa:media:sync"); !errors.Is(err, app.ErrUnknownMachine) {
		t.Fatalf("err = %v", err)
	}
}

func TestATokenTheClusterDidNotIssueIsRefused(t *testing.T) {
	if _, err := newMachineFixture().Exchange(context.Background(), "forged"); !errors.Is(err, app.ErrInvalidServiceAccountToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnExchangeFailsWhenMachineIdentitiesCannotBeRead(t *testing.T) {
	f := newMachineFixture()
	f.machines.fail = errBoom
	if _, err := f.Exchange(context.Background(), "sa:media:backup"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAMachineIdentityCarriesTheRolesItsClientDeclaresInTheClientsOrder(t *testing.T) {
	roles, err := newMachineFixture().Roles(context.Background(), backupJob.ID, "grafana")
	if err != nil || !slices.Equal(roles, []string{"viewer", "editor"}) {
		t.Fatalf("Roles = %v, %v", roles, err)
	}
}

func TestAMachineIdentityWithNoRoleOnAClientGetsNoToken(t *testing.T) {
	f := newMachineFixture()
	f.machines.roles = map[string][]string{"grafana": {"owner"}}
	if _, err := f.Roles(context.Background(), backupJob.ID, "grafana"); !errors.Is(err, app.ErrNoAccess) {
		t.Fatalf("err = %v", err)
	}
}

func TestAMachineIdentityGetsNoTokenForAClientNotInTheCluster(t *testing.T) {
	if _, err := newMachineFixture().Roles(context.Background(), backupJob.ID, "wiki"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestAMachineIdentitysRolesFailWhenTheyCannotBeRead(t *testing.T) {
	f := newMachineFixture()
	f.machines.fail = errBoom
	if _, err := f.Roles(context.Background(), backupJob.ID, "grafana"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnAdminGivesAMachineIdentityASecretItSignsInWith(t *testing.T) {
	f := newMachineFixture()

	secret, err := f.RotateSecret(context.Background(), asRoot, backupJob.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := f.Authorize(context.Background(), backupJob.ID, secret); err != nil {
		t.Fatalf("the new secret = %v", err)
	}
	if err := f.Authorize(context.Background(), backupJob.ID, secret+"x"); !errors.Is(err, app.ErrInvalidClient) {
		t.Fatalf("a wrong secret = %v", err)
	}
}

func TestGivingAMachineIdentityASecretIsAnAuditEventNamingIt(t *testing.T) {
	f := newMachineFixture()
	_, _ = f.RotateSecret(context.Background(), asRoot, backupJob.ID)
	want := audit.Event{Action: audit.MachineSecretRotated, Actor: root, Machine: backupJob.ID, MachineName: "Backup"}
	if len(*f.events) != 1 || (*f.events)[0] != want {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestTheReplacedSecretWorksUntilTheNewOneIsUsed(t *testing.T) {
	f := newMachineFixture()
	old, _ := f.RotateSecret(context.Background(), asRoot, backupJob.ID)
	current, _ := f.RotateSecret(context.Background(), asRoot, backupJob.ID)

	if err := f.Authorize(context.Background(), backupJob.ID, old); err != nil {
		t.Fatalf("the replaced secret before the new one is used = %v", err)
	}
	_ = f.Authorize(context.Background(), backupJob.ID, current)
	if err := f.Authorize(context.Background(), backupJob.ID, old); !errors.Is(err, app.ErrInvalidClient) {
		t.Fatalf("the replaced secret after = %v", err)
	}
}

func TestOnlyAnAdminWhoProvedASecondFactorGivesAMachineIdentityASecret(t *testing.T) {
	for _, c := range []struct {
		name  string
		actor app.Actor
		want  error
	}{
		{"not an Admin", app.Actor{ID: uuid.New(), SecondFactor: true}, app.ErrNotAdmin},
		{"no second factor", app.Actor{ID: root}, app.ErrSecondFactorNeeded},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newMachineFixture()
			if _, err := f.RotateSecret(context.Background(), c.actor, backupJob.ID); !errors.Is(err, c.want) || len(f.hashes.current) != 0 {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAMachineIdentityThatIsGoneGetsNoSecret(t *testing.T) {
	if _, err := newMachineFixture().RotateSecret(context.Background(), asRoot, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestNoSecretIsMadeWithoutRandomness(t *testing.T) {
	f := newMachineFixture()
	f.MachineSignIn = app.NewMachineSignIn(f.machines, serviceAccounts{}, f.hashes, clients{}, admins{root: true}, f.events, noRandom{})
	if _, err := f.RotateSecret(context.Background(), asRoot, backupJob.ID); err == nil || len(*f.events) != 0 {
		t.Fatalf("err = %v, events %+v", err, *f.events)
	}
}

func TestASecretForNoMachineIdentityIsRefused(t *testing.T) {
	if err := newMachineFixture().Authorize(context.Background(), uuid.New(), "anything"); !errors.Is(err, app.ErrInvalidClient) {
		t.Fatalf("err = %v", err)
	}
}

func TestAMachineIdentityWithNoSecretYetIsRefused(t *testing.T) {
	if err := newMachineFixture().Authorize(context.Background(), backupJob.ID, "anything"); !errors.Is(err, app.ErrInvalidClient) {
		t.Fatalf("err = %v", err)
	}
}

func TestASecretCheckFailsWhenMachineIdentitiesCannotBeRead(t *testing.T) {
	f := newMachineFixture()
	f.machines.fail = errBoom
	if err := f.Authorize(context.Background(), backupJob.ID, "anything"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestGivingASecretFailsWhenTheMachineIdentityCannotBeRead(t *testing.T) {
	f := newMachineFixture()
	f.machines.fail = errBoom
	if _, err := f.RotateSecret(context.Background(), asRoot, backupJob.ID); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

type noRandom struct{}

func (noRandom) Read([]byte) (int, error) { return 0, errors.New("no randomness") }
