package clients_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clients.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFileReadsEveryClientObject(t *testing.T) {
	x := clients.NewIndex()
	err := clients.LoadFile(write(t, `apiVersion: identity.jorisjonkers.dev/v1
kind: Client
metadata: {name: stats}
spec: {signIn: gated, host: stats.localhost, label: Stats, roles: [user]}
---
apiVersion: identity.jorisjonkers.dev/v1
kind: Client
metadata: {name: backup}
spec: {signIn: token, host: backup.localhost, roles: [user]}
`), x)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x.ClientForHost(context.Background(), "stats.localhost")
	if err != nil || c.ID != "stats" || c.SignIn != domain.Gated || c.Label != "Stats" {
		t.Fatalf("stats = %+v, %v", c, err)
	}
	if len(x.All()) != 2 {
		t.Fatalf("All = %+v", x.All())
	}
}

func TestLoadFileRefusesWhatIsNotAClient(t *testing.T) {
	for name, body := range map[string]string{
		"not YAML":       "{",
		"not a Client":   "metadata: {name: x}\nspec: {roles: \"user\"}\n",
		"missing a file": "",
	} {
		path := write(t, body)
		if name == "missing a file" {
			path += ".gone"
		}
		if err := clients.LoadFile(path, clients.NewIndex()); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
