package principal_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
)

func TestFromIsWhoWithPut(t *testing.T) {
	id := uuid.New()
	if got := principal.From(principal.With(context.Background(), id)); got != id {
		t.Fatalf("From = %v, want %v", got, id)
	}
	if got := principal.From(context.Background()); got != uuid.Nil {
		t.Fatalf("From of nobody = %v", got)
	}
}
