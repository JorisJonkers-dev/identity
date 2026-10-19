// Package persistence is the legacy context's Postgres adapter: Legacy Auth's database as the
// app.Source, and identity's as the app.Target.
package persistence

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/identity/internal/legacy/app"
	"github.com/JorisJonkers-dev/identity/internal/legacy/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	usersdomain "github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// Querier runs SQL against Legacy Auth's database, whose schema identity does not own.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// AuthAPI is the app.Source: auth-api's tables as its migrations V1 to V10 leave them.
type AuthAPI struct{ db Querier }

// NewAuthAPI returns the Source over db.
func NewAuthAPI(db Querier) *AuthAPI { return &AuthAPI{db: db} }

var _ app.Source = (*AuthAPI)(nil)

// accounts reads each account with its grants; an authenticator counts only once its setup finished.
const accounts = `
SELECT u.id, u.username, u.email, u.first_name, u.last_name, u.password_hash,
       CASE WHEN u.totp_enabled THEN coalesce(u.totp_secret, '') ELSE '' END,
       u.role = 'ADMIN', u.email_confirmed, u.created_at,
       coalesce(array_agg(p.service ORDER BY p.service) FILTER (WHERE p.service IS NOT NULL), '{}')
FROM app_user u LEFT JOIN user_service_permissions p ON p.user_id = u.id
GROUP BY u.id
ORDER BY u.created_at, u.email`

func (a *AuthAPI) Accounts(ctx context.Context) ([]domain.Account, error) {
	rows, err := a.db.Query(ctx, accounts)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Account, error) {
		var x domain.Account
		err := row.Scan(&x.ID, &x.Username, &x.Email, &x.FirstName, &x.LastName, &x.PasswordHash,
			&x.TOTPSecret, &x.Admin, &x.Confirmed, &x.CreatedAt, &x.Services)
		return x, err
	})
}

// Transactor runs queries in one transaction.
type Transactor interface {
	InTx(ctx context.Context, fn func(*queries.Queries) error) error
}

// Identity is the app.Target.
type Identity struct {
	q   queries.Querier
	tx  Transactor
	now func() time.Time
}

// NewIdentity returns the Target over q, importing each person through tx.
func NewIdentity(q queries.Querier, tx Transactor, now func() time.Time) *Identity {
	return &Identity{q: q, tx: tx, now: now}
}

var _ app.Target = (*Identity)(nil)

func (s *Identity) State(ctx context.Context, id uuid.UUID, email usersdomain.Email) (app.State, error) {
	r, err := s.q.LegacyState(ctx, queries.LegacyStateParams{ID: id, Email: email.String()})
	return app.State{Imported: r.Imported, Taken: r.Taken}, err
}

// authenticatorLabel names an imported authenticator app, which Legacy Auth never named.
const authenticatorLabel = "Authenticator app"

func (s *Identity) Import(ctx context.Context, p app.Person) (bool, error) {
	imported := false
	err := s.tx.InTx(ctx, func(q *queries.Queries) error {
		n, err := q.ImportUser(ctx, queries.ImportUserParams{
			ID: p.ID, Email: p.Email.String(), Name: p.Name.String(), PasswordHash: p.PasswordHash, CreatedAt: p.CreatedAt,
		})
		if err != nil || n == 0 {
			return err
		}
		imported = true
		return s.carry(ctx, q, p)
	})
	return imported && err == nil, err
}

// carry adds what comes with a new person: their authenticator app, their roles and their place
// in the admins Group.
func (s *Identity) carry(ctx context.Context, q *queries.Queries, p app.Person) error {
	if p.SealedTOTP != "" {
		if err := q.CreateTOTPFactor(ctx, queries.CreateTOTPFactorParams{
			ID: uuid.New(), UserID: p.ID, Label: authenticatorLabel, SealedSecret: p.SealedTOTP, CreatedAt: s.now(),
		}); err != nil {
			return err
		}
	}
	for _, c := range p.Clients {
		if _, err := q.AssignUser(ctx, queries.AssignUserParams{UserID: p.ID, ClientID: c, Role: "user"}); err != nil {
			return err
		}
	}
	if !p.Admin {
		return nil
	}
	admins, err := q.AdminsGroup(ctx)
	if err != nil {
		return err
	}
	_, err = q.AddMember(ctx, queries.AddMemberParams{GroupID: admins.ID, UserID: p.ID})
	return err
}
