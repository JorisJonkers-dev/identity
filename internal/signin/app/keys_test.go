package app_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// keyStore keeps keys and secrets in memory, as Postgres would under its lock.
type keyStore struct {
	mu      sync.Mutex
	keys    []app.StoredKey
	secrets map[string]string
	deleted time.Time
	fail    map[string]error
}

func newKeyStore() *keyStore {
	return &keyStore{secrets: map[string]string{}, fail: map[string]error{}}
}

func (s *keyStore) WithKeys(_ context.Context, add func([]app.StoredKey) (app.StoredKey, bool, error)) ([]app.StoredKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.fail["WithKeys"]; err != nil {
		return nil, err
	}
	added, made, err := add(append([]app.StoredKey(nil), s.keys...))
	if err != nil {
		return nil, err
	}
	if made {
		s.keys = append(s.keys, added)
	}
	return append([]app.StoredKey(nil), s.keys...), nil
}

func (s *keyStore) DeleteEndedBefore(_ context.Context, t time.Time) error {
	s.deleted = t
	return s.fail["DeleteEndedBefore"]
}

func (s *keyStore) Secret(_ context.Context, name string, mint func() (string, error)) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sealed, ok := s.secrets[name]; ok {
		return sealed, nil
	}
	sealed, err := mint()
	if err != nil {
		return "", err
	}
	s.secrets[name] = sealed
	return sealed, nil
}

// sealer "seals" by prefixing and base64, so a test can see nothing is stored in the clear.
type sealer struct{ failSeal, failOpen error }

func (s sealer) Seal(_ context.Context, plaintext []byte) (string, error) {
	return "vault:v1:" + base64.StdEncoding.EncodeToString(plaintext), s.failSeal
}

func (s sealer) Open(_ context.Context, ciphertext string) ([]byte, error) {
	if s.failOpen != nil {
		return nil, s.failOpen
	}
	return base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, "vault:v1:"))
}

var generated = func() *rsa.PrivateKey { k, _ := rsa.GenerateKey(rand.Reader, 2048); return k }()

type keysFixture struct {
	*app.Keys
	store *keyStore
	made  int
	now   time.Time
}

func newKeysFixture(s app.Sealer) *keysFixture {
	f := &keysFixture{store: newKeyStore(), now: t0}
	f.Keys = app.NewKeys(f.store, s, func() (*rsa.PrivateKey, error) { f.made++; return generated, nil }, func() time.Time { return f.now }, rand.Reader)
	return f
}

func load(t *testing.T, f *keysFixture) app.KeySet {
	t.Helper()
	ks, err := f.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return ks
}

func TestTheFirstLoadMakesAKeyThatSignsForNinetyDays(t *testing.T) {
	f := newKeysFixture(sealer{})
	ks := load(t, f)
	if f.made != 1 || !ks.Private.Equal(generated) || ks.Signing.ID != domain.KeyID(&generated.PublicKey) {
		t.Fatalf("made %d, key set %+v", f.made, ks.Signing)
	}
	if !ks.Signing.SignsFrom.Equal(t0) || !ks.Signing.SignsUntil.Equal(t0.Add(domain.KeyLifetime)) || len(ks.Published) != 1 {
		t.Fatalf("key set = %+v", ks)
	}
}

func TestAKeysPrivateHalfIsStoredOnlySealed(t *testing.T) {
	f := newKeysFixture(sealer{})
	load(t, f)
	der, _ := x509.MarshalPKCS8PrivateKey(generated)
	if s := f.store.keys[0].Sealed; !strings.HasPrefix(s, "vault:v1:") || bytes.Contains([]byte(s), der) {
		t.Fatalf("stored %q", s)
	}
}

func TestALaterLoadReadsTheKeyInsteadOfMakingAnother(t *testing.T) {
	f := newKeysFixture(sealer{})
	first := load(t, f)
	f.now = t0.Add(24 * time.Hour)
	if again := load(t, f); f.made != 1 || again.Signing.ID != first.Signing.ID {
		t.Fatalf("made %d keys", f.made)
	}
}

func TestANewKeySignsOnceTheOldOneHasSignedNinetyDaysAndBothArePublished(t *testing.T) {
	f := newKeysFixture(sealer{})
	first := load(t, f)
	f.now = t0.Add(domain.KeyLifetime)
	next := load(t, f)
	if f.made != 2 || !next.Signing.SignsFrom.Equal(f.now) || len(next.Published) != 2 || next.Published[0].ID != first.Signing.ID {
		t.Fatalf("made %d, next %+v", f.made, next)
	}
}

func TestLoadingDropsKeysThatEndedMoreThanSevenDaysAgo(t *testing.T) {
	f := newKeysFixture(sealer{})
	load(t, f)
	if !f.store.deleted.Equal(t0.Add(-domain.KeyOverlap)) {
		t.Fatalf("deleted before %v", f.store.deleted)
	}
}

