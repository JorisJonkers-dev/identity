// Package app holds the access use cases (docs/adr/0004): Admins manage Groups, memberships and
// Assignments, and sign-in asks which App Roles a User holds on a Client.
package app

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/domain"
)

var (
	ErrNotAdmin      = errors.New("only Admins manage access")
	ErrNotFound      = errors.New("not found")
	ErrNameTaken     = errors.New("a Group has that name")
	ErrAdminsGroup   = errors.New("the admins Group cannot be deleted or assigned")
	ErrLastAdmin     = errors.New("the last Admin cannot leave the admins Group")
	ErrUnknownClient = errors.New("no such Client")
	ErrUnknownRole   = errors.New("the Client declares no such App Role")
	ErrAdminExists   = errors.New("an Admin exists already")
	// ErrSecondFactorNeeded is an Admin whose session did not prove a Second Factor.
	ErrSecondFactorNeeded = errors.New("access is managed only from a session proved with a Second Factor")
)

// Actor is who asks to manage access, and whether their session proved a Second Factor.
type Actor struct {
	ID           uuid.UUID
	SecondFactor bool
}

// Store keeps Groups, memberships and Assignments.
type Store interface {
	Groups(ctx context.Context) ([]domain.Group, error)
	// Group is ErrNotFound when there is none.
	Group(ctx context.Context, id uuid.UUID) (domain.Group, error)
	// CreateGroup is ErrNameTaken when a Group has the name, in any case.
	CreateGroup(ctx context.Context, g domain.Group) error
	// RenameGroup is ErrNotFound or ErrNameTaken.
	RenameGroup(ctx context.Context, id uuid.UUID, name domain.GroupName) error
	// DeleteGroup never deletes the admins Group.
	DeleteGroup(ctx context.Context, id uuid.UUID) error
	Memberships(ctx context.Context) ([]domain.Membership, error)
	// AddMember is ErrNotFound when the Group or the User is not there.
	AddMember(ctx context.Context, m domain.Membership) error
	// RemoveMember refuses with ErrLastAdmin to leave the admins Group empty, even when two
	// removals race.
	RemoveMember(ctx context.Context, m domain.Membership) error
	IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error)
	AdminsGroup(ctx context.Context) (domain.Group, error)
	CountAdmins(ctx context.Context) (int, error)
	// CountUnassigned is the confirmed Users in no Group.
	CountUnassigned(ctx context.Context) (int, error)
	Assignments(ctx context.Context) ([]domain.Assignment, error)
	// Assign is ErrNotFound when the Group is not there.
	Assign(ctx context.Context, a domain.Assignment) error
	Unassign(ctx context.Context, a domain.Assignment) error
	// RolesOf is the App Roles a User's Groups hold on a Client.
	RolesOf(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error)
}

// Clients is the Clients the cluster declares now; a Removed Client is not among them.
type Clients interface {
	Clients() []domain.Client
	Client(ctx context.Context, id string) (domain.Client, bool)
}

// People lists Users, and signs one out everywhere.
type People interface {
	People(ctx context.Context) ([]domain.Person, error)
	SignOut(ctx context.Context, userID uuid.UUID) error
}

// Overview is everything the access page shows.
type Overview struct {
	Groups      []domain.Group
	Memberships []domain.Membership
	// Assignments includes those on Removed Clients, which give no access while it is away.
	Assignments []domain.Assignment
	Clients     []domain.Client
	People      []domain.Person
}

// Service is the access use cases.
type Service struct {
	store   Store
	clients Clients
	people  People
	random  io.Reader
}

// New returns a Service. random is where Group ids come from.
func New(store Store, clients Clients, people People, random io.Reader) *Service {
	return &Service{store: store, clients: clients, people: people, random: random}
}

// IsAdmin reports whether a User is in the admins Group.
func (s *Service) IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error) {
	return s.store.IsAdmin(ctx, userID)
}

// Promote makes a User an Admin. It is how a fresh install gets its first Admin, from a command
// run in the pod, and it refuses with ErrAdminExists when there is one, unless forced
// (docs/adr/0005).
func (s *Service) Promote(ctx context.Context, userID uuid.UUID, force bool) error {
	n, err := s.store.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if n > 0 && !force {
		return ErrAdminExists
	}
	g, err := s.store.AdminsGroup(ctx)
	if err != nil {
		return err
	}
	return s.store.AddMember(ctx, domain.Membership{GroupID: g.ID, UserID: userID})
}

