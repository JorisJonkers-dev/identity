package vault_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/vault"
)

// fakeKV answers Vault's KV version 2 API for one token, keeping each path's latest data, and
// refusing a create (cas 0) where data already is, as Vault does.
type fakeKV struct {
	mu     sync.Mutex
	data   map[string]map[string]string
	writes []map[string]any
}

func (f *fakeKV) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("X-Vault-Token") != "root" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		d, ok := f.data[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errors":[]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": d, "metadata": map[string]any{"version": 1}}})
	case http.MethodPost:
		var in struct {
			Data    map[string]string `json:"data"`
			Options map[string]int    `json:"options"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.writes = append(f.writes, map[string]any{"path": r.URL.Path, "data": in.Data, "options": in.Options})
		if cas, ok := in.Options["cas"]; ok && cas == 0 && f.data[r.URL.Path] != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":["check-and-set parameter did not match the current version"]}`))
			return
		}
		f.data[r.URL.Path] = in.Data
		_, _ = w.Write([]byte(`{"data":{"version":1}}`))
	}
}

func kv(t *testing.T, f *fakeKV) *vault.KV {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return vault.New(srv.URL, vault.Token("root"), http.DefaultClient).KV()
}

const grafanaPath = "secret/data/monitoring/grafana/client"

func TestACreatedSecretReadsBack(t *testing.T) {
	k := kv(t, &fakeKV{data: map[string]map[string]string{}})
	if err := k.Create(t.Context(), grafanaPath, map[string]string{"id": "grafana", "secret": "s1"}); err != nil {
		t.Fatal(err)
	}
	got, err := k.Read(t.Context(), grafanaPath)
	if err != nil || got["id"] != "grafana" || got["secret"] != "s1" {
		t.Fatalf("Read = %v, %v", got, err)
	}
}

func TestCreatingWhereASecretAlreadyIsLeavesItAlone(t *testing.T) {
	f := &fakeKV{data: map[string]map[string]string{"/v1/" + grafanaPath: {"secret": "kept"}}}
	k := kv(t, f)
	if err := k.Create(t.Context(), grafanaPath, map[string]string{"secret": "new"}); err == nil {
		t.Fatal("a create replaced a secret")
	}
	if f.data["/v1/"+grafanaPath]["secret"] != "kept" {
		t.Fatalf("data = %v", f.data)
	}
}

func TestAWriteReplacesTheSecret(t *testing.T) {
	f := &fakeKV{data: map[string]map[string]string{"/v1/" + grafanaPath: {"secret": "old"}}}
	k := kv(t, f)
	if err := k.Write(t.Context(), grafanaPath, map[string]string{"id": "grafana", "secret": "new"}); err != nil {
		t.Fatal(err)
	}
	if got := f.data["/v1/"+grafanaPath]; got["secret"] != "new" {
		t.Fatalf("data = %v", got)
	}
	if opts := f.writes[0]["options"].(map[string]int); len(opts) != 0 {
		t.Fatalf("a write sent options %v", opts)
	}
}

func TestReadingAPathWithNoSecretIsErrNoSecret(t *testing.T) {
	k := kv(t, &fakeKV{data: map[string]map[string]string{}})
	if _, err := k.Read(t.Context(), grafanaPath); !errors.Is(err, vault.ErrNoSecret) {
		t.Fatalf("err = %v, want ErrNoSecret", err)
	}
}

func TestAReadVaultRefusesIsAnErrorNotAMissingSecret(t *testing.T) {
	srv := httptest.NewServer(&fakeKV{data: map[string]map[string]string{}})
	t.Cleanup(srv.Close)
	k := vault.New(srv.URL, vault.Token("wrong"), http.DefaultClient).KV()
	if _, err := k.Read(t.Context(), grafanaPath); err == nil || errors.Is(err, vault.ErrNoSecret) {
		t.Fatalf("err = %v", err)
	}
}

func TestTransitAndKVShareOneSignIn(t *testing.T) {
	f := &fakeVault{tokens: map[string]bool{}, answer: func(path string) (int, string, bool) {
		return http.StatusOK, `{"data":{"data":{"secret":"s"}}}`, path == "/v1/"+grafanaPath
	}}
	v := vault.New(serve(t, f), vault.Kubernetes("kubernetes", "identity", jwtFile(t)), http.DefaultClient)
	if _, err := v.Transit("transit").Seal(t.Context(), "totp", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := v.KV().Read(t.Context(), grafanaPath); err != nil {
		t.Fatal(err)
	}
	if len(f.logins) != 1 {
		t.Fatalf("signed in %d times", len(f.logins))
	}
}
