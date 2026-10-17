package app_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/gate/app"
)

// tokens knows one live Personal Token, "idpat_ada", for Ada on any Client, and counts its checks.
type tokens struct {
	asked []string
	err   error
}

func (t *tokens) Bearer(_ context.Context, secret, clientID string) (app.Person, []string, error) {
	t.asked = append(t.asked, clientID)
	if t.err != nil {
		return app.Person{}, nil, t.err
	}
	if secret != "idpat_ada" {
		return app.Person{}, nil, app.ErrNoSignIn
	}
	return ada, []string{"user"}, nil
}

func withTokens(f *fixture) *tokens {
	t := &tokens{}
	f.SetTokens(t)
	return t
}

func TestAPersonalTokenOnAGatedHostAnswersItsUserAndRoles(t *testing.T) {
	f := newFixture()
	tk := withTokens(f)
	v, err := f.CheckBearer(context.Background(), stats.Host, "idpat_ada")
	if err != nil || v.UserID != ada.UserID || v.Email != ada.Email || !slices.Equal(v.Roles, []string{"user"}) {
		t.Fatalf("CheckBearer = %+v, %v", v, err)
	}
	if !slices.Equal(tk.asked, []string{stats.ID}) {
		t.Fatalf("asked for %v", tk.asked)
	}
}

func TestAPersonalTokenOnATokenHostIsCheckedForItsClient(t *testing.T) {
	f := newFixture()
	tk := withTokens(f)
	if _, err := f.CheckBearer(context.Background(), backupHost, "idpat_ada"); err != nil || !slices.Equal(tk.asked, []string{"backup"}) {
		t.Fatalf("CheckBearer = %v; asked for %v", err, tk.asked)
	}
}

func TestAnInactivePersonalTokenIsNoSignIn(t *testing.T) {
	f := newFixture()
	withTokens(f)
	if _, err := f.CheckBearer(context.Background(), stats.Host, "idpat_revoked"); !errors.Is(err, app.ErrNoSignIn) {
		t.Fatalf("err = %v", err)
	}
}

func TestAPersonalTokenOnAHostWithNoClientIsRefused(t *testing.T) {
	f := newFixture()
	withTokens(f)
	if _, err := f.CheckBearer(context.Background(), "unknown.jorisjonkers.dev", "idpat_ada"); !errors.Is(err, app.ErrUnknownHost) {
		t.Fatalf("err = %v", err)
	}
}

func TestWithoutTokensSetNoPersonalTokenSignsIn(t *testing.T) {
	if _, err := newFixture().CheckBearer(context.Background(), stats.Host, "idpat_ada"); !errors.Is(err, app.ErrNoSignIn) {
		t.Fatalf("err = %v", err)
	}
}

func TestAPersonalTokensAnswerIsKeptForTheCachesTTL(t *testing.T) {
	f := newFixture()
	tk := withTokens(f)
	for _, at := range []time.Duration{0, 29 * time.Second, 31 * time.Second} {
		f.now = t0.Add(at)
		if _, err := f.CheckBearer(context.Background(), stats.Host, "idpat_ada"); err != nil {
			t.Fatal(err)
		}
	}
	if len(tk.asked) != 2 {
		t.Fatalf("asked %d times, want 2", len(tk.asked))
	}
}

func TestARevokedPersonalTokensCachedCheckIsForgotten(t *testing.T) {
	f := newFixture()
	tk := withTokens(f)
	_, _ = f.CheckBearer(context.Background(), stats.Host, "idpat_ada")
	f.Forget([]uuid.UUID{ada.SessionID})
	_, _ = f.CheckBearer(context.Background(), stats.Host, "idpat_ada")
	if len(tk.asked) != 2 {
		t.Fatalf("asked %d times, want 2", len(tk.asked))
	}
}

func TestAFailedPersonalTokenCheckIsPassedOnAndAskedAgain(t *testing.T) {
	f := newFixture()
	tk := withTokens(f)
	tk.err = errBoom
	for range 2 {
		if _, err := f.CheckBearer(context.Background(), stats.Host, "idpat_ada"); !errors.Is(err, errBoom) {
			t.Fatalf("err = %v", err)
		}
	}
	if len(tk.asked) != 2 {
		t.Fatalf("asked %d times, want 2", len(tk.asked))
	}
}

func TestATokenHostWithoutAPersonalTokenNeedsOne(t *testing.T) {
	if _, err := newFixture().Check(context.Background(), backupHost, ""); !errors.Is(err, app.ErrBearerNeeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestAHostWithNoClientAndNoCookieIsUnknown(t *testing.T) {
	if _, err := newFixture().Check(context.Background(), "unknown.jorisjonkers.dev", ""); !errors.Is(err, app.ErrUnknownHost) {
		t.Fatalf("err = %v", err)
	}
}
