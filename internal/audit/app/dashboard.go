package app

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
)

const (
	day  = 24 * time.Hour
	week = 7 * day
)

// Dashboard is how things stand: identity's own counts, over the last day where they are events.
type Dashboard struct {
	Users, UsersThisWeek int
	// Unassigned is the Users waiting for an App Role, the newest first.
	Unassigned             []Person
	SignIns, FailedSignIns int
	// Hours is the last 24 hours, oldest first, the last the hour now.
	Hours                        [24]Hour
	ActiveSessions, SessionUsers int
	TokensExpiring               int
}

// Dashboard counts Users, the Unassigned queue, sign-ins and live sessions, for an Admin whose
// session proved two-factor.
func (s *Service) Dashboard(ctx context.Context, actor Actor) (Dashboard, error) {
	if err := s.admin(ctx, actor); err != nil {
		return Dashboard{}, err
	}
	now := s.now()
	people, err := s.directory.People(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	waiting, err := s.directory.Unassigned(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	var d Dashboard
	if d.ActiveSessions, d.SessionUsers, err = s.directory.ActiveSessions(ctx); err != nil {
		return Dashboard{}, err
	}
	if d.TokensExpiring, err = s.directory.TokensExpiring(ctx); err != nil {
		return Dashboard{}, err
	}
	first := now.Truncate(time.Hour).Add(-23 * time.Hour)
	counted, err := s.store.SignInsByHour(ctx, first)
	if err != nil {
		return Dashboard{}, err
	}
	d.countPeople(people, waiting, now)
	d.layOut(counted, first)
	return d, nil
}

// countPeople counts the Users, those who joined this week, and those waiting, newest first.
func (d *Dashboard) countPeople(people []Person, waiting []uuid.UUID, now time.Time) {
	d.Users = len(people)
	d.Unassigned = make([]Person, 0, len(waiting))
	for _, p := range people {
		if now.Sub(p.CreatedAt) < week {
			d.UsersThisWeek++
		}
		if slices.Contains(waiting, p.ID) {
			d.Unassigned = append(d.Unassigned, p)
		}
	}
	slices.SortStableFunc(d.Unassigned, func(a, b Person) int { return b.CreatedAt.Compare(a.CreatedAt) })
}

// layOut puts the hours that had sign-ins in their place among the 24 from first, and sums them.
func (d *Dashboard) layOut(counted []Hour, first time.Time) {
	for i := range d.Hours {
		d.Hours[i].At = first.Add(time.Duration(i) * time.Hour)
	}
	for _, h := range counted {
		i := int(h.At.Sub(first) / time.Hour)
		if i < 0 || i >= len(d.Hours) {
			continue
		}
		d.Hours[i].SignIns += h.SignIns
		d.Hours[i].Failed += h.Failed
		d.SignIns += h.SignIns
		d.FailedSignIns += h.Failed
	}
}
