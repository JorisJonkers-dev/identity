package oidc

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

func newStorage() (*storage, *requests, *accounts) {
	r, a := newRequests(), &accounts{ada: uuid.New(), sessionID: uuid.New()}
	svc := app.New(r, testClients, a, newAccess(), noLogouts{}, discard{}, time.Now, rand.Reader)
	svc.SetClientSecrets(testSecrets)
	return &storage{svc: svc, ring: NewRing(testKey()), logger: slog.New(slog.DiscardHandler), now: time.Now}, r, a
}

func wantOIDC(t *testing.T, err error, kind string) {
	t.Helper()
	var oe *oidc.Error
	if !errors.As(err, &oe) || string(oe.ErrorType) != kind {
		t.Fatalf("err = %v, want %s", err, kind)
	}
}

// tokenRequest is an op.IDTokenRequest that is not an authorization request.
type tokenRequest struct{ subject string }

func (r tokenRequest) GetAMR() []string       { return nil }
func (r tokenRequest) GetAudience() []string  { return nil }
func (r tokenRequest) GetAuthTime() time.Time { return time.Time{} }
func (r tokenRequest) GetClientID() string    { return "web" }
func (r tokenRequest) GetScopes() []string    { return nil }
func (r tokenRequest) GetSubject() string     { return r.subject }

func TestRequestStorageFailures(t *testing.T) {
	ctx := context.Background()
	s, r, _ := newStorage()
	for name, err := range map[string]error{
		"AuthRequestByID":   func() error { _, err := s.AuthRequestByID(ctx, "not-a-uuid"); return err }(),
		"unknown request":   func() error { _, err := s.AuthRequestByID(ctx, uuid.NewString()); return err }(),
		"SaveAuthCode":      s.SaveAuthCode(ctx, "not-a-uuid", "c"),
		"DeleteAuthRequest": s.DeleteAuthRequest(ctx, "not-a-uuid"),
		"TerminateSession":  s.TerminateSession(ctx, "not-a-uuid", "web"),
	} {
		t.Run(name, func(t *testing.T) { wantOIDC(t, err, "invalid_request") })
	}
	r.fail = errBoom
	for name, err := range map[string]error{
		"CreateAuthRequest":   func() error { _, err := s.CreateAuthRequest(ctx, &oidc.AuthRequest{}, ""); return err }(),
		"SaveAuthCode":        s.SaveAuthCode(ctx, uuid.NewString(), "c"),
		"DeleteAuthRequest":   s.DeleteAuthRequest(ctx, uuid.NewString()),
		"AuthRequestByCode":   func() error { _, err := s.AuthRequestByCode(ctx, "c"); return err }(),
		"GetClientByClientID": func() error { _, err := s.GetClientByClientID(ctx, "broken"); return err }(),
	} {
		t.Run(name, func(t *testing.T) { wantOIDC(t, err, "server_error") })
	}
}

func TestWhatIsNotOffered(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newStorage()
	if _, _, _, err := s.CreateAccessAndRefreshTokens(ctx, nil, ""); err == nil {
		t.Error("refresh tokens were issued")
	}
	if _, err := s.TokenRequestByRefreshToken(ctx, "r"); !errors.Is(err, op.ErrInvalidRefreshToken) {
		t.Errorf("TokenRequestByRefreshToken = %v", err)
	}
	if _, _, err := s.GetRefreshTokenInfo(ctx, "web", "r"); !errors.Is(err, op.ErrInvalidRefreshToken) {
		t.Errorf("GetRefreshTokenInfo = %v", err)
	}
	if err := s.RevokeToken(ctx, "t", "u", "web"); err == nil {
		t.Error("a token was revoked")
	}
	if _, err := s.GetKeyByIDAndClientID(ctx, "k", "web"); err == nil {
		t.Error("a JWT profile key was found")
	}
	if _, err := s.ValidateJWTProfileScopes(ctx, "u", nil); err == nil {
		t.Error("JWT profile scopes validated")
	}
	if err := s.Health(ctx); err != nil {
		t.Error(err)
	}
}

func TestSignOutFailureIsAServerError(t *testing.T) {
	s, _, a := newStorage()
	a.failOut = errBoom
	wantOIDC(t, s.TerminateSession(context.Background(), a.ada.String(), "web"), "server_error")
}

func TestUserinfoForAnEmailScopeHasTheEmailAndNoSession(t *testing.T) {
	s, _, a := newStorage()
	info := new(oidc.UserInfo)
	if err := s.SetUserinfoFromRequest(context.Background(), info, tokenRequest{a.ada.String()}, []string{"openid", "email"}); err != nil {
		t.Fatal(err)
	}
	if info.Email != "ada@example.org" || info.Name != "" || info.Claims["sid"] != nil {
		t.Fatalf("userinfo = %+v", info)
	}
}

