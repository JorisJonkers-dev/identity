// Package app holds the connected context's use cases (identity#23): a tool registers itself, a
// User consents to it acting for them on one MCP resource, and its tokens are issued, rotated,
// checked and revoked.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/connected/domain"
)

var (
	ErrNotFound = errors.New("not found")
	// ErrNotMCPResource is a Client no Admin marked as taking Connected Apps.
	ErrNotMCPResource = errors.New("the Client takes no Connected Apps")
	// ErrStepUpClient is a Step-up Client, which a Connected App's tokens could never satisfy
	// (docs/adr/0019).
	ErrStepUpClient = errors.New("the Client asks for fresh two-factor, which a Connected App cannot give")
	// ErrNoAccess is a User holding no App Role on the Client.
	ErrNoAccess = errors.New("no App Role on the Client")
	// ErrInvalidToken is a token that is unknown, expired, spent or for another Client.
	ErrInvalidToken = errors.New("the token is not valid")
	ErrNotAdmin     = errors.New("only Admins can do this")
	// ErrSecondFactorNeeded is an Admin whose session did not prove a Second Factor.
	ErrSecondFactorNeeded = errors.New("a session that proved a Second Factor is needed")
)

const (
	// AccessLifetime is how long an access token works.
	AccessLifetime = time.Hour
	// RefreshLifetime is how long a refresh token can be spent.
	RefreshLifetime = 30 * 24 * time.Hour
	// unconsentedLifetime is how long a registration nobody consented to is kept.
	unconsentedLifetime = 24 * time.Hour
	// registrationIDBytes and refreshBytes are the randomness of a registration id and a refresh
	// token's secret.
	registrationIDBytes = 16
	refreshBytes        = 32
	// RegistrationPrefix starts every registered client's id, so it is never a Client's.
	RegistrationPrefix = "mcp-"
)

// AccessToken is a live access token: the Connected App it is for, and its User and Client.
type AccessToken struct {
	ID        uuid.UUID
	AppID     uuid.UUID
	UserID    uuid.UUID
	ClientID  string
	ExpiresAt time.Time
}

// RefreshToken is a refresh token as the store keeps it.
type RefreshToken struct {
	ID        uuid.UUID
	AppID     uuid.UUID
	UserID    uuid.UUID
	ClientID  string
	ExpiresAt time.Time
	// RegistrationID is the registered tool the Connected App is, which alone may spend the token.
	RegistrationID string
}

// Store keeps registrations, MCP marks, Connected Apps and their tokens.
type Store interface {
	CreateRegistration(ctx context.Context, r domain.Registration) error
	// Registration is ErrNotFound when there is none.
	Registration(ctx context.Context, clientID string) (domain.Registration, error)
	PruneRegistrations(ctx context.Context, before time.Time) (int64, error)
	MCPResources(ctx context.Context) ([]string, error)
	IsMCPResource(ctx context.Context, clientID string) (bool, error)
	SetMCPResource(ctx context.Context, clientID string, on bool, at time.Time) error
	// Consent is the Connected App for a consent, new or the one already there, and reports
	// whether it is new.
	Consent(ctx context.Context, app domain.ConnectedApp, registrationID string) (domain.ConnectedApp, bool, error)
	// Consented is the Connected App a User already consented to, or ErrNotFound.
	Consented(ctx context.Context, registrationID string, userID uuid.UUID, clientID string) (uuid.UUID, error)
	ConnectedApps(ctx context.Context, userID uuid.UUID) ([]domain.ConnectedApp, error)
	// ConnectedApp is ErrNotFound when there is none.
	ConnectedApp(ctx context.Context, id uuid.UUID) (domain.ConnectedApp, error)
	// DeleteConnectedApp deletes it with its tokens and answers its access tokens' ids.
	DeleteConnectedApp(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error)
	// DeleteForClient deletes every Connected App of a Client, and answers them and their access
	// tokens' ids.
	DeleteForClient(ctx context.Context, clientID string) ([]domain.ConnectedApp, []uuid.UUID, error)
	CountForClient(ctx context.Context, clientID string) (int, error)
	Touch(ctx context.Context, appID uuid.UUID, at time.Time) error
	CreateAccess(ctx context.Context, id, appID uuid.UUID, at, expires time.Time) error
	CreateRefresh(ctx context.Context, id, appID uuid.UUID, hash []byte, at, expires time.Time) error
	// Access is ErrNotFound when there is none.
	Access(ctx context.Context, id uuid.UUID) (AccessToken, error)
	// Refresh is ErrNotFound when there is none.
	Refresh(ctx context.Context, hash []byte) (RefreshToken, error)
	// Spend marks a refresh token spent, and reports false when it already was.
	Spend(ctx context.Context, id uuid.UUID, at time.Time) (bool, error)
	PruneTokens(ctx context.Context, before time.Time) (int64, error)
}

