package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

// StepUps is the app.StepUpSettings: a row per Client with step-up on, none when it is off.
type StepUps struct {
	q   queries.Querier
	now func() time.Time
}

// NewStepUps returns StepUps over q, stamping each change with now.
func NewStepUps(q queries.Querier, now func() time.Time) *StepUps { return &StepUps{q: q, now: now} }

var _ app.StepUpSettings = (*StepUps)(nil)

func (s *StepUps) StepUp(ctx context.Context, clientID string) (int, error) {
	minutes, err := s.q.StepUpMinutes(ctx, clientID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return int(minutes), err
}

func (s *StepUps) SetStepUp(ctx context.Context, clientID string, minutes int) error {
	if minutes == 0 {
		return s.q.ClearStepUp(ctx, clientID)
	}
	return s.q.SetStepUp(ctx, queries.SetStepUpParams{ClientID: clientID, Minutes: int32(minutes), ChangedAt: s.now()}) //nolint:gosec // 5 to 60
}
