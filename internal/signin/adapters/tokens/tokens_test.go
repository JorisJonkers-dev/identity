package tokens_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/tokens"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	tokensapp "github.com/JorisJonkers-dev/identity/internal/tokens/app"
	tokensdomain "github.com/JorisJonkers-dev/identity/internal/tokens/domain"
)

var (
	errBoom = errors.New("boom")
	t0      = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	token   = tokensdomain.Token{ID: uuid.New(), UserID: uuid.New(), ClientID: "grafana", CreatedAt: t0, ExpiresAt: t0.Add(time.Hour)}
)

type fake struct{ err error }

func (f fake) Resolve(context.Context, string) (tokensdomain.Token, error) { return token, f.err }

func (f fake) Check(context.Context, uuid.UUID, string) (tokensapp.Active, error) {
	return tokensapp.Active{Token: token, Roles: []string{"viewer"}}, f.err
}

func TestASecretResolvesToItsTokenAndUser(t *testing.T) {
	t.Parallel()
	id, user, err := tokens.New(fake{}).Resolve(context.Background(), "idpat_x")
	if err != nil || id != token.ID || user != token.UserID {
		t.Fatalf("Resolve = %v, %v, %v", id, user, err)
	}
}

func TestAnUnknownSecretIsNotFoundToSignIn(t *testing.T) {
	t.Parallel()
	if _, _, err := tokens.New(fake{err: tokensapp.ErrNotFound}).Resolve(context.Background(), "x"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestALiveTokenChecksWithItsRoles(t *testing.T) {
	t.Parallel()
	got, err := tokens.New(fake{}).Check(context.Background(), token.ID, "grafana")
	want := app.PersonalToken{ID: token.ID, UserID: token.UserID, ClientID: "grafana", CreatedAt: t0, ExpiresAt: t0.Add(time.Hour)}
	if err != nil || got.ID != want.ID || got.UserID != want.UserID || got.ClientID != want.ClientID || !got.ExpiresAt.Equal(want.ExpiresAt) || len(got.Roles) != 1 {
		t.Fatalf("Check = %+v, %v", got, err)
	}
}

func TestAnInactiveTokenIsInactiveToSignIn(t *testing.T) {
	t.Parallel()
	if _, err := tokens.New(fake{err: tokensapp.ErrInactive}).Check(context.Background(), token.ID, "grafana"); !errors.Is(err, app.ErrInactive) {
		t.Fatalf("err = %v", err)
	}
}

func TestAFailedCheckIsPassedOn(t *testing.T) {
	t.Parallel()
	if _, err := tokens.New(fake{err: errBoom}).Check(context.Background(), token.ID, "grafana"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}
