package persistence_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JorisJonkers-dev/template-go-vue/internal/notes/adapters/persistence"
	"github.com/JorisJonkers-dev/template-go-vue/internal/notes/domain"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/pg"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/pg/pgtest"
)

func repository(t *testing.T) *persistence.Repository {
	t.Helper()
	store, err := pg.Open(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return persistence.New(store.Queries())
}

func text(t *testing.T, raw string) domain.Text {
	t.Helper()
	v, err := domain.NewText(raw)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCreateThenListNewestFirst(t *testing.T) {
	t.Parallel()
	repo := repository(t)
	first, err := repo.Create(t.Context(), text(t, "first"))
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(first.CreatedAt) > time.Minute || first.CreatedAt.Location() != time.UTC || first.Text.String() != "first" {
		t.Fatalf("created %+v", first)
	}
	if _, err := repo.Create(t.Context(), text(t, "second")); err != nil {
		t.Fatal(err)
	}

	notes, err := repo.List(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || notes[0].Text.String() != "second" || notes[1].ID != first.ID {
		t.Fatalf("listed %+v, want second then first", notes)
	}

	if notes, err := repo.List(t.Context(), 1); err != nil || len(notes) != 1 {
		t.Fatalf("List(1) = %d notes, %v", len(notes), err)
	}
}

func TestListRefusesARowTheDomainWouldNot(t *testing.T) {
	t.Parallel()
	url := pgtest.URL(t)
	conn, err := pgx.Connect(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	if _, err := conn.Exec(t.Context(), "INSERT INTO notes (text) VALUES ('   ')"); err != nil {
		t.Fatal(err)
	}
	store, err := pg.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := persistence.New(store.Queries()).List(t.Context(), 10); !errors.Is(err, domain.ErrInvalidText) {
		t.Fatalf("List error = %v, want ErrInvalidText", err)
	}
}

func TestListRefusesALimitOutOfRange(t *testing.T) {
	t.Parallel()
	if _, err := persistence.New(nil).List(t.Context(), -1); err == nil {
		t.Fatal("List(-1) succeeded")
	}
}

func TestFailuresSurface(t *testing.T) {
	t.Parallel()
	store, err := pg.Open(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	repo := persistence.New(store.Queries())
	store.Close()
	if _, err := repo.Create(t.Context(), text(t, "x")); err == nil {
		t.Fatal("Create on a closed pool succeeded")
	}
	if _, err := repo.List(t.Context(), 1); err == nil {
		t.Fatal("List on a closed pool succeeded")
	}
}
