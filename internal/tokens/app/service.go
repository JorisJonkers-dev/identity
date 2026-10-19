// Package app holds the Personal Token use cases (docs/adr/0008, 0017): a User mints, lists and
// revokes their own; an Admin lists and revokes anyone's; and a Client's check finds who a token
// acts for and with which App Roles now.
package app

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/tokens/domain"
)

var (
	ErrNotFound      = errors.New("no such token")
	ErrUnknownClient = errors.New("no such Client")
	// ErrNoAccess is a token for a Client the User holds no App Role on: it could open nothing.
	ErrNoAccess = errors.New("the User holds no App Role on the Client")
	// ErrStepUpClient is a token for a Client with step-up on: a bearer token proves nothing fresh
	// (docs/adr/0019).
	ErrStepUpClient = errors.New("the Client wants a fresh two-factor proof")
	// ErrInactive is a token a Client's check refuses: revoked, expired, another Client's, or one
	// whose User holds no App Role on the Client now.
	ErrInactive           = errors.New("the token is not active")
	ErrNotAdmin           = errors.New("only Admins manage other Users' tokens")
	ErrSecondFactorNeeded = errors.New("other Users' tokens are managed only from a session proved with a Second Factor")
)

const secretBytes = 32

// Store keeps Personal Tokens by their hash.
type Store interface {
	Create(ctx context.Context, t domain.Token, hash []byte) error
	// ByHash is the token with a hash, ErrNotFound when there is none.
	ByHash(ctx context.Context, hash []byte) (domain.Token, error)
	// ByID is ErrNotFound when there is none.
	ByID(ctx context.Context, id uuid.UUID) (domain.Token, error)
	// OfUser is a User's tokens, newest first.
	OfUser(ctx context.Context, userID uuid.UUID) ([]domain.Token, error)
	// Revoke reports whether the User's live token was revoked now.
	Revoke(ctx context.Context, id, userID uuid.UUID, at time.Time) (bool, error)
	// RevokeForClient revokes every live token for a Client and returns them.
	RevokeForClient(ctx context.Context, clientID string, at time.Time) ([]domain.Token, error)
	// LiveForClient is how many tokens for a Client are live at now.
	LiveForClient(ctx context.Context, clientID string, now time.Time) (int, error)
	// Touch records a use, at most once a minute.
	Touch(ctx context.Context, id uuid.UUID, at time.Time) error
	// CountExpiring is the live tokens that expire after now and by before.
	CountExpiring(ctx context.Context, now, before time.Time) (int, error)
}

// Roles is the App Roles a User holds on a Client that it still declares; none for a Removed Client.
type Roles interface {
	Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error)
	// ByClient is every App Role a User holds, per Client, including Removed ones.
	ByClient(ctx context.Context, userID uuid.UUID) (map[string][]string, error)
}

// Clients tells whether a Client is in the cluster, and what it is called.
type Clients interface {
	Exists(ctx context.Context, clientID string) bool
	// StepUp reports whether the Client wants a fresh two-factor proof, which no token gives.
	StepUp(ctx context.Context, clientID string) (bool, error)
	// Label is what a Client is called, or its id when it has no label or is gone.
	Label(ctx context.Context, clientID string) string
}

// Client is a Client a token is for, by id and label.
type Client struct {
	ID, Label string
}

// Admins tells who is an Admin.
type Admins interface {
	IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error)
}

// Audit records minting and revoking a token.
type Audit interface {
	Record(ctx context.Context, e audit.Event)
}

// Actor is an Admin acting on another User's tokens, and whether their session proved a Second
// Factor.
type Actor struct {
	ID           uuid.UUID
	SecondFactor bool
}

// Minted is a new token: its secret, shown once, and what is kept of it.
type Minted struct {
	Secret string
	Token  domain.Token
}

// Active is what a Client's check learns of a live token.
type Active struct {
	Token domain.Token
	Roles []string
}

// Service is the Personal Token use cases.
type Service struct {
	store   Store
	roles   Roles
	clients Clients
	admins  Admins
	audit   Audit
	revoked func(ctx context.Context, tokenID uuid.UUID)
	now     func() time.Time
	random  io.Reader
}

