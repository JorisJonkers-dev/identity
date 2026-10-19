package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// settings is every Client's step-up window; fail makes both methods fail.
type settings struct {
	minutes map[string]int
	fail    error
}

func (s *settings) StepUp(_ context.Context, clientID string) (int, error) {
	return s.minutes[clientID], s.fail
}

func (s *settings) SetStepUp(_ context.Context, clientID string, minutes int) error {
	if s.fail != nil {
		return s.fail
	}
	s.minutes[clientID] = minutes
	return nil
}

// stepping is a fixture whose spa wants two-factor within 15 minutes, and Ada's session proved it
// proved minutes ago.
func stepping(proved time.Duration) (*fixture, *settings) {
	f := newFixture()
	s := &settings{minutes: map[string]int{"spa": 15}}
	f.SetStepUps(s)
	f.accounts.twoFactor = true
	f.accounts.twoFactorAt = t0
	f.now = t0.Add(proved)
	return f, s
}

func TestASessionThatProvedTwoFactorWithinTheWindowSignsIn(t *testing.T) {
	f, _ := stepping(15 * time.Minute)
	if err := f.Complete(context.Background(), f.started(t).ID, "tok"); err != nil {
		t.Fatal(err)
	}
}

func TestASessionThatProvedTwoFactorBeforeTheWindowMustProveItAgain(t *testing.T) {
	f, _ := stepping(15*time.Minute + time.Second)
	if err := f.Complete(context.Background(), f.started(t).ID, "tok"); !errors.Is(err, app.ErrStepUpNeeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestASessionThatNeverProvedTwoFactorMustProveIt(t *testing.T) {
	f, _ := stepping(0)
	f.accounts.twoFactorAt = time.Time{}
	if err := f.Complete(context.Background(), f.started(t).ID, "tok"); !errors.Is(err, app.ErrStepUpNeeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestAUserWithNoTwoFactorIsToldToSetItUp(t *testing.T) {
	f, _ := stepping(0)
	f.accounts.twoFactor, f.accounts.twoFactorAt = false, time.Time{}
	if err := f.Complete(context.Background(), f.started(t).ID, "tok"); !errors.Is(err, app.ErrTwoFactorNeeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestAClientWithStepUpOffSignsInWithoutAProof(t *testing.T) {
	f, s := stepping(0)
	s.minutes["spa"] = 0
	f.accounts.twoFactorAt = time.Time{}
	if err := f.Complete(context.Background(), f.started(t).ID, "tok"); err != nil {
		t.Fatal(err)
	}
}

func TestAStaleSignInIsNotCompleted(t *testing.T) {
	f, _ := stepping(time.Hour)
	r := f.started(t)
	_ = f.Complete(context.Background(), r.ID, "tok")
	if got, _ := f.Request(context.Background(), r.ID); got.SignIn != nil || len(*f.events) != 0 {
		t.Fatalf("request = %+v, events %+v", got, *f.events)
	}
}

func TestASignInFailsWhenStepUpCannotBeRead(t *testing.T) {
	f, s := stepping(0)
	s.fail = errBoom
	if err := f.Complete(context.Background(), f.started(t).ID, "tok"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestASignInFailsWhenTwoFactorCannotBeLookedUp(t *testing.T) {
	f, _ := stepping(time.Hour)
	f.accounts.failHas = errBoom
	if err := f.Complete(context.Background(), f.started(t).ID, "tok"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

// tokens is a Client's live Personal Tokens, by count; revoking takes them all.
type tokens struct {
	live    map[string]int
	revoked []string
	by      uuid.UUID
	err     error
}

func (t *tokens) LiveForClient(_ context.Context, clientID string) (int, error) {
	return t.live[clientID], t.err
}

func (t *tokens) RevokeForClient(_ context.Context, actor uuid.UUID, clientID string) error {
	if t.err != nil {
		return t.err
	}
	t.revoked, t.by = append(t.revoked, clientID), actor
	t.live[clientID] = 0
	return nil
}

// stepUpsFixture is StepUps over in-memory settings, with root an Admin and two live tokens for
// Grafana.
type stepUpsFixture struct {
	*app.StepUps
	settings *settings
	events   *recorded
	tokens   *tokens
}

func newStepUps() *stepUpsFixture {
	s := &settings{minutes: map[string]int{}}
	events := &recorded{}
	tk := &tokens{live: map[string]int{"grafana": 2}}
	cs := clients{"grafana": {ID: "grafana", SignIn: domain.Native}}
	return &stepUpsFixture{StepUps: app.NewStepUps(s, cs, admins{root: true}, events, tk), settings: s, events: events, tokens: tk}
}

var asRoot = app.Actor{ID: root, SecondFactor: true}

func TestAnAdminSetsAClientsStepUpAndReadsItBack(t *testing.T) {
	f := newStepUps()
	if err := f.Set(context.Background(), asRoot, "grafana", 15); err != nil {
		t.Fatal(err)
	}
	if got, err := f.StepUp(context.Background(), asRoot, "grafana"); got.Minutes != 15 || err != nil {
		t.Fatalf("StepUp = %d, %v", got, err)
	}
}

func TestSettingStepUpIsAnAuditEventNamingTheWindow(t *testing.T) {
	f := newStepUps()
	_ = f.Set(context.Background(), asRoot, "grafana", 60)
	if len(*f.events) != 1 || (*f.events)[0] != (audit.Event{Action: audit.StepUpChanged, Actor: root, Client: "grafana", Minutes: 60}) {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestStepUpIsOffFiveFifteenOrSixtyMinutes(t *testing.T) {
	for _, minutes := range []int{0, 5, 15, 60} {
		t.Run(time.Duration(minutes*int(time.Minute)).String(), func(t *testing.T) {
			if err := newStepUps().Set(context.Background(), asRoot, "grafana", minutes); err != nil {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAnyOtherStepUpWindowIsRefusedAndNotKept(t *testing.T) {
	f := newStepUps()
	if err := f.Set(context.Background(), asRoot, "grafana", 30); !errors.Is(err, app.ErrInvalidStepUp) {
		t.Fatalf("err = %v", err)
	}
	if len(f.settings.minutes) != 0 || len(*f.events) != 0 {
		t.Fatalf("kept %v, events %v", f.settings.minutes, *f.events)
	}
}

func TestStepUpOnAClientThatIsNotRegisteredIsNotFound(t *testing.T) {
	f := newStepUps()
	if err := f.Set(context.Background(), asRoot, "wiki", 5); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Set = %v", err)
	}
	if _, err := f.StepUp(context.Background(), asRoot, "wiki"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("StepUp = %v", err)
	}
}

func TestOnlyAnAdminWhoProvedASecondFactorReadsOrSetsStepUp(t *testing.T) {
	for _, tc := range []struct {
		name  string
		actor app.Actor
		want  error
	}{
		{"someone who is not an Admin", app.Actor{ID: uuid.New(), SecondFactor: true}, app.ErrNotAdmin},
		{"an Admin without a Second Factor", app.Actor{ID: root}, app.ErrSecondFactorNeeded},
		{"an Admin check that fails", app.Actor{}, errBoom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newStepUps()
			if err := f.Set(context.Background(), tc.actor, "grafana", 5); !errors.Is(err, tc.want) {
				t.Fatalf("Set = %v", err)
			}
			if _, err := f.StepUp(context.Background(), tc.actor, "grafana"); !errors.Is(err, tc.want) {
				t.Fatalf("StepUp = %v", err)
			}
		})
	}
}

func TestAStepUpTheStoreRefusesIsNotAnAuditEvent(t *testing.T) {
	f := newStepUps()
	f.settings.fail = errBoom
	if err := f.Set(context.Background(), asRoot, "grafana", 5); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
	if len(*f.events) != 0 {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestTheStepUpReadCountsTheTokensTurningItOnWouldRevoke(t *testing.T) {
	if got, err := newStepUps().StepUp(context.Background(), asRoot, "grafana"); got != (app.StepUpState{Tokens: 2}) || err != nil {
		t.Fatalf("StepUp = %+v, %v", got, err)
	}
}

func TestTurningStepUpOnRevokesTheClientsTokensAsTheAdmin(t *testing.T) {
	f := newStepUps()
	_ = f.Set(context.Background(), asRoot, "grafana", 5)
	if len(f.tokens.revoked) != 1 || f.tokens.revoked[0] != "grafana" || f.tokens.by != root {
		t.Fatalf("revoked %v by %v", f.tokens.revoked, f.tokens.by)
	}
}

func TestTurningStepUpOffRevokesNothing(t *testing.T) {
	f := newStepUps()
	_ = f.Set(context.Background(), asRoot, "grafana", 0)
	if len(f.tokens.revoked) != 0 {
		t.Fatalf("revoked %v", f.tokens.revoked)
	}
}

func TestStepUpIsNotSetWhenItsTokensCannotBeRevoked(t *testing.T) {
	f := newStepUps()
	f.tokens.err = errBoom
	if err := f.Set(context.Background(), asRoot, "grafana", 5); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
	if f.settings.minutes["grafana"] != 0 || len(*f.events) != 0 {
		t.Fatalf("set %v, events %v", f.settings.minutes, *f.events)
	}
}

func TestTheStepUpReadFailsWhenTheTokensOrTheSettingCannotBeRead(t *testing.T) {
	for _, name := range []string{"tokens", "setting"} {
		t.Run(name, func(t *testing.T) {
			f := newStepUps()
			if name == "tokens" {
				f.tokens.err = errBoom
			} else {
				f.settings.fail = errBoom
			}
			if _, err := f.StepUp(context.Background(), asRoot, "grafana"); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
