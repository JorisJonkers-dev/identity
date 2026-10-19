package oidc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	connectedapp "github.com/JorisJonkers-dev/identity/internal/connected/app"
)

// asking is a tool's request waiting for Ada's consent in this browser, by its id.
func (e *env) asking(t *testing.T) string {
	t.Helper()
	login := e.loginURL(t, toolQuery(e.registered(t), "https://web.test/"))
	next := redirect(t, e.get(t, login, true), http.StatusSeeOther, consentPath+"?authRequestID=")
	return strings.TrimPrefix(next, consentPath+"?authRequestID=")
}

func (e *env) answer(t *testing.T, body string) page {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, e.issuer+consentAPIPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for name, value := range e.cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value}) //nolint:gosec // a request's cookie carries no attributes
	}
	req.AddCookie(&http.Cookie{Name: cookieName, Value: e.session}) //nolint:gosec // as above
	return e.do(t, req)
}

func next(t *testing.T, p page) string {
	t.Helper()
	var n consentNext
	if err := json.Unmarshal(p.body, &n); err != nil || p.status != http.StatusOK {
		t.Fatalf("answer = %d %s", p.status, p.body)
	}
	return n.Redirect
}

func TestTheConsentPageNamesTheAppTheServiceTheRolesAndWhereItReturns(t *testing.T) {
	e := newEnv(t)
	id := e.asking(t)
	p := e.get(t, consentAPIPath+"?authRequestID="+id, true)
	var got consentPrompt
	_ = json.Unmarshal(p.body, &got)
	if p.status != http.StatusOK || got.App != "Claude Code" || got.Client != "" || fmt.Sprint(got.Roles) != "[user]" || got.RedirectHost != "127.0.0.1:33418" {
		t.Fatalf("prompt = %d %+v", p.status, got)
	}
}

func TestAllowingSendsTheBrowserOnToCollectTheToolsCode(t *testing.T) {
	e := newEnv(t)
	id := e.asking(t)
	callback := next(t, e.answer(t, `{"authRequestID":"`+id+`","allow":true}`))
	back := redirect(t, e.get(t, callback, false), http.StatusFound, "http://127.0.0.1:33418/callback?")
	if u, _ := url.Parse(back); u.Query().Get("code") == "" || u.Query().Get("state") != "st" {
		t.Fatalf("the tool got %s", back)
	}
}

func TestDecliningTellsTheToolThePersonSaidNo(t *testing.T) {
	e := newEnv(t)
	id := e.asking(t)
	u, _ := url.Parse(next(t, e.answer(t, `{"authRequestID":"`+id+`","allow":false}`)))
	q := u.Query()
	if u.Host != "127.0.0.1:33418" || q.Get("error") != "access_denied" || q.Get("state") != "st" || q.Get("iss") != e.issuer {
		t.Fatalf("declined to %s", u)
	}
}

func TestAConsentCallIsRefused(t *testing.T) {
	for _, c := range []struct {
		name   string
		status int
		call   func(t *testing.T, e *env, id string) page
	}{
		{"with a broken request id", http.StatusBadRequest, func(t *testing.T, e *env, _ string) page {
			return e.get(t, consentAPIPath+"?authRequestID=nope", true)
		}},
		{"from another browser", http.StatusBadRequest, func(t *testing.T, e *env, id string) page {
			return e.elsewhere().get(t, consentAPIPath+"?authRequestID="+id, true)
		}},
		{"without a session", http.StatusUnauthorized, func(t *testing.T, e *env, id string) page {
			return e.get(t, consentAPIPath+"?authRequestID="+id, false)
		}},
		{"once answered", http.StatusNotFound, func(t *testing.T, e *env, id string) page {
			e.answer(t, `{"authRequestID":"`+id+`","allow":false}`)
			return e.answer(t, `{"authRequestID":"`+id+`","allow":false}`)
		}},
		{"for a User with no App Role there", http.StatusForbidden, func(t *testing.T, e *env, id string) page {
			e.access.roles = map[string][]string{}
			return e.answer(t, `{"authRequestID":"`+id+`","allow":true}`)
		}},
		{"for a service that stopped taking Connected Apps", http.StatusForbidden, func(t *testing.T, e *env, id string) page {
			e.apps.refuse = connectedapp.ErrStepUpClient
			return e.answer(t, `{"authRequestID":"`+id+`","allow":true}`)
		}},
		{"when the tool cannot be read", http.StatusInternalServerError, func(t *testing.T, e *env, id string) page {
			e.apps.fail["Registration"] = errBoom
			return e.get(t, consentAPIPath+"?authRequestID="+id, true)
		}},
		{"for an answer from another browser", http.StatusBadRequest, func(t *testing.T, e *env, id string) page {
			return e.elsewhere().answer(t, `{"authRequestID":"`+id+`","allow":true}`)
		}},
		{"with an answer that is not JSON", http.StatusBadRequest, func(t *testing.T, e *env, _ string) page {
			return e.answer(t, `{`)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			if p := c.call(t, e, e.asking(t)); p.status != c.status {
				t.Fatalf("got %d %s, want %d", p.status, p.body, c.status)
			}
		})
	}
}

func TestDecliningARequestWithNoStateSendsNone(t *testing.T) {
	u, _ := url.Parse(denied("http://127.0.0.1:1/cb", "", "https://auth.test"))
	if _, has := u.Query()["state"]; has || u.Query().Get("error") != "access_denied" {
		t.Fatalf("declined to %s", u)
	}
}
