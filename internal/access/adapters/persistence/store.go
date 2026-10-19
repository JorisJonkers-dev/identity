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
		groups = append(groups, domain.Group{ID: r.ID, Name: r.Name, Admins: r.Admins, RequiresTwoFactor: r.RequiresTwoFactor})
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
	return domain.Group{ID: r.ID, Name: r.Name, Admins: r.Admins, RequiresTwoFactor: r.RequiresTwoFactor}, nil
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

func (s *Store) SetGroupTwoFactor(ctx context.Context, id uuid.UUID, required bool) error {
	n, err := s.q.SetGroupTwoFactor(ctx, queries.SetGroupTwoFactorParams{ID: id, RequiresTwoFactor: required})
	if err != nil {
		return err
	}
	if n == 0 {
		return app.ErrNotFound
	}
	return nil
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

func (s *Store) AddMember(ctx context.Context, m domain.Membership) (bool, error) {
	n, err := s.q.AddMember(ctx, queries.AddMemberParams{GroupID: m.GroupID, UserID: m.UserID})
	return n > 0, missing(err)
}

// RemoveMember locks the Group first, so of two removals racing for the last two Admins, the
// second sees the first's.
func (s *Store) RemoveMember(ctx context.Context, m domain.Membership) (bool, error) {
	var removed int64
	err := s.tx.InTx(ctx, func(q *queries.Queries) error {
		admins, err := q.LockGroup(ctx, m.GroupID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		removed, err = q.RemoveMember(ctx, queries.RemoveMemberParams{GroupID: m.GroupID, UserID: m.UserID})
		if err != nil {
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
	return err == nil && removed > 0, err
}

func (s *Store) IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error) {
	return s.q.IsAdmin(ctx, userID)
}

func (s *Store) AdminsGroup(ctx context.Context) (domain.Group, error) {
	r, err := s.q.AdminsGroup(ctx)
	if err != nil {
		return domain.Group{}, err
	}
	return domain.Group{ID: r.ID, Name: r.Name, Admins: r.Admins, RequiresTwoFactor: r.RequiresTwoFactor}, nil
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	n, err := s.q.CountAdmins(ctx)
	return int(n), err
}

func (s *Store) Unassigned(ctx context.Context) ([]uuid.UUID, error) {
	return s.q.ListUnassigned(ctx)
}

func (s *Store) CountUnassigned(ctx context.Context) (int, error) {
	n, err := s.q.CountUnassigned(ctx)
	return int(n), err
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

func (s *Store) Assign(ctx context.Context, a domain.Assignment) (bool, error) {
	n, err := s.q.Assign(ctx, queries.AssignParams{GroupID: a.GroupID, ClientID: a.ClientID, Role: a.Role})
	return n > 0, missing(err)
}

func (s *Store) Unassign(ctx context.Context, a domain.Assignment) (bool, error) {
	n, err := s.q.Unassign(ctx, queries.UnassignParams{GroupID: a.GroupID, ClientID: a.ClientID, Role: a.Role})
	return n > 0, err
}

func (s *Store) UserAssignments(ctx context.Context) ([]domain.UserAssignment, error) {
	rows, err := s.q.ListUserAssignments(ctx)
	if err != nil {
		return nil, err
	}
	as := make([]domain.UserAssignment, 0, len(rows))
	for _, r := range rows {
		as = append(as, domain.UserAssignment{UserID: r.UserID, ClientID: r.ClientID, Role: r.Role})
	}
	return as, nil
}

func (s *Store) AssignUser(ctx context.Context, a domain.UserAssignment) (bool, error) {
	n, err := s.q.AssignUser(ctx, queries.AssignUserParams{UserID: a.UserID, ClientID: a.ClientID, Role: a.Role})
	return n > 0, missing(err)
}

func (s *Store) UnassignUser(ctx context.Context, a domain.UserAssignment) (bool, error) {
	n, err := s.q.UnassignUser(ctx, queries.UnassignUserParams{UserID: a.UserID, ClientID: a.ClientID, Role: a.Role})
	return n > 0, err
}

func (s *Store) RolesOf(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error) {
	return s.q.RolesOf(ctx, queries.RolesOfParams{UserID: userID, ClientID: clientID})
}

func (s *Store) RolesByClient(ctx context.Context, userID uuid.UUID) (map[string][]string, error) {
	rows, err := s.q.RolesByClient(ctx, userID)
	if err != nil {
		return nil, err
	}
	held := map[string][]string{}
	for _, r := range rows {
		held[r.ClientID] = append(held[r.ClientID], r.Role)
	}
	return held, nil
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
