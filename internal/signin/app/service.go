// Package app holds the sign-in use cases: keeping authorization requests, completing them from a
// Single sign-on session, and answering for Clients and Users. The OIDC protocol itself is
// zitadel/oidc's (docs/adr/0001); this package is what its storage asks of identity.
package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"slices"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// requestLifetime bounds how long a person has between /authorize and redeeming the code.
const requestLifetime = 10 * time.Minute

var (
	ErrNotFound      = errors.New("not found")
	ErrNoSession     = errors.New("not signed in")
	ErrInvalidClient = errors.New("unknown client or wrong secret")
	ErrNoAccess      = errors.New("no App Role on this Client")
)

// Requests stores authorization requests.
type Requests interface {
	CreateRequest(ctx context.Context, r domain.AuthRequest) error
	RequestByID(ctx context.Context, id uuid.UUID) (domain.AuthRequest, error)
	// TakeByCodeHash removes and returns the request a code was issued for, atomically.
	TakeByCodeHash(ctx context.Context, codeHash []byte) (domain.AuthRequest, error)
	// CompleteRequest records s on a request no one has completed yet, reporting whether it did.
	CompleteRequest(ctx context.Context, id uuid.UUID, s domain.SignIn) (bool, error)
	// SaveCode records the code of a completed request that has none yet, reporting whether it did.
	SaveCode(ctx context.Context, id uuid.UUID, codeHash []byte) (bool, error)
	DeleteRequest(ctx context.Context, id uuid.UUID) error
	// AddSessionClient records that a session signed into a Client.
	AddSessionClient(ctx context.Context, sessionID uuid.UUID, clientID string) error
	// TakeSessionClients removes and returns the Clients the sessions signed into.
	TakeSessionClients(ctx context.Context, sessionIDs []uuid.UUID) ([]SessionClient, error)
}

// SessionClient is a Client a Single sign-on session signed into.
type SessionClient struct {
	SessionID uuid.UUID
	ClientID  string
}

// Logouts tells a native Client that a session it signed into ended, by OIDC back-channel logout.
type Logouts interface {
	Send(ctx context.Context, c domain.Client, userID, sessionID uuid.UUID)
}

// Clients finds Clients, by id or by host; a Removed Client is found by neither.
type Clients interface {
	Client(ctx context.Context, id string) (domain.Client, error)
	ClientForHost(ctx context.Context, host string) (domain.Client, error)
}

// Claims is what an ID token and userinfo say about a User.
type Claims struct {
	Email string
	Name  string
}

// Accounts is the users context as sign-in needs it.
type Accounts interface {
	// SignedIn resolves a session token; ErrNoSession when it names no live session.
	SignedIn(ctx context.Context, sessionToken string) (domain.SignIn, error)
	Claims(ctx context.Context, userID uuid.UUID) (Claims, error)
	SignOutEverywhere(ctx context.Context, userID uuid.UUID) error
}

// Access is the access context as sign-in needs it (docs/adr/0004).
type Access interface {
	// Roles is the App Roles a User's Groups hold on a Client.
	Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error)
}

// Service is the sign-in use cases.
type Service struct {
	requests Requests
	clients  Clients
	accounts Accounts
	access   Access
	logouts  Logouts
	audit    Audit
	now      func() time.Time
	random   io.Reader
}

// Audit records each sign-in to a Client, and each refused for want of an App Role.
type Audit interface {
	Record(ctx context.Context, e audit.Event)
}

// New returns a Service. random is where request ids come from.
func New(requests Requests, clients Clients, accounts Accounts, access Access, logouts Logouts, audit Audit, now func() time.Time, random io.Reader) *Service {
	return &Service{requests: requests, clients: clients, accounts: accounts, access: access, logouts: logouts, audit: audit, now: now, random: random}
}

// Start keeps r, as /authorize received it, for requestLifetime, and returns it with its id.
func (s *Service) Start(ctx context.Context, r domain.AuthRequest) (domain.AuthRequest, error) {
	id, err := uuid.NewRandomFromReader(s.random)
	if err != nil {
		return domain.AuthRequest{}, err
	}
	r.ID, r.SignIn = id, nil
	r.CreatedAt = s.now()
	r.ExpiresAt = r.CreatedAt.Add(requestLifetime)
	if err := s.requests.CreateRequest(ctx, r); err != nil {
		return domain.AuthRequest{}, err
	}
	return r, nil
}

// Request returns a request that has not expired.
func (s *Service) Request(ctx context.Context, id uuid.UUID) (domain.AuthRequest, error) {
	return s.live(s.requests.RequestByID(ctx, id))
}

// Redeem spends a code: it returns the unexpired request the code was issued for, and no later
// call with that code finds it. A failed exchange spends the code too.
func (s *Service) Redeem(ctx context.Context, code string) (domain.AuthRequest, error) {
	return s.live(s.requests.TakeByCodeHash(ctx, hashOf(code)))
}

