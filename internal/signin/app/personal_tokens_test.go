package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

// personalTokens knows one token, for Ada, and records the check it was asked.
type personalTokens struct {
	id, user uuid.UUID
	checked  [2]string
	err      error
}

func (p *personalTokens) Resolve(_ context.Context, secret string) (uuid.UUID, uuid.UUID, error) {
	if secret != "idpat_known" {
		return uuid.Nil, uuid.Nil, app.ErrNotFound
	}
	return p.id, p.user, nil
}

func (p *personalTokens) Check(_ context.Context, id uuid.UUID, clientID string) (app.PersonalToken, error) {
	p.checked = [2]string{id.String(), clientID}
	return app.PersonalToken{ID: id, UserID: p.user, ClientID: clientID}, p.err
}

func withTokens(f *fixture) *personalTokens {
	p := &personalTokens{id: uuid.New(), user: f.accounts.ada}
	f.SetPersonalTokens(p)
	return p
}

func TestASecretResolvesToATokenIDAndItsUser(t *testing.T) {
	f := newFixture()
	p := withTokens(f)
	id, subject, ok := f.ResolvePersonalToken(context.Background(), "idpat_known")
	if !ok || id != "pat-"+p.id.String() || subject != p.user.String() || !app.IsPersonalToken(id) {
		t.Fatalf("Resolve = %q, %q, %v", id, subject, ok)
	}
}

func TestASecretNoTokenHasResolvesToNothing(t *testing.T) {
	f := newFixture()
	withTokens(f)
	if _, _, ok := f.ResolvePersonalToken(context.Background(), "idpat_unknown"); ok {
		t.Fatal("an unknown secret resolved")
	}
}

func TestWithoutPersonalTokensNothingResolves(t *testing.T) {
	if _, _, ok := newFixture().ResolvePersonalToken(context.Background(), "idpat_known"); ok {
		t.Fatal("a secret resolved with no tokens set")
	}
}

func TestAnAccessTokensIDIsNoPersonalTokens(t *testing.T) {
	if app.IsPersonalToken(uuid.NewString()) {
		t.Fatal("a jti read as a Personal Token")
	}
}

func TestAPersonalTokenIsCheckedForTheClientThatAsks(t *testing.T) {
	f := newFixture()
	p := withTokens(f)
	got, err := f.CheckPersonalToken(context.Background(), "pat-"+p.id.String(), "spa")
	if err != nil || got.ID != p.id || p.checked != [2]string{p.id.String(), "spa"} {
		t.Fatalf("Check = %+v, %v; asked %v", got, err, p.checked)
	}
}

func TestACheckOfWhatIsNoPersonalTokenIsInactive(t *testing.T) {
	for name, tc := range map[string]struct {
		id     string
		tokens bool
	}{
		"an access token's id":                {id: uuid.NewString(), tokens: true},
		"a Personal Token id that is no uuid": {id: "pat-nonsense", tokens: true},
		"any id, with no tokens set":          {id: "pat-" + uuid.NewString()},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			if tc.tokens {
				withTokens(f)
			}
			if _, err := f.CheckPersonalToken(context.Background(), tc.id, "spa"); !errors.Is(err, app.ErrInactive) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
