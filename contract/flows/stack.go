package flows

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	mobynet "github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	postgresImage = "postgres:16-alpine"
	vaultImage    = "hashicorp/vault:1.19.0"
	mailpitImage  = "axllent/mailpit:v1.27.10"
	vaultToken    = "flows-root"
	// databaseURL is how identity reaches the throwaway Postgres on the suite's network.
	databaseURL = "postgres://app:app@postgres:5432/app?sslmode=disable" //nolint:gosec // the throwaway Postgres's
	// hostAlias is how a container reaches the machine the suite runs on.
	hostAlias = "host.testcontainers.internal"
)

// stack is identity and what it needs, in containers on one network.
type stack struct {
	issuer   string
	mailAPI  string
	identity *testcontainers.DockerContainer
	// trust is the certificates of the suite's HTTPS proxy in front of the app.
	trust *x509.CertPool
}

// startStack runs identity from images, published at localhost:identityPort, holding the app's
// Client: the browser reaches it at https://localhost:proxyPort, and identity posts its logout
// tokens to the app's own appPort on the suite machine.
func startStack(t *testing.T, images Images, identityPort, proxyPort, appPort int) *stack {
	t.Helper()
	ctx := t.Context()
	nw, err := network.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nw.Remove(context.WithoutCancel(ctx)) })

	run(t, postgresImage,
		testcontainers.WithEnv(map[string]string{"POSTGRES_USER": "app", "POSTGRES_PASSWORD": "app", "POSTGRES_DB": "app"}),
		network.WithNetwork([]string{"postgres"}, nw),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	migrate := run(t, images.Migration,
		testcontainers.WithCmd("ci", "up", "000000000000"),
		testcontainers.WithEnv(map[string]string{
			"DATABASE_HOST": "postgres", "DATABASE_PORT": "5432", "DATABASE_NAME": "app",
			"DATABASE_USERNAME": "app", "DATABASE_PASSWORD": "app",
		}),
		network.WithNetwork(nil, nw),
		testcontainers.WithWaitStrategy(wait.ForExit()),
	)
	if state, err := migrate.State(ctx); err != nil || state.ExitCode != 0 {
		t.Fatalf("flows: the migration failed: %v %+v", err, state)
	}

	vault := run(t, vaultImage,
		testcontainers.WithEnv(map[string]string{"VAULT_DEV_ROOT_TOKEN_ID": vaultToken, "VAULT_DEV_LISTEN_ADDRESS": "0.0.0.0:8200"}),
		testcontainers.WithExposedPorts("8200/tcp"),
		network.WithNetwork([]string{"vault"}, nw),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/v1/sys/health").WithPort("8200/tcp")),
	)
	vaultAddr, err := vault.PortEndpoint(ctx, "8200/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}
	transitKeys(t, vaultAddr)

	mailpit := run(t, mailpitImage,
		testcontainers.WithExposedPorts("8025/tcp"),
		network.WithNetwork([]string{"mailpit"}, nw),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/readyz").WithPort("8025/tcp")),
	)
	s := &stack{issuer: fmt.Sprintf("http://localhost:%d", identityPort)}
	if s.mailAPI, err = mailpit.PortEndpoint(ctx, "8025/tcp", "http"); err != nil {
		t.Fatal(err)
	}

	s.identity = run(t, images.Identity,
		testcontainers.WithEnv(map[string]string{
			"DATABASE_URL": databaseURL,
			"PUBLIC_URL":   s.issuer, "ADDR": ":8080",
			"VAULT_ADDR": "http://vault:8200", "VAULT_TOKEN": vaultToken,
			"SMTP_ADDR": "mailpit:1025", "SMTP_TLS": "none", "MAIL_FROM": "identity@flows.test",
			"CLIENTS_FILE": "/etc/identity/clients.yaml",
		}),
		testcontainers.WithFiles(testcontainers.ContainerFile{
			Reader: strings.NewReader(clients(proxyPort, appPort)), ContainerFilePath: "/etc/identity/clients.yaml", FileMode: 0o444,
		}),
		testcontainers.WithHostPortAccess(appPort),
		testcontainers.WithExposedPorts("8080/tcp"),
		testcontainers.WithHostConfigModifier(func(hc *container.HostConfig) {
			hc.PortBindings = mobynet.PortMap{mobynet.MustParsePort("8080/tcp"): {{
				HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: strconv.Itoa(identityPort),
			}}}
		}),
		network.WithNetwork(nil, nw),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/readyz").WithPort("8080/tcp")),
	)
	return s
}

func run(t *testing.T, image string, opts ...testcontainers.ContainerCustomizer) *testcontainers.DockerContainer {
	t.Helper()
	c, err := testcontainers.Run(t.Context(), image, opts...)
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatalf("flows: %s: %v", image, err)
	}
	return c
}

// transitKeys makes the transit keys identity seals TOTP secrets and its signing keys with.
func transitKeys(t *testing.T, vaultAddr string) {
	t.Helper()
	for _, step := range []struct{ path, body string }{
		{"/v1/sys/mounts/transit", `{"type":"transit"}`},
		{"/v1/transit/keys/identity-totp", `{}`},
		{"/v1/transit/keys/identity-signing", `{}`},
	} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, vaultAddr+step.path, strings.NewReader(step.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Vault-Token", vaultToken)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode >= http.StatusBadRequest {
			t.Fatalf("flows: vault %s = %d", step.path, res.StatusCode)
		}
	}
}

// clients is the app's Client object. The browser reaches the app through the proxy on localhost;
// identity, in its container, posts logout tokens straight to the app through the suite machine.
func clients(proxyPort, appPort int) string {
	base := fmt.Sprintf("https://localhost:%d", proxyPort)
	return fmt.Sprintf(`apiVersion: identity.jorisjonkers.dev/v1
kind: Client
metadata: {name: %s}
spec:
  signIn: native
  host: localhost
  label: Flows
  launchUrl: %s
  redirectUris: [%s/callback]
  credential: pkce
  backChannelLogoutUri: http://%s:%d/back-channel-logout
  roles: [user, editor]
`, ClientID, base, base, hostAlias, appPort)
}
