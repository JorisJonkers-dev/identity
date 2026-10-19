package app_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// consents is the connected context: Claude Code is registered as mcp-1; refuse is what every
// consent fails with when set, and fail what one method fails with.
type consents struct {
	given  map[string]bool
	refuse error
	fail   map[string]error
}

func newConsents() *consents { return &consents{given: map[string]bool{}, fail: map[string]error{}} }

func key(userID uuid.UUID, reg, client string) string { return userID.String() + reg + client }

func (c *consents) Consented(_ context.Context, userID uuid.UUID, reg, client string) error {
	if c.refuse != nil {
		return c.refuse
	}
	if !c.given[key(userID, reg, client)] {
		return app.ErrConsentNeeded
	}
	return nil
}

func (c *consents) Consent(_ context.Context, userID uuid.UUID, reg, client string) error {
	if c.refuse != nil {
		return c.refuse
	}
	if err := c.fail["Consent"]; err != nil {
		return err
	}
	c.given[key(userID, reg, client)] = true
	return nil
}

func (c *consents) Name(_ context.Context, reg string) (string, error) {
	if err := c.fail["Name"]; err != nil {
		return "", err
	}
	if reg != "mcp-1" {
		return "", app.ErrNotFound
	}
	return "Claude Code", nil
}

// connectedFixture is a fixture taking Connected Apps, with Claude Code's request to act on spa.
func connectedFixture(t *testing.T) (*fixture, *consents, domain.AuthRequest) {
	t.Helper()
	f := newFixture()
	c := newConsents()
	f.SetConsents(c)
	r, err := f.Start(context.Background(), domain.AuthRequest{ClientID: "mcp-1", Resource: "spa", RedirectURI: "http://localhost:1/cb", State: "st"})
	if err != nil {
		t.Fatal(err)
	}
	return f, c, r
}

var ctx = context.Background()

