package web_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
	"github.com/JorisJonkers-dev/identity/internal/tokens/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/tokens/app"
	"github.com/JorisJonkers-dev/identity/internal/tokens/domain"
)

var (
	errBoom = errors.New("boom")
	t0      = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	ada     = uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
	root    = uuid.MustParse("00000000-0000-4000-8000-0000000000a3")
	tokenID = uuid.MustParse("00000000-0000-4000-8000-0000000000c1")
	used    = t0.Add(time.Hour)
	cli     = domain.Token{ID: tokenID, UserID: ada, ClientID: "grafana", Name: "grafana-cli", CreatedAt: t0, ExpiresAt: t0.Add(30 * 24 * time.Hour), LastUsedAt: &used}
)

// useCases answers with what a test sets and records what it was asked.
type useCases struct {
	// tokens is what Mine lists; cli when nil.
	tokens                     []domain.Token
	err, mintableErr, rolesErr error
	asked                      []any
	actor                      app.Actor
}

func (u *useCases) Mint(_ context.Context, userID uuid.UUID, name, clientID string, days int) (app.Minted, error) {
	u.asked = append(u.asked, userID, name, clientID, days)
	return app.Minted{Secret: "idpat_secret", Token: cli}, u.err
}

func (u *useCases) Mine(_ context.Context, userID uuid.UUID) ([]domain.Token, error) {
	u.asked = append(u.asked, userID)
	if u.tokens != nil {
		return u.tokens, u.err
	}
	return []domain.Token{cli}, u.err
}

func (u *useCases) Mintable(context.Context, uuid.UUID) ([]app.Client, error) {
	return []app.Client{{ID: "grafana", Label: "Grafana"}}, u.mintableErr
}

func (u *useCases) RevokeMine(_ context.Context, userID, id uuid.UUID) error {
	u.asked = append(u.asked, userID, id)
	return u.err
}

func (u *useCases) Of(_ context.Context, actor app.Actor, userID uuid.UUID) ([]domain.Token, error) {
	u.actor, u.asked = actor, append(u.asked, userID)
	return []domain.Token{cli}, u.err
}

func (u *useCases) RevokeAs(_ context.Context, actor app.Actor, userID, id uuid.UUID) error {
	u.actor, u.asked = actor, append(u.asked, userID, id)
	return u.err
}

func (*useCases) Label(_ context.Context, clientID string) string { return "Label of " + clientID }

func (*useCases) State(domain.Token) domain.State { return domain.Expiring }

func (u *useCases) RolesOf(context.Context, domain.Token) ([]string, error) {
	return []string{"viewer"}, u.rolesErr
}

func as(id uuid.UUID) context.Context {
	return principal.With(context.Background(), principal.Principal{UserID: id, TwoFactor: true})
}

var wantToken = oas.PersonalToken{
	ID: oas.ID(tokenID), Name: "grafana-cli", Client: oas.TokenClient{ID: "grafana", Label: "Label of grafana"},
	Roles: []oas.AppRole{"viewer"}, CreatedAt: t0, ExpiresAt: t0.Add(30 * 24 * time.Hour), LastUsedAt: oas.NewOptDateTime(used),
	State: oas.PersonalTokenStateExpiring,
}

// same compares tokens, whose roles a struct comparison cannot.
func same(a, b oas.PersonalToken) bool {
	return slices.Equal(a.Roles, b.Roles) && a.ID == b.ID && a.Name == b.Name && a.Client == b.Client &&
		a.CreatedAt.Equal(b.CreatedAt) && a.ExpiresAt.Equal(b.ExpiresAt) && a.LastUsedAt == b.LastUsedAt && a.State == b.State
}

func TestAUserListsTheirTokensAndTheAppsTheyMayMintFor(t *testing.T) {
	t.Parallel()
	uc := &useCases{}
	res, err := web.New(uc).ListMyTokens(as(ada))
	mine, _ := res.(*oas.MyTokens)
	if err != nil || len(mine.Tokens) != 1 || !same(mine.Tokens[0], wantToken) || len(mine.Clients) != 1 || mine.Clients[0] != (oas.TokenClient{ID: "grafana", Label: "Grafana"}) {
		t.Fatalf("ListMyTokens = %+v, %v", res, err)
	}
	if uc.asked[0] != ada {
		t.Fatalf("asked for %v", uc.asked)
	}
}

func TestATokenNeverUsedHasNoLastUse(t *testing.T) {
	t.Parallel()
	never := cli
	never.LastUsedAt = nil
	res, _ := web.New(&useCases{tokens: []domain.Token{never}}).ListMyTokens(as(ada))
	if res.(*oas.MyTokens).Tokens[0].LastUsedAt.Set {
		t.Fatal("a token never used has a last use")
	}
}

