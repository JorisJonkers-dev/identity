package clients_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

func TestStatic(t *testing.T) {
	s := clients.Static{"spa": {ID: "spa"}}
	if c, err := s.Client(context.Background(), "spa"); err != nil || c.ID != "spa" {
		t.Fatalf("Client(spa) = %+v, %v", c, err)
	}
	if _, err := s.Client(context.Background(), "nobody"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown Client: %v", err)
	}
}
