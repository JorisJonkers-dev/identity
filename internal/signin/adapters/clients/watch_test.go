package clients_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

func clientObject(name string, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "identity.jorisjonkers.dev/v1",
		"kind":       "Client",
		"metadata":   map[string]any{"name": name},
		"spec":       spec,
	}}
}

func grafana(host string) map[string]any {
	return map[string]any{
		"signIn": "native", "host": host, "label": "Grafana", "description": "Dashboards",
		"launchUrl": "https://" + host + "/", "iconUrl": "https://" + host + "/icon.svg",
		"redirectUris": []any{"https://" + host + "/login/generic_oauth"}, "backChannelLogoutUri": "https://" + host + "/logout",
		"credential": "secret", "secretPath": "secret/data/monitoring/grafana/client",
		"roles": []any{"user", "viewer"},
	}
}

// eventually waits for cond, the way an informer's handlers arrive: soon, not at once.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("never: %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWatchFollowsClientObjects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{clients.Resource(): "ClientList"}, clientObject("grafana", grafana("grafana.jorisjonkers.dev")))
	x := clients.NewIndex()
	synced := clients.Watch(ctx, dyn, x, slog.New(slog.DiscardHandler))
	eventually(t, "the first list arrives", synced)

	c, err := x.ClientForHost(ctx, "grafana.jorisjonkers.dev")
	want := domain.Client{
		ID: "grafana", SignIn: domain.Native, Host: "grafana.jorisjonkers.dev", Label: "Grafana", Description: "Dashboards",
		LaunchURL: "https://grafana.jorisjonkers.dev/", IconURL: "https://grafana.jorisjonkers.dev/icon.svg",
		RedirectURIs: []string{"https://grafana.jorisjonkers.dev/login/generic_oauth"}, Credential: domain.Secret,
		SecretPath: "secret/data/monitoring/grafana/client", Roles: []string{"user", "viewer"},
	}
	if err != nil || c.ID != want.ID || c.SignIn != want.SignIn || c.Label != want.Label || c.Description != want.Description ||
		c.LaunchURL != want.LaunchURL || c.IconURL != want.IconURL || c.RedirectURIs[0] != want.RedirectURIs[0] ||
		c.Credential != want.Credential || c.SecretPath != want.SecretPath || len(c.Roles) != 2 ||
		c.BackChannelLogoutURI != "https://grafana.jorisjonkers.dev/logout" {
		t.Fatalf("ClientForHost = %+v, %v", c, err)
	}

	res := dyn.Resource(clients.Resource())
	if _, err := res.Update(ctx, clientObject("grafana", grafana("metrics.jorisjonkers.dev")), metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a changed host is followed", func() bool {
		_, err := x.ClientForHost(ctx, "metrics.jorisjonkers.dev")
		return err == nil
	})

	if err := res.Delete(ctx, "grafana", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a deleted Client is removed", func() bool {
		_, err := x.Client(ctx, "grafana")
		return errors.Is(err, app.ErrNotFound)
	})

	if _, err := res.Create(ctx, clientObject("grafana", grafana("metrics.jorisjonkers.dev")), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a re-created Client returns", func() bool {
		_, err := x.Client(ctx, "grafana")
		return err == nil
	})

	// An object this version cannot read leaves the Index as it was.
	bad := clientObject("broken", map[string]any{"signIn": "native", "host": "broken.jorisjonkers.dev", "roles": "user"})
	if _, err := res.Create(ctx, bad, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := res.Create(ctx, clientObject("after", grafana("after.jorisjonkers.dev")), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a later Client arrives", func() bool {
		_, err := x.Client(ctx, "after")
		return err == nil
	})
	if _, err := x.Client(ctx, "broken"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unreadable object became a Client: %v", err)
	}
}
