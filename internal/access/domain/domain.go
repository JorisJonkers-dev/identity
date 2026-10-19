// Package domain is the access context's core (docs/adr/0004, 0018): Groups of Users, and
// Assignments giving a Group or a User one App Role on one Client. It sees no transport, no persistence and no
// generated code.
package domain

import (
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ErrInvalidGroupName is a name outside the rules.
var ErrInvalidGroupName = errors.New("a Group name is 1 to 64 characters")

const maxGroupName = 64

// GroupName is what a Group is called, trimmed, 1 to 64 characters.
type GroupName struct{ value string }

// NewGroupName trims raw and bounds it by characters, not bytes.
func NewGroupName(raw string) (GroupName, error) {
	value := strings.TrimSpace(raw)
	if n := utf8.RuneCountInString(value); n < 1 || n > maxGroupName {
		return GroupName{}, ErrInvalidGroupName
	}
	return GroupName{value: value}, nil
}

func (n GroupName) String() string { return n.value }

// Group is a named set of Users. The admins Group makes its members Admins and holds no
// Assignment.
type Group struct {
	ID     uuid.UUID
	Name   string
	Admins bool
	// RequiresTwoFactor holds back the Group's App Roles from a member without two-factor; the
	// admins Group always does.
	RequiresTwoFactor bool
}

// Membership puts one User in one Group.
type Membership struct {
	GroupID uuid.UUID
	UserID  uuid.UUID
}

// Assignment gives a Group one App Role on one Client.
type Assignment struct {
	GroupID  uuid.UUID
	ClientID string
	Role     string
}

// UserAssignment gives one User one App Role on one Client directly, without a Group.
type UserAssignment struct {
	UserID   uuid.UUID
	ClientID string
	Role     string
}

// Client is a Client as access needs it: what it is called and the App Roles it declares.
type Client struct {
	ID    string
	Label string
	Roles []string
}

// Declares reports whether the Client has the App Role.
func (c Client) Declares(role string) bool { return slices.Contains(c.Roles, role) }

// Person is a User as access shows one.
type Person struct {
	ID    uuid.UUID
	Email string
	Name  string
	// TwoFactor is whether they added an authenticator app or a passkey.
	TwoFactor bool
}
