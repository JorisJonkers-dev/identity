package connected_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	connectedapp "github.com/JorisJonkers-dev/identity/internal/connected/app"
	connecteddomain "github.com/JorisJonkers-dev/identity/internal/connected/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/connected"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
)

var errBoom = errors.New("boom")

// apps answers every call with err, and Claude Code for the registration mcp_1.
type apps struct{ err error }

func (a apps) Consented(context.Context, uuid.UUID, string, string) (uuid.UUID, error) {
	return uuid.Nil, a.err
}

func (a apps) Consent(context.Context, uuid.UUID, string, string) (uuid.UUID, error) {
	return uuid.Nil, a.err
}

func (a apps) Registration(context.Context, string) (connecteddomain.Registration, error) {
	return connecteddomain.Registration{Name: "Claude Code"}, a.err
}

func TestAConsentGivenBeforeLetsTheRequestThrough(t *testing.T) {
	if err := connected.New(apps{}).Consented(context.Background(), uuid.New(), "mcp_1", "kb"); err != nil {
		t.Fatal(err)
	}
}

func TestTheConnectedContextsRefusalsReachSignInByItsNames(t *testing.T) {
	for _, c := range []struct {
		name      string
		from      error
		consented error
		consent   error
	}{
		{"no consent yet", connectedapp.ErrNotFound, app.ErrConsentNeeded, app.ErrNotFound},
		{"a Client taking no Connected Apps", connectedapp.ErrNotMCPResource, app.ErrNotConnectable, app.ErrNotConnectable},
		{"a Step-up Client", connectedapp.ErrStepUpClient, app.ErrNotConnectable, app.ErrNotConnectable},
		{"no App Role", connectedapp.ErrNoAccess, app.ErrNoAccess, app.ErrNoAccess},
		{"a failure", errBoom, errBoom, errBoom},
	} {
		t.Run(c.name, func(t *testing.T) {
			consents := connected.New(apps{err: c.from})
			if err := consents.Consented(context.Background(), uuid.New(), "mcp_1", "kb"); !errors.Is(err, c.consented) {
				t.Fatalf("Consented = %v", err)
			}
			if err := consents.Consent(context.Background(), uuid.New(), "mcp_1", "kb"); !errors.Is(err, c.consent) {
				t.Fatalf("Consent = %v", err)
			}
		})
	}
}

func TestARegistrationIsNamedAsItNamedItself(t *testing.T) {
	if name, err := connected.New(apps{}).Name(context.Background(), "mcp_1"); err != nil || name != "Claude Code" {
		t.Fatalf("Name = %q, %v", name, err)
	}
}

func TestAMissingRegistrationIsNotFound(t *testing.T) {
	if _, err := connected.New(apps{err: connectedapp.ErrNotFound}).Name(context.Background(), "mcp_1"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}