// New returns a Service. random is where secrets and ids come from.
func New(store Store, roles Roles, clients Clients, admins Admins, audit Audit, now func() time.Time, random io.Reader) *Service {
	return &Service{store: store, roles: roles, clients: clients, admins: admins, audit: audit, now: now, random: random}
}

// OnRevoked tells f about each token revoked, so what has its checks cached can drop them now.
func (s *Service) OnRevoked(f func(ctx context.Context, tokenID uuid.UUID)) { s.revoked = f }

// Mint makes a token for one of the User's Clients, lasting days, and returns its secret once.
func (s *Service) Mint(ctx context.Context, userID uuid.UUID, rawName, clientID string, days int) (Minted, error) {
	name, err := domain.NewName(rawName)
	if err != nil {
		return Minted{}, err
	}
	lifetime, err := domain.NewLifetime(days)
	if err != nil {
		return Minted{}, err
	}
	if !s.clients.Exists(ctx, clientID) {
		return Minted{}, ErrUnknownClient
	}
	stepUp, err := s.clients.StepUp(ctx, clientID)
	if err != nil {
		return Minted{}, err
	}
	if stepUp {
		return Minted{}, ErrStepUpClient
	}
	roles, err := s.roles.Roles(ctx, userID, clientID)
	if err != nil {
		return Minted{}, err
	}
	if len(roles) == 0 {
		return Minted{}, ErrNoAccess
	}
	id, err := uuid.NewRandomFromReader(s.random)
	if err != nil {
		return Minted{}, err
	}
	raw := make([]byte, secretBytes)
	if _, err := io.ReadFull(s.random, raw); err != nil {
		return Minted{}, fmt.Errorf("read a token: %w", err)
	}
	secret := domain.Prefix + base64.RawURLEncoding.EncodeToString(raw)
	now := s.now()
	t := domain.Token{ID: id, UserID: userID, ClientID: clientID, Name: name.String(), CreatedAt: now, ExpiresAt: lifetime.From(now)}
	if err := s.store.Create(ctx, t, hashOf(secret)); err != nil {
		return Minted{}, err
	}
	s.audit.Record(ctx, audit.Event{Action: audit.TokenMinted, User: userID, Client: clientID})
	return Minted{Secret: secret, Token: t}, nil
}

// Mine is a User's own tokens, newest first.
func (s *Service) Mine(ctx context.Context, userID uuid.UUID) ([]domain.Token, error) {
	return s.store.OfUser(ctx, userID)
}

// Mintable is the Clients a User may mint a token for: those in the cluster they hold an App Role
// on, by label.
func (s *Service) Mintable(ctx context.Context, userID uuid.UUID) ([]Client, error) {
	held, err := s.roles.ByClient(ctx, userID)
	if err != nil {
		return nil, err
	}
	clients := make([]Client, 0, len(held))
	for id, roles := range held {
		if len(roles) == 0 || !s.clients.Exists(ctx, id) {
			continue
		}
		stepUp, err := s.clients.StepUp(ctx, id)
		if err != nil {
			return nil, err
		}
		if !stepUp {
			clients = append(clients, Client{ID: id, Label: s.clients.Label(ctx, id)})
		}
	}
	slices.SortFunc(clients, func(a, b Client) int { return cmp.Or(cmp.Compare(a.Label, b.Label), cmp.Compare(a.ID, b.ID)) })
	return clients, nil
}

// RolesOf is the App Roles a token carries now: its User's on its Client.
func (s *Service) RolesOf(ctx context.Context, t domain.Token) ([]string, error) {
	return s.roles.Roles(ctx, t.UserID, t.ClientID)
}

// Label is what a token's Client is called.
func (s *Service) Label(ctx context.Context, clientID string) string {
	return s.clients.Label(ctx, clientID)
}

// State is how a token stands now.
func (s *Service) State(t domain.Token) domain.State { return t.State(s.now()) }

