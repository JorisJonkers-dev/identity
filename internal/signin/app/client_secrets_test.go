package app_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// hashes keeps Client secret hashes in memory, as Postgres would under its lock.
type hashes struct {
	mu       sync.Mutex
	current  map[string][]byte
	previous map[string][]byte
	fail     map[string]error
}

func (h *hashes) Provision(_ context.Context, clientID string, mint func() ([]byte, error)) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.fail["Provision"]; err != nil {
		return err
	}
	if h.current[clientID] != nil {
		return nil
	}
	made, err := mint()
	if err != nil {
		return err
	}
	h.current[clientID] = made
	return nil
}

func (h *hashes) Rotate(_ context.Context, clientID string, mint func() ([]byte, error)) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	made, err := mint()
	if err != nil {
		return err
	}
	if h.previous[clientID] == nil {
		h.previous[clientID] = h.current[clientID]
	}
	h.current[clientID] = made
	return nil
}

func (h *hashes) Hashes(_ context.Context, clientID string) (current, previous []byte, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.fail["Hashes"]; err != nil {
		return nil, nil, err
	}
	if h.current[clientID] == nil {
		return nil, nil, app.ErrNotFound
	}
	return h.current[clientID], h.previous[clientID], nil
}

func (h *hashes) EndPrevious(_ context.Context, clientID string, current []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.fail["EndPrevious"]; err != nil {
		return err
	}
	if bytes.Equal(h.current[clientID], current) {
		delete(h.previous, clientID)
	}
	return nil
}

// secretVault is Vault KV in memory: each path's id and secret.
type secretVault struct {
	at   map[string][2]string
	fail map[string]error
}

func (v *secretVault) Read(_ context.Context, path string) (string, error) {
	if err := v.fail["Read"]; err != nil {
		return "", err
	}
	s, ok := v.at[path]
	if !ok {
		return "", app.ErrNotFound
	}
	return s[1], nil
}

func (v *secretVault) Create(_ context.Context, path, clientID, secret string) error {
	if err := v.fail["Create"]; err != nil {
		return err
	}
	v.at[path] = [2]string{clientID, secret}
	return nil
}

func (v *secretVault) Write(_ context.Context, path, clientID, secret string) error {
	if err := v.fail["Write"]; err != nil {
		return err
	}
	v.at[path] = [2]string{clientID, secret}
	return nil
}

func (c clients) All() []domain.Client {
	all := make([]domain.Client, 0, len(c))
	for _, cl := range c {
		all = append(all, cl)
	}
	slices.SortFunc(all, func(a, b domain.Client) int { return strings.Compare(a.ID, b.ID) })
	return all
}

type admins map[uuid.UUID]bool

func (a admins) IsAdmin(_ context.Context, id uuid.UUID) (bool, error) {
	if id == uuid.Nil {
		return false, errBoom
	}
	return a[id], nil
}

const grafanaPath = "secret/data/monitoring/grafana/client"

var root = uuid.MustParse("00000000-0000-0000-0000-00000000a0a0")

type secretsFixture struct {
	*app.ClientSecrets
	hashes *hashes
	vault  *secretVault
	events *recorded
	random *counter
}

func newSecretsFixture() *secretsFixture {
	f := &secretsFixture{
		hashes: &hashes{current: map[string][]byte{}, previous: map[string][]byte{}, fail: map[string]error{}},
		vault:  &secretVault{at: map[string][2]string{}, fail: map[string]error{}},
		events: &recorded{},
		random: &counter{},
	}
	cs := clients{
		"grafana": {ID: "grafana", SignIn: domain.Native, Credential: domain.Secret, SecretPath: grafanaPath},
		"wiki":    {ID: "wiki", SignIn: domain.Native, Credential: domain.Secret, SecretPath: "secret/data/wiki/wiki/client"},
		"spa":     {ID: "spa", SignIn: domain.Native, Credential: domain.PKCE},
		"stats":   {ID: "stats", SignIn: domain.Gated},
	}
	f.ClientSecrets = app.NewClientSecrets(f.hashes, f.vault, cs, admins{root: true}, f.events, f.random)
	return f
}

// secretAt is the secret the Application reads at path.
func (f *secretsFixture) secretAt(t *testing.T, path string) string {
	t.Helper()
	s, ok := f.vault.at[path]
	if !ok {
		t.Fatalf("no secret at %s", path)
	}
	return s[1]
}

func (f *secretsFixture) provisioned(t *testing.T) string {
	t.Helper()
	if err := f.ProvisionAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f.secretAt(t, grafanaPath)
}

