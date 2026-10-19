//go:build integration

package users_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
)

const newSubject = "Confirm your new email"

type pending struct {
	Email        string
	PendingEmail string
}

// asked is Ada after asking to change her email to a new address of her own, and that address.
func asked(t *testing.T, env *itest.Env) (*itest.Browser, string) {
	t.Helper()
	ada := env.SignedUp(t, "ada@example.org")
	next := strings.Replace(ada.Email, "ada+", "lovelace+", 1)
	ada.Call(t, http.MethodPost, "/api/v1/profile/email-change", map[string]string{"email": next, "password": itest.Password}, http.StatusAccepted, nil)
	return ada, next
}

func confirmEmail(t *testing.T, b *itest.Browser, token string, want int) {
	t.Helper()
	b.Call(t, http.MethodPost, "/api/v1/email-changes/confirmation", map[string]string{"token": token}, want, nil)
}

func TestAUserChangesTheirEmailThroughTheLinkMailedToTheNewAddress(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada, next := asked(t, env)
	confirmEmail(t, env.Browser(), itest.TokenIn(t, itest.Mailed(t, next, newSubject), "/account/email"), http.StatusNoContent)
	var p pending
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, &p)
	if p.Email != next || p.PendingEmail != "" {
		t.Fatalf("profile = %+v", p)
	}
	env.Browser().Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": next, "password": itest.Password}, http.StatusNoContent, nil)
}

func TestTheOldAddressIsToldTheEmailChanged(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada, next := asked(t, env)
	confirmEmail(t, env.Browser(), itest.TokenIn(t, itest.Mailed(t, next, newSubject), "/account/email"), http.StatusNoContent)
	if text := itest.Mailed(t, ada.Email, "Your sign-in email changed"); !strings.Contains(text, next) {
		t.Fatalf("notice = %q", text)
	}
}

func TestAPendingEmailChangeShowsOnTheProfile(t *testing.T) {
	t.Parallel()
	ada, next := asked(t, itest.Start(t))
	var p pending
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, &p)
	if p.Email != ada.Email || p.PendingEmail != next {
		t.Fatalf("profile = %+v", p)
	}
}

func TestAskingForANewEmailNeedsThePassword(t *testing.T) {
	t.Parallel()
	ada := itest.Start(t).SignedUp(t, "ada@example.org")
	var p problem
	ada.Call(t, http.MethodPost, "/api/v1/profile/email-change", map[string]string{"email": "new@example.org", "password": "not the password"}, http.StatusUnprocessableEntity, &p)
	if p.Detail != "Your password is wrong. Enter it again." {
		t.Fatalf("detail = %q", p.Detail)
	}
}

func TestAnEmailChangeLinkWorksOnce(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	_, next := asked(t, env)
	token := itest.TokenIn(t, itest.Mailed(t, next, newSubject), "/account/email")
	confirmEmail(t, env.Browser(), token, http.StatusNoContent)
	confirmEmail(t, env.Browser(), token, http.StatusUnprocessableEntity)
}

func TestAnEmailChangeLinkExpiresAfterADay(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	_, next := asked(t, env)
	token := itest.TokenIn(t, itest.Mailed(t, next, newSubject), "/account/email")
	env.Pass(24*time.Hour + time.Minute)
	confirmEmail(t, env.Browser(), token, http.StatusUnprocessableEntity)
}

func TestAnAddressInUseGetsTheSameAnswerAndItsLinkFails(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	bob := env.SignedUp(t, "bob@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	ada.Call(t, http.MethodPost, "/api/v1/profile/email-change", map[string]string{"email": bob.Email, "password": itest.Password}, http.StatusAccepted, nil)
	var p problem
	env.Browser().Call(t, http.MethodPost, "/api/v1/email-changes/confirmation", map[string]string{"token": itest.TokenIn(t, itest.Mailed(t, bob.Email, newSubject), "/account/email")}, http.StatusUnprocessableEntity, &p)
	if p.Detail != "This link has expired or was already used. Ask for a new one on the Account page." {
		t.Fatalf("detail = %q", p.Detail)
	}
	var mine pending
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, &mine)
	if mine.Email != ada.Email {
		t.Fatalf("profile = %+v", mine)
	}
}

func TestACancelledEmailChangeIsGoneAndItsLinkFails(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada, next := asked(t, env)
	ada.Call(t, http.MethodDelete, "/api/v1/profile/email-change", nil, http.StatusNoContent, nil)
	var p pending
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, &p)
	if p.PendingEmail != "" {
		t.Fatalf("profile = %+v", p)
	}
	confirmEmail(t, env.Browser(), itest.TokenIn(t, itest.Mailed(t, next, newSubject), "/account/email"), http.StatusUnprocessableEntity)
}

func TestChangingTheEmailIsAnAuditEvent(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada, next := asked(t, env)
	confirmEmail(t, env.Browser(), itest.TokenIn(t, itest.Mailed(t, next, newSubject), "/account/email"), http.StatusNoContent)
	var page struct {
		Events []struct {
			Action, Email string
			User          *struct{ Email string }
		}
	}
	admin.Call(t, http.MethodGet, "/api/v1/audit?kind=people", nil, http.StatusOK, &page)
	e := page.Events[0]
	if e.Action != "email-changed" || e.Email != ada.Email || e.User == nil || e.User.Email != next {
		t.Fatalf("event = %+v", e)
	}
}
