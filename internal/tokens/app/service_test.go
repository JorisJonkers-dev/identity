package app_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/tokens/app"
	"github.com/JorisJonkers-dev/identity/internal/tokens/domain"
)

var (
	errBoom = errors.New("boom")
	t0      = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	ada     = uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
	bob     = uuid.MustParse("00000000-0000-4000-8000-0000000000a2")
	root    = uuid.MustParse("00000000-0000-4000-8000-0000000000a3")
	asAdmin = app.Actor{ID: root, TwoFactor: true}
)

// store keeps tokens in memory; fail makes the named method return its error.
type store struct {
	tokens  map[uuid.UUID]domain.Token
	hashes  map[string]uuid.UUID
	touched []uuid.UUID
	until   [2]time.Time
	fail    map[string]error
}

func newStore() *store {
	return &store{tokens: map[uuid.UUID]domain.Token{}, hashes: map[string]uuid.UUID{}, fail: map[string]error{}}
}

func (s *store) Create(_ context.Context, t domain.Token, hash []byte) error {
	if err := s.fail["Create"]; err != nil {
		return err
	}
	s.tokens[t.ID], s.hashes[string(hash)] = t, t.ID
	return nil
}

func (s *store) ByHash(_ context.Context, hash []byte) (domain.Token, error) {
	id, ok := s.hashes[string(hash)]
	if !ok {
		return domain.Token{}, app.ErrNotFound
	}
	return s.tokens[id], s.fail["ByHash"]
}

func (s *store) ByID(_ context.Context, id uuid.UUID) (domain.Token, error) {
	if err := s.fail["ByID"]; err != nil {
		return domain.Token{}, err
	}
	t, ok := s.tokens[id]
	if !ok {
		return domain.Token{}, app.ErrNotFound
	}
	return t, nil
}

func (s *store) OfUser(_ context.Context, userID uuid.UUID) ([]domain.Token, error) {
	var out []domain.Token
	for _, t := range s.tokens {
		if t.UserID == userID {
			out = append(out, t)
		}
	}
	return out, s.fail["OfUser"]
}

