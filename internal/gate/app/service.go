// Package app holds the Gate's use cases (docs/adr/0003, 0009): it signs Users in for a Client
// whose app cannot speak OIDC, one host at a time, and answers Traefik's check for every request
// from memory.
package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
)

var (
	ErrUnknownHost = errors.New("no gated Client has this host")
	ErrNoSignIn    = errors.New("not signed in on this host")
	ErrNoAccess    = errors.New("no App Role on this Client")
	ErrInvalidCode = errors.New("the sign-in code is wrong, used or expired")
	ErrNotFound    = errors.New("not found")
	// ErrBearerNeeded is a request to a token host without a Personal Token: it has no sign-in page.
	ErrBearerNeeded = errors.New("this host takes only Personal Tokens")
	// ErrStepUpNeeded is a sign-in to a Step-up Client whose session's two-factor proof is older
	// than the Client's window (docs/adr/0019).
	ErrStepUpNeeded = errors.New("the Client wants a fresher two-factor proof")
	// ErrTwoFactorNeeded is a sign-in to a Step-up Client by a User with no two-factor.
	ErrTwoFactorNeeded = errors.New("the Client wants two-factor, and the User has none")
)

const (
	// A code carries a sign-in from the Identity Provider to the host; it is redeemed at once.
	codeLifetime = 2 * time.Minute
	tokenBytes   = 32
	// maxCached bounds the check cache; past it the cache starts again.
	maxCached = 50_000
)

// Client is a gated Client as the Gate needs it.
type Client struct {
	ID    string
	Label string
	Host  string
	// StepUp is how fresh the Client wants two-factor; 0 when it does not. A host sign-in for it
	// lasts only this long.
	StepUp time.Duration
}

// Person is a signed-in User, and what vouches for them: their Single sign-on session, or, for a
// request carrying a Personal Token, that token, so revoking it can Forget its cached checks.
type Person struct {
	UserID    uuid.UUID
	Email     string
	Name      string
	SessionID uuid.UUID
	// TwoFactorAt is when the session last proved two-factor; zero when it never did.
	TwoFactorAt time.Time
	// TwoFactor is whether the User added an authenticator app or a passkey.
	TwoFactor bool
}

// Verdict is what the Gate tells the app about the User of a request.
type Verdict struct {
	UserID uuid.UUID
	Email  string
	Name   string
	Roles  []string
}

// Code is a sign-in on its way from the Identity Provider to a host.
type Code struct {
	Hash       []byte
	Host       string
	SessionID  uuid.UUID
	StateHash  []byte
	ReturnPath string
	ExpiresAt  time.Time
}

// Session is a host's sign-in, made from a Single sign-on session.
type Session struct {
	Host      string
	SessionID uuid.UUID
	CreatedAt time.Time
}

// Clients finds gated Clients by host: ErrUnknownHost for a host with none, or one not gated.
type Clients interface {
	GatedClient(ctx context.Context, host string) (Client, error)
	// BearerClient is the gated or token Client a host belongs to, which takes Personal Tokens;
	// ErrUnknownHost for any other.
	BearerClient(ctx context.Context, host string) (Client, error)
}

// Tokens checks a Personal Token for a Client (docs/adr/0017): the User it acts for and the App
// Roles they hold there now; ErrNoSignIn for one that is not active.
type Tokens interface {
	Bearer(ctx context.Context, secret, clientID string) (Person, []string, error)
}

// Roles is the App Roles a User holds on a Client that it declares.
type Roles interface {
	Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error)
}

// Accounts is the users context as the Gate needs it; ErrNoSignIn when no live session answers.
type Accounts interface {
	SignedIn(ctx context.Context, ssoToken string) (Person, error)
	Vouch(ctx context.Context, sessionID uuid.UUID) (Person, error)
}

// Store keeps codes and host sessions, by hash.
type Store interface {
	CreateCode(ctx context.Context, c Code) error
	// TakeCode removes and returns a code; ErrNotFound when there is none.
	TakeCode(ctx context.Context, hash []byte) (Code, error)
	CreateSession(ctx context.Context, tokenHash []byte, s Session, at time.Time) error
	// SessionByHash is ErrNotFound when there is none.
	SessionByHash(ctx context.Context, tokenHash []byte) (Session, error)
}

type cached struct {
	verdict Verdict
	err     error
	until   time.Time
	session uuid.UUID
}

// Service is the Gate's use cases.
type Service struct {
	store    Store
	clients  Clients
	roles    Roles
	accounts Accounts
	tokens   Tokens
	audit    Audit
	now      func() time.Time
	random   io.Reader
	ttl      time.Duration

	mu    sync.RWMutex
	cache map[string]cached
}

