package app

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// secretBytes is how much randomness a Client secret carries.
const secretBytes = 32

var (
	// ErrNoSecretPath is a Secret Client whose object names nowhere to keep its secret.
	ErrNoSecretPath = errors.New("a Secret Client with no secretPath")
	// ErrNoSecret is a Client that signs in without a secret, so it has none to rotate.
	ErrNoSecret             = errors.New("the Client has no secret")
	ErrNotAdmin             = errors.New("only Admins can do this")
	ErrTwoFactorProofNeeded = errors.New("a session that proved two-factor is needed")
)

// SecretHashes keeps the hashes of Secret Clients' secrets, shared by every replica.
type SecretHashes interface {
	// Provision runs mint, holding a lock every replica takes for the Client, when the Client has no
	// secret yet, and keeps the hash mint returns.
	Provision(ctx context.Context, clientID string, mint func() ([]byte, error)) error
	// Rotate runs mint holding that lock and makes the hash it returns the current one. The hash it
	// replaces stays accepted, unless an earlier one already is.
	Rotate(ctx context.Context, clientID string, mint func() ([]byte, error)) error
	// Hashes is the current hash and the previous one still accepted, nil when none; ErrNotFound
	// when the Client has no secret.
	Hashes(ctx context.Context, clientID string) (current, previous []byte, err error)
	// EndPrevious stops accepting the previous secret, while current is still the current one.
	EndPrevious(ctx context.Context, clientID string, current []byte) error
}

// SecretVault is the Secret Store path where an Application reads its Client's id and secret
// (deploy-kit chapter 17).
type SecretVault interface {
	// Read is the secret at path; ErrNotFound when there is none.
	Read(ctx context.Context, path string) (string, error)
	// Create writes a Client's id and secret at path, and is refused when a secret is there.
	Create(ctx context.Context, path, clientID, secret string) error
	// Write replaces the secret at path.
	Write(ctx context.Context, path, clientID, secret string) error
}

// AllClients is every Client the cluster declares, and each by id.
type AllClients interface {
	Clients
	All() []domain.Client
}

// Admins says who is an Admin.
type Admins interface {
	IsAdmin(ctx context.Context, id uuid.UUID) (bool, error)
}

// Actor is who asks: a User, and whether their session proved two-factor.
type Actor struct {
	ID        uuid.UUID
	TwoFactor bool
}

// ClientSecrets gives each Secret Client a secret in its Application's Secret Store path, checks it
// at the token endpoint, and lets Admins rotate it. identity keeps only the hashes (identity#32).
type ClientSecrets struct {
	hashes  SecretHashes
	vault   SecretVault
	clients AllClients
	admins  Admins
	audit   Audit
	random  io.Reader
}

// NewClientSecrets returns ClientSecrets; random is where secrets come from.
func NewClientSecrets(hashes SecretHashes, vault SecretVault, clients AllClients, admins Admins, audit Audit, random io.Reader) *ClientSecrets {
	return &ClientSecrets{hashes: hashes, vault: vault, clients: clients, admins: admins, audit: audit, random: random}
}

// ProvisionAll gives every Secret Client without a secret one: the secret already at its path, or a
// new one written there. A Client that fails does not stop the others.
func (s *ClientSecrets) ProvisionAll(ctx context.Context) error {
	var errs []error
	for _, c := range s.clients.All() {
		if c.Credential != domain.Secret {
			continue
		}
		if err := s.provision(ctx, c); err != nil {
			errs = append(errs, fmt.Errorf("client %s: %w", c.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (s *ClientSecrets) provision(ctx context.Context, c domain.Client) error {
	if c.SecretPath == "" {
		return ErrNoSecretPath
	}
	return s.hashes.Provision(ctx, c.ID, func() ([]byte, error) {
		secret, err := s.vault.Read(ctx, c.SecretPath)
		if errors.Is(err, ErrNotFound) {
			if secret, err = s.newSecret(); err != nil {
				return nil, err
			}
			err = s.vault.Create(ctx, c.SecretPath, c.ID, secret)
		}
		if err != nil {
			return nil, err
		}
		return hashOf(secret), nil
	})
}

// Authorize checks a Client's secret in constant time: its current secret, or the one a rotation
// replaced, until the current one is first used.
func (s *ClientSecrets) Authorize(ctx context.Context, clientID, secret string) error {
	return checkSecret(ctx, s.hashes, clientID, secret)
}

// checkSecret checks a secret kept under id in hashes, a Client's or a Machine Identity's.
func checkSecret(ctx context.Context, hashes SecretHashes, id, secret string) error {
	current, previous, err := hashes.Hashes(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return ErrInvalidClient
	}
	if err != nil {
		return err
	}
	h := hashOf(secret)
	if subtle.ConstantTimeCompare(h, current) == 1 {
		if previous == nil {
			return nil
		}
		return hashes.EndPrevious(ctx, id, current)
	}
	if subtle.ConstantTimeCompare(h, previous) == 1 {
		return nil
	}
	return ErrInvalidClient
}

// Rotate writes a new secret where the Client's Application reads it, as an Admin. The old secret
// keeps working until the Application first uses the new one.
func (s *ClientSecrets) Rotate(ctx context.Context, actor Actor, clientID string) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	c, err := s.clients.Client(ctx, clientID)
	if err != nil {
		return err
	}
	if c.Credential != domain.Secret {
		return ErrNoSecret
	}
	if c.SecretPath == "" {
		return ErrNoSecretPath
	}
	err = s.hashes.Rotate(ctx, c.ID, func() ([]byte, error) {
		secret, err := s.newSecret()
		if err != nil {
			return nil, err
		}
		if err := s.vault.Write(ctx, c.SecretPath, c.ID, secret); err != nil {
			return nil, err
		}
		return hashOf(secret), nil
	})
	if err != nil {
		return err
	}
	s.audit.Record(ctx, audit.Event{Action: audit.ClientSecretRotated, Actor: actor.ID, Client: c.ID})
	return nil
}

func (s *ClientSecrets) admin(ctx context.Context, actor Actor) error {
	return admin(ctx, s.admins, actor)
}

// admin lets an Admin through, and only from a session that proved two-factor.
func admin(ctx context.Context, admins Admins, actor Actor) error {
	yes, err := admins.IsAdmin(ctx, actor.ID)
	if err != nil {
		return err
	}
	if !yes {
		return ErrNotAdmin
	}
	if !actor.TwoFactor {
		return ErrTwoFactorProofNeeded
	}
	return nil
}

func (s *ClientSecrets) newSecret() (string, error) { return newSecret(s.random) }

func newSecret(random io.Reader) (string, error) {
	b := make([]byte, secretBytes)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