// RevokeMine revokes one of the User's own tokens; ErrNotFound for one that is not theirs.
func (s *Service) RevokeMine(ctx context.Context, userID, tokenID uuid.UUID) error {
	return s.revoke(ctx, uuid.Nil, userID, tokenID)
}

// Of is a User's tokens, for an Admin.
func (s *Service) Of(ctx context.Context, actor Actor, userID uuid.UUID) ([]domain.Token, error) {
	if err := s.admin(ctx, actor); err != nil {
		return nil, err
	}
	return s.store.OfUser(ctx, userID)
}

// RevokeAs revokes a User's token, as an Admin.
func (s *Service) RevokeAs(ctx context.Context, actor Actor, userID, tokenID uuid.UUID) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	return s.revoke(ctx, actor.ID, userID, tokenID)
}

// revoke ends a User's token, recording who did it; revoking it twice is not an error and records
// nothing more.
func (s *Service) revoke(ctx context.Context, actor, userID, tokenID uuid.UUID) error {
	t, err := s.store.ByID(ctx, tokenID)
	if err != nil {
		return err
	}
	if t.UserID != userID {
		return ErrNotFound
	}
	revoked, err := s.store.Revoke(ctx, tokenID, userID, s.now())
	if err != nil {
		return err
	}
	if revoked {
		s.audit.Record(ctx, audit.Event{Action: audit.TokenRevoked, Actor: actor, User: userID, Client: t.ClientID})
		if s.revoked != nil {
			s.revoked(ctx, tokenID)
		}
	}
	return nil
}

// LiveForClient is how many live tokens a Client has, for the Admin about to turn step-up on.
func (s *Service) LiveForClient(ctx context.Context, clientID string) (int, error) {
	return s.store.LiveForClient(ctx, clientID, s.now())
}

// RevokeForClient revokes every live token for a Client, as turning step-up on does; each is an
// Audit Event by actor, and stops at once wherever its checks are cached.
func (s *Service) RevokeForClient(ctx context.Context, actor uuid.UUID, clientID string) error {
	revoked, err := s.store.RevokeForClient(ctx, clientID, s.now())
	if err != nil {
		return err
	}
	for _, t := range revoked {
		s.audit.Record(ctx, audit.Event{Action: audit.TokenRevoked, Actor: actor, User: t.UserID, Client: clientID})
		if s.revoked != nil {
			s.revoked(ctx, t.ID)
		}
	}
	return nil
}

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

// Resolve finds the token a secret names, live or not, for the Provider to check (docs/adr/0017).
func (s *Service) Resolve(ctx context.Context, secret string) (domain.Token, error) {
	if !strings.HasPrefix(secret, domain.Prefix) {
		return domain.Token{}, ErrNotFound
	}
	return s.store.ByHash(ctx, hashOf(secret))
}

// Check is a Client's check of a token it was given: ErrInactive unless the token is that
// Client's, live, and its User holds an App Role there now. A use is recorded.
func (s *Service) Check(ctx context.Context, tokenID uuid.UUID, clientID string) (Active, error) {
	t, err := s.store.ByID(ctx, tokenID)
	if errors.Is(err, ErrNotFound) {
		return Active{}, ErrInactive
	}
	if err != nil {
		return Active{}, err
	}
	now := s.now()
	if t.ClientID != clientID || !t.Live(now) {
		return Active{}, ErrInactive
	}
	stepUp, err := s.clients.StepUp(ctx, clientID)
	if err != nil {
		return Active{}, err
	}
	if stepUp {
		return Active{}, ErrInactive
	}
	roles, err := s.roles.Roles(ctx, t.UserID, clientID)
	if err != nil {
		return Active{}, err
	}
	if len(roles) == 0 {
		return Active{}, ErrInactive
	}
	if err := s.store.Touch(ctx, t.ID, now); err != nil {
		return Active{}, err
	}
	return Active{Token: t, Roles: roles}, nil
}

// ExpiringSoon is how many live tokens end within seven days, for the dashboard.
func (s *Service) ExpiringSoon(ctx context.Context) (int, error) {
	now := s.now()
	return s.store.CountExpiring(ctx, now, now.Add(domain.Soon))
}

func hashOf(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}
