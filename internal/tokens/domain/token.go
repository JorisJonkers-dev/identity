// Package domain is what a Personal Token is (docs/adr/0008): a bearer credential a User mints for
// one Client, for a lifetime they choose, kept only as a hash.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidName     = errors.New("a token's name is 1 to 64 characters")
	ErrInvalidLifetime = errors.New("a token lasts 30, 90 or 365 days")
)

// Prefix starts every Personal Token, so a leaked one is recognised and the Provider knows to look
// its hash up (docs/adr/0017).
const Prefix = "idpat_"

// Lifetime is how long a token lasts.
type Lifetime struct{ days int }

// NewLifetime is days when it is 30, 90 or 365, the lifetimes a User may choose.
func NewLifetime(days int) (Lifetime, error) {
	switch days {
	case 30, 90, 365:
		return Lifetime{days: days}, nil
	}
	return Lifetime{}, ErrInvalidLifetime
}

// From is when a token made at t expires: whole days of 24 hours, whatever the clocks do.
func (l Lifetime) From(t time.Time) time.Time { return t.Add(time.Duration(l.days) * 24 * time.Hour) }

// Name is what a User calls a token, such as backup-script.
type Name struct{ s string }

// NewName trims raw and keeps it when it is 1 to 64 characters.
func NewName(raw string) (Name, error) {
	s := strings.TrimSpace(raw)
	if s == "" || utf8.RuneCountInString(s) > 64 {
		return Name{}, ErrInvalidName
	}
	return Name{s: s}, nil
}

func (n Name) String() string { return n.s }

// Token is a Personal Token as identity keeps it, without its secret.
type Token struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	ClientID   string
	Name       string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	LastUsedAt *time.Time
}

// Soon is how close to its end a token is when the Launcher and the dashboard warn about it.
const Soon = 7 * 24 * time.Hour

// State is how a token stands.
type State string

const (
	Active   State = "active"
	Expiring State = "expiring"
	Expired  State = "expired"
	Revoked  State = "revoked"
)

// State is how the token stands at now: revoked, expired, expiring within Soon, or active.
func (t Token) State(now time.Time) State {
	switch {
	case t.RevokedAt != nil:
		return Revoked
	case !now.Before(t.ExpiresAt):
		return Expired
	case t.ExpiresAt.Sub(now) <= Soon:
		return Expiring
	default:
		return Active
	}
}

// Live reports whether the token may be used at now: not revoked, and not yet expired.
func (t Token) Live(now time.Time) bool { return t.RevokedAt == nil && now.Before(t.ExpiresAt) }