func (s *store) RevokeForClient(_ context.Context, clientID string, at time.Time) ([]domain.Token, error) {
	if err := s.fail["RevokeForClient"]; err != nil {
		return nil, err
	}
	var out []domain.Token
	for id, t := range s.tokens {
		if t.ClientID == clientID && t.RevokedAt == nil && t.ExpiresAt.After(at) {
			t.RevokedAt = &at
			s.tokens[id] = t
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *store) LiveForClient(_ context.Context, clientID string, now time.Time) (int, error) {
	n := 0
	for _, t := range s.tokens {
		if t.ClientID == clientID && t.RevokedAt == nil && t.ExpiresAt.After(now) {
			n++
		}
	}
	return n, s.fail["LiveForClient"]
}

func (s *store) Revoke(_ context.Context, id, userID uuid.UUID, at time.Time) (bool, error) {
	if err := s.fail["Revoke"]; err != nil {
		return false, err
	}
	t := s.tokens[id]
	if t.UserID != userID || t.RevokedAt != nil {
		return false, nil
	}
	t.RevokedAt = &at
	s.tokens[id] = t
	return true, nil
}

func (s *store) Touch(_ context.Context, id uuid.UUID, _ time.Time) error {
	s.touched = append(s.touched, id)
	return s.fail["Touch"]
}

func (s *store) CountExpiring(_ context.Context, now, before time.Time) (int, error) {
	s.until = [2]time.Time{now, before}
	return 2, s.fail["CountExpiring"]
}

// contexts is Ada holding viewer on Grafana and nothing on Wiki; Gone is a Removed Client.
type contexts struct {
	held map[string][]string
	err  error
	// stepUp is the Clients with step-up on; stepUpErr makes asking fail.
	stepUp    map[string]bool
	stepUpErr error
}

func (c *contexts) StepUp(_ context.Context, id string) (bool, error) {
	return c.stepUp[id], c.stepUpErr
}

func (c *contexts) Roles(_ context.Context, userID uuid.UUID, clientID string) ([]string, error) {
	if userID != ada {
		return nil, c.err
	}
	return c.held[clientID], c.err
}

func (c *contexts) ByClient(_ context.Context, userID uuid.UUID) (map[string][]string, error) {
	if userID != ada {
		return nil, c.err
	}
	return c.held, c.err
}

func (*contexts) Exists(_ context.Context, id string) bool {
	return id == "grafana" || id == "wiki" || id == "board"
}

func (*contexts) Label(_ context.Context, id string) string {
	return map[string]string{"grafana": "Grafana", "board": "Board"}[id]
}

type admins struct{ err error }

func (a admins) IsAdmin(_ context.Context, id uuid.UUID) (bool, error) { return id == root, a.err }

type recorded []audit.Event

func (r *recorded) Record(_ context.Context, e audit.Event) { *r = append(*r, e) }

type fixture struct {
	*app.Service
	store    *store
	contexts *contexts
	events   *recorded
	now      time.Time
}

func newFixture() *fixture {
	f := &fixture{
		store:    newStore(),
		contexts: &contexts{held: map[string][]string{"grafana": {"viewer"}, "gone": {"user"}, "wiki": {}}},
		events:   &recorded{},
		now:      t0,
	}
	f.Service = app.New(f.store, f.contexts, f.contexts, admins{}, f.events, func() time.Time { return f.now }, counting())
	return f
}

type reader func([]byte) (int, error)

func (r reader) Read(p []byte) (int, error) { return r(p) }

func counting() reader {
	var n byte
	return func(p []byte) (int, error) {
		n++
		for i := range p {
			p[i] = n
		}
		return len(p), nil
	}
}

func (f *fixture) minted(t *testing.T) app.Minted {
	t.Helper()
	m, err := f.Mint(context.Background(), ada, "grafana-cli", "grafana", 30)
	if err != nil {
		t.Fatal(err)
	}
	*f.events = nil
	return m
}

func TestAMintedTokenIsShownOnceAndKeptAsItsHash(t *testing.T) {
	m := newFixture().minted(t)
	if !strings.HasPrefix(m.Secret, "idpat_") || len(m.Secret) < 40 {
		t.Fatalf("secret = %q", m.Secret)
	}
	f := newFixture()
	m = f.minted(t)
	sum := sha256.Sum256([]byte(m.Secret))
	if f.store.hashes[string(sum[:])] != m.Token.ID {
		t.Fatal("the token is not kept by its secret's hash")
	}
}

func TestAMintedTokenIsForOneClientAndLastsTheDaysChosen(t *testing.T) {
	m := newFixture().minted(t)
	want := domain.Token{ID: m.Token.ID, UserID: ada, ClientID: "grafana", Name: "grafana-cli", CreatedAt: t0, ExpiresAt: t0.Add(30 * 24 * time.Hour)}
	if m.Token != want {
		t.Fatalf("token = %+v", m.Token)
	}
}

func TestMintingIsAnAuditEvent(t *testing.T) {
	f := newFixture()
	_, _ = f.Mint(context.Background(), ada, "grafana-cli", "grafana", 30)
	if want := []audit.Event{{Action: audit.TokenMinted, User: ada, Client: "grafana"}}; !slices.Equal(*f.events, want) {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestMintingIsRefusedForWhatItCannotBe(t *testing.T) {
	for _, tc := range []struct {
		name, tokenName, client string
		days                    int
		want                    error
	}{
		{"an empty name", " ", "grafana", 30, domain.ErrInvalidName},
		{"a lifetime not offered", "cli", "grafana", 45, domain.ErrInvalidLifetime},
		{"an app not in the cluster", "cli", "gone", 30, app.ErrUnknownClient},
		{"an app the User holds no role on", "cli", "wiki", 30, app.ErrNoAccess},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			if _, err := f.Mint(context.Background(), ada, tc.tokenName, tc.client, tc.days); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if len(f.store.tokens) != 0 || len(*f.events) != 0 {
				t.Fatal("a refused token was kept or recorded")
			}
		})
	}
}

func TestMintingFailsWithWhatItNeeds(t *testing.T) {
	for name, broken := range map[string]func(f *fixture){
		"the roles": func(f *fixture) { f.contexts.err = errBoom },
		"the store": func(f *fixture) { f.store.fail["Create"] = errBoom },
		"an id": func(f *fixture) {
			f.Service = app.New(f.store, f.contexts, f.contexts, admins{}, f.events, time.Now, bytes.NewReader(nil))
		},
		"the secret": func(f *fixture) {
			f.Service = app.New(f.store, f.contexts, f.contexts, admins{}, f.events, time.Now, bytes.NewReader(make([]byte, 16)))
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			broken(f)
			if _, err := f.Mint(context.Background(), ada, "cli", "grafana", 30); err == nil {
				t.Fatal("minting went ahead")
			}
			if len(*f.events) != 0 {
				t.Fatal("a failed mint was recorded")
			}
		})
	}
}

func TestAUserSeesTheirOwnTokens(t *testing.T) {
	f := newFixture()
	m := f.minted(t)
	if mine, err := f.Mine(context.Background(), ada); err != nil || len(mine) != 1 || mine[0].ID != m.Token.ID {
		t.Fatalf("Mine = %+v, %v", mine, err)
	}
}

func TestAUserMayMintForTheAppsInTheClusterTheyHoldARoleOn(t *testing.T) {
	f := newFixture()
	f.contexts.held["board"] = []string{"user"}
	got, err := f.Mintable(context.Background(), ada)
	if want := []app.Client{{ID: "board", Label: "Board"}, {ID: "grafana", Label: "Grafana"}}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("Mintable = %+v, %v", got, err)
	}
}