func sha(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func TestANewSecretClientGetsARandomSecretWrittenWhereItsApplicationReadsIt(t *testing.T) {
	f := newSecretsFixture()
	secret := f.provisioned(t)
	if len(secret) < 43 || f.vault.at[grafanaPath][0] != "grafana" {
		t.Fatalf("at %s = %v", grafanaPath, f.vault.at[grafanaPath])
	}
	if !bytes.Equal(f.hashes.current["grafana"], sha(secret)) {
		t.Fatal("the hash kept is not the secret's")
	}
}

func TestEachSecretClientGetsASecretOfItsOwn(t *testing.T) {
	f := newSecretsFixture()
	if f.provisioned(t) == f.secretAt(t, "secret/data/wiki/wiki/client") {
		t.Fatal("two Clients got the same secret")
	}
}

func TestOnlySecretClientsGetASecret(t *testing.T) {
	f := newSecretsFixture()
	f.provisioned(t)
	if len(f.vault.at) != 2 || f.hashes.current["spa"] != nil || f.hashes.current["stats"] != nil {
		t.Fatalf("secrets at %v", f.vault.at)
	}
}

func TestASecretAlreadyAtThePathIsReadNotReplaced(t *testing.T) {
	f := newSecretsFixture()
	f.vault.at[grafanaPath] = [2]string{"grafana", "kept-from-before"}
	if got := f.provisioned(t); got != "kept-from-before" {
		t.Fatalf("secret = %q", got)
	}
	if !bytes.Equal(f.hashes.current["grafana"], sha("kept-from-before")) {
		t.Fatal("the hash kept is not the existing secret's")
	}
}

func TestProvisioningAgainChangesNothing(t *testing.T) {
	f := newSecretsFixture()
	first := f.provisioned(t)
	f.vault.fail["Read"] = errBoom
	if err := f.ProvisionAll(context.Background()); err != nil {
		t.Fatalf("a Client with a secret went back to Vault: %v", err)
	}
	if f.secretAt(t, grafanaPath) != first {
		t.Fatal("the secret changed")
	}
}

func TestASecretClientWithoutASecretPathIsReportedAndTheOthersStillGetTheirs(t *testing.T) {
	f := newSecretsFixture()
	f.ClientSecrets = app.NewClientSecrets(f.hashes, f.vault, clients{
		"lost":    {ID: "lost", SignIn: domain.Native, Credential: domain.Secret},
		"grafana": {ID: "grafana", SignIn: domain.Native, Credential: domain.Secret, SecretPath: grafanaPath},
	}, admins{}, f.events, f.random)
	err := f.ProvisionAll(context.Background())
	if !errors.Is(err, app.ErrNoSecretPath) || !strings.Contains(err.Error(), "lost") {
		t.Fatalf("err = %v", err)
	}
	f.secretAt(t, grafanaPath)
}

func TestProvisioningFailsWhenASecretCannotBeMade(t *testing.T) {
	for _, c := range []struct {
		name  string
		fault func(*secretsFixture)
	}{
		{"the store fails", func(f *secretsFixture) { f.hashes.fail["Provision"] = errBoom }},
		{"Vault cannot be read", func(f *secretsFixture) { f.vault.fail["Read"] = errBoom }},
		{"Vault refuses the write", func(f *secretsFixture) { f.vault.fail["Create"] = errBoom }},
		{"no random bytes", func(f *secretsFixture) { f.random.fail = true }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newSecretsFixture()
			c.fault(f)
			if err := f.ProvisionAll(context.Background()); err == nil {
				t.Fatal("provisioned")
			}
			if f.hashes.current["grafana"] != nil {
				t.Fatal("a hash was kept for a secret the Application cannot read")
			}
		})
	}
}

func TestASecretClientIsAuthorizedByItsSecret(t *testing.T) {
	f := newSecretsFixture()
	if err := f.Authorize(context.Background(), "grafana", f.provisioned(t)); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestAWrongSecretOrAClientWithNoSecretIsRefused(t *testing.T) {
	f := newSecretsFixture()
	f.provisioned(t)
	for _, c := range []struct{ name, client, secret string }{
		{"a wrong secret", "grafana", "wrong"},
		{"another Client's secret", "grafana", f.secretAt(t, "secret/data/wiki/wiki/client")},
		{"a Client with no secret yet", "new", "anything"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := f.Authorize(context.Background(), c.client, c.secret); !errors.Is(err, app.ErrInvalidClient) {
				t.Fatalf("err = %v, want ErrInvalidClient", err)
			}
		})
	}
}

