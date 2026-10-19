// Package persistence is the audit context's Postgres adapter: app.Store over the sqlc queries.
package persistence

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/JorisJonkers-dev/identity/internal/audit/app"
	"github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
)

// Store is the app.Store.
type Store struct{ q queries.Querier }

// New returns a Store over q.
func New(q queries.Querier) *Store { return &Store{q: q} }

var _ app.Store = (*Store)(nil)

func (s *Store) Append(ctx context.Context, e domain.Event) error {
	return s.q.AppendAuditEvent(ctx, queries.AppendAuditEventParams{
		ID: e.ID, At: e.At, Kind: string(e.Action.Kind()), Action: string(e.Action),
		ActorID: id(e.Actor), UserID: id(e.User), ClientID: text(e.Client), GroupID: id(e.Group),
		GroupName: text(e.GroupName), Role: text(e.Role), Email: text(e.Email), Reason: text(e.Reason),
		Minutes: pgtype.Int4{Int32: int32(e.Minutes), Valid: e.Action == domain.StepUpChanged}, //nolint:gosec // 0 to 60
	})
}

func (s *Store) List(ctx context.Context, f domain.Filter, limit int) ([]domain.Event, error) {
	p := queries.ListAuditEventsParams{UserID: id(f.User), ClientID: text(f.Client), Kind: text(string(f.Kind)), Max: int32(min(limit, 1000))} //nolint:gosec // capped above
	if !f.Before.Zero() {
		p.BeforeAt = pgtype.Timestamptz{Time: f.Before.At, Valid: true}
		p.BeforeID = &f.Before.ID
	}
	rows, err := s.q.ListAuditEvents(ctx, p)
	if err != nil {
		return nil, err
	}
	events := make([]domain.Event, 0, len(rows))
	for _, r := range rows {
		events = append(events, domain.Event{
			ID: r.ID, At: r.At, Action: domain.Action(r.Action),
			Actor: deref(r.ActorID), User: deref(r.UserID), Client: r.ClientID.String, Group: deref(r.GroupID),
			GroupName: r.GroupName.String, Role: r.Role.String, Email: r.Email.String, Reason: r.Reason.String,
			Minutes: int(r.Minutes.Int32),
		})
	}
	return events, nil
}

func (s *Store) SignInsByHour(ctx context.Context, since time.Time) ([]app.Hour, error) {
	rows, err := s.q.CountSignInsByHour(ctx, since)
	if err != nil {
		return nil, err
	}
	hours := make([]app.Hour, 0, len(rows))
	for _, r := range rows {
		hours = append(hours, app.Hour{At: r.Hour, SignIns: int(r.SignedIn), Failed: int(r.Failed)})
	}
	return hours, nil
}

func (s *Store) DeleteBefore(ctx context.Context, t time.Time) (int64, error) {
	return s.q.DeleteAuditEventsBefore(ctx, t)
}

func id(u uuid.UUID) *uuid.UUID {
	if u == uuid.Nil {
		return nil
	}
	return &u
}

func deref(u *uuid.UUID) uuid.UUID {
	if u == nil {
		return uuid.Nil
	}
	return *u
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
