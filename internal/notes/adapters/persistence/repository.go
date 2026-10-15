// Package persistence implements the notes Repository port on the sqlc-generated queries.
package persistence

import (
	"context"
	"fmt"
	"math"

	"github.com/JorisJonkers-dev/template-go-vue/internal/notes/domain"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/pg/queries"
)

// Repository maps rows to domain notes, so the domain never sees a generated type.
type Repository struct {
	q queries.Querier
}

var _ domain.Repository = (*Repository)(nil)

// New returns a Repository over q.
func New(q queries.Querier) *Repository {
	return &Repository{q: q}
}

// Create stores text and returns the note the database assigned an id and a time.
func (r *Repository) Create(ctx context.Context, text domain.Text) (domain.Note, error) {
	row, err := r.q.CreateNote(ctx, text.String())
	if err != nil {
		return domain.Note{}, fmt.Errorf("insert note: %w", err)
	}
	return toDomain(row)
}

// List returns at most limit notes, newest first.
func (r *Repository) List(ctx context.Context, limit int) ([]domain.Note, error) {
	if limit < 0 || limit > math.MaxInt32 {
		return nil, fmt.Errorf("select notes: limit %d out of range", limit)
	}
	rows, err := r.q.ListNotes(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("select notes: %w", err)
	}
	notes := make([]domain.Note, 0, len(rows))
	for _, row := range rows {
		n, err := toDomain(row)
		if err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, nil
}

// toDomain re-validates the text (the CHECK bounds its length, but only the domain trims it), and
// hands out times in UTC whatever the connection's time zone.
func toDomain(row queries.Note) (domain.Note, error) {
	text, err := domain.NewText(row.Text)
	if err != nil {
		return domain.Note{}, fmt.Errorf("note %s: %w", row.ID, err)
	}
	return domain.Note{ID: row.ID, Text: text, CreatedAt: row.CreatedAt.UTC()}, nil
}
