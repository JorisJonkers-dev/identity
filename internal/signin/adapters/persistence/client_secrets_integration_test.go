//go:build integration

package persistence_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/persistence"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

func secretStore(t *testing.T, url string) *persistence.ClientSecrets {
	t.Helper()
	store, err := pg.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return persistence.NewClientSecrets(store.Queries(), store, func() time.Time { return keysT0 })
}

func mintOf(hash string) func() ([]byte, error) {
	return func() ([]byte, error) { return []byte(hash), nil }
}

// grafanaHashes is Grafana's current and previous hash.
func grafanaHashes(t *testing.T, s *persistence.ClientSecrets) (current, previous string) {
	t.Helper()
	c, p, err := s.Hashes(t.Context(), "grafana")
	if err != nil {
		t.Fatal(err)
	}
	return string(c), string(p)
}

func TestTwoReplicasSeeingANewClientTogetherMakeOneSecret(t *testing.T) {
	url := pgtest.URL(t)
	a, b := secretStore(t, url), secretStore(t, url)
	var minted atomic.Int32
	slow := mintOf("h1")
	mint := func() ([]byte, error) {
		minted.Add(1)
		time.Sleep(50 * time.Millisecond) // long enough for the other replica to be waiting on the lock
		return slow()
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, s := range []*persistence.ClientSecrets{a, b} {
		wg.Go(func() { errs[i] = s.Provision(t.Context(), "grafana", mint) })
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || minted.Load() != 1 {
		t.Fatalf("minted %d times (%v, %v)", minted.Load(), errs[0], errs[1])
	}
	if current, _ := grafanaHashes(t, b); current != "h1" {
		t.Fatalf("hash = %q", current)
	}
}

func TestAClientWithNoSecretHasNoHashes(t *testing.T) {
	if _, _, err := secretStore(t, pgtest.URL(t)).Hashes(t.Context(), "nobody"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestASecretThatCouldNotBeMadeIsNotKept(t *testing.T) {
	s := secretStore(t, pgtest.URL(t))
	errBoom := errors.New("boom")
	if err := s.Provision(t.Context(), "grafana", func() ([]byte, error) { return nil, errBoom }); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := s.Hashes(t.Context(), "grafana"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a failed secret was kept: %v", err)
	}
}

func TestARotationKeepsTheReplacedSecretUntilItIsEnded(t *testing.T) {
	s := secretStore(t, pgtest.URL(t))
	if err := s.Provision(t.Context(), "grafana", mintOf("h1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Rotate(t.Context(), "grafana", mintOf("h2")); err != nil {
		t.Fatal(err)
	}
	if current, previous := grafanaHashes(t, s); current != "h2" || previous != "h1" {
		t.Fatalf("hashes = %q, %q", current, previous)
	}
	if err := s.EndPrevious(t.Context(), "grafana", []byte("h2")); err != nil {
		t.Fatal(err)
	}
	if current, previous := grafanaHashes(t, s); current != "h2" || previous != "" {
		t.Fatalf("hashes after ending = %q, %q", current, previous)
	}
}

func TestRotatingTwiceBeforeTheNewSecretIsUsedKeepsTheOneTheClientLastUsed(t *testing.T) {
	s := secretStore(t, pgtest.URL(t))
	if err := s.Provision(t.Context(), "grafana", mintOf("h1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Rotate(t.Context(), "grafana", mintOf("h2")); err != nil {
		t.Fatal(err)
	}
	if err := s.Rotate(t.Context(), "grafana", mintOf("h3")); err != nil {
		t.Fatal(err)
	}
	if current, previous := grafanaHashes(t, s); current != "h3" || previous != "h1" {
		t.Fatalf("hashes = %q, %q", current, previous)
	}
}

func TestEndingThePreviousSecretAfterAnotherRotationLeavesItAccepted(t *testing.T) {
	s := secretStore(t, pgtest.URL(t))
	_ = s.Provision(t.Context(), "grafana", mintOf("h1"))
	_ = s.Rotate(t.Context(), "grafana", mintOf("h2"))
	_ = s.Rotate(t.Context(), "grafana", mintOf("h3"))
	if err := s.EndPrevious(t.Context(), "grafana", []byte("h2")); err != nil {
		t.Fatal(err)
	}
	if _, previous := grafanaHashes(t, s); previous != "h1" {
		t.Fatalf("previous = %q", previous)
	}
}

func TestRotatingAClientWithNoSecretYetGivesItOne(t *testing.T) {
	s := secretStore(t, pgtest.URL(t))
	if err := s.Rotate(t.Context(), "grafana", mintOf("h1")); err != nil {
		t.Fatal(err)
	}
	if current, previous := grafanaHashes(t, s); current != "h1" || previous != "" {
		t.Fatalf("hashes = %q, %q", current, previous)
	}
}

func TestAFailedRotationChangesNothing(t *testing.T) {
	s := secretStore(t, pgtest.URL(t))
	_ = s.Provision(t.Context(), "grafana", mintOf("h1"))
	if err := s.Rotate(t.Context(), "grafana", func() ([]byte, error) { return nil, errors.New("vault: answered 500") }); err == nil {
		t.Fatal("rotated")
	}
	if current, previous := grafanaHashes(t, s); current != "h1" || previous != "" {
		t.Fatalf("hashes = %q, %q", current, previous)
	}
}
