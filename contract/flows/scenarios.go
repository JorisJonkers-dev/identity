package flows

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/JorisJonkers-dev/identity/contract/principal"
)

// user is a new User, Ada, holding the app's roles, signed in to identity but not yet to the app.
func (w world) user(t *testing.T, roles ...string) *person {
	t.Helper()
	p := w.stack.signUp(t, "Ada")
	p.id = w.admin.userID(t, p.email)
	for _, role := range roles {
		w.admin.give(t, p, role)
	}
	return p
}

// signIn signs p in to the app through its /login, coming back to /me, and answers what /me says.
func (w world) signIn(t *testing.T, p *person) answer {
	t.Helper()
	return p.b.get(t, w.env.BaseURL+"/login?rd=/me")
}

// me is the Principal the app states for p's session, validated against the contract's schema.
func (w world) me(t *testing.T, a answer) map[string]any {
	t.Helper()
	if a.Status != http.StatusOK || a.URL.Path != "/me" {
		t.Fatalf("flows: the app answered %d at %s: %s", a.Status, a.URL, a.Body)
	}
	var p map[string]any
	if err := json.Unmarshal(a.Body, &p); err != nil {
		t.Fatalf("flows: /me is not JSON: %s", a.Body)
	}
	validate(t, a.Body)
	return p
}

// signedOut waits up to five seconds for the app to stop knowing p.
func (w world) signedOut(t *testing.T, p *person) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if a := p.b.get(t, w.env.BaseURL+"/me"); a.Status == http.StatusUnauthorized {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("flows: the app still knows the User five seconds after identity signed them out")
}

func (w world) signsIn(t *testing.T) {
	ada := w.user(t, "user")

	p := w.me(t, w.signIn(t, ada))

	if p["subject"] != ada.id || p["kind"] != "user" || p["email"] != ada.email || p["via"] != "session" || p["sid"] == nil ||
		!sameRoles(p["roles"], "user") || p["twoFactor"] != false {
		t.Fatalf("flows: /me = %v", p)
	}
}

func (w world) signsOut(t *testing.T) {
	ada := w.user(t, "user")
	w.me(t, w.signIn(t, ada))

	ada.b.call(t, http.MethodDelete, ada.api("/api/v1/session"), nil, http.StatusNoContent, nil)

	w.signedOut(t, ada)
}

func (w world) losesARole(t *testing.T) {
	ada := w.user(t, "user", "editor")
	w.me(t, w.signIn(t, ada))

	w.admin.take(t, ada, "editor")

	w.signedOut(t, ada)
	if p := w.me(t, w.signIn(t, ada)); !sameRoles(p["roles"], "user") {
		t.Fatalf("flows: signing in again states roles %v", p["roles"])
	}
}

func (w world) stepsUp(t *testing.T) {
	ada := w.user(t, "user")
	w.me(t, w.signIn(t, ada))

	if a := ada.b.get(t, w.env.BaseURL+"/fresh?minutes=5"); a.URL.String() != w.stack.issuer+"/security/two-factor?for=step-up" {
		t.Fatalf("flows: asking for fresh two-factor without any went to %s (%d)", a.URL, a.Status)
	}
	ada.addAuthenticator(t)
	a := ada.b.get(t, w.env.BaseURL+"/fresh?minutes=5")

	if a.Status != http.StatusOK || a.URL.Path != "/fresh" {
		t.Fatalf("flows: after proving two-factor, /fresh answered %d at %s: %s", a.Status, a.URL, a.Body)
	}
	validate(t, a.Body)
	var p map[string]any
	_ = json.Unmarshal(a.Body, &p)
	if p["twoFactor"] != true || p["twoFactorAt"] == nil {
		t.Fatalf("flows: /fresh = %v", p)
	}
}

func sameRoles(got any, want ...string) bool {
	list, _ := got.([]any)
	roles := make([]string, 0, len(list))
	for _, r := range list {
		s, _ := r.(string)
		roles = append(roles, s)
	}
	slices.Sort(roles)
	slices.Sort(want)
	return slices.Equal(roles, want)
}

// validate holds a Principal the app stated to the contract's schema.
func validate(t *testing.T, raw []byte) {
	t.Helper()
	schemaJSON, err := fs.ReadFile(principal.Files, "principal.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource("principal.schema.json", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := c.Compile("principal.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	principalDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("flows: %s is not JSON", raw)
	}
	if err := schema.Validate(principalDoc); err != nil {
		t.Fatalf("flows: the app's Principal %s breaks the contract: %v", raw, err)
	}
}