// Client is a Client as the connected context needs it.
type Client struct {
	ID    string
	Label string
	Roles []string
	// StepUp is whether the Client asks for fresh two-factor.
	StepUp bool
}

// Clients finds the Clients the cluster declares.
type Clients interface {
	// Client is ErrNotFound for a Client not in the cluster.
	Client(ctx context.Context, id string) (Client, error)
	// ClientForHost is ErrNotFound for a host no Client serves.
	ClientForHost(ctx context.Context, host string) (Client, error)
}

// Roles is the App Roles a User holds on a Client, directly and through Groups.
type Roles interface {
	Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error)
}

// Admins tells who is an Admin.
type Admins interface {
	IsAdmin(ctx context.Context, id uuid.UUID) (bool, error)
}

// Audit records each change as an Audit Event, after it is made.
type Audit interface {
	Record(ctx context.Context, e audit.Event)
}

// Actor is who asks: a User, and whether their session proved a Second Factor.
type Actor struct {
	ID           uuid.UUID
	SecondFactor bool
}

// Service is the connected use cases.
type Service struct {
	store   Store
	clients Clients
	roles   Roles
	admins  Admins
	audit   Audit
	now     func() time.Time
	random  io.Reader
	// forget drops access tokens from wherever their checks are cached, when they end.
	forget func(tokenIDs []uuid.UUID)
}

// New returns a Service; random is where ids and secrets come from.
func New(store Store, clients Clients, roles Roles, admins Admins, audit Audit, now func() time.Time, random io.Reader) *Service {
	return &Service{store: store, clients: clients, roles: roles, admins: admins, audit: audit, now: now, random: random, forget: func([]uuid.UUID) {}}
}

// OnForget sets what runs when access tokens end before they expire.
func (s *Service) OnForget(forget func(tokenIDs []uuid.UUID)) { s.forget = forget }

// Register makes a registration for a tool that names itself and its redirect URIs.
func (s *Service) Register(ctx context.Context, name string, redirectURIs []string) (domain.Registration, error) {
	b := make([]byte, registrationIDBytes)
	if _, err := io.ReadFull(s.random, b); err != nil {
		return domain.Registration{}, err
	}
	id := RegistrationPrefix + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
	r, err := domain.NewRegistration(id, name, redirectURIs, s.now())
	if err != nil {
		return domain.Registration{}, err
	}
	return r, s.store.CreateRegistration(ctx, r)
}

// Registration is a registered tool, for the token endpoint to know it.
func (s *Service) Registration(ctx context.Context, clientID string) (domain.Registration, error) {
	return s.store.Registration(ctx, clientID)
}

// Resource is the Client an MCP resource URL names, by its host, while an Admin marks it as taking
// Connected Apps.
func (s *Service) Resource(ctx context.Context, resource string) (Client, error) {
	host := hostOf(resource)
	if host == "" {
		return Client{}, ErrNotMCPResource
	}
	c, err := s.clients.ClientForHost(ctx, host)
	if errors.Is(err, ErrNotFound) {
		return Client{}, ErrNotMCPResource
	}
	if err != nil {
		return Client{}, err
	}
	mcp, err := s.store.IsMCPResource(ctx, c.ID)
	if err != nil {
		return Client{}, err
	}
	if !mcp {
		return Client{}, ErrNotMCPResource
	}
	return c, nil
}

