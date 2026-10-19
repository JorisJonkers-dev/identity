package app

import (
	"context"
	"errors"
	"io"
	"slices"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
)

var (
	// ErrInvalidServiceAccountToken is a ServiceAccount token whose signature, issuer, audience or
	// expiry is wrong.
	ErrInvalidServiceAccountToken = errors.New("not a ServiceAccount token for identity")
	// ErrUnknownMachine is a credential no Machine Identity holds.
	ErrUnknownMachine = errors.New("no Machine Identity is bound to that credential")
)

// MachineIdentity is a Machine Identity as sign-in needs it (identity#22).
type MachineIdentity struct {
	ID             uuid.UUID
	Name           string
	Namespace      string
	ServiceAccount string
}

// Machines is the access context's Machine Identities.
type Machines interface {
	// Machine is ErrNotFound when there is none.
	Machine(ctx context.Context, id uuid.UUID) (MachineIdentity, error)
	// MachineByBinding is the Machine Identity bound to a ServiceAccount, or ErrNotFound.
	MachineByBinding(ctx context.Context, namespace, serviceAccount string) (MachineIdentity, error)
	MachineRoles(ctx context.Context, id uuid.UUID, clientID string) ([]string, error)
}

// ServiceAccountTokens checks a projected Kubernetes ServiceAccount token (docs/adr/0008).
type ServiceAccountTokens interface {
	// Verify is the namespace and ServiceAccount a token was issued to, or
	// ErrInvalidServiceAccountToken.
	Verify(ctx context.Context, token string) (namespace, serviceAccount string, err error)
}

// MachineSignIn signs Machine Identities in: inside the cluster with their ServiceAccount token,
// outside it with a client secret Admins rotate. Either way the access token carries the Machine
// Identity's own App Roles on one Client.
type MachineSignIn struct {
	machines Machines
	tokens   ServiceAccountTokens
	hashes   SecretHashes
	clients  Clients
	admins   Admins
	audit    Audit
	random   io.Reader
}

// NewMachineSignIn returns MachineSignIn; random is where secrets come from.
func NewMachineSignIn(machines Machines, tokens ServiceAccountTokens, hashes SecretHashes, clients Clients, admins Admins, audit Audit, random io.Reader) *MachineSignIn {
	return &MachineSignIn{machines: machines, tokens: tokens, hashes: hashes, clients: clients, admins: admins, audit: audit, random: random}
}

// Exchange is the Machine Identity a ServiceAccount token belongs to.
func (m *MachineSignIn) Exchange(ctx context.Context, token string) (MachineIdentity, error) {
	namespace, serviceAccount, err := m.tokens.Verify(ctx, token)
	if err != nil {
		return MachineIdentity{}, err
	}
	mi, err := m.machines.MachineByBinding(ctx, namespace, serviceAccount)
	if errors.Is(err, ErrNotFound) {
		return MachineIdentity{}, ErrUnknownMachine
	}
	return mi, err
}

// Machine is the Machine Identity with an id, or ErrUnknownMachine.
func (m *MachineSignIn) Machine(ctx context.Context, id uuid.UUID) (MachineIdentity, error) {
	mi, err := m.machines.Machine(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return MachineIdentity{}, ErrUnknownMachine
	}
	return mi, err
}

// Authorize checks a Machine Identity's client secret: its current one, or the one a rotation
// replaced until the current one is first used. Any failure to match is ErrInvalidClient.
func (m *MachineSignIn) Authorize(ctx context.Context, id uuid.UUID, secret string) error {
	if _, err := m.Machine(ctx, id); err != nil {
		if errors.Is(err, ErrUnknownMachine) {
			return ErrInvalidClient
		}
		return err
	}
	return checkSecret(ctx, m.hashes, id.String(), secret)
}

// Roles is the App Roles a Machine Identity holds on a Client that the Client still declares, in
// the Client's order. A Removed Client is ErrNotFound, and holding none is ErrNoAccess.
func (m *MachineSignIn) Roles(ctx context.Context, id uuid.UUID, clientID string) ([]string, error) {
	c, err := m.clients.Client(ctx, clientID)
	if err != nil {
		return nil, err
	}
	held, err := m.machines.MachineRoles(ctx, id, clientID)
	if err != nil {
		return nil, err
	}
	roles := []string{}
	for _, role := range c.Roles {
		if slices.Contains(held, role) {
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		return nil, ErrNoAccess
	}
	return roles, nil
}

// RotateSecret gives a Machine Identity a new client secret and answers it, the one time it is
// seen; identity keeps only its hash. The previous secret works until the new one is first used.
func (m *MachineSignIn) RotateSecret(ctx context.Context, actor Actor, id uuid.UUID) (string, error) {
	if err := admin(ctx, m.admins, actor); err != nil {
		return "", err
	}
	mi, err := m.machines.Machine(ctx, id)
	if err != nil {
		return "", err
	}
	var secret string
	err = m.hashes.Rotate(ctx, id.String(), func() ([]byte, error) {
		s, err := newSecret(m.random)
		if err != nil {
			return nil, err
		}
		secret = s
		return hashOf(s), nil
	})
	if err != nil {
		return "", err
	}
	m.audit.Record(ctx, audit.Event{Action: audit.MachineSecretRotated, Actor: actor.ID, Machine: mi.ID, MachineName: mi.Name})
	return secret, nil
}