func TestUserinfoForAProfileScopeHasTheNameAndTheSession(t *testing.T) {
	s, _, a := newStorage()
	info := new(oidc.UserInfo)
	pending := authRequest{domain.AuthRequest{ClientID: "spa", SignIn: &domain.SignIn{UserID: a.ada, SessionID: a.sessionID}}}
	if err := s.SetUserinfoFromRequest(context.Background(), info, pending, []string{"profile"}); err != nil {
		t.Fatal(err)
	}
	if info.Email != "" || info.Name != "Ada" || info.Claims["sid"] != a.sessionID.String() {
		t.Fatalf("userinfo = %+v", info)
	}
}

func TestUserinfoForAnUnfinishedSignInIsRefused(t *testing.T) {
	s, _, _ := newStorage()
	wantOIDC(t, s.SetUserinfoFromRequest(context.Background(), new(oidc.UserInfo), authRequest{domain.AuthRequest{}}, nil), "invalid_request")
}

func TestIntrospectionNamesTheUserATokenWasIssuedTo(t *testing.T) {
	s, _, a := newStorage()
	resp := new(oidc.IntrospectionResponse)
	if err := s.SetIntrospectionFromToken(context.Background(), resp, "t", a.ada.String(), "web"); err != nil || resp.Email != "ada@example.org" || resp.Subject != a.ada.String() {
		t.Fatalf("introspection = %+v, %v", resp, err)
	}
}

func TestIntrospectionOfAnUnknownUserIsRefused(t *testing.T) {
	s, _, _ := newStorage()
	wantOIDC(t, s.SetIntrospectionFromToken(context.Background(), new(oidc.IntrospectionResponse), "t", uuid.NewString(), "web"), "invalid_request")
}

func TestUserinfoFailsAsAServerErrorWhenTheClaimsCannotBeRead(t *testing.T) {
	s, _, a := newStorage()
	a.failClaims = errBoom
	wantOIDC(t, s.SetUserinfoFromToken(context.Background(), new(oidc.UserInfo), "t", a.ada.String(), "web"), "server_error")
}

func TestRolesInTokens(t *testing.T) {
	ctx := context.Background()
	s, _, a := newStorage()
	got, err := s.GetPrivateClaimsFromScopes(ctx, a.ada.String(), "web", nil)
	if err != nil || !slices.Equal(got["roles"].([]string), []string{"user"}) || len(got) != 1 {
		t.Fatalf("access token claims = %v, %v", got, err)
	}
	info := new(oidc.UserInfo)
	if err := s.SetUserinfoFromRequest(ctx, info, tokenRequest{a.ada.String()}, []string{"openid"}); err != nil ||
		!slices.Equal(info.Claims["roles"].([]string), []string{"user"}) {
		t.Fatalf("ID token claims = %+v, %v", info.Claims, err)
	}
	_, err = s.GetPrivateClaimsFromScopes(ctx, "not-a-uuid", "web", nil)
	wantOIDC(t, err, "invalid_request")
	_, err = s.GetPrivateClaimsFromScopes(ctx, a.ada.String(), "removed", nil)
	wantOIDC(t, err, "invalid_request")
	s.svc = app.New(newRequests(), testClients, a, &access{fail: errBoom}, noLogouts{}, discard{}, time.Now, rand.Reader)
	s.svc.SetClientSecrets(testSecrets)
	_, err = s.GetPrivateClaimsFromScopes(ctx, a.ada.String(), "web", nil)
	wantOIDC(t, err, "server_error")
	wantOIDC(t, s.SetUserinfoFromRequest(ctx, new(oidc.UserInfo), tokenRequest{a.ada.String()}, nil), "server_error")
}

func TestAWrapperBeforeSignInVouchesForNobody(t *testing.T) {
	r := authRequest{domain.AuthRequest{}}
	if r.Done() || r.GetSubject() != "" || r.GetAMR() != nil || !r.GetAuthTime().IsZero() || r.GetCodeChallenge() != nil || r.GetACR() != "" {
		t.Fatalf("an unfinished request vouches for someone: %+v", r)
	}
}

func TestAClientRunsWithTheProvidersDefaults(t *testing.T) {
	c := client{testClients["spa"]}
	if len(c.PostLogoutRedirectURIs()) != 1 || c.DevMode() || c.IsScopeAllowed("admin") || c.ClockSkew() != 0 {
		t.Fatalf("client settings = %+v", c)
	}
}

func TestAnHTTPIssuerNeedsInsecure(t *testing.T) {
	s, _, _ := newStorage()
	if _, err := New(Config{Issuer: "http://auth.test"}, s.svc, s.ring, s.logger, time.Now); err == nil {
		t.Fatal("an http Issuer was accepted without Insecure")
	}
}

func TestAMRSaysHowTheSessionWasProved(t *testing.T) {
	for _, tc := range []struct {
		methods []string
		want    []string
	}{
		{nil, []string{"pwd"}},
		{[]string{"pwd"}, []string{"pwd"}},
		{[]string{"pwd", "otp"}, []string{"pwd", "otp", "mfa"}},
	} {
		r := authRequest{domain.AuthRequest{SignIn: &domain.SignIn{Methods: tc.methods}}}
		if got := r.GetAMR(); !slices.Equal(got, tc.want) {
			t.Errorf("GetAMR(%v) = %v, want %v", tc.methods, got, tc.want)
		}
	}
}
