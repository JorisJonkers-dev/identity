// Package assembly builds identity from its contexts and adapters into the routes it serves.
// cmd/identity runs what it builds, and the integration tests drive the very same routes.
package assembly

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/adapters/catalog"
	"github.com/JorisJonkers-dev/identity/internal/access/adapters/people"
	accesspersistence "github.com/JorisJonkers-dev/identity/internal/access/adapters/persistence"
	accessapp "github.com/JorisJonkers-dev/identity/internal/access/app"
	"github.com/JorisJonkers-dev/identity/internal/audit/adapters/directory"
	auditpersistence "github.com/JorisJonkers-dev/identity/internal/audit/adapters/persistence"
	auditweb "github.com/JorisJonkers-dev/identity/internal/audit/adapters/web"
	auditapp "github.com/JorisJonkers-dev/identity/internal/audit/app"
	auditdomain "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	gatecontexts "github.com/JorisJonkers-dev/identity/internal/gate/adapters/contexts"
	gatepersistence "github.com/JorisJonkers-dev/identity/internal/gate/adapters/persistence"
	gateweb "github.com/JorisJonkers-dev/identity/internal/gate/adapters/web"
	gateapp "github.com/JorisJonkers-dev/identity/internal/gate/app"
	launchercontexts "github.com/JorisJonkers-dev/identity/internal/launcher/adapters/contexts"
	launcherfetch "github.com/JorisJonkers-dev/identity/internal/launcher/adapters/fetch"
	launcherweb "github.com/JorisJonkers-dev/identity/internal/launcher/adapters/web"
	launcherapp "github.com/JorisJonkers-dev/identity/internal/launcher/app"
	"github.com/JorisJonkers-dev/identity/internal/platform/httpapi"
	"github.com/JorisJonkers-dev/identity/internal/platform/passwords"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/webui"
	"github.com/JorisJonkers-dev/identity/internal/server"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/accounts"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	signinoidc "github.com/JorisJonkers-dev/identity/internal/signin/adapters/oidc"
	signinpersistence "github.com/JorisJonkers-dev/identity/internal/signin/adapters/persistence"
	signintokens "github.com/JorisJonkers-dev/identity/internal/signin/adapters/tokens"
	signinapp "github.com/JorisJonkers-dev/identity/internal/signin/app"
	tokenscontexts "github.com/JorisJonkers-dev/identity/internal/tokens/adapters/contexts"
	tokenspersistence "github.com/JorisJonkers-dev/identity/internal/tokens/adapters/persistence"
	tokensweb "github.com/JorisJonkers-dev/identity/internal/tokens/adapters/web"
	tokensapp "github.com/JorisJonkers-dev/identity/internal/tokens/app"
	"github.com/JorisJonkers-dev/identity/internal/users/adapters/passkeys"
	"github.com/JorisJonkers-dev/identity/internal/users/adapters/persistence"
	usersweb "github.com/JorisJonkers-dev/identity/internal/users/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/web"
)

const (
	// argon2id takes 46 MiB a hash: four at once stays under 200 MiB, and a check that cannot start
	// within hashWait answers 503 rather than queueing.
	maxConcurrentHashes = 4
	hashWait            = 3 * time.Second
	// A Gate check is answered from memory for this long; a sign-out or a role taken away reaches a
	// gated host within it (docs/adr/0009).
	gateCacheTTL = 30 * time.Second
	// A Client that does not send its icon within this long shows its label's first letter instead.
	iconTimeout = 5 * time.Second
)

// Config is what identity is built from.
type Config struct {
	Store *pg.Store
	// PublicURL is the Issuer, where mailed links point, with no trailing slash.
	PublicURL string
	Mailer    app.Mailer
	// Sealer seals TOTP secrets; without one, Users cannot add authenticator apps.
	Sealer app.Sealer
	// Index is the Clients the cluster declares, and Synced whether they are all listed yet.
	Index  *clients.Index
	Synced func() bool
	// IconTransport fetches Clients' icons; nil is the default transport.
	IconTransport http.RoundTripper
	// Grafana is where the dashboard links for charts over time; nil links nowhere.
	Grafana *url.URL
	Logger  *slog.Logger
	Version string
}

// Identity is identity built: its routes, the count of Users waiting for a Group, which /metrics
// reports, and Prune, which removes Audit Events older than a year and runs on a schedule.
type Identity struct {
	Routes     server.Routes
	Unassigned func(ctx context.Context) (int, error)
	Prune      func(ctx context.Context) (int64, error)
}

