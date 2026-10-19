//go:build integration

package users_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
)

// lastEvent is the newest Audit Event of kind, as the Admin reads it.
func lastEvent(t *testing.T, admin *itest.Browser, kind string) (action string, actor, user string) {
	t.Helper()
	var page struct {
		Events []struct {
			Action      string
			Actor, User *struct{ Email string }
		}
	}
	admin.Call(t, http.MethodGet, "/api/v1/audit?kind="+kind, nil, http.StatusOK, &page)
	e := page.Events[0]
	if e.Actor != nil {
		actor = e.Actor.Email
	}
	if e.User != nil {
		user = e.User.Email
	}
	return e.Action, actor, user
}

func TestAnAdminRenamesAPerson(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	admin.Call(t, http.MethodPatch, "/api/v1/access/users/"+admin.UserID(t, ada.Email), map[string]string{"name": "Ada King"}, http.StatusNoContent, nil)
	var p profile
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, &p)
	if p.Name != "Ada King" {
		t.Fatalf("profile = %+v", p)
	}
	if action, actor, user := lastEvent(t, admin, "people"); action != "user-renamed" || actor != admin.Email || user != ada.Email {
		t.Fatalf("event = %s by %s about %s", action, actor, user)
	}
}

func TestOnlyAnAdminRenamesSomeoneElse(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	bob := env.SignedUp(t, "bob@example.org")
	bob.Call(t, http.MethodPatch, "/api/v1/access/users/"+admin.UserID(t, ada.Email), map[string]string{"name": "Mallory"}, http.StatusForbidden, nil)
}

func TestAnAdminStartsAnEmailChangeThatWaitsOnTheLink(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	next := strings.Replace(ada.Email, "ada+", "lovelace+", 1)
	admin.Call(t, http.MethodPost, "/api/v1/access/users/"+admin.UserID(t, ada.Email)+"/email-change", map[string]string{"email": next}, http.StatusAccepted, nil)
	var p pending
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, &p)
	if p.Email != ada.Email || p.PendingEmail != next {
		t.Fatalf("profile = %+v", p)
	}
	confirmEmail(t, env.Browser(), itest.TokenIn(t, itest.Mailed(t, next, newSubject), "/account/email"), http.StatusNoContent)
	itest.Mailed(t, ada.Email, "Your sign-in email changed")
}

func TestAnAdminDeletesAPerson(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	adaID := admin.UserID(t, ada.Email)
	admin.Call(t, http.MethodDelete, "/api/v1/access/users/"+adaID, nil, http.StatusNoContent, nil)
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
	var o struct{ Users []struct{ ID string } }
	admin.Call(t, http.MethodGet, "/api/v1/access", nil, http.StatusOK, &o)
	if slices.ContainsFunc(o.Users, func(u struct{ ID string }) bool { return u.ID == adaID }) {
		t.Fatalf("users = %+v", o.Users)
	}
	if action, actor, _ := lastEvent(t, admin, "people"); action != "account-deleted" || actor != admin.Email {
		t.Fatalf("event = %s by %s", action, actor)
	}
}

func TestTheLastAdminCannotBeDeleted(t *testing.T) {
	t.Parallel()
	admin := itest.Start(t).Admin(t, "root@example.org")
	var p problem
	admin.Call(t, http.MethodDelete, "/api/v1/access/users/"+admin.UserID(t, admin.Email), nil, http.StatusUnprocessableEntity, &p)
	if p.Detail != "That is the last Admin. Make someone else an Admin first." {
		t.Fatalf("detail = %q", p.Detail)
	}
}

func TestOnlyAnAdminDeletesSomeoneElse(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	bob := env.SignedUp(t, "bob@example.org")
	bob.Call(t, http.MethodDelete, "/api/v1/access/users/"+admin.UserID(t, ada.Email), nil, http.StatusForbidden, nil)
}

func TestDeletingSomeoneWhoIsGoneIsNotFound(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	id := admin.UserID(t, ada.Email)
	admin.Call(t, http.MethodDelete, "/api/v1/access/users/"+id, nil, http.StatusNoContent, nil)
	admin.Call(t, http.MethodDelete, "/api/v1/access/users/"+id, nil, http.StatusNotFound, nil)
}
