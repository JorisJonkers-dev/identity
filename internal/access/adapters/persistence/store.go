// Package persistence is the access context's Postgres adapter: app.Store over the sqlc queries.
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

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

// Transactor runs queries in one transaction.
type Transactor interface {
	InTx(ctx context.Context, fn func(*queries.Queries) error) error
}

// Store is the app.Store.
type Store struct {
	q  queries.Querier
	tx Transactor
}

// New returns a Store over q, running what must be atomic through tx.
func New(q queries.Querier, tx Transactor) *Store { return &Store{q: q, tx: tx} }

var _ app.Store = (*Store)(nil)

func (s *Store) Groups(ctx context.Context) ([]domain.Group, error) {
	rows, err := s.q.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	groups := make([]domain.Group, 0, len(rows))
	for _, r := range rows {
		groups = append(groups, domain.Group{ID: r.ID, Name: r.Name, Admins: r.Admins})
	}
	return groups, nil
}

func (s *Store) Group(ctx context.Context, id uuid.UUID) (domain.Group, error) {
	r, err := s.q.GroupByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Group{}, app.ErrNotFound
	}
	if err != nil {
		return domain.Group{}, err
	}
	return domain.Group{ID: r.ID, Name: r.Name, Admins: r.Admins}, nil
}

func (s *Store) CreateGroup(ctx context.Context, g domain.Group) error {
	return taken(s.q.CreateGroup(ctx, queries.CreateGroupParams{ID: g.ID, Name: g.Name}))
}

func (s *Store) RenameGroup(ctx context.Context, id uuid.UUID, name domain.GroupName) error {
	n, err := s.q.RenameGroup(ctx, queries.RenameGroupParams{ID: id, Name: name.String()})
	if err != nil {
		return taken(err)
	}
	if n == 0 {
		return app.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteGroup(ctx, id)
}

func (s *Store) Memberships(ctx context.Context) ([]domain.Membership, error) {
	rows, err := s.q.ListMemberships(ctx)
	if err != nil {
		return nil, err
	}
	ms := make([]domain.Membership, 0, len(rows))
	for _, r := range rows {
		ms = append(ms, domain.Membership{GroupID: r.GroupID, UserID: r.UserID})
	}
	return ms, nil
}

func (s *Store) AddMember(ctx context.Context, m domain.Membership) error {
	return missing(s.q.AddMember(ctx, queries.AddMemberParams{GroupID: m.GroupID, UserID: m.UserID}))
}

// RemoveMember locks the Group first, so of two removals racing for the last two Admins, the
// second sees the first's.
func (s *Store) RemoveMember(ctx context.Context, m domain.Membership) error {
	return s.tx.InTx(ctx, func(q *queries.Queries) error {
		admins, err := q.LockGroup(ctx, m.GroupID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := q.RemoveMember(ctx, queries.RemoveMemberParams{GroupID: m.GroupID, UserID: m.UserID}); err != nil {
			return err
		}
		if !admins {
			return nil
		}
		left, err := q.CountMembers(ctx, m.GroupID)
		if err != nil {
			return err
		}
		if left == 0 {
			return app.ErrLastAdmin
		}
		return nil
	})
}

func (s *Store) IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error) {
	return s.q.IsAdmin(ctx, userID)
}

func (s *Store) Assignments(ctx context.Context) ([]domain.Assignment, error) {
	rows, err := s.q.ListAssignments(ctx)
	if err != nil {
		return nil, err
	}
	as := make([]domain.Assignment, 0, len(rows))
	for _, r := range rows {
		as = append(as, domain.Assignment{GroupID: r.GroupID, ClientID: r.ClientID, Role: r.Role})
	}
	return as, nil
}

func (s *Store) Assign(ctx context.Context, a domain.Assignment) error {
	return missing(s.q.Assign(ctx, queries.AssignParams{GroupID: a.GroupID, ClientID: a.ClientID, Role: a.Role}))
}

func (s *Store) Unassign(ctx context.Context, a domain.Assignment) error {
	return s.q.Unassign(ctx, queries.UnassignParams{GroupID: a.GroupID, ClientID: a.ClientID, Role: a.Role})
}

func (s *Store) RolesOf(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error) {
	return s.q.RolesOf(ctx, queries.RolesOfParams{UserID: userID, ClientID: clientID})
}

func taken(err error) error {
	if code(err) == uniqueViolation {
		return app.ErrNameTaken
	}
	return err
}

func missing(err error) error {
	if code(err) == foreignKeyViolation {
		return app.ErrNotFound
	}
	return err
}

func code(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
