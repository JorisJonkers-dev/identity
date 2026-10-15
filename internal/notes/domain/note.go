// Package domain is the notes context's pure core: its value objects, entities and ports. It
// imports no transport, persistence or generated code (enforced by depguard).
package domain

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxTextLength is the longest note, in characters. db/migrations holds the same rule as a CHECK.
const MaxTextLength = 500

// ErrInvalidText is returned for text that is empty after trimming, or longer than MaxTextLength.
var ErrInvalidText = fmt.Errorf("a note needs between 1 and %d characters of text", MaxTextLength)

// Text is a note's validated text. The zero value is never handed out by NewText.
type Text struct {
	value string
}

// NewText trims raw and checks its length.
func NewText(raw string) (Text, error) {
	trimmed := strings.TrimSpace(raw)
	if n := utf8.RuneCountInString(trimmed); n == 0 || n > MaxTextLength {
		return Text{}, fmt.Errorf("%w: got %d", ErrInvalidText, n)
	}
	return Text{value: trimmed}, nil
}

func (t Text) String() string { return t.value }

// Note is a stored note.
type Note struct {
	ID        uuid.UUID
	Text      Text
	CreatedAt time.Time
}

// Repository is the port the persistence adapter implements.
type Repository interface {
	Create(ctx context.Context, text Text) (Note, error)
	// List returns at most limit notes, newest first.
	List(ctx context.Context, limit int) ([]Note, error)
}
