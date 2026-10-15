// Package domain is the users context's core: who a User is and the rules on what they enter.
// It sees no transport, no persistence and no generated code.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// The rules a caller can break, each answered with its own message.
var (
	ErrInvalidEmail = errors.New("enter a valid email address")
	ErrInvalidName  = errors.New("a name is 1 to 100 characters")
	ErrWeakPassword = errors.New("a password is 12 to 128 characters")
)

const (
	maxEmail    = 254
	maxName     = 100
	minPassword = 12
	maxPassword = 128
)

// Email is a trimmed, lower-cased address with one @ and something on each side of it.
type Email struct{ value string }

// NewEmail normalises raw and refuses what cannot be an address.
func NewEmail(raw string) (Email, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	local, host, found := strings.Cut(value, "@")
	if !found || local == "" || !strings.Contains(host, ".") || strings.Contains(host, "@") ||
		len(value) > maxEmail || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return Email{}, ErrInvalidEmail
	}
	return Email{value: value}, nil
}

func (e Email) String() string { return e.value }

// Name is what a User is called, trimmed, 1 to 100 characters.
type Name struct{ value string }

// NewName trims raw and bounds it by characters, not bytes.
func NewName(raw string) (Name, error) {
	value := strings.TrimSpace(raw)
	if n := utf8.RuneCountInString(value); n < 1 || n > maxName {
		return Name{}, ErrInvalidName
	}
	return Name{value: value}, nil
}

func (n Name) String() string { return n.value }

// Password is a candidate password, 12 to 128 characters. It is never stored, only its hash.
type Password struct{ value string }

// NewPassword bounds raw by characters; it is not trimmed, because spaces are characters too.
func NewPassword(raw string) (Password, error) {
	if n := utf8.RuneCountInString(raw); n < minPassword || n > maxPassword {
		return Password{}, ErrWeakPassword
	}
	return Password{value: raw}, nil
}

func (p Password) String() string { return p.value }

// User is a person registered with the Identity Provider.
type User struct {
	ID           uuid.UUID
	Email        Email
	Name         Name
	PasswordHash string
	ConfirmedAt  *time.Time
	CreatedAt    time.Time
}

// Confirmed reports whether the User has confirmed their email, which signing in needs.
func (u User) Confirmed() bool { return u.ConfirmedAt != nil }
