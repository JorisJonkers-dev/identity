// Package app imports Legacy Auth's people into identity once (docs/adr/0006, identity#24).
package app

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/legacy/domain"
	usersdomain "github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// Why an Account is not imported.
const (
	SkipUnconfirmed = "email not confirmed"
	SkipBadEmail    = "not a usable email"
	SkipBadName     = "not a usable name"
	SkipImported    = "already imported"
	SkipTaken       = "email already in identity"
)

// Person is an Account as identity keeps it: its bcrypt hash as it is, its authenticator secret
// sealed, and each grant a direct `user` role on a Client (docs/adr/0018).
type Person struct {
	ID           uuid.UUID
	Email        usersdomain.Email
	Name         usersdomain.Name
	PasswordHash string
	// SealedTOTP is the authenticator app's secret, sealed by Vault, or empty.
	SealedTOTP string
	Admin      bool
	Clients    []string
	CreatedAt  time.Time
}

// State is whether an Account is already in identity under its own id, and whether its email is.
type State struct{ Imported, Taken bool }

// Source reads Legacy Auth's Accounts.
type Source interface {
	Accounts(ctx context.Context) ([]domain.Account, error)
}

// Target is identity's store.
type Target interface {
	State(ctx context.Context, id uuid.UUID, email usersdomain.Email) (State, error)
	// Import adds p, its authenticator app, its roles and its place in the admins Group in one
	// transaction, and reports false when p was already there.
	Import(ctx context.Context, p Person) (bool, error)
}

// Sealer seals an authenticator secret as identity seals its own (docs/adr/0013).
type Sealer interface {
	Seal(ctx context.Context, plaintext []byte) (string, error)
}

// Outcome is what the import did, or would do, with one Account.
type Outcome struct {
	Email string
	// Skipped says why the Account is not imported, or is empty when it is.
	Skipped       string
	Admin         bool
	Authenticator bool
	Clients       []string
}

// Importer imports Accounts.
type Importer struct {
	source  Source
	target  Target
	sealer  Sealer
	clients map[string]string
}

// New returns an Importer giving each service's grant a role on the Client clients names for it,
// or on the Client named like the service.
func New(source Source, target Target, sealer Sealer, clients map[string]string) *Importer {
	return &Importer{source: source, target: target, sealer: sealer, clients: clients}
}

// Run imports every Account not yet in identity, or with dryRun says only what it would do. Each
// person goes in their own transaction, so a run that stops part way is finished by running again.
func (i *Importer) Run(ctx context.Context, dryRun bool) ([]Outcome, error) {
	accounts, err := i.source.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	outcomes := make([]Outcome, 0, len(accounts))
	for _, a := range accounts {
		o, err := i.one(ctx, a, dryRun)
		if err != nil {
			return outcomes, err
		}
		outcomes = append(outcomes, o)
	}
	return outcomes, nil
}

func (i *Importer) one(ctx context.Context, a domain.Account, dryRun bool) (Outcome, error) {
	email, name, skip := usable(a)
	if skip != "" {
		return Outcome{Email: a.Email, Skipped: skip}, nil
	}
	state, err := i.target.State(ctx, a.ID, email)
	if err != nil {
		return Outcome{}, err
	}
	if state.Imported {
		return Outcome{Email: email.String(), Skipped: SkipImported}, nil
	}
	if state.Taken {
		return Outcome{Email: email.String(), Skipped: SkipTaken}, nil
	}
	p := Person{ID: a.ID, Email: email, Name: name, PasswordHash: a.PasswordHash, Admin: a.Admin, Clients: i.clientsOf(a), CreatedAt: a.CreatedAt}
	o := Outcome{Email: email.String(), Admin: p.Admin, Authenticator: a.TOTPSecret != "", Clients: p.Clients}
	if dryRun {
		return o, nil
	}
	if a.TOTPSecret != "" {
		if p.SealedTOTP, err = i.sealer.Seal(ctx, []byte(a.TOTPSecret)); err != nil {
			return Outcome{}, err
		}
	}
	imported, err := i.target.Import(ctx, p)
	if err != nil {
		return Outcome{}, err
	}
	if !imported {
		return Outcome{Email: email.String(), Skipped: SkipImported}, nil
	}
	return o, nil
}

// usable is a's email and name as identity takes them, or why a cannot be imported.
func usable(a domain.Account) (usersdomain.Email, usersdomain.Name, string) {
	if !a.Confirmed {
		return usersdomain.Email{}, usersdomain.Name{}, SkipUnconfirmed
	}
	email, err := usersdomain.NewEmail(a.Email)
	if err != nil {
		return usersdomain.Email{}, usersdomain.Name{}, SkipBadEmail
	}
	name, err := usersdomain.NewName(a.FullName())
	if err != nil {
		return email, usersdomain.Name{}, SkipBadName
	}
	return email, name, ""
}

// clientsOf is each Client a's grants give a role on, once each, in order.
func (i *Importer) clientsOf(a domain.Account) []string {
	ids := make([]string, 0, len(a.Services))
	for _, s := range a.Services {
		ids = append(ids, domain.ClientFor(s, i.clients))
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}
