//go:build integration

// Package tokens_test checks Personal Tokens the way a User, an Admin and a Client's API reach
// them: over HTTP, against the real routes, Postgres, Vault and mail (docs/testing-standards.md).
package tokens_test

import (
	"crypto/sha256"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

const grafanaSecret = "grafana-client-secret"

func secretHash(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

var (
	grafana = signindomain.Client{
		ID: "grafana", SignIn: signindomain.Native, Host: "grafana.test", Label: "Grafana", Roles: []string{"user", "viewer"},
		Credential: signindomain.Secret, SecretHash: secretHash(grafanaSecret), RedirectURIs: []string{"https://grafana.test/callback"},
	}
	wiki = signindomain.Client{
		ID: "wiki", SignIn: signindomain.Native, Host: "wiki.test", Label: "Wiki", Roles: []string{"user"},
		Credential: signindomain.Secret, SecretHash: secretHash("wiki-secret"), RedirectURIs: []string{"https://wiki.test/callback"},
	}
)

type token struct {
	ID, Name, State      string
	Client               struct{ ID, Label string }
	CreatedAt, ExpiresAt time.Time
}

type minted struct {
	Token  token
	Secret string
}

type problem struct{ Detail string }

// world is identity with Grafana and Wiki, an Admin, and Ada, whose Group holds viewer on Grafana.
type world struct {
	env          *itest.Env
	admin, ada   *itest.Browser
	adaID, group string
}

func start(t *testing.T) world {
	t.Helper()
	env := itest.Start(t, grafana, wiki)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	w := world{env: env, admin: admin, ada: ada, adaID: admin.UserID(t, ada.Email)}
	var g struct{ ID string }
	admin.Call(t, http.MethodPost, "/api/v1/access/groups", map[string]string{"name": "Viewers"}, http.StatusCreated, &g)
	w.group = g.ID
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+g.ID+"/members/"+w.adaID, nil, http.StatusNoContent, nil)
	admin.Call(t, http.MethodPut, "/api/v1/access/groups/"+g.ID+"/roles/grafana/viewer", nil, http.StatusNoContent, nil)
	return w
}

func (w world) mint(t *testing.T) minted {
	t.Helper()
	var m minted
	w.ada.Call(t, http.MethodPost, "/api/v1/profile/tokens", map[string]any{"name": "grafana-cli", "clientId": "grafana", "days": 30}, http.StatusCreated, &m)
	return m
}

func TestAUserMintsATokenForAnAppTheyHaveARoleIn(t *testing.T) {
	t.Parallel()
	m := start(t).mint(t)
	if !strings.HasPrefix(m.Secret, "idpat_") || m.Token.Name != "grafana-cli" || m.Token.Client.Label != "Grafana" || m.Token.State != "active" {
		t.Fatalf("minted = %+v", m)
	}
	if days := m.Token.ExpiresAt.Sub(m.Token.CreatedAt).Hours() / 24; days != 30 {
		t.Fatalf("lasts %v days", days)
	}
}

func TestAUserSeesTheirTokensWithoutTheirSecretAndTheAppsTheyMayMintFor(t *testing.T) {
	t.Parallel()
	w := start(t)
	m := w.mint(t)
	var mine struct {
		Tokens  []map[string]any
		Clients []struct{ ID, Label string }
	}
	w.ada.Call(t, http.MethodGet, "/api/v1/profile/tokens", nil, http.StatusOK, &mine)
	if len(mine.Tokens) != 1 || mine.Tokens[0]["id"] != m.Token.ID || mine.Tokens[0]["secret"] != nil || !slices.Equal(mine.Tokens[0]["roles"].([]any), []any{"viewer"}) {
		t.Fatalf("tokens = %+v", mine.Tokens)
	}
	if len(mine.Clients) != 1 || mine.Clients[0].ID != "grafana" {
		t.Fatalf("clients = %+v", mine.Clients)
	}
}

func TestATokenForAnAppTheUserHasNoRoleInIsRefused(t *testing.T) {
	t.Parallel()
	w := start(t)
	var p problem
	w.ada.Call(t, http.MethodPost, "/api/v1/profile/tokens", map[string]any{"name": "wiki-bot", "clientId": "wiki", "days": 90}, http.StatusUnprocessableEntity, &p)
	if p.Detail != "You have no role in that app, so a token for it could open nothing." {
		t.Fatalf("detail = %q", p.Detail)
	}
}

func TestATokenLastsOnlyThirtyNinetyOrThreeHundredSixtyFiveDays(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.ada.Call(t, http.MethodPost, "/api/v1/profile/tokens", map[string]any{"name": "forever", "clientId": "grafana", "days": 3650}, http.StatusBadRequest, nil)
}

func TestTheAppTheTokenIsForSeesItsUserAndTheRolesTheyHoldNow(t *testing.T) {
	t.Parallel()
	w := start(t)
	m := w.mint(t)
	got := w.env.Introspect(t, "grafana", grafanaSecret, m.Secret)
	if got["active"] != true || got["sub"] != w.adaID || got["client_id"] != "grafana" || got["email"] != w.ada.Email {
		t.Fatalf("introspection = %+v", got)
	}
	if roles, _ := got["roles"].([]any); !slices.Equal(roles, []any{"viewer"}) {
		t.Fatalf("roles = %v", got["roles"])
	}
}

func TestAnotherAppSeesTheTokenAsInactive(t *testing.T) {
	t.Parallel()
	w := start(t)
	m := w.mint(t)
	if got := w.env.Introspect(t, "wiki", "wiki-secret", m.Secret); got["active"] != false || got["sub"] != nil {
		t.Fatalf("introspection = %+v", got)
	}
}

func TestATokenNoOneMintedIsInactive(t *testing.T) {
	t.Parallel()
	w := start(t)
	if got := w.env.Introspect(t, "grafana", grafanaSecret, "idpat_made-up"); got["active"] != false {
		t.Fatalf("introspection = %+v", got)
	}
}

func TestARevokedTokenIsInactive(t *testing.T) {
	t.Parallel()
	w := start(t)
	m := w.mint(t)
	w.ada.Call(t, http.MethodDelete, "/api/v1/profile/tokens/"+m.Token.ID, nil, http.StatusNoContent, nil)
	if got := w.env.Introspect(t, "grafana", grafanaSecret, m.Secret); got["active"] != false {
		t.Fatalf("introspection = %+v", got)
	}
}

func TestTakingTheRoleAwayMakesTheTokenInactiveAtOnce(t *testing.T) {
	t.Parallel()
	w := start(t)
	m := w.mint(t)
	w.admin.Call(t, http.MethodDelete, "/api/v1/access/groups/"+w.group+"/roles/grafana/viewer", nil, http.StatusNoContent, nil)
	if got := w.env.Introspect(t, "grafana", grafanaSecret, m.Secret); got["active"] != false {
		t.Fatalf("introspection = %+v", got)
	}
}

func TestOnlyTheTokensHashIsStored(t *testing.T) {
	t.Parallel()
	w := start(t)
	m := w.mint(t)
	row, err := w.env.Queries().PersonalTokenByHash(t.Context(), secretHash(m.Secret))
	if err != nil || row.ID.String() != m.Token.ID {
		t.Fatalf("by hash = %+v, %v", row, err)
	}
}

func TestAUserCannotRevokeSomeoneElsesToken(t *testing.T) {
	t.Parallel()
	w := start(t)
	m := w.mint(t)
	w.admin.Call(t, http.MethodDelete, "/api/v1/profile/tokens/"+m.Token.ID, nil, http.StatusNotFound, nil)
	if got := w.env.Introspect(t, "grafana", grafanaSecret, m.Secret); got["active"] != true {
		t.Fatalf("introspection = %+v", got)
	}
}

func TestAnAdminSeesAndRevokesAUsersToken(t *testing.T) {
	t.Parallel()
	w := start(t)
	m := w.mint(t)
	var list struct{ Tokens []token }
	w.admin.Call(t, http.MethodGet, "/api/v1/access/users/"+w.adaID+"/tokens", nil, http.StatusOK, &list)
	if len(list.Tokens) != 1 || list.Tokens[0].ID != m.Token.ID {
		t.Fatalf("tokens = %+v", list.Tokens)
	}
	w.admin.Call(t, http.MethodDelete, "/api/v1/access/users/"+w.adaID+"/tokens/"+m.Token.ID, nil, http.StatusNoContent, nil)
	w.admin.Call(t, http.MethodGet, "/api/v1/access/users/"+w.adaID+"/tokens", nil, http.StatusOK, &list)
	if list.Tokens[0].State != "revoked" {
		t.Fatalf("state = %q", list.Tokens[0].State)
	}
}

func TestOnlyAnAdminSeesSomeoneElsesTokens(t *testing.T) {
	t.Parallel()
	w := start(t)
	w.ada.Call(t, http.MethodGet, "/api/v1/access/users/"+w.adaID+"/tokens", nil, http.StatusForbidden, nil)
}

func TestMintingAndRevokingATokenAreAuditEvents(t *testing.T) {
	t.Parallel()
	w := start(t)
	m := w.mint(t)
	w.ada.Call(t, http.MethodDelete, "/api/v1/profile/tokens/"+m.Token.ID, nil, http.StatusNoContent, nil)
	var page struct {
		Events []struct {
			Action string
			Client *struct{ ID string }
		}
	}
	w.admin.Call(t, http.MethodGet, "/api/v1/audit?"+url.Values{"kind": {"tokens"}}.Encode(), nil, http.StatusOK, &page)
	var actions []string
	for _, e := range page.Events {
		if e.Client == nil || e.Client.ID != "grafana" {
			t.Fatalf("event without its Client: %+v", e)
		}
		actions = append(actions, e.Action)
	}
	if !slices.Equal(actions, []string{"token-revoked", "token-minted"}) {
		t.Fatalf("actions = %v", actions)
	}
}

func TestAPersonalTokenGetsNoUserinfo(t *testing.T) {
	t.Parallel()
	w := start(t)
	m := w.mint(t)
	if status := w.env.Userinfo(t, m.Secret); status == http.StatusOK {
		t.Fatal("userinfo answered a Personal Token")
	}
}