// Audit records each sign-in to a gated Client, and each refused for want of an App Role.
type Audit interface {
	Record(ctx context.Context, e audit.Event)
}

// New returns a Service whose checks are cached for ttl: a role taken away or a sign-out reaches a
// host within ttl.
func New(store Store, clients Clients, roles Roles, accounts Accounts, audit Audit, now func() time.Time, random io.Reader, ttl time.Duration) *Service {
	return &Service{store: store, clients: clients, roles: roles, accounts: accounts, audit: audit, now: now, random: random, ttl: ttl, cache: map[string]cached{}}
}

// SetTokens lets gated and token hosts take Personal Tokens; until it is called, none does. It is
// set once, before the Gate serves.
func (s *Service) SetTokens(t Tokens) { s.tokens = t }

// CheckBearer answers Traefik for a request carrying a Personal Token, on a gated or a token host.
// Its answer is cached as a cookie's is, so a revoked token reaches a host within the cache's ttl.
func (s *Service) CheckBearer(ctx context.Context, host, secret string) (Verdict, error) {
	key := "bearer\x00" + host + "\x00" + string(hashOf(secret))
	return s.cached(key, func() (Verdict, uuid.UUID, time.Time, error) {
		client, err := s.clients.BearerClient(ctx, host)
		if err != nil {
			return Verdict{}, uuid.Nil, time.Time{}, err
		}
		if s.tokens == nil {
			return Verdict{}, uuid.Nil, time.Time{}, ErrNoSignIn
		}
		p, roles, err := s.tokens.Bearer(ctx, secret, client.ID)
		if err != nil {
			return Verdict{}, uuid.Nil, time.Time{}, err
		}
		return Verdict{UserID: p.UserID, Email: p.Email, Name: p.Name, Roles: roles}, p.SessionID, time.Time{}, nil
	})
}

// Client is the gated Client a host belongs to.
func (s *Service) Client(ctx context.Context, host string) (Client, error) {
	return s.clients.GatedClient(ctx, host)
}

// Check answers Traefik for one request to a gated host carrying the host's cookie. Its answer is
// kept for the cache's ttl, so most requests touch nothing but memory.
func (s *Service) Check(ctx context.Context, host, cookie string) (Verdict, error) {
	if cookie == "" {
		_, err := s.clients.GatedClient(ctx, host)
		if errors.Is(err, ErrUnknownHost) {
			if _, bearer := s.clients.BearerClient(ctx, host); bearer == nil {
				return Verdict{}, ErrBearerNeeded
			}
		}
		if err != nil {
			return Verdict{}, err
		}
		return Verdict{}, ErrNoSignIn
	}
	hash := hashOf(cookie)
	return s.cached(host+"\x00"+string(hash), func() (Verdict, uuid.UUID, time.Time, error) { return s.check(ctx, host, hash) })
}

// cached answers a check from memory while its answer is fresh, and keeps a fresh answer for ttl,
// or until the sign-in it rests on ends if that is sooner, with the Single sign-on session it came
// from, so Forget can drop it: an allowed User, or a refusal for want of a sign-in or an App Role.
// Any other failure is asked again next time.
func (s *Service) cached(key string, check func() (Verdict, uuid.UUID, time.Time, error)) (Verdict, error) {
	now := s.now()
	s.mu.RLock()
	c, ok := s.cache[key]
	s.mu.RUnlock()
	if ok && now.Before(c.until) {
		return c.verdict, c.err
	}
	v, session, ends, err := check()
	if err == nil || errors.Is(err, ErrNoAccess) || errors.Is(err, ErrNoSignIn) {
		until := now.Add(s.ttl)
		if !ends.IsZero() && ends.Before(until) {
			until = ends
		}
		s.mu.Lock()
		if len(s.cache) >= maxCached {
			s.cache = map[string]cached{}
		}
		s.cache[key] = cached{verdict: v, err: err, until: until, session: session}
		s.mu.Unlock()
	}
	return v, err
}

