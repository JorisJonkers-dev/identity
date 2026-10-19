//go:build integration

package connected_test

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// kb is a gated app serving an MCP endpoint at https://kb.test/mcp.
var kb = signindomain.Client{ID: "kb", SignIn: signindomain.Gated, Host: "kb.test", Label: "Knowledge", Roles: []string{"user", "editor"}}

const (
	redirectURI = "http://127.0.0.1:33418/callback"
	verifier    = "a-tool-verifier-of-enough-length-for-pkce-0123456789"
)

// world is identity with Knowledge taking Connected Apps and Ada holding its user role.
type world struct {
	env        *itest.Env
	admin, ada *itest.Browser
}

func newWorld(t *testing.T) world {
	t.Helper()
	env := itest.Start(t, kb)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	admin.Call(t, http.MethodPut, "/api/v1/access/users/"+admin.UserID(t, ada.Email)+"/roles/kb/user", nil, http.StatusNoContent, nil)
	admin.Call(t, http.MethodPut, "/api/v1/apps/kb/mcp", map[string]bool{"on": true}, http.StatusNoContent, nil)
	return world{env: env, admin: admin, ada: ada}
}

// register is a tool registering itself, as an MCP client does, and answers its client_id.
func (w world) register(t *testing.T) string {
	t.Helper()
	var reg struct {
		ClientID string `json:"client_id"`
	}
	w.env.Browser().Call(t, http.MethodPost, "/oidc/register", map[string]any{"client_name": "Claude Code", "redirect_uris": []string{redirectURI}}, http.StatusCreated, &reg)
	return reg.ClientID
}

func authorize(clientID, resource string) string {
	sum := sha256.Sum256([]byte(verifier))
	return "/oidc/authorize?" + url.Values{
		"client_id": {clientID}, "redirect_uri": {redirectURI}, "response_type": {"code"}, "state": {"st"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}, "resource": {resource},
	}.Encode()
}

// connect is Ada connecting a newly registered tool to Knowledge; it answers the tool's id and
// the tokens its code buys.
func (w world) connect(t *testing.T) (string, map[string]any) {
	t.Helper()
	tool := w.register(t)
	consent := w.ada.Follow(t, w.ada.Follow(t, authorize(tool, "https://kb.test/mcp")))
	id := strings.TrimPrefix(consent, "/connect?authRequestID=")
	var next struct{ Redirect string }
	w.ada.Call(t, http.MethodPost, "/oidc/consent", map[string]any{"authRequestID": id, "allow": true}, http.StatusOK, &next)
	back, _ := url.Parse(w.ada.Follow(t, next.Redirect))
	status, tokens := w.env.Token(t, url.Values{
		"grant_type": {"authorization_code"}, "code": {back.Query().Get("code")}, "client_id": {tool},
		"redirect_uri": {redirectURI}, "code_verifier": {verifier},
	}, "", "")
	if status != http.StatusOK {
		t.Fatalf("token = %d %v", status, tokens)
	}
	return tool, tokens
}

func (w world) refresh(t *testing.T, tool string, tokens map[string]any) (int, map[string]any) {
	t.Helper()
	return w.env.Token(t, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tokens["refresh_token"].(string)}, "client_id": {tool}}, "", "")
}

func bearer(tokens map[string]any) string { return "Bearer " + tokens["access_token"].(string) }

func TestAToolAdaConnectedReachesKnowledgeAsHerThroughTheGate(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	_, tokens := w.connect(t)
	got := w.env.GateCheck(t, "kb.test", bearer(tokens))
	if got.StatusCode != http.StatusOK || got.Header.Get("X-Auth-Request-User") != w.admin.UserID(t, w.ada.Email) || got.Header.Get("X-Auth-Request-Roles") != "user" {
		t.Fatalf("gate = %d %v", got.StatusCode, got.Header)
	}
}

func TestTheGatePointsAToolWithoutATokenToKnowledgesMetadata(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	got := w.env.GateCheck(t, "kb.test", "")
	if got.StatusCode != http.StatusUnauthorized || !strings.Contains(got.Header.Get("WWW-Authenticate"), `resource_metadata="https://`+itest.Host+`/.well-known/oauth-protected-resource/kb.test"`) {
		t.Fatalf("gate = %d %v", got.StatusCode, got.Header)
	}
}

