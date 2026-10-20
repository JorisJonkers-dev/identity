package contexts_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/connected/adapters/contexts"
	"github.com/JorisJonkers-dev/identity/internal/connected/app"
	signinapp "github.com/JorisJonkers-dev/identity/internal/signin/app"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var errBoom = errors.New("boom")

// index declares kb at kb.test; "broken" fails.
type index struct{}

var kb = signindomain.Client{ID: "kb", Host: "kb.test", Label: "Knowledge", Roles: []string{"user", "editor"}}

func (index) Client(_ context.Context, id string) (signindomain.Client, error) {
	return lookup(id == "kb", id == "broken")
}

func (index) ClientForHost(_ context.Context, host string) (signindomain.Client, error) {
	return lookup(host == "kb.test", host == "broken.test")
}

func lookup(found, broken bool) (signindomain.Client, error) {
	switch {
	case broken:
		return signindomain.Client{}, errBoom
	case found:
		return kb, nil
	}
	return signindomain.Client{}, signinapp.ErrNotFound
}

type stepUps struct {
	minutes int
	err     error
}

func (s stepUps) StepUp(context.Context, string) (int, error) { return s.minutes, s.err }

type access struct{}

func (access) Roles(context.Context, uuid.UUID, string) ([]string, error) {
	return []string{"editor"}, nil
}

func TestAClientIsFoundByIdOrHostWithItsLabelRolesAndStepUp(t *testing.T) {
	c := contexts.New(index{}, stepUps{minutes: 15}, access{})
	byID, err := c.Client(context.Background(), "kb")
	if err != nil || byID.ID != "kb" || byID.Label != "Knowledge" || !slices.Equal(byID.Roles, kb.Roles) || !byID.StepUp {
		t.Fatalf("Client = %+v, %v", byID, err)
	}
	if byHost, err := c.ClientForHost(context.Background(), "kb.test"); err != nil || byHost.ID != "kb" {
		t.Fatalf("ClientForHost = %+v, %v", byHost, err)
	}
}

func TestAClientWithStepUpOffAsksForNone(t *testing.T) {
	if c, _ := contexts.New(index{}, stepUps{}, access{}).Client(context.Background(), "kb"); c.StepUp {
		t.Fatal("step-up is on")
	}
}

func TestAClientNotInTheClusterIsNotFound(t *testing.T) {
	if _, err := contexts.New(index{}, stepUps{}, access{}).ClientForHost(context.Background(), "nowhere.test"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestFindingAClientFails(t *testing.T) {
	for _, c := range []struct {
		name    string
		id      string
		stepUps stepUps
	}{
		{"when the index does", "broken", stepUps{}},
		{"when its step-up cannot be read", "kb", stepUps{err: errBoom}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := contexts.New(index{}, c.stepUps, access{}).Client(context.Background(), c.id); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAUsersRolesComeFromAccess(t *testing.T) {
	roles, err := contexts.New(index{}, stepUps{}, access{}).Roles(context.Background(), uuid.New(), "kb")
	if err != nil || !slices.Equal(roles, []string{"editor"}) {
		t.Fatalf("Roles = %v, %v", roles, err)
	}
}

func TestAClientsLabelIsReadWithoutItsStepUp(t *testing.T) {
	label, err := contexts.New(index{}, stepUps{err: errBoom}, access{}).Label(context.Background(), "kb")
	if err != nil || label != "Knowledge" {
		t.Fatalf("Label = %q, %v", label, err)
	}
}

func TestTheLabelOfAClientNotInTheClusterIsNotFound(t *testing.T) {
	if _, err := contexts.New(index{}, stepUps{}, access{}).Label(context.Background(), "gone"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}
