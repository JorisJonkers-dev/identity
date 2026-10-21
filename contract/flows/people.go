package flows

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

const password = "a flows passphrase of length" //nolint:gosec // a test User's, in a throwaway identity

var confirmLink = regexp.MustCompile(`/confirm\?token=([^\s"<>]+)`)

// person is a User signed in to identity in their own browser.
type person struct {
	s     *stack
	b     *browser
	email string
	id    string
}

func (p *person) api(path string) string { return p.s.issuer + path }

// signUp is a new User, confirmed and signed in to identity.
func (s *stack) signUp(t *testing.T, name string) *person {
	t.Helper()
	p := &person{s: s, b: newBrowser(t, s.trust), email: strings.ToLower(name+"+"+rand.Text()[:8]) + "@flows.test"}
	p.b.call(t, http.MethodPost, p.api("/api/v1/signup"), map[string]string{"email": p.email, "name": name, "password": password}, http.StatusAccepted, nil)
	p.b.call(t, http.MethodPost, p.api("/api/v1/confirmations"), map[string]string{"token": s.confirmationToken(t, p.email), "password": password}, http.StatusNoContent, nil)
	p.b.call(t, http.MethodPost, p.api("/api/v1/session"), map[string]string{"email": p.email, "password": password}, http.StatusNoContent, nil)
	return p
}

// newAdmin is an Admin whose session proved two-factor, as managing access needs.
func (s *stack) newAdmin(t *testing.T) *person {
	t.Helper()
	a := s.signUp(t, "Admin")
	if code, out, err := s.identity.Exec(t.Context(), []string{"/identity", "admin", "promote", a.email}); err != nil || code != 0 {
		t.Fatalf("flows: promoting the Admin = %d %v %v", code, err, out)
	}
	secret := a.addAuthenticator(t)
	a.b.call(t, http.MethodDelete, a.api("/api/v1/session"), nil, http.StatusNoContent, nil)
	a.b.call(t, http.MethodPost, a.api("/api/v1/session"), map[string]string{"email": a.email, "password": password}, http.StatusAccepted, nil)
	// The code that added the app is spent, so the next window's code proves it.
	a.b.call(t, http.MethodPost, a.api("/api/v1/session/two-factor"), map[string]string{"code": code(t, secret, time.Now().Add(30*time.Second))}, http.StatusNoContent, nil)
	return a
}

// addAuthenticator adds an authenticator app, which proves the session's two-factor now, and
// returns its secret.
func (p *person) addAuthenticator(t *testing.T) string {
	t.Helper()
	var setup struct{ Secret string }
	p.b.call(t, http.MethodPost, p.api("/api/v1/profile/two-factor-methods/totp"), nil, http.StatusOK, &setup)
	p.b.call(t, http.MethodPost, p.api("/api/v1/profile/two-factor-methods/totp/confirmation"),
		map[string]string{"label": "Phone", "code": code(t, setup.Secret, time.Now()), "password": password}, http.StatusNoContent, nil)
	return setup.Secret
}

func code(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	c, err := totp.GenerateCode(secret, at)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// userID is the id of the User signed up as email, as an Admin's access page lists it.
func (p *person) userID(t *testing.T, email string) string {
	t.Helper()
	var overview struct{ Users []struct{ ID, Email string } }
	p.b.call(t, http.MethodGet, p.api("/api/v1/access"), nil, http.StatusOK, &overview)
	i := slices.IndexFunc(overview.Users, func(u struct{ ID, Email string }) bool { return u.Email == email })
	if i < 0 {
		t.Fatalf("flows: no User %s", email)
	}
	return overview.Users[i].ID
}

// give gives who the app's role, as an Admin.
func (p *person) give(t *testing.T, who *person, role string) {
	t.Helper()
	p.b.call(t, http.MethodPut, p.api("/api/v1/access/users/"+who.id+"/roles/"+ClientID+"/"+role), nil, http.StatusNoContent, nil)
}

// take takes the app's role from who, as an Admin.
func (p *person) take(t *testing.T, who *person, role string) {
	t.Helper()
	p.b.call(t, http.MethodDelete, p.api("/api/v1/access/users/"+who.id+"/roles/"+ClientID+"/"+role), nil, http.StatusNoContent, nil)
}

// confirmationToken is the token in the confirmation mail sent to email.
func (s *stack) confirmationToken(t *testing.T, email string) string {
	t.Helper()
	b := newBrowser(t, s.trust)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var found struct{ Messages []struct{ ID string } }
		b.call(t, http.MethodGet, s.mailAPI+"/api/v1/search?query="+url.QueryEscape(fmt.Sprintf("to:%q", email)), nil, http.StatusOK, &found)
		if len(found.Messages) > 0 {
			var message struct{ Text string }
			b.call(t, http.MethodGet, s.mailAPI+"/api/v1/message/"+found.Messages[0].ID, nil, http.StatusOK, &message)
			if m := confirmLink.FindStringSubmatch(message.Text); m != nil {
				return m[1]
			}
			t.Fatalf("flows: no confirmation link in the mail to %s", email)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("flows: no mail reached %s", email)
	return ""
}
