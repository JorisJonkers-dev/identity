// Package web is the audit context's inbound HTTP adapter: listAuditEvents over the use cases, for
// the signed-in User.
package web

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
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
}

// Audit implements listAuditEvents.
type Audit struct{ uc UseCases }

// New returns an Audit calling uc.
func New(uc UseCases) *Audit { return &Audit{uc: uc} }

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
			return problem(http.StatusBadRequest, "That page no longer exists. Reload the page."), nil
		}
		f.Before = c
	}
	p := principal.From(ctx)
	page, err := h.uc.List(ctx, app.Actor{ID: p.UserID, SecondFactor: p.SecondFactor}, f)
	switch {
	case errors.Is(err, app.ErrNotAdmin):
		return problem(http.StatusForbidden, "Only Admins read the audit log."), nil
	case errors.Is(err, app.ErrSecondFactorNeeded):
		return problem(http.StatusForbidden, "Sign out, then sign in again with a passkey or authenticator app to read the audit log. If you have neither, add one on the Security page first."), nil
	case err != nil:
		return nil, err
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
