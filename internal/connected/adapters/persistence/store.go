// Package persistence is the connected context's Postgres adapter: app.Store over the sqlc queries.
package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/identity/internal/connected/app"
	"github.com/JorisJonkers-dev/identity/internal/connected/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
)

const (
	kindAccess  = "access"
	kindRefresh = "refresh"
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

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

func (s *Store) CreateRegistration(ctx context.Context, r domain.Registration) error {
	return s.q.CreateRegistration(ctx, queries.CreateRegistrationParams{ClientID: r.ClientID, Name: r.Name, RedirectUris: r.RedirectURIs, CreatedAt: r.CreatedAt})
}

func (s *Store) Registration(ctx context.Context, clientID string) (domain.Registration, error) {
	r, err := s.q.RegistrationByID(ctx, clientID)
	if err != nil {
		return domain.Registration{}, notFound(err)
	}
	return domain.Registration{ClientID: r.ClientID, Name: r.Name, RedirectURIs: r.RedirectUris, CreatedAt: r.CreatedAt}, nil
}

func (s *Store) PruneRegistrations(ctx context.Context, before time.Time) (int64, error) {
	return s.q.PruneRegistrations(ctx, before)
}

func (s *Store) MCPResources(ctx context.Context) ([]string, error) {
	return s.q.MCPResources(ctx)
}

func (s *Store) IsMCPResource(ctx context.Context, clientID string) (bool, error) {
	return s.q.IsMCPResource(ctx, clientID)
}

func (s *Store) SetMCPResource(ctx context.Context, clientID string, on bool, at time.Time) error {
	if on {
		return s.q.MarkMCPResource(ctx, queries.MarkMCPResourceParams{ClientID: clientID, CreatedAt: at})
	}
	return s.q.UnmarkMCPResource(ctx, clientID)
}

// Consent is new when the id kept is the one offered.
func (s *Store) Consent(ctx context.Context, a domain.ConnectedApp, registrationID string) (domain.ConnectedApp, bool, error) {
	id, err := s.q.ConsentToConnectedApp(ctx, queries.ConsentToConnectedAppParams{
		ID: a.ID, RegistrationID: registrationID, UserID: a.UserID, ClientID: a.ClientID, CreatedAt: a.CreatedAt,
	})
	if err != nil {
		return domain.ConnectedApp{}, false, err
	}
	if id == a.ID {
		return a, true, nil
	}
	kept, err := s.ConnectedApp(ctx, id)
	return kept, false, err
}

func (s *Store) Consented(ctx context.Context, registrationID string, userID uuid.UUID, clientID string) (uuid.UUID, error) {
	id, err := s.q.ConnectedAppFor(ctx, queries.ConnectedAppForParams{RegistrationID: registrationID, UserID: userID, ClientID: clientID})
	return id, notFound(err)
}

func (s *Store) ConnectedApps(ctx context.Context, userID uuid.UUID) ([]domain.ConnectedApp, error) {
	rows, err := s.q.ConnectedAppsOfUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	apps := make([]domain.ConnectedApp, 0, len(rows))
	for _, r := range rows {
		apps = append(apps, domain.ConnectedApp{ID: r.ID, UserID: userID, ClientID: r.ClientID, Name: r.Name, CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt})
	}
	return apps, nil
}

func (s *Store) ConnectedApp(ctx context.Context, id uuid.UUID) (domain.ConnectedApp, error) {
	r, err := s.q.ConnectedAppByID(ctx, id)
	if err != nil {
		return domain.ConnectedApp{}, notFound(err)
	}
	return domain.ConnectedApp{ID: r.ID, UserID: r.UserID, ClientID: r.ClientID, Name: r.Name, CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt}, nil
}

func (s *Store) DeleteConnectedApp(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	var tokens []uuid.UUID
	err := s.tx.InTx(ctx, func(q *queries.Queries) error {
		var err error
		if tokens, err = q.AccessTokensOfApp(ctx, id); err != nil {
			return err
		}
		_, err = q.DeleteConnectedApp(ctx, id)
		return err
	})
	return tokens, err
}

func (s *Store) DeleteForClient(ctx context.Context, clientID string) ([]domain.ConnectedApp, []uuid.UUID, error) {
	var apps []domain.ConnectedApp
	var tokens []uuid.UUID
	err := s.tx.InTx(ctx, func(q *queries.Queries) error {
		rows, err := q.ConnectedAppsForClient(ctx, clientID)
		if err != nil {
			return err
		}
		for _, r := range rows {
			ids, err := q.AccessTokensOfApp(ctx, r.ID)
			if err != nil {
				return err
			}
			apps = append(apps, domain.ConnectedApp{ID: r.ID, UserID: r.UserID, ClientID: clientID, Name: r.Name})
			tokens = append(tokens, ids...)
		}
		return q.DeleteConnectedAppsForClient(ctx, clientID)
	})
	if err != nil {
		return nil, nil, err
	}
	return apps, tokens, nil
}

func (s *Store) CountForClient(ctx context.Context, clientID string) (int, error) {
	n, err := s.q.CountConnectedAppsForClient(ctx, clientID)
	return int(n), err
}

func (s *Store) Touch(ctx context.Context, appID uuid.UUID, at time.Time) error {
	return s.q.TouchConnectedApp(ctx, queries.TouchConnectedAppParams{ID: appID, LastUsedAt: &at})
}

func (s *Store) CreateAccess(ctx context.Context, id, appID uuid.UUID, at, expires time.Time) error {
	return s.q.CreateConnectedAppToken(ctx, queries.CreateConnectedAppTokenParams{ID: id, ConnectedAppID: appID, Kind: kindAccess, CreatedAt: at, ExpiresAt: expires})
}

func (s *Store) CreateRefresh(ctx context.Context, id, appID uuid.UUID, hash []byte, at, expires time.Time) error {
	return s.q.CreateConnectedAppToken(ctx, queries.CreateConnectedAppTokenParams{ID: id, ConnectedAppID: appID, Kind: kindRefresh, Hash: hash, CreatedAt: at, ExpiresAt: expires})
}

func (s *Store) Access(ctx context.Context, id uuid.UUID) (app.AccessToken, error) {
	r, err := s.q.ConnectedAppAccess(ctx, id)
	if err != nil {
		return app.AccessToken{}, notFound(err)
	}
	return app.AccessToken{ID: r.ID, AppID: r.AppID, UserID: r.UserID, ClientID: r.ClientID, ExpiresAt: r.ExpiresAt}, nil
}

func (s *Store) Refresh(ctx context.Context, hash []byte) (app.RefreshToken, error) {
	r, err := s.q.ConnectedAppRefresh(ctx, hash)
	if err != nil {
		return app.RefreshToken{}, notFound(err)
	}
	return app.RefreshToken{ID: r.ID, AppID: r.AppID, UserID: r.UserID, ClientID: r.ClientID, ExpiresAt: r.ExpiresAt}, nil
}

func (s *Store) Spend(ctx context.Context, id uuid.UUID, at time.Time) (bool, error) {
	n, err := s.q.SpendRefresh(ctx, queries.SpendRefreshParams{ID: id, UsedAt: &at})
	return n > 0, err
}

func (s *Store) PruneTokens(ctx context.Context, before time.Time) (int64, error) {
	return s.q.PruneConnectedAppTokens(ctx, before)
}
