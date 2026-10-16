//go:build integration

package clients_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"

	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
)

const k3sImage = "rancher/k3s:v1.34.1-k3s1"

var crds = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

// cluster runs a real API server with the Client CRD from deploy/crd installed and established.
func cluster(t *testing.T) dynamic.Interface {
	t.Helper()
	ctx := context.Background()
	ctr, err := k3s.Run(ctx, k3sImage)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatal(err)
	}
	kubeconfig, err := ctr.GetKubeConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		t.Fatal(err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile("../../../../deploy/crd/clients.identity.jorisjonkers.dev.yaml")
	if err != nil {
		t.Fatal(err)
	}
	crd := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(raw, &crd.Object); err != nil {
		t.Fatal(err)
	}
	if _, err := dyn.Resource(crds).Create(ctx, crd, metav1.CreateOptions{}); err != nil {
		t.Fatalf("the CRD is refused: %v", err)
	}
	eventually(t, "the CRD is established", func() bool {
		got, err := dyn.Resource(crds).Get(ctx, crd.GetName(), metav1.GetOptions{})
		if err != nil {
			return false
		}
		conds, _, _ := unstructured.NestedSlice(got.Object, "status", "conditions")
		for _, c := range conds {
			m, _ := c.(map[string]any)
			if m["type"] == "Established" && m["status"] == "True" {
				return true
			}
		}
		return false
	})
	return dyn
}

func with(spec map[string]any, changes map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range spec {
		out[k] = v
	}
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

func TestTheAPIServerHoldsClientsToTheEstate(t *testing.T) {
	ctx := context.Background()
	dyn := cluster(t)
	res := dyn.Resource(clients.Resource())

	x := clients.NewIndex()
	watchCtx, stop := context.WithCancel(ctx)
	defer stop()
	synced := clients.Watch(watchCtx, dyn, x, slog.New(slog.DiscardHandler))
	eventually(t, "the first list arrives", synced)

	native := grafana("grafana.jorisjonkers.dev")
	gated := map[string]any{"signIn": "gated", "host": "sonarr.jorisjonkers.dev", "label": "Sonarr", "roles": []any{"user"}}
	token := map[string]any{"signIn": "token", "host": "api.jorisjonkers.dev", "roles": []any{"user", "writer"}}
	pkce := with(native, map[string]any{"credential": "pkce", "secretPath": nil})
	for name, spec := range map[string]map[string]any{"grafana": native, "sonarr": gated, "api": token, "spa": with(pkce, map[string]any{"host": "spa.jorisjonkers.dev", "redirectUris": []any{"https://spa.jorisjonkers.dev/cb"}, "launchUrl": nil, "iconUrl": nil, "backChannelLogoutUri": nil})} {
		if _, err := res.Create(ctx, clientObject(name, spec), metav1.CreateOptions{}); err != nil {
			t.Fatalf("a valid %s Client is refused: %v", name, err)
		}
	}

	refused := map[string]map[string]any{
		"a host outside the estate":                      with(native, map[string]any{"host": "grafana.example.org", "redirectUris": []any{"https://grafana.example.org/cb"}, "launchUrl": nil, "iconUrl": nil}),
		"a redirect over http":                           with(native, map[string]any{"redirectUris": []any{"http://grafana.jorisjonkers.dev/cb"}}),
		"a redirect to another host":                     with(native, map[string]any{"redirectUris": []any{"https://evil.jorisjonkers.dev/cb"}}),
		"a redirect to a host that only starts the same": with(native, map[string]any{"redirectUris": []any{"https://grafana.jorisjonkers.dev.evil.org/cb"}}),
		"a launch URL on another host":                   with(native, map[string]any{"launchUrl": "https://evil.jorisjonkers.dev/"}),
		"an icon over http":                              with(native, map[string]any{"iconUrl": "http://grafana.jorisjonkers.dev/icon.svg"}),
		"a native Client without a credential":           with(native, map[string]any{"credential": nil, "secretPath": nil}),
		"a native Client without redirects":              with(native, map[string]any{"redirectUris": nil}),
		"a gated Client with redirects":                  with(gated, map[string]any{"redirectUris": []any{"https://sonarr.jorisjonkers.dev/cb"}}),
		"a back-channel logout to another host":          with(native, map[string]any{"backChannelLogoutUri": "https://evil.jorisjonkers.dev/logout"}),
		"a gated Client with a back-channel logout":      with(gated, map[string]any{"backChannelLogoutUri": "https://sonarr.jorisjonkers.dev/logout"}),
		"a secret Client without a path":                 with(native, map[string]any{"secretPath": nil}),
		"a PKCE Client with a secret path":               with(pkce, map[string]any{"secretPath": "secret/data/x/y/client"}), //nolint:gosec // a path, not a secret
		"roles that do not start with user":              with(native, map[string]any{"roles": []any{"viewer", "user"}}),
		"a token Client with a label":                    with(token, map[string]any{"label": "API"}),
		"a gated Client without a label":                 with(gated, map[string]any{"label": nil}),
		"an unknown mode":                                with(gated, map[string]any{"signIn": "proxy"}),
	}
	for what, spec := range refused {
		if _, err := res.Create(ctx, clientObject("refused", spec), metav1.CreateOptions{}); err == nil {
			_ = res.Delete(ctx, "refused", metav1.DeleteOptions{})
			t.Errorf("the API server accepted %s", what)
		}
	}

	eventually(t, "the valid Clients reach the Index", func() bool {
		_, err := x.ClientForHost(ctx, "sonarr.jorisjonkers.dev")
		_, err2 := x.Client(ctx, "spa")
		return err == nil && err2 == nil
	})
	if err := res.Delete(ctx, "sonarr", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a deleted Client is removed", func() bool {
		_, err := x.ClientForHost(ctx, "sonarr.jorisjonkers.dev")
		return err != nil
	})
	if _, err := res.Create(ctx, clientObject("sonarr", gated), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a re-created Client returns", func() bool {
		_, err := x.ClientForHost(ctx, "sonarr.jorisjonkers.dev")
		return err == nil
	})
}