// check is a host cookie's verdict, and when the host sign-in ends: for a Step-up Client, its
// window after the sign-in; otherwise never (the zero time).
func (s *Service) check(ctx context.Context, host string, hash []byte) (Verdict, uuid.UUID, time.Time, error) {
	client, err := s.clients.GatedClient(ctx, host)
	if err != nil {
		return Verdict{}, uuid.Nil, time.Time{}, err
	}
	gs, err := s.store.SessionByHash(ctx, hash)
	if errors.Is(err, ErrNotFound) || err == nil && gs.Host != host {
		return Verdict{}, uuid.Nil, time.Time{}, ErrNoSignIn
	}
	if err != nil {
		return Verdict{}, uuid.Nil, time.Time{}, err
	}
	var ends time.Time
	if client.StepUp > 0 {
		ends = gs.CreatedAt.Add(client.StepUp)
		if !s.now().Before(ends) {
			return Verdict{}, gs.SessionID, time.Time{}, ErrNoSignIn
		}
	}
	p, err := s.accounts.Vouch(ctx, gs.SessionID)
	if err != nil {
		return Verdict{}, gs.SessionID, ends, err
	}
	v, err := s.verdict(ctx, client, p)
	return v, gs.SessionID, ends, err
}

// Forget drops the cached checks of ended Single sign-on sessions and revoked Personal Tokens, by
// their ids, so the end reaches every gated host at once rather than when the cache runs out. On
// other replicas the cache's ttl still bounds it.
func (s *Service) Forget(sessionIDs []uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, c := range s.cache {
		if slices.Contains(sessionIDs, c.session) {
			delete(s.cache, key)
		}
	}
}

func (s *Service) verdict(ctx context.Context, client Client, p Person) (Verdict, error) {
	roles, err := s.roles.Roles(ctx, p.UserID, client.ID)
	if err != nil {
		return Verdict{}, err
	}
	if len(roles) == 0 {
		return Verdict{}, ErrNoAccess
	}
	return Verdict{UserID: p.UserID, Email: p.Email, Name: p.Name, Roles: roles}, nil
}

// Start runs on the Identity Provider's host, where the Single sign-on session is: it checks the
// User may use the host's Client and returns the code to carry back to the host. The state is
// the one the host's page set in that browser.
func (s *Service) Start(ctx context.Context, ssoToken, host, state, returnPath string) (string, error) {
	client, err := s.clients.GatedClient(ctx, host)
	if err != nil {
		return "", err
	}
	if state == "" {
		return "", ErrInvalidCode
	}
	p, err := s.accounts.SignedIn(ctx, ssoToken)
	if err != nil {
		return "", err
	}
	if _, err := s.verdict(ctx, client, p); err != nil {
		if errors.Is(err, ErrNoAccess) {
			s.audit.Record(ctx, audit.Event{Action: audit.SignInFailed, User: p.UserID, Client: client.ID, Reason: audit.ReasonNoAccess})
		}
		return "", err
	}
	if client.StepUp > 0 && s.now().Sub(p.TwoFactorAt) > client.StepUp {
		if !p.TwoFactor {
			return "", ErrTwoFactorNeeded
		}
		return "", ErrStepUpNeeded
	}
	code, err := s.token()
	if err != nil {
		return "", err
	}
	c := Code{
		Hash: hashOf(code), Host: host, SessionID: p.SessionID, StateHash: hashOf(state),
		ReturnPath: SafePath(returnPath), ExpiresAt: s.now().Add(codeLifetime),
	}
	if err := s.store.CreateCode(ctx, c); err != nil {
		return "", err
	}
	s.audit.Record(ctx, audit.Event{Action: audit.SignedIn, User: p.UserID, Client: client.ID})
	return code, nil
}

// Finish runs on the host's /.auth/callback: it redeems the code once, in the browser that holds
// the state it was started with, and returns the host cookie and where to go.
func (s *Service) Finish(ctx context.Context, host, code, state string) (string, string, error) {
	c, err := s.store.TakeCode(ctx, hashOf(code))
	if errors.Is(err, ErrNotFound) {
		return "", "", ErrInvalidCode
	}
	if err != nil {
		return "", "", err
	}
	now := s.now()
	if !now.Before(c.ExpiresAt) || c.Host != host || subtle.ConstantTimeCompare(c.StateHash, hashOf(state)) != 1 {
		return "", "", ErrInvalidCode
	}
	token, err := s.token()
	if err != nil {
		return "", "", err
	}
	if err := s.store.CreateSession(ctx, hashOf(token), Session{Host: host, SessionID: c.SessionID}, now); err != nil {
		return "", "", err
	}
	return token, c.ReturnPath, nil
}

// NewState is a fresh state for a host's sign-in page to set in its cookie.
func (s *Service) NewState() (string, error) { return s.token() }

// SafePath is p when it is a path on the same host, and "/" otherwise, so a sign-in never sends the
// browser off the host it came from.
func SafePath(p string) string {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.ContainsAny(p, "\\\r\n") {
		return "/"
	}
	return p
}

func (s *Service) token() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := io.ReadFull(s.random, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashOf(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
