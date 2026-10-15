package web_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/notes/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/notes/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
)

// useCases answers with whatever the test sets, and records the limit it was asked for.
type useCases struct {
	note      domain.Note
	notes     []domain.Note
	err       error
	limitSeen int
}

func (u *useCases) Create(context.Context, string) (domain.Note, error) { return u.note, u.err }

func (u *useCases) List(_ context.Context, limit int) ([]domain.Note, error) {
	u.limitSeen = limit
	return u.notes, u.err
}

var errStore = errors.New("store down")

func note(t *testing.T, text string) domain.Note {
	t.Helper()
	txt, err := domain.NewText(text)
	if err != nil {
		t.Fatal(err)
	}
	return domain.Note{ID: uuid.New(), Text: txt, CreatedAt: time.Unix(1_700_000_000, 0).UTC()}
}

func TestCreateNoteReturnsTheNote(t *testing.T) {
	n := note(t, "hello")
	res, err := web.New(&useCases{note: n}).CreateNote(t.Context(), &oas.NewNote{Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := res.(*oas.Note)
	if !ok || got.ID != n.ID || got.Text != "hello" || !got.CreatedAt.Equal(n.CreatedAt) {
		t.Fatalf("CreateNote = %#v, want the note", res)
	}
}

func TestCreateNoteRefusesInvalidTextAsA422Problem(t *testing.T) {
	res, err := web.New(&useCases{err: domain.ErrInvalidText}).CreateNote(t.Context(), &oas.NewNote{})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := res.(*oas.CreateNoteUnprocessableEntity)
	if !ok || got.StatusCode != http.StatusUnprocessableEntity || got.Response.Status != http.StatusUnprocessableEntity {
		t.Fatalf("CreateNote = %#v, want a 422 problem", res)
	}
}

func TestCreateNotePassesOtherFailuresOn(t *testing.T) {
	if _, err := web.New(&useCases{err: errStore}).CreateNote(t.Context(), &oas.NewNote{Text: "x"}); !errors.Is(err, errStore) {
		t.Fatalf("CreateNote error = %v, want %v", err, errStore)
	}
}

func TestListNotesMapsEveryNoteAndPassesTheLimit(t *testing.T) {
	uc := &useCases{notes: []domain.Note{note(t, "a"), note(t, "b")}}
	res, err := web.New(uc).ListNotes(t.Context(), oas.ListNotesParams{Limit: oas.NewOptInt32(7)})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := res.(*oas.NoteList)
	if !ok || len(got.Items) != 2 || got.Items[0].Text != "a" || got.Items[1].Text != "b" {
		t.Fatalf("ListNotes = %#v, want both notes in order", res)
	}
	if uc.limitSeen != 7 {
		t.Fatalf("use case asked for %d notes, want 7", uc.limitSeen)
	}
}

func TestListNotesWithoutALimitAsksForTheDefault(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc).ListNotes(t.Context(), oas.ListNotesParams{})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := res.(*oas.NoteList); !ok || got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("ListNotes = %#v, want an empty, non-nil list", res)
	}
	if uc.limitSeen != 0 {
		t.Fatalf("use case asked for %d notes, want 0 (its default)", uc.limitSeen)
	}
}

func TestListNotesPassesFailuresOn(t *testing.T) {
	if _, err := web.New(&useCases{err: errStore}).ListNotes(t.Context(), oas.ListNotesParams{}); !errors.Is(err, errStore) {
		t.Fatalf("ListNotes error = %v, want %v", err, errStore)
	}
}
