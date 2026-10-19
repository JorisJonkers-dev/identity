// Package web is the audit context's inbound HTTP adapter: listAuditEvents over the use cases, for
// the signed-in User.
package web

import (
	"context"
	"encoding/base64"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/audit/app"
	"github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/httpx"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
)

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	List(ctx context.Context, actor app.Actor, f domain.Filter) (app.Page, error)
	Dashboard(ctx context.Context, actor app.Actor) (app.Dashboard, error)
}

// Audit implements listAuditEvents and getDashboard.
type Audit struct {
	uc      UseCases
	grafana *url.URL
}

// New returns an Audit calling uc. grafana, when not nil, is where the dashboard links for charts
// over time.
func New(uc UseCases, grafana *url.URL) *Audit { return &Audit{uc: uc, grafana: grafana} }

// GetDashboard implements getDashboard.
func (h *Audit) GetDashboard(ctx context.Context) (oas.GetDashboardRes, error) {
	d, err := h.uc.Dashboard(ctx, actor(ctx))
	if err != nil {
		return refusal(err)
	}
	res := &oas.Dashboard{
		Users: count(d.Users), UsersThisWeek: count(d.UsersThisWeek),
		Unassigned: make([]oas.WaitingUser, 0, len(d.Unassigned)),
		SignIns:    count(d.SignIns), FailedSignIns: count(d.FailedSignIns),
		Hours:          make([]oas.SignInHour, 0, len(d.Hours)),
		ActiveSessions: count(d.ActiveSessions), SessionUsers: count(d.SessionUsers),
		TokensExpiring: count(d.TokensExpiring),
	}
	for _, p := range d.Unassigned {
		res.Unassigned = append(res.Unassigned, oas.WaitingUser{ID: oas.ID(p.ID), Email: oas.Email(p.Email), Name: p.Name, ConfirmedAt: p.CreatedAt})
	}
	for _, hr := range d.Hours {
		res.Hours = append(res.Hours, oas.SignInHour{At: hr.At, SignIns: count(hr.SignIns), Failed: count(hr.Failed)})
	}
	if h.grafana != nil {
		res.GrafanaUrl = oas.NewOptURI(*h.grafana)
	}
	return res, nil
}

// count is n as the contract's Count, which no count here comes near the limit of.
func count(n int) oas.Count { return oas.Count(min(n, math.MaxInt32)) } //nolint:gosec // capped at MaxInt32 first

func actor(ctx context.Context) app.Actor {
	p := principal.From(ctx)
	return app.Actor{ID: p.UserID, SecondFactor: p.SecondFactor}
}

// refusal answers a reader who is not allowed with why, and passes anything else on.
func refusal(err error) (*oas.ProblemStatusCode, error) {
	switch {
	case errors.Is(err, app.ErrNotAdmin):
		return problem(http.StatusForbidden, "Only Admins can see this page."), nil
	case errors.Is(err, app.ErrSecondFactorNeeded):
		return problem(http.StatusForbidden, "This page needs a Second Factor. If you have a passkey or authenticator app, sign out and sign in again with it. If you have neither, add a Second Factor on the Security page first, then sign out and sign in again."), nil
	default:
		return nil, err
	}
}

// ListAuditEvents implements listAuditEvents.
func (h *Audit) ListAuditEvents(ctx context.Context, params oas.ListAuditEventsParams) (oas.ListAuditEventsRes, error) {
	f := domain.Filter{
		User:   uuid.UUID(params.User.Or(oas.ID{})),
		Client: string(params.Client.Or("")),
		Kind:   domain.Kind(params.Kind.Or("")),
	}
	if before, ok := params.Before.Get(); ok {
		c, ok := decode(string(before))
		if !ok {
			return problem(http.StatusBadRequest, "The list has changed since this page loaded. Reload the page to see the current list."), nil
		}
		f.Before = c
	}
	page, err := h.uc.List(ctx, actor(ctx), f)
	if err != nil {
		return refusal(err)
	}
	res := &oas.AuditPage{Events: make([]oas.AuditEvent, 0, len(page.Entries))}
	for _, e := range page.Entries {
		res.Events = append(res.Events, event(e))
	}
	if !page.Next.Zero() {
		res.Next = oas.NewOptAuditCursor(oas.AuditCursor(encode(page.Next)))
	}
	return res, nil
}

func event(e app.Entry) oas.AuditEvent {
	out := oas.AuditEvent{
		ID: oas.ID(e.ID), At: e.At, Kind: oas.AuditKind(e.Kind), Action: oas.AuditAction(e.Action),
		Actor: person(e.Actor), User: person(e.User),
	}
	if e.Client != "" {
		out.Client = oas.NewOptAuditClient(oas.AuditClient{ID: oas.ClientId(e.Client), Label: e.ClientLabel})
	}
	if e.GroupName != "" {
		out.GroupName = oas.NewOptString(e.GroupName)
	}
	if e.Role != "" {
		out.Role = oas.NewOptAppRole(oas.AppRole(e.Role))
	}
	if e.Email != "" {
		out.Email = oas.NewOptString(e.Email)
	}
	if e.Reason != "" {
		out.Reason = oas.NewOptAuditEventReason(oas.AuditEventReason(e.Reason))
	}
	if e.MachineName != "" {
		out.MachineName = oas.NewOptString(e.MachineName)
	}
	if e.Action == domain.StepUpChanged {
		out.Minutes = oas.NewOptAuditEventMinutes(oas.AuditEventMinutes(e.Minutes)) //nolint:gosec // 0 to 60
	}
	return out
}

func person(p *app.Person) oas.OptAuditPerson {
	if p == nil {
		return oas.OptAuditPerson{}
	}
	out := oas.AuditPerson{ID: oas.ID(p.ID)}
	if p.Email != "" {
		out.Email = oas.NewOptEmail(oas.Email(p.Email))
		out.Name = oas.NewOptString(p.Name)
	}
	return oas.NewOptAuditPerson(out)
}

// encode writes a cursor as its time in nanoseconds and its id, so a page boundary never rounds.
func encode(c domain.Cursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.At.UnixNano(), 10) + "." + c.ID.String()))
}

// decode reads a cursor encode wrote, reporting whether s was one.
func decode(s string) (domain.Cursor, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return domain.Cursor{}, false
	}
	nanos, id, ok := strings.Cut(string(raw), ".")
	if !ok {
		return domain.Cursor{}, false
	}
	n, err := strconv.ParseInt(nanos, 10, 64)
	if err != nil {
		return domain.Cursor{}, false
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return domain.Cursor{}, false
	}
	return domain.Cursor{At: time.Unix(0, n).UTC(), ID: u}, true
}

func problem(status int, detail string) *oas.ProblemStatusCode {
	return &oas.ProblemStatusCode{StatusCode: status, Response: httpx.Problem(status, detail)}
}