func TestAConnectedAppsRequestWaitsForTheUsersConsent(t *testing.T) {
	f, _, r := connectedFixture(t)
	if err := f.Complete(ctx, r.ID, "tok"); !errors.Is(err, app.ErrConsentNeeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestTheConsentPageNamesTheAppTheClientAndTheRolesItWouldActWith(t *testing.T) {
	f, _, r := connectedFixture(t)
	p, err := f.Prompt(ctx, r.ID, "tok")
	if err != nil || p.App != "Claude Code" || p.ClientLabel != "" || !slices.Equal(p.Roles, []string{"viewer", "editor"}) || p.RedirectURI != "http://localhost:1/cb" {
		t.Fatalf("Prompt = %+v, %v", p, err)
	}
}

func TestApprovingCompletesTheRequestAndAnotherRequestNeedsNoConsentAgain(t *testing.T) {
	f, _, r := connectedFixture(t)
	if err := f.Approve(ctx, r.ID, "tok"); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Request(ctx, r.ID); got.SignIn == nil || got.SignIn.UserID != f.accounts.ada {
		t.Fatalf("request = %+v", got)
	}
	again, _ := f.Start(ctx, domain.AuthRequest{ClientID: "mcp-1", Resource: "spa"})
	if err := f.Complete(ctx, again.ID, "tok"); err != nil {
		t.Fatalf("Complete = %v", err)
	}
}

func TestAConnectedAppsSignInIsNoSignInToTheClient(t *testing.T) {
	f, c, r := connectedFixture(t)
	c.given[key(f.accounts.ada, "mcp-1", "spa")] = true
	if err := f.Complete(ctx, r.ID, "tok"); err != nil {
		t.Fatal(err)
	}
	if len(f.requests.signed) != 0 || len(*f.events) != 0 {
		t.Fatalf("signed %v, events %+v", f.requests.signed, *f.events)
	}
}

func TestDenyingForgetsTheRequestAndAnswersWhereToTellTheTool(t *testing.T) {
	f, _, r := connectedFixture(t)
	got, err := f.Deny(ctx, r.ID, "tok")
	if err != nil || got.RedirectURI != "http://localhost:1/cb" || got.State != "st" {
		t.Fatalf("Deny = %+v, %v", got, err)
	}
	if _, err := f.Request(ctx, r.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("the request is kept: %v", err)
	}
}

func TestAUserWithNoAppRoleOnTheClientCannotConsent(t *testing.T) {
	f, c, r := connectedFixture(t)
	f.access.roles = map[string][]string{}
	if err := f.Approve(ctx, r.ID, "tok"); !errors.Is(err, app.ErrNoAccess) || len(c.given) != 0 {
		t.Fatalf("err = %v", err)
	}
	want := audit.Event{Action: audit.SignInFailed, User: f.accounts.ada, Client: "spa", Reason: audit.ReasonNoAccess}
	if (*f.events)[0] != want {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestAConsentForAClientThatTakesNoConnectedAppsIsRefused(t *testing.T) {
	f, c, r := connectedFixture(t)
	c.refuse = app.ErrNotConnectable
	if err := f.Approve(ctx, r.ID, "tok"); !errors.Is(err, app.ErrNotConnectable) {
		t.Fatalf("Approve = %v", err)
	}
	if err := f.Complete(ctx, r.ID, "tok"); !errors.Is(err, app.ErrNotConnectable) {
		t.Fatalf("Complete = %v", err)
	}
}

func TestOnlyAConnectedAppsLiveRequestIsAskedAbout(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   func(f *fixture, r domain.AuthRequest) uuid.UUID
	}{
		{"not a Connected App's", func(f *fixture, _ domain.AuthRequest) uuid.UUID { return f.started(t).ID }},
		{"unknown", func(*fixture, domain.AuthRequest) uuid.UUID { return uuid.New() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _, r := connectedFixture(t)
			if _, err := f.Prompt(ctx, tc.id(f, r), "tok"); !errors.Is(err, app.ErrNotFound) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestOnlyASignedInUserIsAskedAbout(t *testing.T) {
	f, _, r := connectedFixture(t)
	if _, err := f.Deny(ctx, r.ID, "not-a-session"); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("err = %v", err)
	}
}

func TestWithoutConsentsNoConnectedAppsRequestIsAnswered(t *testing.T) {
	f := newFixture()
	r, _ := f.Start(ctx, domain.AuthRequest{ClientID: "mcp-1", Resource: "spa"})
	if err := f.Complete(ctx, r.ID, "tok"); !errors.Is(err, app.ErrNotConnectable) {
		t.Fatalf("Complete = %v", err)
	}
	if err := f.Approve(ctx, r.ID, "tok"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("Approve = %v", err)
	}
}

func TestAConnectedAppsRequestFails(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spoil func(f *fixture, c *consents)
		ask   func(f *fixture, id uuid.UUID) error
	}{
		{"for a prompt when the name cannot be read", func(_ *fixture, c *consents) { c.fail["Name"] = errBoom }, prompt},
		{"for an approval when the consent cannot be kept", func(_ *fixture, c *consents) { c.fail["Consent"] = errBoom }, approve},
		{"for an approval when the request cannot be completed", func(f *fixture, _ *consents) { f.requests.fail["CompleteRequest"] = errBoom }, approve},
		{"for a request that cannot be read", func(f *fixture, _ *consents) { f.requests.fail["RequestByID"] = errBoom }, approve},
		{"for a denial of a request that cannot be read", func(f *fixture, _ *consents) { f.requests.fail["RequestByID"] = errBoom }, deny},
		{"for a completion whose roles cannot be read", func(f *fixture, _ *consents) { f.access.fail = errBoom }, complete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, c, r := connectedFixture(t)
			tc.spoil(f, c)
			if err := tc.ask(f, r.ID); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func prompt(f *fixture, id uuid.UUID) error {
	_, err := f.Prompt(ctx, id, "tok")
	return err
}

func approve(f *fixture, id uuid.UUID) error { return f.Approve(ctx, id, "tok") }

func deny(f *fixture, id uuid.UUID) error {
	_, err := f.Deny(ctx, id, "tok")
	return err
}

func complete(f *fixture, id uuid.UUID) error { return f.Complete(ctx, id, "tok") }

func TestAConsentPromptFailsForAClientThatLeft(t *testing.T) {
	f, _, _ := connectedFixture(t)
	r, _ := f.Start(ctx, domain.AuthRequest{ClientID: "mcp-1", Resource: "broken"})
	if _, err := f.Prompt(ctx, r.ID, "tok"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAUserWithNoAppRoleOnTheClientIsNotAskedToConsent(t *testing.T) {
	f, _, r := connectedFixture(t)
	f.access.roles = map[string][]string{}
	if _, err := f.Prompt(ctx, r.ID, "tok"); !errors.Is(err, app.ErrNoAccess) {
		t.Fatalf("err = %v", err)
	}
}

func TestASignInThatIsNoConnectedAppsCannotBeDenied(t *testing.T) {
	f, _, _ := connectedFixture(t)
	r := f.started(t)
	if _, err := f.Deny(ctx, r.ID, "tok"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if _, err := f.Request(ctx, r.ID); err != nil {
		t.Fatalf("the sign-in is gone: %v", err)
	}
}

func TestAConnectedAppsRequestAsksNoFreshTwoFactor(t *testing.T) {
	f, c, r := connectedFixture(t)
	f.SetStepUps(&settings{minutes: map[string]int{"spa": 15}})
	c.given[key(f.accounts.ada, "mcp-1", "spa")] = true
	if err := f.Complete(ctx, r.ID, "tok"); err != nil {
		t.Fatalf("err = %v", err)
	}
}
