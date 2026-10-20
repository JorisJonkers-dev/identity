package oidc

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

var tokenIssued = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

// personalTokens knows one live token for Ada, and fails as a test sets it to.
type personalTokens struct {
	id, user uuid.UUID
	err      error
}

func (p *personalTokens) Resolve(_ context.Context, secret string) (uuid.UUID, uuid.UUID, error) {
	if secret != "idpat_known" {
		return uuid.Nil, uuid.Nil, app.ErrNotFound
	}
	return p.id, p.user, nil
}

func (p *personalTokens) Check(_ context.Context, id uuid.UUID, clientID string) (app.PersonalToken, error) {
	return app.PersonalToken{
		ID: id, UserID: p.user, ClientID: clientID, CreatedAt: tokenIssued, ExpiresAt: tokenIssued.Add(30 * 24 * time.Hour), Roles: []string{"viewer"},
	}, p.err
}

func storageWithTokens() (*storage, *personalTokens, *accounts) {
	s, _, a := newStorage()
	p := &personalTokens{id: uuid.New(), user: a.ada}
	s.svc.SetPersonalTokens(p)
	return s, p, a
}

func TestAPersonalTokenDecryptsToItsIDAndUser(t *testing.T) {
	s, p, _ := storageWithTokens()
	got, err := newCrypto([32]byte{1}, s.svc.ResolvePersonalToken).Decrypt("idpat_known")
	if err != nil || got != "pat-"+p.id.String()+":"+p.user.String() {
		t.Fatalf("Decrypt = %q, %v", got, err)
	}
}

func TestATokenTheProviderSealedStillDecrypts(t *testing.T) {
	s, _, _ := storageWithTokens()
	c := newCrypto([32]byte{1}, s.svc.ResolvePersonalToken)
	sealed, err := c.Encrypt("request-id")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.Decrypt(sealed); err != nil || got != "request-id" {
		t.Fatalf("Decrypt = %q, %v", got, err)
	}
}

func TestAnUnknownTokenDecryptsToNothing(t *testing.T) {
	s, _, _ := storageWithTokens()
	if _, err := (tokenDecrypter{resolve: s.svc.ResolvePersonalToken}).Decrypt("idpat_unknown"); !errors.Is(err, errNotPersonalToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestIntrospectingAPersonalTokenAnswersItsUserAndRolesForTheClient(t *testing.T) {
	s, p, a := storageWithTokens()
	resp := new(oidc.IntrospectionResponse)
	if err := s.SetIntrospectionFromToken(context.Background(), resp, "pat-"+p.id.String(), a.ada.String(), "spa"); err != nil {
		t.Fatal(err)
	}
	if resp.Subject != a.ada.String() || resp.ClientID != "spa" || !slices.Equal(resp.Audience, oidc.Audience{"spa"}) || resp.TokenType != "Bearer" {
		t.Fatalf("response = %+v", resp)
	}
	if !resp.IssuedAt.AsTime().Equal(tokenIssued) || !resp.Expiration.AsTime().Equal(tokenIssued.Add(30*24*time.Hour)) || resp.Claims["email"] != "ada@example.org" {
		t.Fatalf("response = %+v", resp)
	}
	if roles, _ := resp.Claims["roles"].([]string); !slices.Equal(roles, []string{"viewer"}) {
		t.Fatalf("roles = %v", resp.Claims["roles"])
	}
}

func TestAPersonalTokensIntrospectionSaysItCameByPersonalToken(t *testing.T) {
	s, p, a := storageWithTokens()
	resp := new(oidc.IntrospectionResponse)

	_ = s.SetIntrospectionFromToken(context.Background(), resp, "pat-"+p.id.String(), a.ada.String(), "spa")

	if resp.Claims["via"] != "personal-token" || resp.Claims["kind"] != "user" || resp.Claims["sid"] != nil || resp.Claims["act"] != nil {
		t.Fatalf("claims = %v", resp.Claims)
	}
}

func TestAnInactivePersonalTokenIsNotIntrospected(t *testing.T) {
	s, p, a := storageWithTokens()
	p.err = app.ErrInactive
	if err := s.SetIntrospectionFromToken(context.Background(), new(oidc.IntrospectionResponse), "pat-"+p.id.String(), a.ada.String(), "spa"); err == nil {
		t.Fatal("an inactive token was introspected")
	}
}

func TestAPersonalTokenWhoseUserCannotBeReadIsNotIntrospected(t *testing.T) {
	s, p, a := storageWithTokens()
	a.failClaims = errBoom
	if err := s.SetIntrospectionFromToken(context.Background(), new(oidc.IntrospectionResponse), "pat-"+p.id.String(), a.ada.String(), "spa"); err == nil {
		t.Fatal("a token was introspected without its User")
	}
}

func TestUserinfoRefusesAPersonalToken(t *testing.T) {
	s, p, a := storageWithTokens()
	if err := s.SetUserinfoFromToken(context.Background(), new(oidc.UserInfo), "pat-"+p.id.String(), a.ada.String(), ""); err == nil {
		t.Fatal("userinfo answered a Personal Token")
	}
}
