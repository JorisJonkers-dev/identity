package web_test

import (
	"net/http"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/gate/app"
)

func bearer(token string) map[string]string {
	h := forwarded("/api/backup")
	h["Authorization"] = token
	return h
}

func TestAPersonalTokenPassesItsUserOn(t *testing.T) {
	uc := &useCases{}
	rec := get(t, serve(t, uc), "http://identity:8080/gate/check", bearer("Bearer idpat_ada"))
	if rec.Code != http.StatusOK || rec.Header().Get("X-Auth-Request-User") != "00000000-0000-4000-8000-0000000000a1" || rec.Header().Get("X-Auth-Request-Roles") != "viewer" {
		t.Fatalf("check = %d %v", rec.Code, rec.Header())
	}
	if len(uc.asked) != 2 || uc.asked[0] != "tribelt.jorisjonkers.dev" || uc.asked[1] != "idpat_ada" {
		t.Fatalf("asked %v", uc.asked)
	}
}

func TestTheBearerSchemeIsReadWhateverItsCase(t *testing.T) {
	uc := &useCases{}
	if rec := get(t, serve(t, uc), "http://identity:8080/gate/check", bearer("bearer idpat_ada")); rec.Code != http.StatusOK || uc.asked[1] != "idpat_ada" {
		t.Fatalf("check = %d, asked %v", rec.Code, uc.asked)
	}
}

func TestAnAppsOwnBearerTokenIsLeftToTheCookieCheck(t *testing.T) {
	for _, header := range []string{"Bearer glsa_grafana-api-key", "Basic YWRhOnB3", "idpat_without-a-scheme"} {
		t.Run(header, func(t *testing.T) {
			uc := &useCases{}
			get(t, serve(t, uc), "http://identity:8080/gate/check", bearer(header))
			if len(uc.asked) != 2 || uc.asked[1] != "" {
				t.Fatalf("asked %v, want the cookie check", uc.asked)
			}
		})
	}
}

func TestAPersonalTokenThatIsNotGoodIsABare401(t *testing.T) {
	for _, err := range []error{app.ErrNoSignIn, app.ErrNoAccess} {
		t.Run(err.Error(), func(t *testing.T) {
			rec := get(t, serve(t, &useCases{bearerErr: err}), "http://identity:8080/gate/check", bearer("Bearer idpat_revoked"))
			if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != `Bearer realm="tribelt.jorisjonkers.dev", error="invalid_token"` || rec.Body.Len() != 0 {
				t.Fatalf("check = %d %v %q", rec.Code, rec.Header(), rec.Body)
			}
		})
	}
}

func TestAPersonalTokenOnAHostWithNoClientIsRefused(t *testing.T) {
	if rec := get(t, serve(t, &useCases{bearerErr: app.ErrUnknownHost}), "http://identity:8080/gate/check", bearer("Bearer idpat_ada")); rec.Code != http.StatusForbidden {
		t.Fatalf("check = %d", rec.Code)
	}
}

func TestAFailedPersonalTokenCheckIsAServerError(t *testing.T) {
	if rec := get(t, serve(t, &useCases{bearerErr: errBoom}), "http://identity:8080/gate/check", bearer("Bearer idpat_ada")); rec.Code != http.StatusInternalServerError {
		t.Fatalf("check = %d", rec.Code)
	}
}

func TestATokenHostWithoutAPersonalTokenAsksForOne(t *testing.T) {
	rec := get(t, serve(t, &useCases{checkErr: app.ErrBearerNeeded}), "http://identity:8080/gate/check", forwarded("/api/backup"))
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != `Bearer realm="tribelt.jorisjonkers.dev"` {
		t.Fatalf("check = %d %v", rec.Code, rec.Header())
	}
}

const resourceMetadata = `resource_metadata="https://auth.test/.well-known/oauth-protected-resource/tribelt.jorisjonkers.dev"`

func TestAConnectedAppsTokenIsCheckedAsABearerToken(t *testing.T) {
	uc := &useCases{}
	if rec := get(t, serve(t, uc), "http://identity:8080/gate/check", bearer("Bearer idapp_sealed")); rec.Code != http.StatusOK || uc.asked[1] != "idapp_sealed" {
		t.Fatalf("check = %d, asked %v", rec.Code, uc.asked)
	}
}

func TestAnMCPResourcesHostPointsAToolWithoutATokenToItsMetadata(t *testing.T) {
	rec := get(t, serve(t, &useCases{checkErr: app.ErrBearerNeeded, mcp: true}), "http://identity:8080/gate/check", forwarded("/mcp"))
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != `Bearer realm="tribelt.jorisjonkers.dev", `+resourceMetadata {
		t.Fatalf("check = %d %v", rec.Code, rec.Header())
	}
}

func TestAnMCPResourcesHostPointsAToolWithABadTokenToItsMetadata(t *testing.T) {
	rec := get(t, serve(t, &useCases{bearerErr: app.ErrNoSignIn, mcp: true}), "http://identity:8080/gate/check", bearer("Bearer idapp_expired"))
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != `Bearer realm="tribelt.jorisjonkers.dev", error="invalid_token", `+resourceMetadata {
		t.Fatalf("check = %d %v", rec.Code, rec.Header())
	}
}

func TestAGatedMCPResourcesSignInPageAlsoPointsAToolToItsMetadata(t *testing.T) {
	rec := get(t, serve(t, &useCases{checkErr: app.ErrNoSignIn, mcp: true}), "http://identity:8080/gate/check", forwarded("/mcp"))
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != `Bearer realm="tribelt.jorisjonkers.dev", `+resourceMetadata {
		t.Fatalf("check = %d %v", rec.Code, rec.Header())
	}
}

func TestAGatedHostTakingNoConnectedAppsSendsNoChallengeWithItsSignInPage(t *testing.T) {
	if rec := get(t, serve(t, &useCases{checkErr: app.ErrNoSignIn}), "http://identity:8080/gate/check", forwarded("/")); rec.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("challenge %q", rec.Header().Get("WWW-Authenticate"))
	}
}
