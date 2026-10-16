package oidc

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/crypto"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// zitadel/oidc defines the logout token (OpenID Connect Back-Channel Logout 1.0) but never sends
// one, so sending is done here, with its claims type and its signing (docs/adr/0001).
const (
	logoutEvent    = "http://schemas.openid.net/event/backchannel-logout"
	logoutLifetime = 2 * time.Minute
	logoutTimeout  = 5 * time.Second
)

// Logouts is the app.Logouts: it signs a logout token and posts it to the Client, in the
// background, so a sign-out never waits on an app.
type Logouts struct {
	issuer string
	ring   *Ring
	client *http.Client
	logger *slog.Logger
	now    func() time.Time
	sent   sync.WaitGroup
}

var _ app.Logouts = (*Logouts)(nil)

// NewLogouts returns Logouts for the Issuer, signing with ring.
func NewLogouts(issuer string, ring *Ring, client *http.Client, logger *slog.Logger, now func() time.Time) *Logouts {
	return &Logouts{issuer: issuer, ring: ring, client: client, logger: logger, now: now}
}

// Send posts a logout token for the session to the Client's back-channel logout URI.
func (l *Logouts) Send(ctx context.Context, c domain.Client, userID, sessionID uuid.UUID) {
	token := l.token(c.ID, userID, sessionID)
	ctx = context.WithoutCancel(ctx)
	l.sent.Go(func() {
		if err := l.post(ctx, c.BackChannelLogoutURI, token); err != nil {
			l.logger.WarnContext(ctx, "back-channel logout", "client", c.ID, "error", err)
		}
	})
}

// Wait returns once every logout token sent so far has been posted or has failed.
func (l *Logouts) Wait() { l.sent.Wait() }

// token signs the logout token with the type the specification asks for. The ring's key is RSA,
// which go-jose always signs with, and these claims always marshal.
func (l *Logouts) token(clientID string, userID, sessionID uuid.UUID) string {
	key := l.ring.Signing()
	signer, _ := jose.NewSigner(
		jose.SigningKey{Algorithm: key.SignatureAlgorithm(), Key: jose.JSONWebKey{Key: key.Key(), KeyID: key.ID()}},
		(&jose.SignerOptions{}).WithType("logout+jwt"),
	)
	now := l.now()
	token, _ := crypto.Sign(&oidc.LogoutTokenClaims{
		Issuer: l.issuer, Subject: userID.String(), Audience: oidc.Audience{clientID}, SessionID: sessionID.String(),
		IssuedAt: oidc.FromTime(now), Expiration: oidc.FromTime(now.Add(logoutLifetime)), JWTID: uuid.NewString(),
		Events: map[string]any{logoutEvent: struct{}{}},
	}, signer)
	return token
}

func (l *Logouts) post(ctx context.Context, target, token string) error {
	ctx, cancel := context.WithTimeout(ctx, logoutTimeout)
	defer cancel()
	body := url.Values{"logout_token": {token}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4<<10))
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("the Client answered %d", res.StatusCode)
	}
	return nil
}
