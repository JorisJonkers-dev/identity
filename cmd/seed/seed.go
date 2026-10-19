//go:build dev

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/adapters/catalog"
	accesspeople "github.com/JorisJonkers-dev/identity/internal/access/adapters/people"
	accesspersistence "github.com/JorisJonkers-dev/identity/internal/access/adapters/persistence"
	accessapp "github.com/JorisJonkers-dev/identity/internal/access/app"
	"github.com/JorisJonkers-dev/identity/internal/assembly"
	auditpersistence "github.com/JorisJonkers-dev/identity/internal/audit/adapters/persistence"
	auditapp "github.com/JorisJonkers-dev/identity/internal/audit/app"
	auditdomain "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/vault"
	"github.com/JorisJonkers-dev/identity/internal/server"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	"github.com/JorisJonkers-dev/identity/internal/users/adapters/devcode"
	userspersistence "github.com/JorisJonkers-dev/identity/internal/users/adapters/persistence"
	usersapp "github.com/JorisJonkers-dev/identity/internal/users/app"
)

// run seeds the identity DATABASE_URL names, unless joris@example.org already exists there.
func run(ctx context.Context, getenv func(string) string, stdout io.Writer, logger *slog.Logger) error {
	store, err := pg.Open(ctx, getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer store.Close()
	users := usersapp.New(userspersistence.New(store.Queries()), nil, nil, nil, nil, "")
	_, err = users.UserByEmail(ctx, joris.email)
	if err == nil {
		_, _ = fmt.Fprintln(stdout, "Already seeded.")
		hints(stdout)
		return nil
	}
	if !errors.Is(err, usersapp.ErrNotFound) {
		return err
	}

	index := clients.NewIndex()
	if err := clients.LoadFile(getenv("CLIENTS_FILE"), index); err != nil {
		return err
	}
	index.Put(oldWiki)
	c := &clock{at: time.Now()}
	mail := &mailbox{last: map[string]string{}}
	v := vault.New(getenv("VAULT_ADDR"), vault.Token(getenv("VAULT_TOKEN")), &http.Client{Timeout: 5 * time.Second})
	transit := v.Transit("transit")
	built, err := assembly.New(ctx, assembly.Config{
		Store: store, PublicURL: "http://localhost:5173", Mailer: mail,
		Sealer: transit.Key("identity-totp"), KeySealer: transit.Key("identity-signing"), KV: v.KV(),
		TOTPMatch: devcode.Match, Index: index, Synced: func() bool { return true },
		Logger: logger, Version: "seed", Now: c.now,
	})
	if err != nil {
		return err
	}
	q := store.Queries()
	audit := auditapp.New(auditpersistence.New(q), nil, nil, logger, c.now, rand.Reader)
	s := &seeder{
		handler: server.New(logger, "seed", built.Routes).Handler(),
		mail:    mail,
		clock:   c,
		users:   users,
		access:  accessapp.New(accesspersistence.New(q, store), catalog.New(index), accesspeople.New(users), audit, rand.Reader),
		audit:   audit,
		ids:     map[string]uuid.UUID{},
		end:     time.Now(),
	}
	if err := s.seed(ctx); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "Seeded the design canvas's people, Groups, apps, tokens and a day of Audit Events.")
	hints(stdout)
	return nil
}

func hints(w io.Writer) {
	_, _ = fmt.Fprintf(w, "Sign in at http://localhost:5173 as %s with the password %q and the code %s.\n", joris.email, password, devcode.Code)
	_, _ = fmt.Fprintln(w, "Everyone else signs in with the same password, as name@example.org. Mail lands in http://localhost:28025.")
}

// seeder runs the seed through identity's own API and use cases, on a clock it moves.
type seeder struct {
	handler http.Handler
	mail    *mailbox
	clock   *clock
	users   *usersapp.Service
	access  *accessapp.Service
	audit   *auditapp.Service
	ids     map[string]uuid.UUID
	// end is the moment the seed's last day ends: when it started.
	end time.Time
}

// at sets the clock to ago before the seed started.
func (s *seeder) at(ago time.Duration) { s.clock.set(s.end.Add(-ago)) }

