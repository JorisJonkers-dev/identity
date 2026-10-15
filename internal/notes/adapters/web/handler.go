// Package web is the notes context's inbound HTTP adapter: it implements the notes operations of
// the generated ogen Handler and maps between the contract's types and the domain's.
package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/JorisJonkers-dev/template-go-vue/internal/notes/domain"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/httpx"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/oas"
)

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	Create(ctx context.Context, raw string) (domain.Note, error)
	List(ctx context.Context, limit int) ([]domain.Note, error)
}

// Handler implements the notes operations. Business rules stay in the use cases.
type Handler struct {
	uc UseCases
}

// New returns a Handler calling uc.
func New(uc UseCases) *Handler {
	return &Handler{uc: uc}
}

// CreateNote implements createNote.
func (h *Handler) CreateNote(ctx context.Context, req *oas.NewNote) (oas.CreateNoteRes, error) {
	note, err := h.uc.Create(ctx, req.Text)
	if errors.Is(err, domain.ErrInvalidText) {
		return &oas.CreateNoteUnprocessableEntity{
			StatusCode: http.StatusUnprocessableEntity,
			Response:   httpx.Problem(http.StatusUnprocessableEntity, domain.ErrInvalidText.Error()),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	out := toNote(note)
	return &out, nil
}

// ListNotes implements listNotes.
func (h *Handler) ListNotes(ctx context.Context, params oas.ListNotesParams) (oas.ListNotesRes, error) {
	notes, err := h.uc.List(ctx, int(params.Limit.Or(0)))
	if err != nil {
		return nil, err
	}
	items := make([]oas.Note, 0, len(notes))
	for _, n := range notes {
		items = append(items, toNote(n))
	}
	return &oas.NoteList{Items: items}, nil
}

func toNote(n domain.Note) oas.Note {
	return oas.Note{ID: n.ID, Text: n.Text.String(), CreatedAt: n.CreatedAt}
}