// New builds identity.
func New(cfg Config) (*Identity, error) {
	// users caps an Admin's session at 12 hours, and access lists users' Users: each needs the other.
	var access *accessapp.Service
	admins := adminsFunc(func(ctx context.Context, id uuid.UUID) (bool, error) { return access.IsAdmin(ctx, id) })
	// Set once the Provider and the Gate exist, before the server serves anything.
	var signedOut app.SignedOut
	// Audit names Users, so it is made after users, which records through it.
	var audit *auditapp.Service
	recorder := recorderFunc(func(ctx context.Context, e auditdomain.Event) { audit.Record(ctx, e) })
	opts := []app.Option{app.WithAdmins(admins), app.WithAudit(recorder), app.WithSignedOut(func(ctx context.Context, userID uuid.UUID, sessionIDs []uuid.UUID) {
		signedOut(ctx, userID, sessionIDs)
	})}
	ceremonies, err := passkeyCeremonies(cfg.PublicURL)
	if err != nil {
		return nil, err
	}
	opts = append(opts, app.WithWebAuthn(ceremonies))
	if cfg.Sealer != nil {
		opts = append(opts, app.WithSealer(cfg.Sealer))
	}
	q := cfg.Store.Queries()
	users := app.New(persistence.New(q), passwords.NewArgon2id(maxConcurrentHashes, hashWait), cfg.Mailer, time.Now, rand.Reader, cfg.PublicURL, opts...)
	access = accessapp.New(accesspersistence.New(q, cfg.Store), catalog.New(cfg.Index), people.New(users), recorder, rand.Reader)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	ring := signinoidc.NewRing(key)
	logouts := signinoidc.NewLogouts(cfg.PublicURL, ring, &http.Client{}, cfg.Logger, time.Now)
	signIn := signinapp.New(signinpersistence.New(q), cfg.Index, accounts.New(users), access, logouts, recorder, time.Now, rand.Reader)
	tokenContexts := tokenscontexts.New(signIn, access, cfg.Index)
	tokens := tokensapp.New(tokenspersistence.New(q), tokenContexts, tokenContexts, admins, recorder, time.Now, rand.Reader)
	signIn.SetPersonalTokens(signintokens.New(tokens))
	audit = auditapp.New(auditpersistence.New(q), admins, directory.New(users, access, tokens, cfg.Index), cfg.Logger, time.Now, rand.Reader)
	launcherContexts := launchercontexts.New(cfg.Index, access)
	transport := cfg.IconTransport
	if transport == nil {
		transport = http.DefaultTransport
	}
	launcher := launcherapp.New(launcherContexts, launcherContexts, launcherfetch.New(transport, iconTimeout), time.Now)
	api, err := httpapi.New(cfg.Logger, cfg.Version, users, access, auditweb.New(audit, cfg.Grafana), launcher, tokensweb.New(tokens))
	if err != nil {
		return nil, err
	}
	provider, err := signinoidc.New(signinoidc.Config{
		Issuer:        cfg.PublicURL,
		Insecure:      strings.HasPrefix(cfg.PublicURL, "http://"),
		SessionCookie: usersweb.CookieName,
	}, signIn, ring, cfg.Logger, time.Now)
	if err != nil {
		return nil, err
	}
	others := gatecontexts.New(signIn, users)
	gate := gateapp.New(gatepersistence.New(q), others, others, others, recorder, time.Now, rand.Reader, gateCacheTTL)
	gate.SetTokens(others)
	signedOut = endSessions(gate, signIn, cfg.Logger)
	gateHandler, err := gateweb.New(gate, gateweb.Config{Issuer: cfg.PublicURL, SessionCookie: usersweb.CookieName}, cfg.Logger)
	if err != nil {
		return nil, err
	}
	return &Identity{
		Routes: server.Routes{
			API:   api,
			OIDC:  provider,
			Gate:  gateHandler,
			Icons: launcherweb.NewIcons(launcher, cfg.Logger),
			Web:   webui.Handler(web.Dist()),
			Ready: func(ctx context.Context) error {
				if !cfg.Synced() {
					return errClientsNotListed
				}
				return cfg.Store.Ping(ctx)
			},
		},
		Unassigned: access.Unassigned,
		Prune:      audit.Prune,
	}, nil
}

var errClientsNotListed = errors.New("the Client objects are not listed yet")

// passkeyCeremonies runs WebAuthn for the Identity Provider: its host is the relying party, and
// the Issuer the one origin a passkey is made or used on.
func passkeyCeremonies(publicURL string) (*passkeys.Ceremonies, error) {
	u, err := url.Parse(publicURL)
	if err != nil {
		return nil, err
	}
	return passkeys.New(u.Hostname(), publicURL)
}

// recorderFunc records Audit Events through a function.
type recorderFunc func(ctx context.Context, e auditdomain.Event)

func (f recorderFunc) Record(ctx context.Context, e auditdomain.Event) { f(ctx, e) }

// adminsFunc is app.Admins as a function.
type adminsFunc func(ctx context.Context, id uuid.UUID) (bool, error)

func (f adminsFunc) IsAdmin(ctx context.Context, id uuid.UUID) (bool, error) { return f(ctx, id) }

// endSessions ends what was signed in through ended sessions: the Gate's cached checks on this
// replica, and native Clients through back-channel logout (docs/adr/0007).
func endSessions(gate *gateapp.Service, signIn *signinapp.Service, logger *slog.Logger) app.SignedOut {
	return func(ctx context.Context, userID uuid.UUID, sessionIDs []uuid.UUID) {
		gate.Forget(sessionIDs)
		if err := signIn.SignedOut(ctx, userID, sessionIDs); err != nil {
			logger.ErrorContext(ctx, "back-channel logout", "error", err)
		}
	}
}