func TestListingTokensFailsWithWhatItReads(t *testing.T) {
	t.Parallel()
	for name, uc := range map[string]*useCases{"the tokens": {err: errBoom}, "the apps": {mintableErr: errBoom}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := web.New(uc).ListMyTokens(as(ada)); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAUserMintsATokenAndGetsItsSecretOnce(t *testing.T) {
	t.Parallel()
	uc := &useCases{}
	res, err := web.New(uc).MintToken(as(ada), &oas.TokenDraft{Name: "grafana-cli", ClientId: "grafana", Days: 90})
	if m, ok := res.(*oas.MintedToken); err != nil || !ok || m.Secret != "idpat_secret" || !same(m.Token, wantToken) {
		t.Fatalf("MintToken = %+v, %v", res, err)
	}
	if want := []any{ada, "grafana-cli", "grafana", 90}; len(uc.asked) != 4 || uc.asked[0] != want[0] || uc.asked[3] != want[3] {
		t.Fatalf("asked = %v", uc.asked)
	}
}

func TestAUserRevokesTheirToken(t *testing.T) {
	t.Parallel()
	uc := &useCases{}
	res, err := web.New(uc).RevokeMyToken(as(ada), oas.RevokeMyTokenParams{PatId: oas.ID(tokenID)})
	if _, ok := res.(*oas.RevokeMyTokenNoContent); err != nil || !ok || uc.asked[0] != ada || uc.asked[1] != tokenID {
		t.Fatalf("RevokeMyToken = %+v, %v, asked %v", res, err, uc.asked)
	}
}

func TestAnAdminListsAndRevokesAUsersTokensAsThemself(t *testing.T) {
	t.Parallel()
	uc := &useCases{}
	h := web.New(uc)
	list, err := h.ListUserTokens(as(root), oas.ListUserTokensParams{UserId: oas.ID(ada)})
	if l, ok := list.(*oas.TokenList); err != nil || !ok || len(l.Tokens) != 1 || uc.actor != (app.Actor{ID: root, TwoFactor: true}) {
		t.Fatalf("ListUserTokens = %+v, %v", list, err)
	}
	res, err := h.RevokeUserToken(as(root), oas.RevokeUserTokenParams{UserId: oas.ID(ada), PatId: oas.ID(tokenID)})
	if _, ok := res.(*oas.RevokeUserTokenNoContent); err != nil || !ok {
		t.Fatalf("RevokeUserToken = %+v, %v", res, err)
	}
}

func TestARefusalSaysWhy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err    error
		status int
		detail string
	}{
		{domain.ErrInvalidName, 422, "Enter a token name of 1 to 64 characters."},
		{domain.ErrInvalidLifetime, 422, "Choose 30, 90 or 365 days."},
		{app.ErrUnknownClient, 422, "That app is not registered."},
		{app.ErrNoAccess, 422, "You have no role in that app, so a token for it could open nothing."},
		{app.ErrNotFound, 404, "That token no longer exists. Reload the page."},
		{app.ErrNotAdmin, 403, "Only Admins manage other people's tokens."},
		{app.ErrTwoFactorProofNeeded, 403, "This page needs two-factor. If you have a passkey or authenticator app, sign out and sign in again with it. If you have neither, add a two-factor method on the Security page first, then sign out and sign in again."},
	} {
		for name, call := range map[string]func(h *web.Tokens) (any, error){
			"minting":            func(h *web.Tokens) (any, error) { return h.MintToken(as(ada), &oas.TokenDraft{}) },
			"revoking one's own": func(h *web.Tokens) (any, error) { return h.RevokeMyToken(as(ada), oas.RevokeMyTokenParams{}) },
			"listing a User's":   func(h *web.Tokens) (any, error) { return h.ListUserTokens(as(root), oas.ListUserTokensParams{}) },
			"revoking a User's":  func(h *web.Tokens) (any, error) { return h.RevokeUserToken(as(root), oas.RevokeUserTokenParams{}) },
		} {
			t.Run(tc.err.Error()+" when "+name, func(t *testing.T) {
				t.Parallel()
				res, err := call(web.New(&useCases{err: tc.err}))
				p, ok := res.(*oas.ProblemStatusCode)
				if err != nil || !ok || p.StatusCode != tc.status || p.Response.Detail.Value != tc.detail {
					t.Fatalf("answer = %+v, %v", res, err)
				}
			})
		}
	}
}

func TestAnswersFailWhenATokensRolesCannotBeRead(t *testing.T) {
	t.Parallel()
	for name, call := range map[string]func(h *web.Tokens) error{
		"listing one's own": func(h *web.Tokens) error { _, err := h.ListMyTokens(as(ada)); return err },
		"minting":           func(h *web.Tokens) error { _, err := h.MintToken(as(ada), &oas.TokenDraft{}); return err },
		"listing a User's": func(h *web.Tokens) error {
			_, err := h.ListUserTokens(as(root), oas.ListUserTokensParams{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := call(web.New(&useCases{rolesErr: errBoom})); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAFailureTheContractDoesNotNameIsPassedOn(t *testing.T) {
	t.Parallel()
	if _, err := web.New(&useCases{err: errBoom}).MintToken(as(ada), &oas.TokenDraft{}); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestATokenForAStepUpAppSaysWhyItCannotBeMade(t *testing.T) {
	t.Parallel()
	res, err := web.New(&useCases{err: app.ErrStepUpClient}).MintToken(as(uuid.New()), &oas.TokenDraft{Name: "cli", ClientId: "grafana", Days: 30})
	p, ok := res.(*oas.ProblemStatusCode)
	if err != nil || !ok || p.StatusCode != 422 || p.Response.Detail.Value != "Label of grafana asks for two-factor each time it opens, so a token cannot open it." {
		t.Fatalf("MintToken = %+v, %v", res, err)
	}
}