func hostOf(resource string) string {
	rest, ok := strings.CutPrefix(resource, "https://")
	if !ok {
		return ""
	}
	host, _, _ := strings.Cut(rest, "/")
	host, _, _ = strings.Cut(host, ":")
	return strings.ToLower(host)
}

// Consent is the Connected App a User's consent makes: for a Client marked as an MCP resource,
// that is not a Step-up Client, on which they hold an App Role. Consenting again answers the one
// already there.
func (s *Service) Consent(ctx context.Context, userID uuid.UUID, registrationID, clientID string) (uuid.UUID, error) {
	r, err := s.store.Registration(ctx, registrationID)
	if err != nil {
		return uuid.Nil, err
	}
	c, err := s.consentable(ctx, userID, clientID)
	if err != nil {
		return uuid.Nil, err
	}
	id, err := uuid.NewRandomFromReader(s.random)
	if err != nil {
		return uuid.Nil, err
	}
	app, created, err := s.store.Consent(ctx, domain.ConnectedApp{ID: id, UserID: userID, ClientID: c.ID, Name: r.Name, CreatedAt: s.now()}, registrationID)
	if err != nil {
		return uuid.Nil, err
	}
	if created {
		s.audit.Record(ctx, audit.Event{Action: audit.ConnectedAppAuthorized, User: userID, Client: c.ID, AppName: r.Name})
	}
	return app.ID, nil
}

// Consented is the Connected App a User already consented to for a registration and Client, so
// the consent screen is shown once; ErrNotFound when there is none. The rules a consent meets are
// checked again, since roles and step-up change.
func (s *Service) Consented(ctx context.Context, userID uuid.UUID, registrationID, clientID string) (uuid.UUID, error) {
	if _, err := s.consentable(ctx, userID, clientID); err != nil {
		return uuid.Nil, err
	}
	return s.store.Consented(ctx, registrationID, userID, clientID)
}

func (s *Service) consentable(ctx context.Context, userID uuid.UUID, clientID string) (Client, error) {
	c, err := s.clients.Client(ctx, clientID)
	if err != nil {
		return Client{}, err
	}
	mcp, err := s.store.IsMCPResource(ctx, c.ID)
	if err != nil {
		return Client{}, err
	}
	switch {
	case !mcp:
		return Client{}, ErrNotMCPResource
	case c.StepUp:
		return Client{}, ErrStepUpClient
	}
	roles, err := s.rolesOn(ctx, userID, c)
	if err != nil {
		return Client{}, err
	}
	if len(roles) == 0 {
		return Client{}, ErrNoAccess
	}
	return c, nil
}

// rolesOn is the App Roles a User holds on a Client that the Client declares, in its order.
func (s *Service) rolesOn(ctx context.Context, userID uuid.UUID, c Client) ([]string, error) {
	held, err := s.roles.Roles(ctx, userID, c.ID)
	if err != nil {
		return nil, err
	}
	roles := []string{}
	for _, r := range c.Roles {
		if slices.Contains(held, r) {
			roles = append(roles, r)
		}
	}
	return roles, nil
}

// RolesFor is the App Roles a consent would give, for the consent screen to name.
func (s *Service) RolesFor(ctx context.Context, userID uuid.UUID, clientID string) (Client, []string, error) {
	c, err := s.clients.Client(ctx, clientID)
	if err != nil {
		return Client{}, nil, err
	}
	roles, err := s.rolesOn(ctx, userID, c)
	return c, roles, err
}

