//go:build integration

package vault_test

import (
	"bytes"
	"errors"
	"net/http"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/JorisJonkers-dev/identity/internal/platform/vault"
)

const vaultImage = "hashicorp/vault:1.19.0"

// TestTransitAgainstARealVault seals and opens through a dev-mode Vault, the same server the cluster
// runs, so the request and answer shapes are Vault's, not the fake's.
func TestTransitAgainstARealVault(t *testing.T) {
	c, err := testcontainers.Run(t.Context(), vaultImage,
		testcontainers.WithEnv(map[string]string{"VAULT_DEV_ROOT_TOKEN_ID": "root", "VAULT_DEV_LISTEN_ADDRESS": "0.0.0.0:8200"}),
		testcontainers.WithExposedPorts("8200/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/v1/sys/health").WithPort("8200/tcp")),
	)
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := c.PortEndpoint(t.Context(), "8200/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ path, body string }{
		{"/v1/sys/mounts/transit", `{"type":"transit"}`},
		{"/v1/transit/keys/identity-totp", `{}`},
	} {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, addr+step.path, bytes.NewBufferString(step.body))
		req.Header.Set("X-Vault-Token", "root")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode >= 300 {
			t.Fatalf("%s = %d", step.path, res.StatusCode)
		}
	}

	tr := vault.NewTransit(addr, "transit", vault.Token("root"), http.DefaultClient)
	sealed, err := tr.Seal(t.Context(), "identity-totp", []byte("JBSWY3DPEHPK3PXP"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix([]byte(sealed), []byte("vault:v1:")) || bytes.Contains([]byte(sealed), []byte("JBSWY3DPEHPK3PXP")) {
		t.Fatalf("sealed = %q", sealed)
	}
	plain, err := tr.Open(t.Context(), "identity-totp", sealed)
	if err != nil || string(plain) != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("Open = %q, %v", plain, err)
	}
	if _, err := tr.Open(t.Context(), "identity-totp", "vault:v1:AAAA"); !errors.Is(err, vault.ErrSealed) {
		t.Fatalf("a forged ciphertext = %v, want ErrSealed", err)
	}
	wrong := vault.NewTransit(addr, "transit", vault.Token("not-root"), http.DefaultClient)
	if _, err := wrong.Seal(t.Context(), "identity-totp", []byte("x")); err == nil {
		t.Fatal("a wrong token sealed")
	}
}
