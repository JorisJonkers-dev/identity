package domain_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var t0 = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

func key(id string, from time.Time) domain.SigningKey {
	return domain.SigningKey{ID: id, SignsFrom: from, SignsUntil: from.Add(domain.KeyLifetime)}
}

func TestAKeySignsForNinetyDaysFromItsStart(t *testing.T) {
	t.Parallel()
	k := key("a", t0)
	for _, tc := range []struct {
		name  string
		at    time.Time
		signs bool
	}{
		{"before it starts", t0.Add(-time.Second), false},
		{"as it starts", t0, true},
		{"a moment before 90 days", t0.Add(domain.KeyLifetime - time.Second), true},
		{"at 90 days", t0.Add(domain.KeyLifetime), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := k.Signs(tc.at); got != tc.signs {
				t.Fatalf("signs = %v", got)
			}
		})
	}
}

func TestAKeyIsPublishedForSevenDaysAfterItStopsSigning(t *testing.T) {
	t.Parallel()
	k := key("a", t0)
	end := t0.Add(domain.KeyLifetime)
	for _, tc := range []struct {
		name      string
		at        time.Time
		published bool
	}{
		{"before it starts", t0.Add(-time.Second), false},
		{"while it signs", t0, true},
		{"a moment before 7 days after", end.Add(domain.KeyOverlap - time.Second), true},
		{"7 days after it stopped", end.Add(domain.KeyOverlap), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := k.Published(tc.at); got != tc.published {
				t.Fatalf("published = %v", got)
			}
		})
	}
}

func TestTheCurrentKeyIsTheOneThatStartedLastInWhateverOrder(t *testing.T) {
	t.Parallel()
	old, next := key("old", t0), key("next", t0.Add(time.Hour))
	for name, keys := range map[string][]domain.SigningKey{"newest first": {next, old}, "oldest first": {old, next}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, ok := domain.Current(keys, t0.Add(2*time.Hour)); !ok || got.ID != "next" {
				t.Fatalf("current = %+v, %v", got, ok)
			}
		})
	}
}

func TestThereIsNoCurrentKeyWhenNoneSigns(t *testing.T) {
	t.Parallel()
	if _, ok := domain.Current([]domain.SigningKey{key("old", t0)}, t0.Add(domain.KeyLifetime)); ok {
		t.Fatal("an ended key signs")
	}
}

func TestJWKSPublishesTheKeysInTheirWindow(t *testing.T) {
	t.Parallel()
	now := t0.Add(domain.KeyLifetime + time.Hour)
	ended, current, gone := key("ended", t0), key("current", t0.Add(domain.KeyLifetime)), key("gone", t0.Add(-domain.KeyLifetime-domain.KeyOverlap))
	got := domain.Published([]domain.SigningKey{gone, ended, current}, now)
	if len(got) != 2 || got[0].ID != "ended" || got[1].ID != "current" {
		t.Fatalf("published = %+v", got)
	}
}

func TestAKeysIDComesFromItsPublicHalf(t *testing.T) {
	t.Parallel()
	a, _ := rsa.GenerateKey(rand.Reader, 2048)
	b, _ := rsa.GenerateKey(rand.Reader, 2048)
	if id := domain.KeyID(&a.PublicKey); id != domain.KeyID(&a.PublicKey) || id == domain.KeyID(&b.PublicKey) || len(id) != 16 {
		t.Fatalf("ids %q and %q", id, domain.KeyID(&b.PublicKey))
	}
}
