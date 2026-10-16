package vault_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/vault"
)

// fakeVault answers transit and Kubernetes login like Vault does, with "vault:v1:" plus base64.
type fakeVault struct {
	mu      sync.Mutex
	tokens  map[string]bool
	logins  []map[string]string
	answer  func(path string) (int, string, bool)
	revoked bool
}

func (f *fakeVault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var in map[string]string
	_ = json.NewDecoder(r.Body).Decode(&in)
	if f.answer != nil {
		if code, body, ok := f.answer(r.URL.Path); ok {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
			return
		}
	}
	if r.URL.Path == "/v1/auth/kubernetes/login" {
		f.logins = append(f.logins, in)
		_, _ = w.Write([]byte(`{"auth":{"client_token":"t2"}}`))
		f.tokens["t2"] = true
		return
	}
	if !f.tokens[r.Header.Get("X-Vault-Token")] || f.revoked {
		f.revoked = false
		w.WriteHeader(http.StatusForbidden)
		return
	}
	switch r.URL.Path {
	case "/v1/transit/encrypt/totp":
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"ciphertext": "vault:v1:" + in["plaintext"]}})
	case "/v1/transit/decrypt/totp":
		if !strings.HasPrefix(in["ciphertext"], "vault:v1:") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"plaintext": strings.TrimPrefix(in["ciphertext"], "vault:v1:")}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func serve(t *testing.T, f *fakeVault) string {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestSealAndOpenWithAToken(t *testing.T) {
	f := &fakeVault{tokens: map[string]bool{"root": true}}
	tr := vault.NewTransit(serve(t, f)+"/", "transit", vault.Token("root"), http.DefaultClient)
	sealed, err := tr.Seal(t.Context(), "totp", []byte("JBSWY3DPEHPK3PXP"))
	if err != nil || !strings.HasPrefix(sealed, "vault:v1:") || strings.Contains(sealed, "JBSWY3DPEHPK3PXP") {
		t.Fatalf("Seal = %q, %v", sealed, err)
	}
	plain, err := tr.Open(t.Context(), "totp", sealed)
	if err != nil || string(plain) != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("Open = %q, %v", plain, err)
	}
	if _, err := tr.Open(t.Context(), "totp", "not-a-ciphertext"); !errors.Is(err, vault.ErrSealed) {
		t.Fatalf("Open of garbage = %v, want ErrSealed", err)
	}
}

func TestAKeyNamesItsTransitKey(t *testing.T) {
	f := &fakeVault{tokens: map[string]bool{"root": true}}
	k := vault.NewTransit(serve(t, f), "transit", vault.Token("root"), http.DefaultClient).Key("totp")
	sealed, err := k.Seal(t.Context(), []byte("s"))
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := k.Open(t.Context(), sealed); err != nil || string(plain) != "s" {
		t.Fatalf("Open = %q, %v", plain, err)
	}
}

func TestKubernetesLoginAndARefusedTokenSignsInAgain(t *testing.T) {
	jwt := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(jwt, []byte("the-sa-jwt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &fakeVault{tokens: map[string]bool{}}
	tr := vault.NewTransit(serve(t, f), "transit", vault.Kubernetes("kubernetes", "identity", jwt), http.DefaultClient)
	if _, err := tr.Seal(t.Context(), "totp", []byte("x")); err != nil {
		t.Fatal(err)
	}
	f.revoked = true
	if _, err := tr.Seal(t.Context(), "totp", []byte("x")); err != nil {
		t.Fatalf("a revoked token was not replaced: %v", err)
	}
	if len(f.logins) != 2 || f.logins[0]["role"] != "identity" || f.logins[0]["jwt"] != "the-sa-jwt" {
		t.Fatalf("logins = %v", f.logins)
	}
}

func TestFailures(t *testing.T) {
	ctx := context.Background()
	missing := vault.NewTransit("http://127.0.0.1:1", "transit", vault.Kubernetes("kubernetes", "identity", "/no/such/token"), http.DefaultClient)
	if _, err := missing.Seal(ctx, "totp", nil); err == nil || !strings.Contains(err.Error(), "service account token") {
		t.Errorf("no service account token: %v", err)
	}
	down := vault.NewTransit("http://127.0.0.1:1", "transit", vault.Token("t"), http.DefaultClient)
	if _, err := down.Seal(ctx, "totp", nil); err == nil {
		t.Error("an unreachable Vault sealed")
	}
	if _, err := down.Open(ctx, "totp", "vault:v1:eA=="); err == nil || errors.Is(err, vault.ErrSealed) {
		t.Errorf("an unreachable Vault opened: %v", err)
	}
	bad := vault.NewTransit("http://bad host", "transit", vault.Token("t"), http.DefaultClient)
	if _, err := bad.Seal(ctx, "totp", nil); err == nil {
		t.Error("a bad address sealed")
	}
	refusing := &fakeVault{tokens: map[string]bool{}, answer: func(path string) (int, string, bool) {
		return http.StatusInternalServerError, "", strings.HasSuffix(path, "/login")
	}}
	jwtFile := filepath.Join(t.TempDir(), "token")
	_ = os.WriteFile(jwtFile, []byte("j"), 0o600)
	if _, err := vault.NewTransit(serve(t, refusing), "transit", vault.Kubernetes("kubernetes", "identity", jwtFile), http.DefaultClient).Seal(ctx, "totp", nil); err == nil || err.Error() != "vault: answered 500" {
		t.Errorf("a refused login: %v", err)
	}
	//nolint:gosec // Vault answers with no token in them, not credentials
	for name, answer := range map[string]string{
		"no token":      `{"auth":{}}`,
		"not JSON":      `{`,
		"bad plaintext": `{"data":{"plaintext":"%%%"}}`,
	} {
		f := &fakeVault{tokens: map[string]bool{"root": true}, answer: func(path string) (int, string, bool) {
			switch {
			case name == "no token" && strings.HasSuffix(path, "/login"):
				return http.StatusOK, answer, true
			case name == "not JSON" && strings.HasSuffix(path, "/encrypt/totp"):
				return http.StatusOK, answer, true
			case name == "bad plaintext" && strings.HasSuffix(path, "/decrypt/totp"):
				return http.StatusOK, answer, true
			}
			return 0, "", false
		}}
		addr := serve(t, f)
		jwt := filepath.Join(t.TempDir(), "token")
		_ = os.WriteFile(jwt, []byte("j"), 0o600)
		login := vault.Token("root")
		if name == "no token" {
			login = vault.Kubernetes("kubernetes", "identity", jwt)
		}
		tr := vault.NewTransit(addr, "transit", login, http.DefaultClient)
		var err error
		if name == "bad plaintext" {
			_, err = tr.Open(ctx, "totp", "vault:v1:"+base64.StdEncoding.EncodeToString([]byte("x")))
		} else {
			_, err = tr.Seal(ctx, "totp", []byte("x"))
		}
		if err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	never := &fakeVault{tokens: map[string]bool{}}
	if _, err := vault.NewTransit(serve(t, never), "transit", vault.Token("wrong"), http.DefaultClient).Open(ctx, "totp", "vault:v1:eA=="); err == nil || errors.Is(err, vault.ErrSealed) {
		t.Errorf("a token refused twice: %v", err)
	}
}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("cut off") }
func (failingBody) Close() error             { return nil }

func TestACutOffAnswerIsAnError(t *testing.T) {
	c := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}, Header: http.Header{}}, nil
	})}
	if _, err := vault.NewTransit("http://vault.test", "transit", vault.Token("t"), c).Seal(t.Context(), "totp", nil); err == nil {
		t.Fatal("a cut-off answer sealed")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