func TestTheAppsToMintForFailWhenTheRolesCannotBeRead(t *testing.T) {
	f := newFixture()
	f.contexts.err = errBoom
	if _, err := f.Mintable(context.Background(), ada); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestATokenCarriesTheRolesItsUserHoldsOnItsClientNow(t *testing.T) {
	f := newFixture()
	m := f.minted(t)
	f.contexts.held["grafana"] = []string{"editor", "viewer"}
	if roles, err := f.RolesOf(context.Background(), m.Token); err != nil || !slices.Equal(roles, []string{"editor", "viewer"}) {
		t.Fatalf("RolesOf = %v, %v", roles, err)
	}
}

func TestATokensClientIsCalledByItsLabel(t *testing.T) {
	if got := newFixture().Label(context.Background(), "grafana"); got != "Grafana" {
		t.Fatalf("label = %q", got)
	}
}

func TestATokensStateIsReadAtNow(t *testing.T) {
	f := newFixture()
	m := f.minted(t)
	f.now = t0.Add(25 * 24 * time.Hour)
	if got := f.State(m.Token); got != domain.Expiring {
		t.Fatalf("state = %s", got)
	}
}

func TestAUserRevokesTheirTokenAndItIsRecorded(t *testing.T) {
	f := newFixture()
	m := f.minted(t)
	if err := f.RevokeMine(context.Background(), ada, m.Token.ID); err != nil || f.store.tokens[m.Token.ID].RevokedAt == nil {
		t.Fatalf("RevokeMine = %v", err)
	}
	if want := []audit.Event{{Action: audit.TokenRevoked, User: ada, Client: "grafana"}}; !slices.Equal(*f.events, want) {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestRevokingATokenSaysWhichOnceSoItsCachedChecksCanGo(t *testing.T) {
	f := newFixture()
	m := f.minted(t)
	var told []uuid.UUID
	f.OnRevoked(func(_ context.Context, id uuid.UUID) { told = append(told, id) })
	for range 2 {
		_ = f.RevokeMine(context.Background(), ada, m.Token.ID)
	}
	if !slices.Equal(told, []uuid.UUID{m.Token.ID}) {
		t.Fatalf("told %v", told)
	}
}

func TestRevokingATokenTwiceRecordsItOnce(t *testing.T) {
	f := newFixture()
	m := f.minted(t)
	for range 2 {
		if err := f.RevokeMine(context.Background(), ada, m.Token.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(*f.events) != 1 {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestAUserCannotRevokeAnotherUsersToken(t *testing.T) {
	f := newFixture()
	m := f.minted(t)
	if err := f.RevokeMine(context.Background(), bob, m.Token.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestRevokingATokenThatDoesNotExistIsNotFound(t *testing.T) {
	if err := newFixture().RevokeMine(context.Background(), ada, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestRevokingFailsWhenTheStoreDoes(t *testing.T) {
	f := newFixture()
	m := f.minted(t)
	f.store.fail["Revoke"] = errBoom
	if err := f.RevokeMine(context.Background(), ada, m.Token.ID); !errors.Is(err, errBoom) || len(*f.events) != 0 {
		t.Fatalf("err = %v, events %+v", err, *f.events)
	}
}

func TestAnAdminSeesAndRevokesAUsersTokenAsThemself(t *testing.T) {
	f := newFixture()
	m := f.minted(t)
	if got, err := f.Of(context.Background(), asAdmin, ada); err != nil || len(got) != 1 {
		t.Fatalf("Of = %+v, %v", got, err)
	}
	if err := f.RevokeAs(context.Background(), asAdmin, ada, m.Token.ID); err != nil {
		t.Fatal(err)
	}
	if want := []audit.Event{{Action: audit.TokenRevoked, Actor: root, User: ada, Client: "grafana"}}; !slices.Equal(*f.events, want) {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestOnlyAnAdminWithTwoFactorManagesOthersTokens(t *testing.T) {
	for _, tc := range []struct {
		name  string
		actor app.Actor
		want  error
	}{
		{"someone who is not an Admin", app.Actor{ID: bob, TwoFactor: true}, app.ErrNotAdmin},
		{"an Admin whose session proved no two-factor", app.Actor{ID: root}, app.ErrTwoFactorProofNeeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			m := f.minted(t)
			if _, err := f.Of(context.Background(), tc.actor, ada); !errors.Is(err, tc.want) {
				t.Fatalf("Of = %v", err)
			}
			if err := f.RevokeAs(context.Background(), tc.actor, ada, m.Token.ID); !errors.Is(err, tc.want) {
				t.Fatalf("RevokeAs = %v", err)
			}
		})
	}
}

func TestManagingOthersTokensFailsWhenTheAdminCheckCannotBeMade(t *testing.T) {
	f := newFixture()
	f.Service = app.New(f.store, f.contexts, f.contexts, admins{err: errBoom}, f.events, time.Now, counting())
	if _, err := f.Of(context.Background(), asAdmin, ada); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestASecretResolvesToItsTokenWhetherLiveOrNot(t *testing.T) {
	f := newFixture()
	m := f.minted(t)
	_ = f.RevokeMine(context.Background(), ada, m.Token.ID)
	if got, err := f.Resolve(context.Background(), m.Secret); err != nil || got.ID != m.Token.ID {
		t.Fatalf("Resolve = %+v, %v", got, err)
	}
}

func TestASecretWithoutThePrefixResolvesToNothing(t *testing.T) {
	f := newFixture()
	m := f.minted(t)
	if _, err := f.Resolve(context.Background(), strings.TrimPrefix(m.Secret, "idpat_")); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestALiveTokenChecksAsActiveWithTheRolesItsUserHoldsNow(t *testing.T) {
	f := newFixture()
	m := f.minted(t)
	got, err := f.Check(context.Background(), m.Token.ID, "grafana")
	if err != nil || got.Token.ID != m.Token.ID || !slices.Equal(got.Roles, []string{"viewer"}) {
		t.Fatalf("Check = %+v, %v", got, err)
	}
	if !slices.Equal(f.store.touched, []uuid.UUID{m.Token.ID}) {
		t.Fatalf("touched = %v", f.store.touched)
	}
}

func TestATokenChecksAsInactiveWhenItCannotBeUsed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		client string
		change func(f *fixture, id uuid.UUID)
	}{
		{"for another app", "wiki", func(*fixture, uuid.UUID) {}},
		{"once revoked", "grafana", func(f *fixture, id uuid.UUID) { _ = f.RevokeMine(context.Background(), ada, id) }},
		{"once expired", "grafana", func(f *fixture, _ uuid.UUID) { f.now = t0.Add(30 * 24 * time.Hour) }},
		{"once its User holds no role there", "grafana", func(f *fixture, _ uuid.UUID) { f.contexts.held["grafana"] = nil }},
		{"when no token has its id", "grafana", func(f *fixture, id uuid.UUID) { delete(f.store.tokens, id) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			m := f.minted(t)
			tc.change(f, m.Token.ID)
			if _, err := f.Check(context.Background(), m.Token.ID, tc.client); !errors.Is(err, app.ErrInactive) {
				t.Fatalf("err = %v", err)
			}
			if len(f.store.touched) != 0 {
				t.Fatal("an inactive token was recorded as used")
			}
		})
	}
}

func TestACheckFailsWithWhatItReads(t *testing.T) {
	for _, tc := range []struct {
		name   string
		broken func(f *fixture)
	}{
		{"the token", func(f *fixture) { f.store.fail["ByID"] = errBoom }},
		{"the roles", func(f *fixture) { f.contexts.err = errBoom }},
		{"the record of its use", func(f *fixture) { f.store.fail["Touch"] = errBoom }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			m := f.minted(t)
			tc.broken(f)
			if _, err := f.Check(context.Background(), m.Token.ID, "grafana"); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestTheDashboardCountsTokensEndingWithinSevenDays(t *testing.T) {
	f := newFixture()
	n, err := f.ExpiringSoon(context.Background())
	if err != nil || n != 2 || f.store.until != [2]time.Time{t0, t0.Add(7 * 24 * time.Hour)} {
		t.Fatalf("ExpiringSoon = %d, %v; window %v", n, err, f.store.until)
	}
}

func TestATokenForAStepUpClientIsRefused(t *testing.T) {
	f := newFixture()
	f.contexts.stepUp = map[string]bool{"grafana": true}
	if _, err := f.Mint(context.Background(), ada, "cli", "grafana", 30); !errors.Is(err, app.ErrStepUpClient) {
		t.Fatalf("err = %v", err)
	}
}

func TestAStepUpClientIsNotOfferedForTokens(t *testing.T) {
	f := newFixture()
	f.contexts.stepUp = map[string]bool{"grafana": true}
	if got, err := f.Mintable(context.Background(), ada); err != nil || len(got) != 0 {
		t.Fatalf("Mintable = %+v, %v", got, err)
	}
}

func TestATokenForAClientThatTurnedStepUpOnIsInactive(t *testing.T) {
	f := newFixture()
	m, _ := f.Mint(context.Background(), ada, "cli", "grafana", 30)
	f.contexts.stepUp = map[string]bool{"grafana": true}
	if _, err := f.Check(context.Background(), m.Token.ID, "grafana"); !errors.Is(err, app.ErrInactive) {
		t.Fatalf("err = %v", err)
	}
}

func TestTurningStepUpOnRevokesEveryLiveTokenForTheClientAsAuditEvents(t *testing.T) {
	var forgotten []uuid.UUID
	f := newFixture()
	f.OnRevoked(func(_ context.Context, id uuid.UUID) { forgotten = append(forgotten, id) })
	m, _ := f.Mint(context.Background(), ada, "cli", "grafana", 30)
	if n, _ := f.LiveForClient(context.Background(), "grafana"); n != 1 {
		t.Fatalf("live = %d", n)
	}
	*f.events = nil
	if err := f.RevokeForClient(context.Background(), root, "grafana"); err != nil {
		t.Fatal(err)
	}
	if len(forgotten) != 1 || forgotten[0] != m.Token.ID || len(*f.events) != 1 || (*f.events)[0] != (audit.Event{Action: audit.TokenRevoked, Actor: root, User: ada, Client: "grafana"}) {
		t.Fatalf("forgot %v, events %+v", forgotten, *f.events)
	}
	if n, _ := f.LiveForClient(context.Background(), "grafana"); n != 0 {
		t.Fatalf("live after = %d", n)
	}
}

func TestTurningStepUpOnRevokesTheClientsTokensWithNoCacheToTell(t *testing.T) {
	f := newFixture()
	_, _ = f.Mint(context.Background(), ada, "cli", "grafana", 30)
	if err := f.RevokeForClient(context.Background(), root, "grafana"); err != nil {
		t.Fatal(err)
	}
	if n, _ := f.LiveForClient(context.Background(), "grafana"); n != 0 {
		t.Fatalf("live after = %d", n)
	}
}

func TestStepUpChecksFailWhenStepUpCannotBeRead(t *testing.T) {
	for _, step := range []struct {
		name string
		call func(f *fixture, tokenID uuid.UUID) error
	}{
		{"minting", func(f *fixture, _ uuid.UUID) error {
			_, err := f.Mint(context.Background(), ada, "cli", "grafana", 30)
			return err
		}},
		{"offering", func(f *fixture, _ uuid.UUID) error { _, err := f.Mintable(context.Background(), ada); return err }},
		{"checking", func(f *fixture, id uuid.UUID) error {
			_, err := f.Check(context.Background(), id, "grafana")
			return err
		}},
	} {
		t.Run(step.name, func(t *testing.T) {
			f := newFixture()
			m, _ := f.Mint(context.Background(), ada, "cli", "grafana", 30)
			f.contexts.stepUpErr = errBoom
			if err := step.call(f, m.Token.ID); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestRevokingAClientsTokensFailsWhenTheStoreDoes(t *testing.T) {
	f := newFixture()
	f.store.fail["RevokeForClient"] = errBoom
	if err := f.RevokeForClient(context.Background(), root, "grafana"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}
