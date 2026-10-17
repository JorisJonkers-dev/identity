package oidc

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var logoutAt = time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)

func newLogouts(t *testing.T) (*Logouts, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	return NewLogouts("https://auth.test", NewRing(testKey()), &http.Client{}, slog.New(slog.NewTextHandler(&logs, nil)), func() time.Time { return logoutAt }), &logs
}

func TestALogoutTokenIsSignedAndNamesTheSession(t *testing.T) {
	got := make(chan *http.Request, 1)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got <- r
		w.WriteHeader(http.StatusOK)
	}))
	defer app.Close()
	l, logs := newLogouts(t)
	user, session := uuid.New(), uuid.New()
	l.Send(context.Background(), domain.Client{ID: "spa", BackChannelLogoutURI: app.URL + "/logout"}, user, session)
	l.Wait()

	r := <-got
	if r.Method != http.MethodPost || r.URL.Path != "/logout" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("request = %s %s %s", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
	}
	signed, err := jose.ParseSigned(r.PostForm.Get("logout_token"), []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		t.Fatal(err)
	}
	if typ := signed.Signatures[0].Protected.ExtraHeaders["typ"]; typ != "logout+jwt" {
		t.Fatalf("typ = %v", typ)
	}
	if kid := signed.Signatures[0].Protected.KeyID; kid != domain.KeyID(&testKey().PublicKey) {
		t.Fatalf("kid = %q", kid)
	}
	payload, err := signed.Verify(&testKey().PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	_ = json.Unmarshal(payload, &claims)
	for name, want := range map[string]any{
		"iss": "https://auth.test", "sub": user.String(), "sid": session.String(),
		"iat": float64(logoutAt.Unix()), "exp": float64(logoutAt.Add(2 * time.Minute).Unix()),
	} {
		if claims[name] != want {
			t.Errorf("%s = %v, want %v", name, claims[name], want)
		}
	}
	if aud, _ := claims["aud"].([]any); len(aud) != 1 || aud[0] != "spa" {
		t.Errorf("aud = %v", claims["aud"])
	}
	if _, err := uuid.Parse(claims["jti"].(string)); err != nil {
		t.Errorf("jti = %v", claims["jti"])
	}
	if events, _ := claims["events"].(map[string]any); events == nil || events[logoutEvent] == nil {
		t.Errorf("events = %v", claims["events"])
	}
	if _, nonce := claims["nonce"]; nonce {
		t.Error("a logout token carries no nonce")
	}
	if logs.Len() != 0 {
		t.Fatalf("logged %s", logs)
	}
}

func TestAClientThatRefusesTheLogoutIsLogged(t *testing.T) {
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fine" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	defer app.Close()
	l, logs := newLogouts(t)
	for _, target := range []string{app.URL + "/fine", app.URL + "/refuses", gone.URL, "http://bad host/"} {
		l.Send(context.Background(), domain.Client{ID: target, BackChannelLogoutURI: target}, uuid.New(), uuid.New())
	}
	l.Wait()
	if n := strings.Count(logs.String(), "back-channel logout"); n != 3 {
		t.Fatalf("logged %d failures, want 3: %s", n, logs)
	}
	if strings.Contains(logs.String(), "/fine") || !strings.Contains(logs.String(), "answered 400") {
		t.Fatalf("logs = %s", logs)
	}
}
