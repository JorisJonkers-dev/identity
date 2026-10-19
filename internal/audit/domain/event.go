// Package domain is what an Audit Event is: what happened, to whom, by whom, on which Client.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Kind is the family an action belongs to, which the Audit page filters by.
type Kind string

const (
	KindSignIn Kind = "sign-in"
	KindPeople Kind = "people"
	KindAccess Kind = "access"
	KindTokens Kind = "tokens"
)

// Action is what happened.
type Action string

const (
	SignedIn     Action = "signed-in"
	SignInFailed Action = "sign-in-failed"
	SignedOut    Action = "signed-out"

	UserCreated     Action = "user-created"
	UserRenamed     Action = "user-renamed"
	PasswordChanged Action = "password-changed"
	// EmailChanged is a User's sign-in email changing; Email is the address it replaced.
	EmailChanged Action = "email-changed"
	// AccountDeleted is a User deleting their account, or an Admin deleting it for them.
	AccountDeleted      Action = "account-deleted"
	SecondFactorAdded   Action = "second-factor-added"
	SecondFactorRemoved Action = "second-factor-removed"
	AdminPromoted       Action = "admin-promoted"

	GroupCreated  Action = "group-created"
	GroupRenamed  Action = "group-renamed"
	GroupDeleted  Action = "group-deleted"
	MemberAdded   Action = "member-added"
	MemberRemoved Action = "member-removed"
	RoleAssigned  Action = "role-assigned"
	RoleRemoved   Action = "role-removed"
	// ClientSecretRotated is an Admin giving a Client a new secret (identity#32).
	ClientSecretRotated Action = "client-secret-rotated"
	// StepUpChanged is an Admin setting how fresh a Client wants two-factor (docs/adr/0019).
	StepUpChanged Action = "step-up-changed"

	TokenMinted  Action = "token-minted"
	TokenRevoked Action = "token-revoked"
)

// Kind is the family the action belongs to; an unknown action has none.
func (a Action) Kind() Kind {
	switch a {
	case SignedIn, SignInFailed, SignedOut:
		return KindSignIn
	case UserCreated, UserRenamed, PasswordChanged, EmailChanged, AccountDeleted, SecondFactorAdded, SecondFactorRemoved, AdminPromoted:
		return KindPeople
	case GroupCreated, GroupRenamed, GroupDeleted, MemberAdded, MemberRemoved, RoleAssigned, RoleRemoved, ClientSecretRotated, StepUpChanged:
		return KindAccess
	case TokenMinted, TokenRevoked:
		return KindTokens
	}
	return ""
}

// Why a sign-in failed.
const (
	ReasonPassword     = "password"
	ReasonSecondFactor = "second-factor"
	ReasonNoAccess     = "no-access"
)

// Event is one Audit Event. Zero ids and empty strings are absent.
type Event struct {
	ID     uuid.UUID
	At     time.Time
	Action Action
	// Actor did it, when someone other than the User did: an Admin, for one.
	Actor     uuid.UUID
	User      uuid.UUID
	Client    string
	Group     uuid.UUID
	GroupName string
	Role      string
	// Email is the address a failed sign-in gave; it may belong to no User.
	Email  string
	Reason string
	// Minutes is the step-up window a StepUpChanged event set, 0 for off.
	Minutes int
}

// Filter narrows a listing; a zero field does not filter.
type Filter struct {
	User   uuid.UUID
	Client string
	Kind   Kind
	// Before lists only events older than this one, for the next page.
	Before Cursor
}

// Cursor is a place in the newest-first listing: an event's time and id.
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// Zero reports whether c is no place, the start of the listing.
func (c Cursor) Zero() bool { return c.At.IsZero() }