// IssueAccess makes a Connected App an access token that works for an hour.
func (s *Service) IssueAccess(ctx context.Context, appID uuid.UUID) (uuid.UUID, time.Time, error) {
	id, err := uuid.NewRandomFromReader(s.random)
	if err != nil {
		return uuid.Nil, time.Time{}, err
	}
	now := s.now()
	expires := now.Add(AccessLifetime)
	return id, expires, s.store.CreateAccess(ctx, id, appID, now, expires)
}

// IssueRefresh makes a Connected App a refresh token and answers its secret, the one time it is
// seen; the store keeps only its hash.
func (s *Service) IssueRefresh(ctx context.Context, appID uuid.UUID) (string, error) {
	b := make([]byte, refreshBytes)
	if _, err := io.ReadFull(s.random, b); err != nil {
		return "", err
	}
	id, err := uuid.NewRandomFromReader(s.random)
	if err != nil {
		return "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(b)
	now := s.now()
	return secret, s.store.CreateRefresh(ctx, id, appID, hashOf(secret), now, now.Add(RefreshLifetime))
}

// Refresh spends a refresh token and answers its Connected App, for new tokens. A refresh token
// spent a second time means it leaked: the Connected App is revoked.
func (s *Service) Refresh(ctx context.Context, secret string) (RefreshToken, error) {
	t, err := s.store.Refresh(ctx, hashOf(secret))
	if errors.Is(err, ErrNotFound) {
		return RefreshToken{}, ErrInvalidToken
	}
	if err != nil {
		return RefreshToken{}, err
	}
	if !s.now().Before(t.ExpiresAt) {
		return RefreshToken{}, ErrInvalidToken
	}
	spent, err := s.store.Spend(ctx, t.ID, s.now())
	if err != nil {
		return RefreshToken{}, err
	}
	if !spent {
		if err := s.end(ctx, t.AppID); err != nil {
			return RefreshToken{}, err
		}
		return RefreshToken{}, ErrInvalidToken
	}
	return t, nil
}

// Check is a resource server's check of an access token: live, for this Client, while its User
// holds an App Role there, with the App Roles they hold now.
func (s *Service) Check(ctx context.Context, tokenID uuid.UUID, clientID string) (AccessToken, []string, error) {
	t, err := s.store.Access(ctx, tokenID)
	if errors.Is(err, ErrNotFound) {
		return AccessToken{}, nil, ErrInvalidToken
	}
	if err != nil {
		return AccessToken{}, nil, err
	}
	if t.ClientID != clientID || !s.now().Before(t.ExpiresAt) {
		return AccessToken{}, nil, ErrInvalidToken
	}
	c, err := s.clients.Client(ctx, clientID)
	if errors.Is(err, ErrNotFound) {
		return AccessToken{}, nil, ErrInvalidToken
	}
	if err != nil {
		return AccessToken{}, nil, err
	}
	roles, err := s.rolesOn(ctx, t.UserID, c)
	if err != nil {
		return AccessToken{}, nil, err
	}
	if len(roles) == 0 {
		return AccessToken{}, nil, ErrInvalidToken
	}
	if err := s.store.Touch(ctx, t.AppID, s.now()); err != nil {
		return AccessToken{}, nil, err
	}
	return t, roles, nil
}

// ConnectedApps is a User's Connected Apps, for the Security page.
func (s *Service) ConnectedApps(ctx context.Context, userID uuid.UUID) ([]domain.ConnectedApp, error) {
	return s.store.ConnectedApps(ctx, userID)
}

// Revoke ends a User's own Connected App: its tokens stop working at once.
func (s *Service) Revoke(ctx context.Context, userID, id uuid.UUID) error {
	app, err := s.store.ConnectedApp(ctx, id)
	if err != nil {
		return err
	}
	if app.UserID != userID {
		return ErrNotFound
	}
	if err := s.end(ctx, id); err != nil {
		return err
	}
	s.audit.Record(ctx, audit.Event{Action: audit.ConnectedAppRevoked, User: userID, Client: app.ClientID, AppName: app.Name})
	return nil
}

func (s *Service) end(ctx context.Context, id uuid.UUID) error {
	tokens, err := s.store.DeleteConnectedApp(ctx, id)
	if err != nil {
		return err
	}
	s.forget(tokens)
	return nil
}

// CountForClient is how many Connected Apps turning step-up on would revoke.
func (s *Service) CountForClient(ctx context.Context, clientID string) (int, error) {
	return s.store.CountForClient(ctx, clientID)
}

// RevokeForClient ends every Connected App of a Client, as turning step-up on does; each is an
// Audit Event by the Admin, and their tokens stop working at once.
func (s *Service) RevokeForClient(ctx context.Context, actor uuid.UUID, clientID string) error {
	apps, tokens, err := s.store.DeleteForClient(ctx, clientID)
	if err != nil {
		return err
	}
	s.forget(tokens)
	for _, a := range apps {
		s.audit.Record(ctx, audit.Event{Action: audit.ConnectedAppRevoked, Actor: actor, User: a.UserID, Client: clientID, AppName: a.Name})
	}
	return nil
}

// MCPResources is the Clients an Admin marked as taking Connected Apps.
func (s *Service) MCPResources(ctx context.Context) ([]string, error) {
	return s.store.MCPResources(ctx)
}

// IsMCPResource reports whether a Client takes Connected Apps.
func (s *Service) IsMCPResource(ctx context.Context, clientID string) (bool, error) {
	return s.store.IsMCPResource(ctx, clientID)
}

// SetMCPResource lets a Client take Connected Apps, or stops it, as an Admin. Stopping keeps the
// Connected Apps it has; an Admin revokes those by turning step-up on, or a User revokes their own.
func (s *Service) SetMCPResource(ctx context.Context, actor Actor, clientID string, on bool) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	if _, err := s.clients.Client(ctx, clientID); err != nil {
		return err
	}
	if err := s.store.SetMCPResource(ctx, clientID, on, s.now()); err != nil {
		return err
	}
	action := audit.MCPResourceUnmarked
	if on {
		action = audit.MCPResourceMarked
	}
	s.audit.Record(ctx, audit.Event{Action: action, Actor: actor.ID, Client: clientID})
	return nil
}