func (s *Service) live(r domain.AuthRequest, err error) (domain.AuthRequest, error) {
	if err != nil {
		return domain.AuthRequest{}, err
	}
	if !s.now().Before(r.ExpiresAt) {
		return domain.AuthRequest{}, ErrNotFound
	}
	return r, nil
}

// Complete lets the Single sign-on session behind sessionToken vouch for request id. It fails with
// ErrNoSession when there is no live session, so the caller asks the person to sign in, and with
// ErrNoAccess when the User holds no App Role on the Client, so no code is issued.
func (s *Service) Complete(ctx context.Context, id uuid.UUID, sessionToken string) error {
	r, err := s.Request(ctx, id)
	if err != nil {
		return err
	}
	signIn, err := s.accounts.SignedIn(ctx, sessionToken)
	if err != nil {
		return err
	}
	roles, err := s.Roles(ctx, signIn.UserID, r.ClientID)
	if err != nil {
		return err
	}
	if len(roles) == 0 {
		s.audit.Record(ctx, audit.Event{Action: audit.SignInFailed, User: signIn.UserID, Client: r.ClientID, Reason: audit.ReasonNoAccess})
		return ErrNoAccess
	}
	done, err := s.requests.CompleteRequest(ctx, id, signIn)
	if err != nil {
		return err
	}
	if !done {
		return ErrNotFound
	}
	if err := s.requests.AddSessionClient(ctx, signIn.SessionID, r.ClientID); err != nil {
		return err
	}
	s.audit.Record(ctx, audit.Event{Action: audit.SignedIn, User: signIn.UserID, Client: r.ClientID})
	return nil
}

// SignedOut sends a logout token to every native Client the ended sessions signed into that takes
// one (docs/adr/0007). A Client that has since gone, or takes none, is skipped.
func (s *Service) SignedOut(ctx context.Context, userID uuid.UUID, sessionIDs []uuid.UUID) error {
	signed, err := s.requests.TakeSessionClients(ctx, sessionIDs)
	if err != nil {
		return err
	}
	for _, sc := range signed {
		c, err := s.clients.Client(ctx, sc.ClientID)
		if err != nil || c.BackChannelLogoutURI == "" {
			continue
		}
		s.logouts.Send(ctx, c, userID, sc.SessionID)
	}
	return nil
}

// SaveCode records the code issued for a completed request; a request gets one code only.
func (s *Service) SaveCode(ctx context.Context, id uuid.UUID, code string) error {
	saved, err := s.requests.SaveCode(ctx, id, hashOf(code))
	if err != nil {
		return err
	}
	if !saved {
		return ErrNotFound
	}
	return nil
}

// Finish forgets a request once its code is redeemed.
func (s *Service) Finish(ctx context.Context, id uuid.UUID) error {
	return s.requests.DeleteRequest(ctx, id)
}

// Client returns a Client that signs in over OIDC itself: a native one. Gated and token Clients
// are not OIDC clients, so they are not found here.
func (s *Service) Client(ctx context.Context, id string) (domain.Client, error) {
	c, err := s.clients.Client(ctx, id)
	if err != nil {
		return domain.Client{}, err
	}
	if c.SignIn != domain.Native {
		return domain.Client{}, ErrNotFound
	}
	return c, nil
}

// ClientForHost returns the Client a host belongs to, whatever its mode. A host with no Client is
// refused with ErrNotFound.
func (s *Service) ClientForHost(ctx context.Context, host string) (domain.Client, error) {
	return s.clients.ClientForHost(ctx, host)
}

// AuthorizeSecret checks a confidential Client's secret, in constant time. A public Client, an
// unknown one and a wrong secret all fail with ErrInvalidClient.
func (s *Service) AuthorizeSecret(ctx context.Context, clientID, secret string) error {
	c, err := s.Client(ctx, clientID)
	if errors.Is(err, ErrNotFound) {
		return ErrInvalidClient
	}
	if err != nil {
		return err
	}
	if c.Public() || len(c.SecretHash) == 0 || subtle.ConstantTimeCompare(hashOf(secret), c.SecretHash) != 1 {
		return ErrInvalidClient
	}
	return nil
}

// Roles is the App Roles a User holds on a Client that the Client still declares, in the Client's
// order. A token carries these and nothing about Groups. A Removed Client is ErrNotFound.
func (s *Service) Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error) {
	c, err := s.clients.Client(ctx, clientID)
	if err != nil {
		return nil, err
	}
	held, err := s.access.Roles(ctx, userID, clientID)
	if err != nil {
		return nil, err
	}
	roles := []string{}
	for _, role := range c.Roles {
		if slices.Contains(held, role) {
			roles = append(roles, role)
		}
	}
	return roles, nil
}

// Claims returns what tokens say about a User.
func (s *Service) Claims(ctx context.Context, userID uuid.UUID) (Claims, error) {
	return s.accounts.Claims(ctx, userID)
}

// SignOutEverywhere ends every session of a User (docs/adr/0007).
func (s *Service) SignOutEverywhere(ctx context.Context, userID uuid.UUID) error {
	return s.accounts.SignOutEverywhere(ctx, userID)
}

func hashOf(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}
