package persistence

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JorisJonkers-dev/identity/internal/access/app"
	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
)

var _ app.MachineStore = (*Store)(nil)

func machine(r queries.MachineIdentity) domain.Machine {
	return domain.Machine{ID: r.ID, Name: r.Name, Namespace: r.Namespace, ServiceAccount: r.ServiceAccount, CreatedAt: r.CreatedAt}
}

func found(r queries.MachineIdentity, err error) (domain.Machine, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Machine{}, app.ErrNotFound
	}
	if err != nil {
		return domain.Machine{}, err
	}
	return machine(r), nil
}

func (s *Store) Machines(ctx context.Context) ([]domain.Machine, error) {
	rows, err := s.q.ListMachines(ctx)
	if err != nil {
		return nil, err
	}
	machines := make([]domain.Machine, 0, len(rows))
	for _, r := range rows {
		machines = append(machines, machine(r))
	}
	return machines, nil
}

func (s *Store) Machine(ctx context.Context, id uuid.UUID) (domain.Machine, error) {
	return found(s.q.MachineByID(ctx, id))
}

func (s *Store) MachineByBinding(ctx context.Context, namespace, serviceAccount string) (domain.Machine, error) {
	return found(s.q.MachineByBinding(ctx, queries.MachineByBindingParams{Namespace: namespace, ServiceAccount: serviceAccount}))
}

func (s *Store) CreateMachine(ctx context.Context, m domain.Machine) error {
	err := s.q.CreateMachine(ctx, queries.CreateMachineParams{
		ID: m.ID, Name: m.Name, Namespace: m.Namespace, ServiceAccount: m.ServiceAccount, CreatedAt: m.CreatedAt,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		if pgErr.ConstraintName == "machine_identities_binding_key" {
			return app.ErrBindingTaken
		}
		return app.ErrMachineNameTaken
	}
	return err
}

// DeleteMachine deletes the Machine Identity and the client secret kept under its id at once.
func (s *Store) DeleteMachine(ctx context.Context, id uuid.UUID) (bool, error) {
	deleted := false
	err := s.tx.InTx(ctx, func(q *queries.Queries) error {
		if err := q.DeleteClientSecret(ctx, id.String()); err != nil {
			return err
		}
		n, err := q.DeleteMachine(ctx, id)
		deleted = n > 0
		return err
	})
	return deleted && err == nil, err
}

func (s *Store) MachineAssignments(ctx context.Context) ([]domain.MachineAssignment, error) {
	rows, err := s.q.MachineAssignments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.MachineAssignment, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.MachineAssignment{MachineID: r.MachineID, ClientID: r.ClientID, Role: r.Role})
	}
	return out, nil
}

func (s *Store) AssignMachine(ctx context.Context, a domain.MachineAssignment) (bool, error) {
	n, err := s.q.AssignMachine(ctx, queries.AssignMachineParams{MachineID: a.MachineID, ClientID: a.ClientID, Role: a.Role})
	return n > 0, missing(err)
}

func (s *Store) UnassignMachine(ctx context.Context, a domain.MachineAssignment) (bool, error) {
	n, err := s.q.UnassignMachine(ctx, queries.UnassignMachineParams{MachineID: a.MachineID, ClientID: a.ClientID, Role: a.Role})
	return n > 0, err
}

func (s *Store) MachineRolesOf(ctx context.Context, id uuid.UUID, clientID string) ([]string, error) {
	return s.q.MachineRolesOf(ctx, queries.MachineRolesOfParams{MachineID: id, ClientID: clientID})
}
