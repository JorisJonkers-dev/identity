package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

func loaded(t *testing.T, ids ...string) app.KeySet {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ks := app.KeySet{Signing: domain.SigningKey{ID: ids[0], Public: &private.PublicKey}, Private: private}
	for _, id := range ids {
		ks.Published = append(ks.Published, domain.SigningKey{ID: id, Public: &private.PublicKey})
	}
	return ks
}

func TestARingSignsWithTheKeyItLoadedAndPublishesEveryOneInItsWindow(t *testing.T) {
	ks := loaded(t, "current", "previous")
	r, err := LoadRing(context.Background(), func(context.Context) (app.KeySet, error) { return ks, nil })
	if err != nil {
		t.Fatal(err)
	}
	if r.Signing().ID() != "current" || r.Signing().Key() != ks.Private {
		t.Fatalf("signing with %s", r.Signing().ID())
	}
	if pub := r.Published(); len(pub) != 2 || pub[1].ID() != "previous" || pub[1].Use() != "sig" || pub[1].Key() != ks.Published[1].Public {
		t.Fatalf("published %+v", pub)
	}
}

func TestARingThatCannotLoadItsKeysFails(t *testing.T) {
	if _, err := LoadRing(context.Background(), func(context.Context) (app.KeySet, error) { return app.KeySet{}, errBoom }); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestARefreshTakesTheKeysAsTheyStandNow(t *testing.T) {
	sets := []app.KeySet{loaded(t, "first"), loaded(t, "second", "first")}
	calls := 0
	r, _ := LoadRing(context.Background(), func(context.Context) (app.KeySet, error) {
		calls++
		return sets[calls-1], nil
	})
	if err := r.Refresh(context.Background()); err != nil || r.Signing().ID() != "second" || len(r.Published()) != 2 {
		t.Fatalf("after refresh: %v, signing %s", err, r.Signing().ID())
	}
}

func TestAFailedRefreshKeepsTheKeysTheRingHas(t *testing.T) {
	fail := false
	r, _ := LoadRing(context.Background(), func(context.Context) (app.KeySet, error) {
		if fail {
			return app.KeySet{}, errBoom
		}
		return loaded(t, "kept"), nil
	})
	fail = true
	if err := r.Refresh(context.Background()); !errors.Is(err, errBoom) || r.Signing().ID() != "kept" {
		t.Fatalf("refresh = %v, signing %s", err, r.Signing().ID())
	}
}

func TestAFixedRingNeverChanges(t *testing.T) {
	r := NewRing(testKey())
	if err := r.Refresh(context.Background()); err != nil || r.Signing().ID() != domain.KeyID(&testKey().PublicKey) {
		t.Fatalf("refresh = %v", err)
	}
}