// Unassigned is how many Users wait in no Group, for the signup alert.
func (s *Service) Unassigned(ctx context.Context) (int, error) {
	return s.store.CountUnassigned(ctx)
}

// Roles is the App Roles a User holds on a Client: the union over their Groups. Being an Admin
// adds none.
func (s *Service) Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error) {
	return s.store.RolesOf(ctx, userID, clientID)
}

// admin lets an Admin through, and only from a session that proved a Second Factor (docs/adr/0005).
func (s *Service) admin(ctx context.Context, actor Actor) error {
	yes, err := s.store.IsAdmin(ctx, actor.ID)
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

// Overview returns every Group, member, Assignment, Client and User.
func (s *Service) Overview(ctx context.Context, actor Actor) (Overview, error) {
	if err := s.admin(ctx, actor); err != nil {
		return Overview{}, err
	}
	var o Overview
	var err error
	if o.Groups, err = s.store.Groups(ctx); err != nil {
		return Overview{}, err
	}
	if o.Memberships, err = s.store.Memberships(ctx); err != nil {
		return Overview{}, err
	}
	if o.Assignments, err = s.store.Assignments(ctx); err != nil {
		return Overview{}, err
	}
	if o.People, err = s.people.People(ctx); err != nil {
		return Overview{}, err
	}
	o.Clients = s.clients.Clients()
	return o, nil
}

// CreateGroup makes an empty Group.
func (s *Service) CreateGroup(ctx context.Context, actor Actor, name string) (domain.Group, error) {
	if err := s.admin(ctx, actor); err != nil {
		return domain.Group{}, err
	}
	n, err := domain.NewGroupName(name)
	if err != nil {
		return domain.Group{}, err
	}
	id, err := uuid.NewRandomFromReader(s.random)
	if err != nil {
		return domain.Group{}, err
	}
	g := domain.Group{ID: id, Name: n.String()}
	if err := s.store.CreateGroup(ctx, g); err != nil {
		return domain.Group{}, err
	}
	return g, nil
}

// RenameGroup changes what a Group is called; no token carries it, so no app notices.
func (s *Service) RenameGroup(ctx context.Context, actor Actor, id uuid.UUID, name string) (domain.Group, error) {
	if err := s.admin(ctx, actor); err != nil {
		return domain.Group{}, err
	}
	n, err := domain.NewGroupName(name)
	if err != nil {
		return domain.Group{}, err
	}
	if err := s.store.RenameGroup(ctx, id, n); err != nil {
		return domain.Group{}, err
	}
	return s.store.Group(ctx, id)
}

// DeleteGroup deletes a Group with its memberships and Assignments. The admins Group stays.
func (s *Service) DeleteGroup(ctx context.Context, actor Actor, id uuid.UUID) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	g, err := s.store.Group(ctx, id)
	if err != nil {
		return err
	}
	if g.Admins {
		return ErrAdminsGroup
	}
	return s.store.DeleteGroup(ctx, id)
}

// AddMember puts a User in a Group.
func (s *Service) AddMember(ctx context.Context, actor Actor, m domain.Membership) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	return s.store.AddMember(ctx, m)
}

// SignOutUser ends every session a User has (docs/adr/0007).
func (s *Service) SignOutUser(ctx context.Context, actor Actor, userID uuid.UUID) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	return s.people.SignOut(ctx, userID)
}

// RemoveMember takes a User out of a Group; the admins Group keeps at least one.
func (s *Service) RemoveMember(ctx context.Context, actor Actor, m domain.Membership) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	return s.store.RemoveMember(ctx, m)
}

// Assign gives a Group an App Role the Client declares. The admins Group holds none, so being an
// Admin never opens an app.
func (s *Service) Assign(ctx context.Context, actor Actor, a domain.Assignment) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	g, err := s.store.Group(ctx, a.GroupID)
	if err != nil {
		return err
	}
	if g.Admins {
		return ErrAdminsGroup
	}
	c, ok := s.clients.Client(ctx, a.ClientID)
	if !ok {
		return ErrUnknownClient
	}
	if !c.Declares(a.Role) {
		return ErrUnknownRole
	}
	return s.store.Assign(ctx, a)
}

// Unassign takes an App Role from a Group, also on a Removed Client.
func (s *Service) Unassign(ctx context.Context, actor Actor, a domain.Assignment) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	return s.store.Unassign(ctx, a)
}
