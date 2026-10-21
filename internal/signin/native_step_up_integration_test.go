//go:build integration

package signin_test

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fresh is what a native app sends to ask for a two-factor proof no older than maxAge.
func fresh(maxAge time.Duration) url.Values {
	return url.Values{"acr_values": {"mfa"}, "max_age": {strconv.Itoa(int(maxAge.Seconds()))}}
}

func TestAnAppAskingForAFreshProofSendsAStaleSessionToStepUp(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.ada.AddAuthenticator(t)
	w.env.Pass(6 * time.Minute)

	login := w.ada.Authorize(t, wiki, fresh(5*time.Minute))

	id := login[strings.Index(login, "authRequestID=")+len("authRequestID="):]
	if next := w.ada.Follow(t, login); next != "/signin/step-up?authRequestID="+id {
		t.Fatalf("login went to %s", next)
	}
}

func TestAProofInsideWhatTheAppAskedForSignsInWithoutAsking(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.ada.AddAuthenticator(t)
	w.env.Pass(4 * time.Minute)

	if next := w.ada.Follow(t, w.ada.Authorize(t, wiki, fresh(5*time.Minute))); !strings.HasPrefix(next, callback) {
		t.Fatalf("login went to %s", next)
	}
}

func TestProvingTwoFactorAgainFinishesTheSignInTheAppAskedAFreshProofFor(t *testing.T) {
	t.Parallel()
	w := start(t)
	secret := w.ada.AddAuthenticator(t)
	w.env.Pass(6 * time.Minute)
	login := w.ada.Authorize(t, wiki, fresh(5*time.Minute))
	w.ada.Follow(t, login)

	w.ada.Call(t, http.MethodPost, "/api/v1/session/step-up", nil, http.StatusOK, nil)
	w.ada.Call(t, http.MethodPost, "/api/v1/session/step-up/code", map[string]string{"code": w.code(t, secret)}, http.StatusNoContent, nil)

	if next := w.ada.Follow(t, login); !strings.HasPrefix(next, callback) {
		t.Fatalf("login went to %s", next)
	}
}

func TestAnAppAskingForAFreshProofSendsAUserWithNoTwoFactorToSetItUp(t *testing.T) {
	t.Parallel()
	w := start(t)

	if next := w.ada.Follow(t, w.ada.Authorize(t, wiki, fresh(5*time.Minute))); next != "/security/two-factor?for=step-up" {
		t.Fatalf("login went to %s", next)
	}
}

func TestAMaxAgeWithoutAskingForTwoFactorChangesNothing(t *testing.T) {
	t.Parallel()
	w := start(t)

	if next := w.ada.Follow(t, w.ada.Authorize(t, wiki, url.Values{"max_age": {"300"}})); !strings.HasPrefix(next, callback) {
		t.Fatalf("login went to %s", next)
	}
}

func TestTheClientsStricterWindowWinsOverWhatTheAppAskedFor(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.ada.AddAuthenticator(t)
	w.stepUp(t, 5)
	w.env.Pass(6 * time.Minute)

	if next := w.ada.Follow(t, w.ada.Authorize(t, wiki, fresh(15*time.Minute))); !strings.HasPrefix(next, "/signin/step-up?") {
		t.Fatalf("login went to %s", next)
	}
}

func TestAskingForTwoFactorWithoutAMaxAgeAcceptsAProofUpToAnHourOld(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.ada.AddAuthenticator(t)
	w.env.Pass(61 * time.Minute)

	if next := w.ada.Follow(t, w.ada.Authorize(t, wiki, url.Values{"acr_values": {"mfa"}})); !strings.HasPrefix(next, "/signin/step-up?") {
		t.Fatalf("login went to %s", next)
	}
}

func TestWhatTheAppAskedForWinsOverTheClientsLooserWindow(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.ada.AddAuthenticator(t)
	w.stepUp(t, 15)
	w.env.Pass(6 * time.Minute)

	if next := w.ada.Follow(t, w.ada.Authorize(t, wiki, fresh(5*time.Minute))); !strings.HasPrefix(next, "/signin/step-up?") {
		t.Fatalf("login went to %s", next)
	}
}
