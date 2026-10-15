package web_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/users/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// useCases answers with what a test sets and records what it was asked.
type useCases struct {
	err        error
	userErr    error
	token      string
	user       domain.User
	renamed    string
	changed    [4]string
	signedOut  string
	authorised string
}

func (u *useCases) SignUp(context.Context, string, string, string) error { return u.err }
func (u *useCases) Confirm(context.Context, string) error                 { return u.err }
func (u *useCases) SignIn(context.Context, string, string) (string, error) {
	return u.token, u.err
}

func (u *useCases) SignOut(_ context.Context, token string) error {
	u.signedOut = token
	return u.err
}

func (u *useCases) Authenticate(_ context.Context, token string) (domain.User, error) {
	u.authorised = token
	return u.user, u.err
}

func (u *useCases) Rename(_ context.Context, _ uuid.UUID, name string) error {
	u.renamed = name
	return u.err
}

func (u *useCases) ChangePassword(_ context.Context, id uuid.UUID, token, current, next string) error {
	u.changed = [4]string{id.String(), token, current, next}
	return u.err
}

func (u *useCases) User(context.Context, uuid.UUID) (domain.User, error) { return u.user, u.userErr }

func ada(t *testing.T) domain.User {
	t.Helper()
	e, _ := domain.NewEmail("ada@example.org")
	n, _ := domain.NewName("Ada")
	return domain.User{ID: uuid.New(), Email: e, Name: n, CreatedAt: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
}

// signedIn returns ctx as HandleSession leaves it for a session token.
func signedIn(t *testing.T, h *web.Handler, token string) context.Context {
	t.Helper()
	ctx, err := h.HandleSession(context.Background(), oas.GetProfileOperation, oas.Session{APIKey: token})
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func wantProblem(t *testing.T, res any, err error, status int, detail string) {
	t.Helper()
	if err != nil {
		t.Fatalf("err = %v, want a problem", err)
	}
	p, ok := res.(*oas.ProblemStatusCode)
	if !ok || p.StatusCode != status || p.Response.Detail.Value != detail {
		t.Fatalf("res = %#v, want a %d problem saying %q", res, status, detail)
	}
}

func TestSignUpAndConfirm(t *testing.T) {
	uc := &useCases{}
	h := web.New(uc)
	ctx := context.Background()
	if res, err := h.SignUp(ctx, &oas.SignUp{Email: "a@b.c", Name: "Ada", Password: "pw"}); err != nil {
		t.Fatal(err)
	} else if _, ok := res.(*oas.SignUpAccepted); !ok {
		t.Fatalf("SignUp = %#v", res)
	}
	if res, err := h.ConfirmEmail(ctx, &oas.Confirmation{Token: "t"}); err != nil {
		t.Fatal(err)
	} else if _, ok := res.(*oas.ConfirmEmailNoContent); !ok {
		t.Fatalf("ConfirmEmail = %#v", res)
	}
}

func TestEveryRefusalIsAProblemSayingWhatToFix(t *testing.T) {
	cases := map[error]struct {
		status int
		detail string
	}{
		domain.ErrInvalidEmail:    {http.StatusUnprocessableEntity, "Enter a valid email address."},
		domain.ErrInvalidName:     {http.StatusUnprocessableEntity, "A name is 1 to 100 characters."},
		domain.ErrWeakPassword:    {http.StatusUnprocessableEntity, "A password is 12 to 128 characters."},
		app.ErrInvalidToken:       {http.StatusUnprocessableEntity, "This link has expired or was already used. Sign up again for a new one."},
		app.ErrInvalidCredentials: {http.StatusUnauthorized, "The email or the password is wrong."},
	}
	for err, want := range cases {
		h := web.New(&useCases{err: err})
		res, gotErr := h.SignUp(context.Background(), &oas.SignUp{})
		wantProblem(t, res, gotErr, want.status, want.detail)
	}
}

func TestAnUnexpectedFailurePassesOnForA500(t *testing.T) {
	boom := errors.New("boom")
	h := web.New(&useCases{err: boom})
	ctx := context.Background()
	if _, err := h.SignUp(ctx, &oas.SignUp{}); !errors.Is(err, boom) {
		t.Errorf("SignUp err = %v", err)
	}
	if _, err := h.ConfirmEmail(ctx, &oas.Confirmation{}); !errors.Is(err, boom) {
		t.Errorf("ConfirmEmail err = %v", err)
	}
	if _, err := h.SignIn(ctx, &oas.SignIn{}); !errors.Is(err, boom) {
		t.Errorf("SignIn err = %v", err)
	}
	if _, err := h.SignOut(ctx); !errors.Is(err, boom) {
		t.Errorf("SignOut err = %v", err)
	}
	if _, err := h.UpdateProfile(ctx, &oas.ProfileUpdate{}); !errors.Is(err, boom) {
		t.Errorf("UpdateProfile err = %v", err)
	}
	if _, err := h.ChangePassword(ctx, &oas.PasswordChange{}); !errors.Is(err, boom) {
		t.Errorf("ChangePassword err = %v", err)
	}
}

func TestSignInSetsAHostOnlySessionCookie(t *testing.T) {
	h := web.New(&useCases{token: "tok"})
	res, err := h.SignIn(context.Background(), &oas.SignIn{Email: "a@b.c", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	got := res.(*oas.SignInNoContent).SetCookie
	for _, part := range []string{"__Host-identity_session=tok", "Path=/", "Max-Age=2592000", "HttpOnly", "Secure", "SameSite=Lax"} {
		if !strings.Contains(got, part) {
			t.Errorf("Set-Cookie %q lacks %q", got, part)
		}
	}
	if strings.Contains(got, "Domain=") {
		t.Errorf("Set-Cookie %q names a Domain, so it would reach other hosts", got)
	}
}

func TestSignOutEndsThisSessionAndClearsTheCookie(t *testing.T) {
	uc := &useCases{user: ada(t)}
	h := web.New(uc)
	res, err := h.SignOut(signedIn(t, h, "tok"))
	if err != nil || uc.signedOut != "tok" {
		t.Fatalf("SignOut = %v, ended %q", err, uc.signedOut)
	}
	if got := res.(*oas.SignOutNoContent).SetCookie; !strings.Contains(got, "__Host-identity_session=;") || !strings.Contains(got, "Max-Age=0") {
		t.Fatalf("Set-Cookie %q does not clear the cookie", got)
	}
}

func TestHandleSessionRefusesWhatAuthenticateRefuses(t *testing.T) {
	uc := &useCases{err: app.ErrNoSession}
	if _, err := web.New(uc).HandleSession(context.Background(), oas.GetProfileOperation, oas.Session{APIKey: "x"}); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("err = %v", err)
	}
	if uc.authorised != "x" {
		t.Fatalf("authenticated %q", uc.authorised)
	}
}

func TestProfile(t *testing.T) {
	u := ada(t)
	uc := &useCases{user: u}
	h := web.New(uc)
	ctx := signedIn(t, h, "tok")

	res, err := h.GetProfile(ctx)
	p, ok := res.(*oas.Profile)
	if err != nil || !ok || p.Email != "ada@example.org" || p.Name != "Ada" || !p.CreatedAt.Equal(u.CreatedAt) {
		t.Fatalf("GetProfile = %#v, %v", res, err)
	}

	if res, err := h.UpdateProfile(ctx, &oas.ProfileUpdate{Name: "Ada King"}); err != nil || uc.renamed != "Ada King" {
		t.Fatalf("UpdateProfile = %#v, %v, renamed %q", res, err, uc.renamed)
	} else if _, ok := res.(*oas.Profile); !ok {
		t.Fatalf("UpdateProfile = %#v", res)
	}
	uc.userErr = errors.New("boom")
	if _, err := h.UpdateProfile(ctx, &oas.ProfileUpdate{Name: "Ada"}); !errors.Is(err, uc.userErr) {
		t.Fatalf("re-reading the User failing: %v", err)
	}
	uc.err = domain.ErrInvalidName
	res2, err := h.UpdateProfile(ctx, &oas.ProfileUpdate{})
	wantProblem(t, res2, err, http.StatusUnprocessableEntity, "A name is 1 to 100 characters.")
}

func TestChangePassword(t *testing.T) {
	u := ada(t)
	uc := &useCases{user: u}
	h := web.New(uc)
	ctx := signedIn(t, h, "tok")

	res, err := h.ChangePassword(ctx, &oas.PasswordChange{Current: "old", Next: "new"})
	if _, ok := res.(*oas.ChangePasswordNoContent); err != nil || !ok {
		t.Fatalf("ChangePassword = %#v, %v", res, err)
	}
	if uc.changed != [4]string{u.ID.String(), "tok", "old", "new"} {
		t.Fatalf("asked to change %v", uc.changed)
	}
	uc.err = app.ErrInvalidCredentials
	res, err = h.ChangePassword(ctx, &oas.PasswordChange{})
	wantProblem(t, res, err, http.StatusUnprocessableEntity, "The current password is wrong.")
	uc.err = domain.ErrWeakPassword
	res, err = h.ChangePassword(ctx, &oas.PasswordChange{})
	wantProblem(t, res, err, http.StatusUnprocessableEntity, "A password is 12 to 128 characters.")
}
