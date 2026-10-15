package clients

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

func TestADeleteSeenOnlyAfterTheFactStillRemoves(t *testing.T) {
	ctx := context.Background()
	x := NewIndex()
	x.Put(domain.Client{ID: "grafana", Host: "grafana.jorisjonkers.dev"})
	h := handlers(ctx, x, slog.New(slog.DiscardHandler))

	h.OnDelete("not an object")
	if _, err := x.Client(ctx, "grafana"); err != nil {
		t.Fatalf("a delete of nothing removed a Client: %v", err)
	}
	u := &unstructured.Unstructured{}
	u.SetName("grafana")
	h.OnDelete(cache.DeletedFinalStateUnknown{Key: "grafana", Obj: u})
	if _, err := x.Client(ctx, "grafana"); err == nil {
		t.Fatal("a late delete left the Client")
	}
}

func TestOnlyAnObjectIsAClient(t *testing.T) {
	if _, err := fromObject("not an object"); !errors.Is(err, errNotAClient) {
		t.Fatalf("fromObject = %v", err)
	}
}
