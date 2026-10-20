// Package app holds the access use cases (docs/adr/0004): Admins manage Groups, memberships and
// Assignments, and sign-in asks which App Roles a User holds on a Client.
package app

import (
	"context"
	"errors"
	"io"
	"time"

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
	// ErrAdminsNeedTwoFactor is turning off the admins Group's two-factor requirement.
	ErrAdminsNeedTwoFactor = errors.New("the admins Group always requires two-factor")
	// ErrTwoFactorProofNeeded is an Admin whose session did not prove two-factor.
	ErrTwoFactorProofNeeded = errors.New("access is managed only from a session that proved two-factor")
)

// Actor is who asks to manage access, and whether their session proved two-factor.
type Actor struct {
	ID        uuid.UUID
	TwoFactor bool
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
	// SetGroupTwoFactor is ErrNotFound when the Group is not there.
	SetGroupTwoFactor(ctx context.Context, id uuid.UUID, required bool) error
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
	// CountUnassigned is the confirmed Users, Admins aside, who hold no App Role by any route.
	CountUnassigned(ctx context.Context) (int, error)
	// Unassigned is the Users CountUnassigned counts.
	Unassigned(ctx context.Context) ([]uuid.UUID, error)
	Assignments(ctx context.Context) ([]domain.Assignment, error)
	// Assign is ErrNotFound when the Group is not there, and reports whether it is new.
	Assign(ctx context.Context, a domain.Assignment) (bool, error)
	// Unassign reports whether the Assignment was there.
	Unassign(ctx context.Context, a domain.Assignment) (bool, error)
	UserAssignments(ctx context.Context) ([]domain.UserAssignment, error)
	// AssignUser is ErrNotFound when the User is not there, and reports whether it is new.
	AssignUser(ctx context.Context, a domain.UserAssignment) (bool, error)
	// UnassignUser reports whether the Assignment was there.
	UnassignUser(ctx context.Context, a domain.UserAssignment) (bool, error)
	// RolesOf is the App Roles a User holds on a Client, directly and through their Groups.
	RolesOf(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error)
	// HeldBackClients is the Clients a User's Two-factor Groups give roles on while they have no
	// two-factor.
	HeldBackClients(ctx context.Context, userID uuid.UUID) ([]string, error)
	// RolesByClient is every App Role a User holds, directly and through their Groups, per Client.
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
	// TwoFactorRequired tells a User a Group they are in now holds back its roles until they set
	// two-factor up.
	TwoFactorRequired(ctx context.Context, userID uuid.UUID, group string) error
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
	// UserAssignments is the App Roles given to Users directly, on Removed Clients too.
	UserAssignments []domain.UserAssignment
	Clients         []domain.Client
	People          []domain.Person
}

// Service is the access use cases.
type Service struct {
	store    Store
	clients  Clients
	people   People
	audit    Audit
	random   io.Reader
	machines MachineStore
	now      func() time.Time
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

// Unassigned is how many Users wait for an App Role, for the signup alert.
func (s *Service) Unassigned(ctx context.Context) (int, error) {
	return s.store.CountUnassigned(ctx)
}

// UnassignedUsers is who waits for an App Role, for the dashboard's queue.
func (s *Service) UnassignedUsers(ctx context.Context) ([]uuid.UUID, error) {
	return s.store.Unassigned(ctx)
}

// Roles is the App Roles a User holds on a Client: their own and their Groups', combined. Being an
// Admin adds none.
func (s *Service) Roles(ctx context.Context, userID uuid.UUID, clientID string) ([]string, error) {
	return s.store.RolesOf(ctx, userID, clientID)
}

// HeldBackClients is the Clients a User's Two-factor Groups would open if they set two-factor up.
func (s *Service) HeldBackClients(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return s.store.HeldBackClients(ctx, userID)
}

// RolesByClient is every App Role a User holds, per Client, including Removed ones.
func (s *Service) RolesByClient(ctx context.Context, userID uuid.UUID) (map[string][]string, error) {
	return s.store.RolesByClient(ctx, userID)
}

// admin lets an Admin through, and only from a session that proved two-factor (docs/adr/0005).
func (s *Service) admin(ctx context.Context, actor Actor) error {
	yes, err := s.store.IsAdmin(ctx, actor.ID)
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
	if o.UserAssignments, err = s.store.UserAssignments(ctx); err != nil {
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

// AssignUser gives one User an App Role the Client declares, without a Group (docs/adr/0018).
func (s *Service) AssignUser(ctx context.Context, actor Actor, a domain.UserAssignment) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	c, ok := s.clients.Client(ctx, a.ClientID)
	if !ok {
		return ErrUnknownClient
	}
	if !c.Declares(a.Role) {
		return ErrUnknownRole
	}
	assigned, err := s.store.AssignUser(ctx, a)
	if assigned {
		s.audit.Record(ctx, audit.Event{Action: audit.RoleAssigned, Actor: actor.ID, User: a.UserID, Client: a.ClientID, Role: a.Role})
	}
	return err
}

// UnassignUser takes an App Role from a User, also on a Removed Client. Their Groups' roles stay.
func (s *Service) UnassignUser(ctx context.Context, actor Actor, a domain.UserAssignment) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	removed, err := s.store.UnassignUser(ctx, a)
	if removed {
		s.audit.Record(ctx, audit.Event{Action: audit.RoleRemoved, Actor: actor.ID, User: a.UserID, Client: a.ClientID, Role: a.Role})
	}
	return err
}

// SetTwoFactor makes a Group require two-factor, or stop: a member without it holds none of the
// Group's App Roles while it does (identity#83). The admins Group always requires it.
func (s *Service) SetTwoFactor(ctx context.Context, actor Actor, id uuid.UUID, required bool) error {
	if err := s.admin(ctx, actor); err != nil {
		return err
	}
	g, err := s.store.Group(ctx, id)
	if err != nil {
		return err
	}
	if g.Admins && !required {
		return ErrAdminsNeedTwoFactor
	}
	if err := s.store.SetGroupTwoFactor(ctx, id, required); err != nil {
		return err
	}
	if required && !g.RequiresTwoFactor {
		if err := s.tellHeldBack(ctx, g); err != nil {
			return err
		}
	}
	action := audit.GroupTwoFactorDropped
	if required {
		action = audit.GroupTwoFactorRequired
	}
	s.audit.Record(ctx, audit.Event{Action: action, Actor: actor.ID, Group: id, GroupName: g.Name})
	return nil
}

// tellHeldBack mails each member of a Group that has just started requiring two-factor and who has
// none, once (identity#88).
func (s *Service) tellHeldBack(ctx context.Context, g domain.Group) error {
	members, err := s.store.Memberships(ctx)
	if err != nil {
		return err
	}
	people, err := s.people.People(ctx)
	if err != nil {
		return err
	}
	without := map[uuid.UUID]bool{}
	for _, p := range people {
		without[p.ID] = !p.TwoFactor
	}
	for _, m := range members {
		if m.GroupID == g.ID && without[m.UserID] {
			if err := s.people.TwoFactorRequired(ctx, m.UserID, g.Name); err != nil {
				return err
			}
		}
	}
	return nil
}
