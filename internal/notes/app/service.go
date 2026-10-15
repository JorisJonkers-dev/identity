// Package app holds the notes context's use cases: the only entry points the inbound adapters call.
package app

import (
	"context"
	"fmt"

	"github.com/JorisJonkers-dev/template-go-vue/internal/notes/domain"
)

const (
	// DefaultLimit is how many notes List returns when the caller names no limit.
	DefaultLimit = 50
	// MaxLimit is the most notes one List returns.
	MaxLimit = 100
)

// Service runs the notes use cases against a repository.
type Service struct {
	repo domain.Repository
}

// New returns a Service backed by repo.
func New(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

// Create validates raw and stores it as a note.
func (s *Service) Create(ctx context.Context, raw string) (domain.Note, error) {
	text, err := domain.NewText(raw)
	if err != nil {
		return domain.Note{}, err
	}
	note, err := s.repo.Create(ctx, text)
	if err != nil {
		return domain.Note{}, fmt.Errorf("create note: %w", err)
	}
	return note, nil
}

// List returns up to limit notes, newest first. A limit below 1 means DefaultLimit.
func (s *Service) List(ctx context.Context, limit int) ([]domain.Note, error) {
	switch {
	case limit < 1:
		limit = DefaultLimit
	case limit > MaxLimit:
		limit = MaxLimit
	}
	notes, err := s.repo.List(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	return notes, nil
}
