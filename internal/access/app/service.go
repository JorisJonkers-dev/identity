// Package app holds the access use cases (docs/adr/0004): Admins manage Groups, memberships and
// Assignments, and sign-in asks which App Roles a User holds on a Client.
package app

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
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
	// AddMember is ErrNotFound when the Group or the User is not there, and reports whether the
	// User was not a member already.
	AddMember(ctx context.Context, m domain.Membership) (bool, error)
	// RemoveMember refuses with ErrLastAdmin to leave the admins Group empty, even when two
	// removals race, and reports whether the User was a member.
	RemoveMember(ctx context.Context, m domain.Membership) (bool, error)
	IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error)
	AdminsGroup(ctx context.Context) (domain.Group, error)
	CountAdmins(ctx context.Context) (int, error)
	// CountUnassigned is the confirmed Users in no Group.
	CountUnassigned(ctx context.Context) (int, error)
	Assignments(ctx context.Context) ([]domain.Assignment, error)
	// Assign is ErrNotFound when the Group is not there, and reports whether it is new.
	Assign(ctx context.Context, a domain.Assignment) (bool, error)
	// Unassign reports whether the Assignment was there.
	Unassign(ctx context.Context, a domain.Assignment) (bool, error)
	// RolesOf is the App Roles a User's Groups hold on a Client.
	RolesOf(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error)
	// RolesByClient is every App Role a User's Groups hold, per Client.
	RolesByClient(ctx context.Context, userID uuid.UUID) (map[string][]string, error)
}

// Clients is the Clients the cluster declares now; a Removed Client is not among them.
type Clients interface {
	Clients() []domain.Client
	Client(ctx context.Context, id string) (domain.Client, bool)
}

// People lists Users, and signs one out everywhere.
type People interface {
	People(ctx context.Context) ([]domain.Person, error)
	// SignOut signs a User out everywhere at the actor's asking.
	SignOut(ctx context.Context, actor, userID uuid.UUID) error
}

// Audit records each change as an Audit Event, after it is made.
type Audit interface {
	Record(ctx context.Context, e audit.Event)
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
	audit   Audit
	random  io.Reader
}

// New returns a Service. random is where Group ids come from.
func New(store Store, clients Clients, people People, audit Audit, random io.Reader) *Service {
	return &Service{store: store, clients: clients, people: people, audit: audit, random: random}
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
	added, err := s.store.AddMember(ctx, domain.Membership{GroupID: g.ID, UserID: userID})
	if added {
		s.audit.Record(ctx, audit.Event{Action: audit.AdminPromoted, User: userID, Group: g.ID, GroupName: g.Name})
	}
	return err
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

// RolesByClient is every App Role a User holds, per Client, including Removed ones.
func (s *Service) RolesByClient(ctx context.Context, userID uuid.UUID) (map[string][]string, error) {
	return s.store.RolesByClient(ctx, userID)
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
	s.audit.Record(ctx, audit.Event{Action: audit.GroupCreated, Actor: actor.ID, Group: g.ID, GroupName: g.Name})
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
	s.audit.Record(ctx, audit.Event{Action: audit.GroupRenamed, Actor: actor.ID, Group: id, GroupName: n.String()})
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
	if err := s.store.DeleteGroup(ctx, id); err != nil {
		return err
	}
	s.audit.Record(ctx, audit.Event{Action: audit.GroupDeleted, Actor: actor.ID, Group: id, GroupName: g.Name})
	return nil
}

// AddMember puts a User in a Group.
func (s *Service) AddMember(ctx context.Context, actor Actor, m domain.Membership) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	added, err := s.store.AddMember(ctx, m)
	if added {
		s.recordMembership(ctx, audit.MemberAdded, actor, m)
	}
	return err
}

// recordMembership records a membership change, naming the Group as it is called now.
func (s *Service) recordMembership(ctx context.Context, action audit.Action, actor Actor, m domain.Membership) {
	e := audit.Event{Action: action, Actor: actor.ID, User: m.UserID, Group: m.GroupID}
	if g, err := s.store.Group(ctx, m.GroupID); err == nil {
		e.GroupName = g.Name
	}
	s.audit.Record(ctx, e)
}

// SignOutUser ends every session a User has (docs/adr/0007).
func (s *Service) SignOutUser(ctx context.Context, actor Actor, userID uuid.UUID) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	return s.people.SignOut(ctx, actor.ID, userID)
}

// RemoveMember takes a User out of a Group; the admins Group keeps at least one.
func (s *Service) RemoveMember(ctx context.Context, actor Actor, m domain.Membership) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	removed, err := s.store.RemoveMember(ctx, m)
	if removed {
		s.recordMembership(ctx, audit.MemberRemoved, actor, m)
	}
	return err
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
	assigned, err := s.store.Assign(ctx, a)
	if assigned {
		s.audit.Record(ctx, audit.Event{Action: audit.RoleAssigned, Actor: actor.ID, Client: a.ClientID, Group: g.ID, GroupName: g.Name, Role: a.Role})
	}
	return err
}

// Unassign takes an App Role from a Group, also on a Removed Client.
func (s *Service) Unassign(ctx context.Context, actor Actor, a domain.Assignment) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	removed, err := s.store.Unassign(ctx, a)
	if removed {
		e := audit.Event{Action: audit.RoleRemoved, Actor: actor.ID, Client: a.ClientID, Group: a.GroupID, Role: a.Role}
		if g, err := s.store.Group(ctx, a.GroupID); err == nil {
			e.GroupName = g.Name
		}
		s.audit.Record(ctx, e)
	}
	return err
}
