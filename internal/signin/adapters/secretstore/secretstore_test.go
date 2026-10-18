package secretstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/vault"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/secretstore"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

var errBoom = errors.New("boom")

// kv is Vault KV in memory; fail makes every call fail.
type kv struct {
	data map[string]map[string]string
	fail error
}

func (k *kv) Read(_ context.Context, path string) (map[string]string, error) {
	if k.fail != nil {
		return nil, k.fail
	}
	d, ok := k.data[path]
	if !ok {
		return nil, vault.ErrNoSecret
	}
	return d, nil
}

func (k *kv) Create(_ context.Context, path string, data map[string]string) error {
	k.data[path] = data
	return k.fail
}

func (k *kv) Write(_ context.Context, path string, data map[string]string) error {
	k.data[path] = data
	return k.fail
}

const path = "secret/data/monitoring/grafana/client"

func TestAClientsIDAndSecretAreWrittenUnderTheKeysItsApplicationReads(t *testing.T) {
	for _, c := range []struct {
		name  string
		write func(secretstore.Store) error
	}{
		{"created", func(s secretstore.Store) error { return s.Create(t.Context(), path, "grafana", "s1") }},
		{"replaced", func(s secretstore.Store) error { return s.Write(t.Context(), path, "grafana", "s1") }},
	} {
		t.Run(c.name, func(t *testing.T) {
			k := &kv{data: map[string]map[string]string{}}
			if err := c.write(secretstore.New(k)); err != nil {
				t.Fatal(err)
			}
			if got := k.data[path]; len(got) != 2 || got["id"] != "grafana" || got["secret"] != "s1" {
				t.Fatalf("data = %v", got)
			}
		})
	}
}

func TestTheSecretIsReadBack(t *testing.T) {
	k := &kv{data: map[string]map[string]string{path: {"id": "grafana", "secret": "s1"}}}
	if got, err := secretstore.New(k).Read(t.Context(), path); err != nil || got != "s1" {
		t.Fatalf("Read = %q, %v", got, err)
	}
}

func TestAPathWithNoSecretIsNotFound(t *testing.T) {
	if _, err := secretstore.New(&kv{data: map[string]map[string]string{}}).Read(t.Context(), path); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDataWithoutASecretIsAnErrorNotAMissingSecret(t *testing.T) {
	k := &kv{data: map[string]map[string]string{path: {"id": "grafana"}}}
	_, err := secretstore.New(k).Read(t.Context(), path)
	if err == nil || errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestVaultFailingIsPassedOn(t *testing.T) {
	if _, err := secretstore.New(&kv{fail: errBoom}).Read(t.Context(), path); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}