func TestAuthorizingFailsWhenTheHashesCannotBeRead(t *testing.T) {
	f := newSecretsFixture()
	secret := f.provisioned(t)
	f.hashes.fail["Hashes"] = errBoom
	if err := f.Authorize(context.Background(), "grafana", secret); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnAdminRotatesASecretAndTheApplicationReadsTheNewOne(t *testing.T) {
	f := newSecretsFixture()
	old := f.provisioned(t)
	if err := f.Rotate(context.Background(), app.Actor{ID: root, TwoFactor: true}, "grafana"); err != nil {
		t.Fatal(err)
	}
	if renewed := f.secretAt(t, grafanaPath); renewed == old || f.vault.at[grafanaPath][0] != "grafana" {
		t.Fatalf("at %s = %v", grafanaPath, f.vault.at[grafanaPath])
	}
}

func TestTheOldSecretWorksUntilTheNewOneIsFirstUsed(t *testing.T) {
	f := newSecretsFixture()
	old := f.provisioned(t)
	if err := f.Rotate(context.Background(), app.Actor{ID: root, TwoFactor: true}, "grafana"); err != nil {
		t.Fatal(err)
	}
	renewed := f.secretAt(t, grafanaPath)
	ctx := context.Background()
	if err := f.Authorize(ctx, "grafana", old); err != nil {
		t.Fatalf("the old secret before the new one was used: %v", err)
	}
	if err := f.Authorize(ctx, "grafana", renewed); err != nil {
		t.Fatalf("the new secret: %v", err)
	}
	if err := f.Authorize(ctx, "grafana", old); !errors.Is(err, app.ErrInvalidClient) {
		t.Fatalf("the old secret after the new one was used: %v", err)
	}
}

func TestUsingTheNewSecretFailsWhenTheOldOneCannotBeEnded(t *testing.T) {
	f := newSecretsFixture()
	f.provisioned(t)
	if err := f.Rotate(context.Background(), app.Actor{ID: root, TwoFactor: true}, "grafana"); err != nil {
		t.Fatal(err)
	}
	f.hashes.fail["EndPrevious"] = errBoom
	if err := f.Authorize(context.Background(), "grafana", f.secretAt(t, grafanaPath)); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestARotationIsAnAuditEventNamingTheAdminAndTheClient(t *testing.T) {
	f := newSecretsFixture()
	f.provisioned(t)
	if err := f.Rotate(context.Background(), app.Actor{ID: root, TwoFactor: true}, "grafana"); err != nil {
		t.Fatal(err)
	}
	want := []audit.Event{{Action: audit.ClientSecretRotated, Actor: root, Client: "grafana"}}
	if !slices.Equal(*f.events, want) {
		t.Fatalf("events = %v", *f.events)
	}
}

func TestARotationIsRefused(t *testing.T) {
	for _, c := range []struct {
		name   string
		actor  app.Actor
		client string
		want   error
	}{
		{"for someone who is not an Admin", app.Actor{ID: uuid.New(), TwoFactor: true}, "grafana", app.ErrNotAdmin},
		{"for an Admin without a two-factor method", app.Actor{ID: root}, "grafana", app.ErrTwoFactorProofNeeded},
		{"for a Client that is not there", app.Actor{ID: root, TwoFactor: true}, "nobody", app.ErrNotFound},
		{"for a public Client", app.Actor{ID: root, TwoFactor: true}, "spa", app.ErrNoSecret},
		{"for a gated Client", app.Actor{ID: root, TwoFactor: true}, "stats", app.ErrNoSecret},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newSecretsFixture()
			if err := f.Rotate(context.Background(), c.actor, c.client); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if len(*f.events) != 0 || len(f.vault.at) != 0 {
				t.Fatalf("a refused rotation wrote %v and recorded %v", f.vault.at, *f.events)
			}
		})
	}
}

func TestARotationFails(t *testing.T) {
	for _, c := range []struct {
		name  string
		actor uuid.UUID
		fault func(*secretsFixture)
	}{
		{"when Admins cannot be read", uuid.Nil, func(*secretsFixture) {}},
		{"when Vault refuses the write", root, func(f *secretsFixture) { f.vault.fail["Write"] = errBoom }},
		{"with no random bytes", root, func(f *secretsFixture) { f.random.fail = true }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newSecretsFixture()
			c.fault(f)
			if err := f.Rotate(context.Background(), app.Actor{ID: c.actor, TwoFactor: true}, "grafana"); err == nil {
				t.Fatal("rotated")
			}
			if len(*f.events) != 0 {
				t.Fatalf("a failed rotation recorded %v", *f.events)
			}
		})
	}
}

func TestASecretClientWithoutASecretPathCannotBeRotated(t *testing.T) {
	f := newSecretsFixture()
	f.ClientSecrets = app.NewClientSecrets(f.hashes, f.vault, clients{
		"lost": {ID: "lost", SignIn: domain.Native, Credential: domain.Secret},
	}, admins{root: true}, f.events, f.random)
	if err := f.Rotate(context.Background(), app.Actor{ID: root, TwoFactor: true}, "lost"); !errors.Is(err, app.ErrNoSecretPath) {
		t.Fatalf("err = %v", err)
	}
}
