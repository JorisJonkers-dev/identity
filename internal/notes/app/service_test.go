package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/template-go-vue/internal/notes/app"
	"github.com/JorisJonkers-dev/template-go-vue/internal/notes/domain"
)

// memory is an in-memory domain.Repository; it records the last limit it was asked for.
type memory struct {
	notes     []domain.Note
	lastLimit int
	err       error
}

func (m *memory) Create(_ context.Context, text domain.Text) (domain.Note, error) {
	if m.err != nil {
		return domain.Note{}, m.err
	}
	n := domain.Note{ID: uuid.New(), Text: text, CreatedAt: time.Now()}
	m.notes = append([]domain.Note{n}, m.notes...)
	return n, nil
}

func (m *memory) List(_ context.Context, limit int) ([]domain.Note, error) {
	m.lastLimit = limit
	if m.err != nil {
		return nil, m.err
	}
	return m.notes[:min(limit, len(m.notes))], nil
}

func TestCreateStoresTrimmedText(t *testing.T) {
	repo := &memory{}
	note, err := app.New(repo).Create(t.Context(), "  Buy milk ")
	if err != nil {
		t.Fatal(err)
	}
	if note.Text.String() != "Buy milk" || len(repo.notes) != 1 {
		t.Fatalf("created %+v, stored %d", note, len(repo.notes))
	}
}

func TestCreateRefusesInvalidTextWithoutStoring(t *testing.T) {
	repo := &memory{}
	if _, err := app.New(repo).Create(t.Context(), "   "); !errors.Is(err, domain.ErrInvalidText) {
		t.Fatalf("error = %v, want ErrInvalidText", err)
	}
	if len(repo.notes) != 0 {
		t.Fatal("an invalid note reached the repository")
	}
}

func TestCreateWrapsRepositoryFailure(t *testing.T) {
	boom := errors.New("boom")
	if _, err := app.New(&memory{err: boom}).Create(t.Context(), "x"); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
}

func TestListDefaultsAndCapsTheLimit(t *testing.T) {
	cases := []struct{ asked, want int }{
		{0, app.DefaultLimit},
		{-3, app.DefaultLimit},
		{7, 7},
		{app.MaxLimit + 1, app.MaxLimit},
	}
	for _, tc := range cases {
		repo := &memory{}
		if _, err := app.New(repo).List(t.Context(), tc.asked); err != nil {
			t.Fatal(err)
		}
		if repo.lastLimit != tc.want {
			t.Fatalf("List(%d) asked the repository for %d, want %d", tc.asked, repo.lastLimit, tc.want)
		}
	}
}

func TestListWrapsRepositoryFailure(t *testing.T) {
	boom := errors.New("boom")
	if _, err := app.New(&memory{err: boom}).List(t.Context(), 1); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
}
