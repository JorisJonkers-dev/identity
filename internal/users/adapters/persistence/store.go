// Package persistence is the users context's Postgres adapter: app.Store over the sqlc queries.
package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

const uniqueViolation = "23505"

// Store is the app.Store.
type Store struct{ q queries.Querier }

// New returns a Store over q.
func New(q queries.Querier) *Store { return &Store{q: q} }

var _ app.Store = (*Store)(nil)

func (s *Store) CreateUser(ctx context.Context, u domain.User) error {
	err := s.q.CreateUser(ctx, queries.CreateUserParams{
		ID: u.ID, Email: u.Email.String(), Name: u.Name.String(), PasswordHash: u.PasswordHash, CreatedAt: u.CreatedAt,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return app.ErrEmailTaken
	}
	return err
}

func (s *Store) UserByEmail(ctx context.Context, e domain.Email) (domain.User, error) {
	row, err := s.q.UserByEmail(ctx, e.String())
	if err != nil {
		return domain.User{}, notFound(err)
	}
	return toUser(row.ID, row.Email, row.Name, row.PasswordHash, row.CreatedAt)
}

func (s *Store) UserByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	row, err := s.q.UserByID(ctx, id)
	if err != nil {
		return domain.User{}, notFound(err)
	}
	return toUser(row.ID, row.Email, row.Name, row.PasswordHash, row.CreatedAt)
}

func (s *Store) UpdateName(ctx context.Context, id uuid.UUID, n domain.Name) error {
	return s.q.UpdateUserName(ctx, queries.UpdateUserNameParams{ID: id, Name: n.String()})
}

func (s *Store) UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error {
	return s.q.UpdateUserPassword(ctx, queries.UpdateUserPasswordParams{ID: id, PasswordHash: hash})
}

func (s *Store) CreateSignup(ctx context.Context, su app.Signup) error {
	return s.q.CreateSignup(ctx, queries.CreateSignupParams{
		TokenHash: su.TokenHash, Email: su.Email.String(), Name: su.Name.String(), PasswordHash: su.PasswordHash, ExpiresAt: su.ExpiresAt,
	})
}

func (s *Store) TakeSignup(ctx context.Context, hash []byte) (app.Signup, error) {
	row, err := s.q.TakeSignup(ctx, hash)
	if err != nil {
		return app.Signup{}, notFound(err)
	}
	email, err := domain.NewEmail(row.Email)
	if err != nil {
		return app.Signup{}, err
	}
	name, err := domain.NewName(row.Name)
	if err != nil {
		return app.Signup{}, err
	}
	return app.Signup{TokenHash: hash, Email: email, Name: name, PasswordHash: row.PasswordHash, ExpiresAt: row.ExpiresAt}, nil
}

func (s *Store) CreateSession(ctx context.Context, sess app.Session) error {
	return s.q.CreateSession(ctx, queries.CreateSessionParams{
		TokenHash: sess.TokenHash, UserID: sess.UserID, CreatedAt: sess.CreatedAt, ExpiresAt: sess.ExpiresAt,
	})
}

func (s *Store) SessionByHash(ctx context.Context, hash []byte) (app.Session, error) {
	row, err := s.q.SessionByHash(ctx, hash)
	if err != nil {
		return app.Session{}, notFound(err)
	}
	return app.Session{TokenHash: row.TokenHash, UserID: row.UserID, LastSeenAt: row.LastSeenAt, ExpiresAt: row.ExpiresAt}, nil
}

func (s *Store) TouchSession(ctx context.Context, hash []byte, lastSeen, expires time.Time) error {
	return s.q.TouchSession(ctx, queries.TouchSessionParams{TokenHash: hash, LastSeenAt: lastSeen, ExpiresAt: expires})
}

func (s *Store) DeleteSession(ctx context.Context, hash []byte) error {
	return s.q.DeleteSession(ctx, hash)
}

func (s *Store) DeleteOtherSessions(ctx context.Context, userID uuid.UUID, keep []byte) error {
	return s.q.DeleteOtherSessions(ctx, queries.DeleteOtherSessionsParams{UserID: userID, TokenHash: keep})
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// toUser rebuilds a User from a row, through the domain's constructors, so a row the rules would
// refuse is an error rather than a User nobody could have made.
func toUser(id uuid.UUID, rawEmail, rawName, hash string, created time.Time) (domain.User, error) {
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return domain.User{}, err
	}
	name, err := domain.NewName(rawName)
	if err != nil {
		return domain.User{}, err
	}
	return domain.User{ID: id, Email: email, Name: name, PasswordHash: hash, CreatedAt: created}, nil
}
