//go:build integration

package persistence_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/platform/vault"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/persistence"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var keysT0 = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

// replica is one identity's view of the keys: its own pool, the shared database and Vault.
type replica struct {
	*app.Keys
	store *pg.Store
}

func newReplica(t *testing.T, url string, now *time.Time) replica {
	t.Helper()
	store, err := pg.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	addr, token := itest.Vault(t)
	sealer := vault.NewTransit(addr, "transit", vault.Token(token), &http.Client{Timeout: 5 * time.Second}).Key("identity-signing")
	clock := func() time.Time { return *now }
	keys := app.NewKeys(persistence.NewKeys(store.Queries(), store, clock), sealer,
		func() (*rsa.PrivateKey, error) { return rsa.GenerateKey(rand.Reader, 2048) }, clock, rand.Reader)
	return replica{Keys: keys, store: store}
}

func TestTwoReplicasStartingTogetherAgreeOnOneSigningKey(t *testing.T) {
	url, now := pgtest.URL(t), keysT0
	a, b := newReplica(t, url, &now), newReplica(t, url, &now)
	var wg sync.WaitGroup
	sets := make([]app.KeySet, 2)
	errs := make([]error, 2)
	for i, r := range []replica{a, b} {
		wg.Go(func() { sets[i], errs[i] = r.Load(t.Context()) })
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || sets[0].Signing.ID != sets[1].Signing.ID {
		t.Fatalf("replicas signed with %s and %s (%v, %v)", sets[0].Signing.ID, sets[1].Signing.ID, errs[0], errs[1])
	}
	if rows, _ := a.store.Queries().ListSigningKeys(t.Context()); len(rows) != 1 {
		t.Fatalf("%d keys stored", len(rows))
	}
}

func TestATokenSignedBeforeARestartVerifiesAfterIt(t *testing.T) {
	url, now := pgtest.URL(t), keysT0
	before, err := newReplica(t, url, &now).Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: before.Private}, (&jose.SignerOptions{}).WithHeader("kid", before.Signing.ID))
	if err != nil {
		t.Fatal(err)
	}
	signed, err := signer.Sign([]byte(`{"sub":"ada"}`))
	if err != nil {
		t.Fatal(err)
	}
	now = keysT0.Add(time.Hour)
	after, err := newReplica(t, url, &now).Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Published) != 1 || after.Published[0].ID != before.Signing.ID {
		t.Fatalf("after the restart JWKS has %+v", after.Published)
	}
	if _, err := signed.Verify(after.Published[0].Public); err != nil {
		t.Fatalf("a token from before the restart does not verify: %v", err)
	}
}

func TestAPrivateKeyIsStoredOnlyAsVaultCiphertext(t *testing.T) {
	url, now := pgtest.URL(t), keysT0
	r := newReplica(t, url, &now)
	ks, err := r.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := r.store.Queries().ListSigningKeys(t.Context())
	der, _ := x509.MarshalPKCS8PrivateKey(ks.Private)
	if !strings.HasPrefix(rows[0].SealedPrivate, "vault:v") || strings.Contains(rows[0].SealedPrivate, string(der)) {
		t.Fatalf("stored %q", rows[0].SealedPrivate[:20])
	}
}

func TestANewKeySignsAfterNinetyDaysAndTheOldOneLeavesJWKSSevenDaysLater(t *testing.T) {
	url, now := pgtest.URL(t), keysT0
	r := newReplica(t, url, &now)
	first, _ := r.Load(t.Context())
	for _, tc := range []struct {
		name      string
		at        time.Time
		published int
		newKey    bool
	}{
		{"a day before 90 days, the first key still signs alone", keysT0.Add(domain.KeyLifetime - 24*time.Hour), 1, false},
		{"at 90 days a new key signs and both are published", keysT0.Add(domain.KeyLifetime), 2, true},
		{"7 days later the first key leaves JWKS", keysT0.Add(domain.KeyLifetime + domain.KeyOverlap), 1, true},
	} {
		now = tc.at
		ks, err := r.Load(t.Context())
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(ks.Published) != tc.published || (ks.Signing.ID != first.Signing.ID) != tc.newKey {
			t.Fatalf("%s: signing %s, %d published", tc.name, ks.Signing.ID, len(ks.Published))
		}
	}
}

func TestAKeyNoTokenCanNeedIsDeleted(t *testing.T) {
	url, now := pgtest.URL(t), keysT0
	r := newReplica(t, url, &now)
	_, _ = r.Load(t.Context())
	now = keysT0.Add(domain.KeyLifetime + domain.KeyOverlap + time.Hour)
	_, _ = r.Load(t.Context())
	if rows, _ := r.store.Queries().ListSigningKeys(t.Context()); len(rows) != 1 || !rows[0].SignsFrom.Equal(now) {
		t.Fatalf("stored %+v", rows)
	}
}

func TestTheProvidersSecretIsTheSameOnEveryReplicaAndAcrossRestarts(t *testing.T) {
	url, now := pgtest.URL(t), keysT0
	a, err := newReplica(t, url, &now).Secret(t.Context(), "oidc-crypto")
	if err != nil {
		t.Fatal(err)
	}
	b, err := newReplica(t, url, &now).Secret(t.Context(), "oidc-crypto")
	if err != nil || a != b || a == ([32]byte{}) {
		t.Fatalf("secrets %x and %x, %v", a, b, err)
	}
}
