package vault_test

import (
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
	tr := vault.New(serve(t, f)+"/", vault.Token("root"), http.DefaultClient).Transit("transit")
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
	k := vault.New(serve(t, f), vault.Token("root"), http.DefaultClient).Transit("transit").Key("totp")
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
	tr := vault.New(serve(t, f), vault.Kubernetes("kubernetes", "identity", jwt), http.DefaultClient).Transit("transit")
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

func TestSealingWithoutAServiceAccountTokenSaysWhatIsMissing(t *testing.T) {
	missing := vault.New("http://127.0.0.1:1", vault.Kubernetes("kubernetes", "identity", "/no/such/token"), http.DefaultClient).Transit("transit")
	if _, err := missing.Seal(t.Context(), "totp", nil); err == nil || !strings.Contains(err.Error(), "service account token") {
		t.Fatalf("err = %v", err)
	}
}

func TestAnUnreachableVaultNeitherSealsNorOpens(t *testing.T) {
	down := vault.New("http://127.0.0.1:1", vault.Token("t"), http.DefaultClient).Transit("transit")
	if _, err := down.Seal(t.Context(), "totp", nil); err == nil {
		t.Error("an unreachable Vault sealed")
	}
	if _, err := down.Open(t.Context(), "totp", "vault:v1:eA=="); err == nil || errors.Is(err, vault.ErrSealed) {
		t.Errorf("an unreachable Vault opened: %v", err)
	}
}

func TestABadVaultAddressDoesNotSeal(t *testing.T) {
	bad := vault.New("http://bad host", vault.Token("t"), http.DefaultClient).Transit("transit")
	if _, err := bad.Seal(t.Context(), "totp", nil); err == nil {
		t.Fatal("a bad address sealed")
	}
}

func jwtFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("j"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// answering is a Vault that answers body on the path ending in suffix, and as usual elsewhere.
func answering(t *testing.T, suffix, body string) string {
	t.Helper()
	return serve(t, &fakeVault{tokens: map[string]bool{"root": true}, answer: func(path string) (int, string, bool) {
		return http.StatusOK, body, strings.HasSuffix(path, suffix)
	}})
}

func TestARefusedLoginReportsVaultsStatus(t *testing.T) {
	refusing := &fakeVault{tokens: map[string]bool{}, answer: func(path string) (int, string, bool) {
		return http.StatusInternalServerError, "", strings.HasSuffix(path, "/login")
	}}
	tr := vault.New(serve(t, refusing), vault.Kubernetes("kubernetes", "identity", jwtFile(t)), http.DefaultClient).Transit("transit")
	if _, err := tr.Seal(t.Context(), "totp", nil); err == nil || err.Error() != "vault: answered 500" {
		t.Fatalf("err = %v", err)
	}
}

func TestALoginAnswerWithoutATokenIsAnError(t *testing.T) {
	tr := vault.New(answering(t, "/login", `{"auth":{}}`), vault.Kubernetes("kubernetes", "identity", jwtFile(t)), http.DefaultClient).Transit("transit")
	if _, err := tr.Seal(t.Context(), "totp", []byte("x")); err == nil {
		t.Fatal("sealed without a token")
	}
}

func TestAnEncryptAnswerThatIsNotJSONIsAnError(t *testing.T) {
	tr := vault.New(answering(t, "/encrypt/totp", `{`), vault.Token("root"), http.DefaultClient).Transit("transit")
	if _, err := tr.Seal(t.Context(), "totp", []byte("x")); err == nil {
		t.Fatal("sealed with a broken answer")
	}
}

func TestADecryptAnswerWithBadPlaintextIsAnError(t *testing.T) {
	tr := vault.New(answering(t, "/decrypt/totp", `{"data":{"plaintext":"%%%"}}`), vault.Token("root"), http.DefaultClient).Transit("transit")
	if _, err := tr.Open(t.Context(), "totp", "vault:v1:"+base64.StdEncoding.EncodeToString([]byte("x"))); err == nil {
		t.Fatal("opened bad plaintext")
	}
}

func TestATokenRefusedTwiceIsAnErrorNotASealedValue(t *testing.T) {
	never := &fakeVault{tokens: map[string]bool{}}
	if _, err := vault.New(serve(t, never), vault.Token("wrong"), http.DefaultClient).Transit("transit").Open(t.Context(), "totp", "vault:v1:eA=="); err == nil || errors.Is(err, vault.ErrSealed) {
		t.Fatalf("err = %v", err)
	}
}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("cut off") }
func (failingBody) Close() error             { return nil }

func TestACutOffAnswerIsAnError(t *testing.T) {
	c := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}, Header: http.Header{}}, nil
	})}
	if _, err := vault.New("http://vault.test", vault.Token("t"), c).Transit("transit").Seal(t.Context(), "totp", nil); err == nil {
		t.Fatal("a cut-off answer sealed")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