// MCPState is whether a Client takes Connected Apps and how many it has, as an Admin sees it.
func (s *Service) MCPState(ctx context.Context, actor Actor, clientID string) (bool, int, error) {
	if err := s.admin(ctx, actor); err != nil {
		return false, 0, err
	}
	if _, err := s.clients.Client(ctx, clientID); err != nil {
		return false, 0, err
	}
	on, err := s.store.IsMCPResource(ctx, clientID)
	if err != nil {
		return false, 0, err
	}
	n, err := s.store.CountForClient(ctx, clientID)
	return on, n, err
}

// admin is nil for an Admin whose session proved a Second Factor.
func (s *Service) admin(ctx context.Context, actor Actor) error {
	yes, err := s.admins.IsAdmin(ctx, actor.ID)
	if err != nil {
		return err
	}
	if !yes {
		return ErrNotAdmin
	}
	if !actor.SecondFactor {
		return ErrSecondFactorNeeded
	}
	return nil
}

// Label is a Client's label, for the Security page; its id when it has none or is gone.
func (s *Service) Label(ctx context.Context, clientID string) string {
	c, err := s.clients.Client(ctx, clientID)
	if err != nil || c.Label == "" {
		return clientID
	}
	return c.Label
}

// Prune removes registrations nobody consented to within a day, and expired tokens.
func (s *Service) Prune(ctx context.Context) (int64, error) {
	now := s.now()
	regs, err := s.store.PruneRegistrations(ctx, now.Add(-unconsentedLifetime))
	if err != nil {
		return 0, err
	}
	tokens, err := s.store.PruneTokens(ctx, now)
	return regs + tokens, err
}

func hashOf(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}
