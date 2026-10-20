package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/gate/app"
)

// steppingUp is the Gate with step-up at 5 minutes on Tribelt stats, and Ada's session having
// proved two-factor proved ago.
func steppingUp(proved time.Duration, twoFactor bool) *fixture {
	f := newFixture()
	gated := stats
	gated.StepUp = 5 * time.Minute
	f.Service = app.New(f.store, clients{stats.Host: gated}, f.roles, f.accounts, f.events, func() time.Time { return f.now }, &counter{}, 30*time.Second)
	f.accounts.proof, f.accounts.twoFactor = t0, twoFactor
	f.now = t0.Add(proved)
	return f
}

func TestAStepUpHostSignsInWithAProofInsideItsWindow(t *testing.T) {
	f := steppingUp(5*time.Minute, true)
	if _, err := f.Start(context.Background(), "sso", stats.Host, "s", "/", 0); err != nil {
		t.Fatal(err)
	}
}

func TestAStepUpHostAsksForAFresherProof(t *testing.T) {
	f := steppingUp(5*time.Minute+time.Second, true)
	if _, err := f.Start(context.Background(), "sso", stats.Host, "s", "/", 0); !errors.Is(err, app.ErrStepUpNeeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestAStepUpHostSendsAUserWithNoTwoFactorToSetItUp(t *testing.T) {
	f := steppingUp(time.Minute, false)
	f.accounts.proof = time.Time{}
	if _, err := f.Start(context.Background(), "sso", stats.Host, "s", "/", 0); !errors.Is(err, app.ErrTwoFactorNeeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestAStepUpHostSignInEndsAtTheWindowEvenInsideASingleSignOnSession(t *testing.T) {
	f := steppingUp(0, true)
	cookie := f.signedIn(t)
	f.now = t0.Add(4*time.Minute + 50*time.Second)
	if _, err := f.Check(context.Background(), stats.Host, cookie); err != nil {
		t.Fatalf("inside the window = %v", err)
	}
	// The answer was cached ten seconds before the sign-in ends; the cache keeps it no longer.
	f.now = t0.Add(5 * time.Minute)
	if _, err := f.Check(context.Background(), stats.Host, cookie); !errors.Is(err, app.ErrNoSignIn) {
		t.Fatalf("at the window = %v", err)
	}
}

func TestAStepUpHostsAnswerIsKeptOnlyForTheCachesTTLWhenItsWindowEndsLater(t *testing.T) {
	f := steppingUp(0, true)
	cookie := f.signedIn(t)
	if _, err := f.Check(context.Background(), stats.Host, cookie); err != nil {
		t.Fatal(err)
	}
	f.roles.held[stats.ID] = nil
	f.now = t0.Add(31 * time.Second)
	if _, err := f.Check(context.Background(), stats.Host, cookie); !errors.Is(err, app.ErrNoAccess) {
		t.Fatalf("after the TTL, inside the window = %v", err)
	}
}

func TestAnAppAskingForAFresherProofThanTheSessionHasSendsItToStepUp(t *testing.T) {
	f := newFixture()
	f.accounts.proof, f.accounts.twoFactor = t0, true
	f.now = t0.Add(10 * time.Minute)

	if _, err := f.Start(context.Background(), "sso", stats.Host, "s", "/", 5*time.Minute); !errors.Is(err, app.ErrStepUpNeeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnAppAskingForAProofTheSessionAlreadyHasSignsInAtOnce(t *testing.T) {
	f := newFixture()
	f.accounts.proof, f.accounts.twoFactor = t0, true
	f.now = t0.Add(4 * time.Minute)

	if _, err := f.Start(context.Background(), "sso", stats.Host, "s", "/", 5*time.Minute); err != nil {
		t.Fatal(err)
	}
}

func TestAnAppAskingForAProofSendsAUserWithNoTwoFactorToSetItUp(t *testing.T) {
	f := newFixture()

	if _, err := f.Start(context.Background(), "sso", stats.Host, "s", "/", 5*time.Minute); !errors.Is(err, app.ErrTwoFactorNeeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestTheStricterOfTheAppsAskAndTheClientsWindowWins(t *testing.T) {
	f := steppingUp(3*time.Minute, true)

	if _, err := f.Start(context.Background(), "sso", stats.Host, "s", "/", 2*time.Minute); !errors.Is(err, app.ErrStepUpNeeded) {
		t.Fatalf("the app's ask = %v", err)
	}
	if _, err := f.Start(context.Background(), "sso", stats.Host, "s", "/", 10*time.Minute); err != nil {
		t.Fatalf("the Client's window = %v", err)
	}
}

func TestASignInToAHostDropsItsSessionsCachedChecksOnEveryHost(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	cookie := f.signedIn(t)
	_, _ = f.Check(ctx, stats.Host, cookie)
	reads := f.store.reads

	f.signedIn(t)

	if _, _ = f.Check(ctx, stats.Host, cookie); f.store.reads != reads+1 {
		t.Fatal("the earlier host cookie's check was still answered from the cache")
	}
}