func (s *seeder) seed(ctx context.Context) error {
	for _, p := range people {
		s.at(signedUp(p))
		if err := s.signUp(ctx, p); err != nil {
			return err
		}
	}
	s.at(longAgo)
	for i, p := range []person{joris, femke} {
		if err := s.access.Promote(ctx, s.ids[p.email], i > 0); err != nil {
			return err
		}
	}
	if err := s.enrolAll(ctx); err != nil {
		return err
	}
	admin, err := s.signedIn(ctx, joris)
	if err != nil {
		return err
	}
	if err := s.groups(ctx, admin); err != nil {
		return err
	}
	if err := s.mint(ctx); err != nil {
		return err
	}
	if err := s.day(ctx); err != nil {
		return err
	}
	return s.lastHours(ctx)
}

// signUp signs p up and confirms the mailed link, as they would in the browser.
func (s *seeder) signUp(ctx context.Context, p person) error {
	b := newBrowser(s.handler)
	if err := b.call(ctx, http.MethodPost, "/api/v1/signup", map[string]string{"email": p.email, "name": p.name, "password": password}, http.StatusAccepted, nil); err != nil {
		return err
	}
	token, err := s.mail.confirmation(p.email)
	if err != nil {
		return err
	}
	if err := b.call(ctx, http.MethodPost, "/api/v1/confirmations", map[string]string{"token": token, "password": password}, http.StatusNoContent, nil); err != nil {
		return err
	}
	u, err := s.users.UserByEmail(ctx, p.email)
	if err != nil {
		return err
	}
	s.ids[p.email] = u.ID
	return nil
}

// enrolAll adds an authenticator app for everyone who has one, confirmed with the dev code.
func (s *seeder) enrolAll(ctx context.Context) error {
	for _, p := range people {
		if !p.authenticator {
			continue
		}
		b, err := s.signedIn(ctx, person{name: p.name, email: p.email}) // no authenticator app yet
		if err != nil {
			return err
		}
		if err := b.call(ctx, http.MethodPost, "/api/v1/profile/second-factors/totp", nil, http.StatusOK, nil); err != nil {
			return err
		}
		if err := b.call(ctx, http.MethodPost, "/api/v1/profile/second-factors/totp/confirmation",
			map[string]string{"label": "Phone", "code": devcode.Code, "password": password}, http.StatusNoContent, nil); err != nil {
			return err
		}
	}
	return nil
}