func TestLoadingFailsWithWhatItNeeds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		sealer sealer
		setUp  func(f *keysFixture)
	}{
		{"the store", sealer{}, func(f *keysFixture) { f.store.fail["WithKeys"] = errBoom }},
		{"sealing a new key", sealer{failSeal: errBoom}, func(*keysFixture) {}},
		{"opening the key", sealer{failOpen: errBoom}, func(*keysFixture) {}},
		{"dropping old keys", sealer{}, func(f *keysFixture) { f.store.fail["DeleteEndedBefore"] = errBoom }},
		{"making a key", sealer{}, func(f *keysFixture) {
			f.Keys = app.NewKeys(f.store, sealer{}, func() (*rsa.PrivateKey, error) { return nil, errBoom }, time.Now, rand.Reader)
		}},
		{"encoding a key", sealer{}, func(f *keysFixture) {
			f.Keys = app.NewKeys(f.store, sealer{}, func() (*rsa.PrivateKey, error) {
				return &rsa.PrivateKey{PublicKey: rsa.PublicKey{N: big.NewInt(0)}}, nil
			}, time.Now, rand.Reader)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newKeysFixture(tc.sealer)
			tc.setUp(f)
			if _, err := f.Load(context.Background()); err == nil {
				t.Fatal("keys loaded")
			}
		})
	}
}

func TestAStoredKeyThatIsNotAnRSAKeyIsRefused(t *testing.T) {
	for name, sealed := range map[string]string{
		"not a key at all": "vault:v1:" + base64.StdEncoding.EncodeToString([]byte("nonsense")),
		"an EC key":        "vault:v1:" + base64.StdEncoding.EncodeToString(ecKey(t)),
	} {
		t.Run(name, func(t *testing.T) {
			f := newKeysFixture(sealer{})
			f.store.keys = []app.StoredKey{{SigningKey: domain.SigningKey{ID: "k", Public: &generated.PublicKey, SignsFrom: t0, SignsUntil: t0.Add(time.Hour)}, Sealed: sealed}}
			if _, err := f.Load(context.Background()); err == nil {
				t.Fatal("a key that is not RSA loaded")
			}
		})
	}
}

func TestAKeySetWithoutTheCurrentKeysPrivateHalfIsRefused(t *testing.T) {
	f := newKeysFixture(sealer{})
	f.store.fail["WithKeys"] = nil
	broken := &brokenStore{keyStore: f.store}
	f.Keys = app.NewKeys(broken, sealer{}, func() (*rsa.PrivateKey, error) { return generated, nil }, func() time.Time { return t0 }, rand.Reader)
	if _, err := f.Load(context.Background()); !errors.Is(err, app.ErrNoSigningKey) {
		t.Fatalf("err = %v", err)
	}
}

// brokenStore loses every key it is given, as a store whose writes vanish would.
type brokenStore struct{ *keyStore }

func (b *brokenStore) WithKeys(_ context.Context, add func([]app.StoredKey) (app.StoredKey, bool, error)) ([]app.StoredKey, error) {
	_, _, err := add(nil)
	return nil, err
}

func TestASecretIsMadeOnceAndReadTheSameAfterwards(t *testing.T) {
	f := newKeysFixture(sealer{})
	a, err := f.Secret(context.Background(), "oidc-crypto")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := f.Secret(context.Background(), "oidc-crypto")
	if a != b || a == ([32]byte{}) || !strings.HasPrefix(f.store.secrets["oidc-crypto"], "vault:v1:") {
		t.Fatalf("secrets %x and %x", a, b)
	}
}

func TestASecretFailsWithWhatItNeeds(t *testing.T) {
	for _, tc := range []struct {
		name string
		keys func(f *keysFixture) *app.Keys
	}{
		{"randomness", func(f *keysFixture) *app.Keys {
			return app.NewKeys(f.store, sealer{}, nil, time.Now, bytes.NewReader(nil))
		}},
		{"sealing", func(f *keysFixture) *app.Keys {
			return app.NewKeys(f.store, sealer{failSeal: errBoom}, nil, time.Now, rand.Reader)
		}},
		{"opening", func(f *keysFixture) *app.Keys {
			return app.NewKeys(f.store, sealer{failOpen: errBoom}, nil, time.Now, rand.Reader)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newKeysFixture(sealer{})
			if _, err := tc.keys(f).Secret(context.Background(), "oidc-crypto"); err == nil {
				t.Fatal("a secret came out")
			}
		})
	}
}

func TestASecretOfTheWrongLengthIsRefused(t *testing.T) {
	f := newKeysFixture(sealer{})
	f.store.secrets["oidc-crypto"] = "vault:v1:" + base64.StdEncoding.EncodeToString([]byte("short"))
	if _, err := f.Secret(context.Background(), "oidc-crypto"); err == nil {
		t.Fatal("a short secret was taken")
	}
}

func ecKey(t *testing.T) []byte {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return der
}
