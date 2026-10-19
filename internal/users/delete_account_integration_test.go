//go:build integration

package users_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var stats = signindomain.Client{ID: "stats", SignIn: signindomain.Gated, Host: "stats.test", Label: "Stats", Roles: []string{"user"}}

func deleteAccount(t *testing.T, b *itest.Browser, password string, want int, out any) {
	t.Helper()
	b.Call(t, http.MethodPost, "/api/v1/profile/deletion", map[string]string{"password": password}, want, out)
}

func TestAUserDeletesTheirAccountWithTheirPassword(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada := env.SignedUp(t, "ada@example.org")
	deleteAccount(t, ada, itest.Password, http.StatusNoContent, nil)
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
	env.Browser().Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": ada.Email, "password": itest.Password}, http.StatusUnauthorized, nil)
}

func TestDeletingAnAccountNeedsThePassword(t *testing.T) {
	t.Parallel()
	ada := itest.Start(t).SignedUp(t, "ada@example.org")
	var p problem
	deleteAccount(t, ada, "not the password", http.StatusUnprocessableEntity, &p)
	if p.Detail != "Your password is wrong. Enter it again." {
		t.Fatalf("detail = %q", p.Detail)
	}
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, nil)
}

func TestAUserWithTwoFactorProvesItFreshlyBeforeDeleting(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada := env.SignedUp(t, "ada@example.org")
	secret := ada.AddAuthenticator(t)
	env.Pass(6 * time.Minute)
	var p problem
	deleteAccount(t, ada, itest.Password, http.StatusForbidden, &p)
	if p.Detail != "Confirm it is you with two-factor first." {
		t.Fatalf("detail = %q", p.Detail)
	}
	ada.Call(t, http.MethodPost, "/api/v1/session/step-up", nil, http.StatusOK, nil)
	code, err := totp.GenerateCode(secret, env.Now())
	if err != nil {
		t.Fatal(err)
	}
	ada.Call(t, http.MethodPost, "/api/v1/session/step-up/code", map[string]string{"code": code}, http.StatusNoContent, nil)
	deleteAccount(t, ada, itest.Password, http.StatusNoContent, nil)
}

func TestADeletedUsersEmailSignsUpAgain(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada := env.SignedUp(t, "ada@example.org")
	deleteAccount(t, ada, itest.Password, http.StatusNoContent, nil)
	again := env.Browser()
	again.Call(t, http.MethodPost, "/api/v1/signup", map[string]string{"email": ada.Email, "name": "Ada", "password": itest.Password}, http.StatusAccepted, nil)
	token := itest.TokenIn(t, itest.Mailed(t, ada.Email, "Confirm your email"), "/confirm")
	again.Call(t, http.MethodPost, "/api/v1/confirmations", map[string]string{"token": token, "password": itest.Password}, http.StatusNoContent, nil)
}

func TestNothingOfADeletedUserWorksAfterwards(t *testing.T) {
	t.Parallel()
	env := itest.Start(t, stats)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	adaID := admin.UserID(t, ada.Email)
	admin.Call(t, http.MethodPut, "/api/v1/access/users/"+adaID+"/roles/stats/user", nil, http.StatusNoContent, nil)
	var m struct{ Secret string }
	ada.Call(t, http.MethodPost, "/api/v1/profile/tokens", map[string]any{"name": "script", "clientId": "stats", "days": 30}, http.StatusCreated, &m)
	if res := env.GateCheck(t, "stats.test", "Bearer "+m.Secret); res.StatusCode != http.StatusOK {
		t.Fatalf("before = %d", res.StatusCode)
	}
	phone := ada.SignedInAgain(t, "")
	deleteAccount(t, ada, itest.Password, http.StatusNoContent, nil)
	phone.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
	if res := env.GateCheck(t, "stats.test", "Bearer "+m.Secret); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("the token after = %d", res.StatusCode)
	}
	var o struct {
		Users           []struct{ ID string }
		UserAssignments []struct{ UserID string }
	}
	admin.Call(t, http.MethodGet, "/api/v1/access", nil, http.StatusOK, &o)
	if len(o.Users) != 1 || len(o.UserAssignments) != 0 {
		t.Fatalf("overview = %+v", o)
	}
}

func TestTheLastAdminCannotDeleteThemself(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t).Admin(t, "root@example.org")
	var p problem
	deleteAccount(t, admin, itest.Password, http.StatusUnprocessableEntity, &p)
	if p.Detail != "You are the last Admin. Make someone else an Admin before you delete your account." {
		t.Fatalf("detail = %q", p.Detail)
	}
}

func TestAuditEventsAboutADeletedUserNameNoOneAndTheDeletionIsOne(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	adaID := admin.UserID(t, ada.Email)
	deleteAccount(t, ada, itest.Password, http.StatusNoContent, nil)
	var page struct {
		Events []struct {
			Action string
			User   *struct{ ID, Email string }
		}
	}
	admin.Call(t, http.MethodGet, "/api/v1/audit?user="+adaID, nil, http.StatusOK, &page)
	if len(page.Events) < 2 || page.Events[0].Action != "account-deleted" {
		t.Fatalf("events = %+v", page.Events)
	}
	for _, e := range page.Events {
		if e.User == nil || e.User.ID != adaID || e.User.Email != "" {
			t.Fatalf("an event still names the User: %+v", e)
		}
	}
}