// signedIn is p's browser, signed in with their password and, if they have an authenticator app,
// the dev code.
func (s *seeder) signedIn(ctx context.Context, p person) (*browser, error) {
	b := newBrowser(s.handler)
	status := http.StatusNoContent
	if p.authenticator {
		status = http.StatusAccepted
	}
	if err := b.call(ctx, http.MethodPost, "/api/v1/session", map[string]string{"email": p.email, "password": password}, status, nil); err != nil {
		return nil, err
	}
	if p.authenticator {
		if err := b.call(ctx, http.MethodPost, "/api/v1/session/second-factor", map[string]string{"code": devcode.Code}, http.StatusNoContent, nil); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// groups makes the canvas's Groups as Joris, with their members and Assignments, except what the
// last hours add.
func (s *seeder) groups(ctx context.Context, admin *browser) error {
	for _, g := range groups {
		var made struct{ ID string }
		if err := admin.call(ctx, http.MethodPost, "/api/v1/access/groups", map[string]string{"name": g.name}, http.StatusCreated, &made); err != nil {
			return err
		}
		s.ids["group:"+g.name] = uuid.MustParse(made.ID)
		for _, m := range g.members {
			if err := s.addMember(ctx, admin, g.name, m); err != nil {
				return err
			}
		}
		for _, a := range g.roles {
			if err := s.assign(ctx, admin, g.name, a); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *seeder) addMember(ctx context.Context, admin *browser, groupName string, p person) error {
	path := fmt.Sprintf("/api/v1/access/groups/%s/members/%s", s.ids["group:"+groupName], s.ids[p.email])
	return admin.call(ctx, http.MethodPut, path, nil, http.StatusNoContent, nil)
}

func (s *seeder) assign(ctx context.Context, admin *browser, groupName string, a assignment) error {
	path := fmt.Sprintf("/api/v1/access/groups/%s/roles/%s/%s", s.ids["group:"+groupName], a.client, a.role)
	return admin.call(ctx, http.MethodPut, path, nil, http.StatusNoContent, nil)
}

// mint makes each Personal Token when its owner did, and revokes the ones that were revoked.
func (s *seeder) mint(ctx context.Context) error {
	for _, t := range tokens {
		s.at(t.ago)
		b, err := s.signedIn(ctx, t.owner)
		if err != nil {
			return err
		}
		var minted struct{ Token struct{ ID string } }
		if err := b.call(ctx, http.MethodPost, "/api/v1/profile/tokens", map[string]any{"name": t.name, "clientId": t.clientID, "days": t.days}, http.StatusCreated, &minted); err != nil {
			return err
		}
		if t.revoked {
			s.at(t.ago - 24*time.Hour)
			if err := b.call(ctx, http.MethodDelete, "/api/v1/profile/tokens/"+minted.Token.ID, nil, http.StatusNoContent, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// apps is where each person signs in to, by the Groups they will be in.
var apps = map[string][]string{
	joris.email: {"grafana", "jellyfin", "grimoire"}, ada.email: {"grafana", "vault", "jellyfin"},
	femke.email: {"vault", "grafana"}, pieter.email: {"jellyfin"}, lotte.email: {"jellyfin", "grimoire"},
	daan.email: {"grimoire", "jellyfin"}, ruben.email: {"grimoire"}, iris.email: {"grimoire", "jellyfin"},
	thijs.email: {"grimoire"},
}

// day is the last 24 hours of signing in: people sign in to identity, then to their apps, busier
// in the evening; and four sign-ins fail.
func (s *seeder) day(ctx context.Context) error {
	regulars := []person{joris, ada, femke, noor, sam, pieter, lotte, daan, ruben, iris, thijs}
	for hour := 23; hour >= 1; hour-- {
		for i := range 2 + (24-hour)%7 {
			ago := time.Duration(hour)*time.Hour - time.Duration(i)*7*time.Minute
			if err := s.visit(ctx, regulars[(hour*5+i)%len(regulars)], ago, hour > 4); err != nil {
				return err
			}
		}
	}
	return s.failures(ctx)
}

// visit is p signing in ago, then to their apps, and signing out again 40 minutes later if leaves;
// the evening's sessions stay, as the dashboard's active ones.
func (s *seeder) visit(ctx context.Context, p person, ago time.Duration, leaves bool) error {
	s.at(ago)
	b, err := s.signedIn(ctx, p)
	if err != nil {
		return err
	}
	for _, client := range apps[p.email] {
		s.audit.Record(ctx, auditdomain.Event{Action: auditdomain.SignedIn, User: s.ids[p.email], Client: client})
	}
	if !leaves {
		return nil
	}
	s.at(ago - 40*time.Minute)
	return b.call(ctx, http.MethodDelete, "/api/v1/session", nil, http.StatusNoContent, nil)
}

// failures are the day's four failed sign-ins: Sam twice, a mistyped address, and Mila.
func (s *seeder) failures(ctx context.Context) error {
	for _, f := range []struct {
		ago   time.Duration
		email string
	}{{19 * time.Hour, sam.email}, {11 * time.Hour, "sam.devries@example.org"}, {40 * time.Minute, mila.email}, {150 * time.Minute, sam.email}} {
		s.at(f.ago)
		b := newBrowser(s.handler)
		if err := b.call(ctx, http.MethodPost, "/api/v1/session", map[string]string{"email": f.email, "password": "not-the-password"}, http.StatusUnauthorized, nil); err != nil {
			return err
		}
	}
	return nil
}

// lastHours is canvas frame C4: Ada mints a token, Joris adds Noor to Family and gives Family
// viewer on Grafana.
func (s *seeder) lastHours(ctx context.Context) error {
	s.at(3 * time.Hour)
	b, err := s.signedIn(ctx, ada)
	if err != nil {
		return err
	}
	if err := b.call(ctx, http.MethodPost, "/api/v1/profile/tokens", map[string]any{"name": "grafana-dashboards", "clientId": "grafana", "days": 90}, http.StatusCreated, nil); err != nil {
		return err
	}
	s.at(2 * time.Hour)
	admin, err := s.signedIn(ctx, joris)
	if err != nil {
		return err
	}
	if err := s.addMember(ctx, admin, "Family", noor); err != nil {
		return err
	}
	s.at(90 * time.Minute)
	return s.assign(ctx, admin, "Family", assignment{"grafana", "viewer"})
}
