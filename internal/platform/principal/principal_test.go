package principal_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
)

func TestFromIsWhoWithPut(t *testing.T) {
	p := principal.Principal{UserID: uuid.New(), TwoFactor: true}
	if got := principal.From(principal.With(context.Background(), p)); got != p {
		t.Fatalf("From = %+v, want %+v", got, p)
	}
	if got := principal.From(context.Background()); got != (principal.Principal{}) {
		t.Fatalf("From of nobody = %+v", got)
	}
}