func TestATokenFromTheRefreshTokenWorksAndTheSpentOneDoesNot(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	tool, first := w.connect(t)
	status, next := w.refresh(t, tool, first)
	if status != http.StatusOK || w.env.GateCheck(t, "kb.test", bearer(next)).StatusCode != http.StatusOK {
		t.Fatalf("refresh = %d %v", status, next)
	}
	if status, _ := w.refresh(t, tool, first); status != http.StatusBadRequest {
		t.Fatalf("the spent refresh token answered %d", status)
	}
}

func TestARefreshTokenSpentTwiceDisconnectsTheTool(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	tool, first := w.connect(t)
	_, next := w.refresh(t, tool, first)
	w.refresh(t, tool, first)
	if got := w.env.GateCheck(t, "kb.test", bearer(next)); got.StatusCode != http.StatusUnauthorized {
		t.Fatalf("gate = %d", got.StatusCode)
	}
}

func TestAdaDisconnectsAToolAndItsTokenStopsAtOnce(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	_, tokens := w.connect(t)
	w.env.GateCheck(t, "kb.test", bearer(tokens)) // cached
	var list struct{ Apps []struct{ ID string } }
	w.ada.Call(t, http.MethodGet, "/api/v1/profile/connected-apps", nil, http.StatusOK, &list)
	w.ada.Call(t, http.MethodDelete, "/api/v1/profile/connected-apps/"+list.Apps[0].ID, nil, http.StatusNoContent, nil)
	if got := w.env.GateCheck(t, "kb.test", bearer(tokens)); got.StatusCode != http.StatusUnauthorized {
		t.Fatalf("gate = %d", got.StatusCode)
	}
}

func TestTurningStepUpOnDisconnectsKnowledgesTools(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	_, tokens := w.connect(t)
	w.admin.Call(t, http.MethodPut, "/api/v1/apps/kb/step-up", map[string]int{"minutes": 5}, http.StatusNoContent, nil)
	var list struct{ Apps []any }
	w.ada.Call(t, http.MethodGet, "/api/v1/profile/connected-apps", nil, http.StatusOK, &list)
	if len(list.Apps) != 0 || w.env.GateCheck(t, "kb.test", bearer(tokens)).StatusCode != http.StatusUnauthorized {
		t.Fatalf("apps %v still connected", list.Apps)
	}
}

func TestDeletingAdaDisconnectsHerTools(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	_, tokens := w.connect(t)
	w.env.GateCheck(t, "kb.test", bearer(tokens)) // cached
	w.admin.Call(t, http.MethodDelete, "/api/v1/access/users/"+w.admin.UserID(t, w.ada.Email), nil, http.StatusNoContent, nil)
	if got := w.env.GateCheck(t, "kb.test", bearer(tokens)); got.StatusCode != http.StatusUnauthorized {
		t.Fatalf("gate = %d", got.StatusCode)
	}
}

func TestAToolAskingForAnAppThatTakesNoToolsIsTurnedBack(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.admin.Call(t, http.MethodPut, "/api/v1/apps/kb/mcp", map[string]bool{"on": false}, http.StatusNoContent, nil)
	back, _ := url.Parse(w.ada.Follow(t, authorize(w.register(t), "https://kb.test/mcp")))
	if back.Query().Get("error") != "invalid_target" {
		t.Fatalf("authorize went to %s", back)
	}
}

func TestConnectingAndDisconnectingAreAuditEvents(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.connect(t)
	var list struct{ Apps []struct{ ID string } }
	w.ada.Call(t, http.MethodGet, "/api/v1/profile/connected-apps", nil, http.StatusOK, &list)
	w.ada.Call(t, http.MethodDelete, "/api/v1/profile/connected-apps/"+list.Apps[0].ID, nil, http.StatusNoContent, nil)
	var page struct {
		Events []struct{ Action, AppName string }
	}
	w.admin.Call(t, http.MethodGet, "/api/v1/audit?kind=tokens", nil, http.StatusOK, &page)
	if len(page.Events) != 2 || page.Events[0].Action != "connected-app-revoked" || page.Events[1].Action != "connected-app-authorized" || page.Events[1].AppName != "Claude Code" {
		t.Fatalf("events = %+v", page.Events)
	}
}
