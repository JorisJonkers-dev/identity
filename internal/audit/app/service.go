// Package app holds the audit use cases: recording an Audit Event in the database and the logs,
// listing them for Admins, and pruning those older than a year.
package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/audit/domain"
)

var (
	ErrNotAdmin = errors.New("only Admins read the audit log and the dashboard")
	// ErrSecondFactorNeeded is an Admin whose session did not prove a Second Factor.
	ErrSecondFactorNeeded = errors.New("the audit log and the dashboard are read only from a session proved with a Second Factor")
)

const (
	// Retention is how long an Audit Event is kept in the database; Loki keeps its own copy.
	Retention = 365 * 24 * time.Hour
	pageSize  = 50
)

// Store keeps Audit Events.
type Store interface {
	Append(ctx context.Context, e domain.Event) error
	// List is at most limit events matching f, newest first.
	List(ctx context.Context, f domain.Filter, limit int) ([]domain.Event, error)
	// DeleteBefore removes events older than t and reports how many.
	DeleteBefore(ctx context.Context, t time.Time) (int64, error)
	// SignInsByHour counts sign-ins to the Identity Provider and failed ones, per hour that has
	// any, since a time.
	SignInsByHour(ctx context.Context, since time.Time) ([]Hour, error)
}

// Hour is the sign-ins and failed sign-ins in the hour that starts At.
type Hour struct {
	At              time.Time
	SignIns, Failed int
}

// Admins tells who is an Admin.
type Admins interface {
	IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error)
}

// Person is a User as the audit log names them, and when they confirmed their email.
type Person struct {
	ID          uuid.UUID
	Name, Email string
	CreatedAt   time.Time
}

// Directory names the Users and Clients events refer to, and counts what the dashboard shows.
type Directory interface {
	People(ctx context.Context) ([]Person, error)
	// Unassigned is the Users in no Group.
	Unassigned(ctx context.Context) ([]uuid.UUID, error)
	// ActiveSessions is how many Single sign-on sessions are live, and how many Users hold them.
	ActiveSessions(ctx context.Context) (sessions, users int, err error)
	// TokensExpiring is how many live Personal Tokens end within seven days.
	TokensExpiring(ctx context.Context) (int, error)
	// ClientLabel is what a Client is called, or its id when it is gone.
	ClientLabel(ctx context.Context, id string) string
}

// Actor is who asks to read the audit log, and whether their session proved a Second Factor.
type Actor struct {
	ID           uuid.UUID
	SecondFactor bool
}

// Entry is an event with the names it refers to; a Person is nil when the event names none, and
// has only an ID when the User is gone.
type Entry struct {
	domain.Event
	Kind        domain.Kind
	Actor, User *Person
	ClientLabel string
}

// Page is one page of the listing, and where the next begins; a zero Next is the last page.
type Page struct {
	Entries []Entry
	Next    domain.Cursor
}

// Service is the audit use cases.
type Service struct {
	store     Store
	admins    Admins
	directory Directory
	logger    *slog.Logger
	now       func() time.Time
	random    io.Reader
}

// New returns a Service. random is where event ids come from.
func New(store Store, admins Admins, directory Directory, logger *slog.Logger, now func() time.Time, random io.Reader) *Service {
	return &Service{store: store, admins: admins, directory: directory, logger: logger, now: now, random: random}
}

// Record keeps e, after the change it describes was made: it is logged as JSON first, so Loki has
// it even when the database refuses it, and a refusal is logged rather than undoing the change.
func (s *Service) Record(ctx context.Context, e domain.Event) {
	id, err := uuid.NewRandomFromReader(s.random)
	if err != nil {
		s.logger.ErrorContext(ctx, "audit event not recorded", "error", err, "action", string(e.Action))
		return
	}
	e.ID, e.At = id, s.now().UTC()
	s.logger.LogAttrs(ctx, slog.LevelInfo, "audit event", slog.Attr{Key: "audit", Value: slog.GroupValue(attrs(e)...)})
	if err := s.store.Append(ctx, e); err != nil {
		s.logger.ErrorContext(ctx, "audit event not stored", "error", err, "id", e.ID.String())
	}
}

// attrs is e as log fields, leaving out what it does not name.
func attrs(e domain.Event) []slog.Attr {
	a := []slog.Attr{
		slog.String("id", e.ID.String()),
		slog.Time("at", e.At),
		slog.String("kind", string(e.Action.Kind())),
		slog.String("action", string(e.Action)),
	}
	for _, f := range []struct {
		key string
		id  uuid.UUID
	}{{"actor_id", e.Actor}, {"user_id", e.User}, {"group_id", e.Group}} {
		if f.id != uuid.Nil {
			a = append(a, slog.String(f.key, f.id.String()))
		}
	}
	for _, f := range []struct{ key, value string }{
		{"client_id", e.Client}, {"group_name", e.GroupName}, {"role", e.Role}, {"email", e.Email}, {"reason", e.Reason},
	} {
		if f.value != "" {
			a = append(a, slog.String(f.key, f.value))
		}
	}
	return a
}

// admin lets an Admin through, and only from a session that proved a Second Factor.
func (s *Service) admin(ctx context.Context, actor Actor) error {
	admin, err := s.admins.IsAdmin(ctx, actor.ID)
	if err != nil {
		return err
	}
	if !admin {
		return ErrNotAdmin
	}
	if !actor.SecondFactor {
		return ErrSecondFactorNeeded
	}
	return nil
}

// List returns a page of events matching f, newest first, to an Admin whose session proved a
// Second Factor.
func (s *Service) List(ctx context.Context, actor Actor, f domain.Filter) (Page, error) {
	if err := s.admin(ctx, actor); err != nil {
		return Page{}, err
	}
	events, err := s.store.List(ctx, f, pageSize+1)
	if err != nil {
		return Page{}, err
	}
	people, err := s.directory.People(ctx)
	if err != nil {
		return Page{}, err
	}
	var p Page
	if len(events) > pageSize {
		events = events[:pageSize]
		last := events[pageSize-1]
		p.Next = domain.Cursor{At: last.At, ID: last.ID}
	}
	byID := make(map[uuid.UUID]Person, len(people))
	for _, person := range people {
		byID[person.ID] = person
	}
	p.Entries = make([]Entry, 0, len(events))
	for _, e := range events {
		entry := Entry{Event: e, Kind: e.Action.Kind(), Actor: named(byID, e.Actor), User: named(byID, e.User)}
		if e.Client != "" {
			entry.ClientLabel = s.directory.ClientLabel(ctx, e.Client)
		}
		p.Entries = append(p.Entries, entry)
	}
	return p, nil
}

func named(people map[uuid.UUID]Person, id uuid.UUID) *Person {
	if id == uuid.Nil {
		return nil
	}
	if p, ok := people[id]; ok {
		return &p
	}
	return &Person{ID: id}
}

// Prune removes the events older than Retention, and reports how many it removed.
func (s *Service) Prune(ctx context.Context) (int64, error) {
	return s.store.DeleteBefore(ctx, s.now().Add(-Retention))
}
