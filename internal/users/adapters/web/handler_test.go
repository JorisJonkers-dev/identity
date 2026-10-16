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
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
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
	methods    []string
	renamed    string
	changed    [4]string
	signedOut  string
	authorised string
	asked      []string
	factors    []app.TOTPFactor
}

func (u *useCases) SignUp(context.Context, string, string, string) error { return u.err }
func (u *useCases) Confirm(context.Context, string, string) error        { return u.err }
func (u *useCases) SignIn(context.Context, string, string) (string, error) {
	return u.token, u.err
}

func (u *useCases) SignOut(_ context.Context, token string) error {
	u.signedOut = token
	return u.err
}

func (u *useCases) SignedIn(_ context.Context, token string) (domain.User, app.Session, error) {
	u.authorised = token
	return u.user, app.Session{Methods: u.methods}, u.err
}

func (u *useCases) CompleteSignIn(_ context.Context, challenge, code string) (string, error) {
	u.asked = append(u.asked, challenge, code)
	return u.token, u.err
}

func (u *useCases) SecondFactors(context.Context, uuid.UUID) ([]app.TOTPFactor, error) {
	return u.factors, u.err
}

func (u *useCases) StartTOTP(context.Context, uuid.UUID) (app.TOTPSetup, error) {
	return app.TOTPSetup{Secret: "SECRET", URI: "otpauth://totp/x"}, u.err
}

func (u *useCases) ConfirmTOTP(_ context.Context, _ uuid.UUID, token, label, code, password string) error {
	u.asked = append(u.asked, token, label, code, password)
	return u.err
}

func (u *useCases) RemoveTOTP(_ context.Context, _, factor uuid.UUID, token, password string) error {
	u.asked = append(u.asked, factor.String(), token, password)
	return u.err
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

// admins names who is an Admin; err makes every check fail.
type admins struct {
	ids map[uuid.UUID]bool
	err error
}

func (a admins) IsAdmin(_ context.Context, id uuid.UUID) (bool, error) { return a.ids[id], a.err }

func ada(t *testing.T) domain.User {
	t.Helper()
	e, _ := domain.NewEmail("ada@example.org")
	n, _ := domain.NewName("Ada")
	return domain.User{ID: uuid.New(), Email: e, Name: n, CreatedAt: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
}

// signedIn returns ctx as HandleSession leaves it for a session token.
func signedIn(t *testing.T, h *web.Handler) context.Context {
	t.Helper()
	ctx, err := h.HandleSession(context.Background(), oas.GetProfileOperation, oas.Session{APIKey: "tok"})
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
	h := web.New(uc, admins{})
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
		domain.ErrInvalidName:     {http.StatusUnprocessableEntity, "Enter a name of 1 to 100 characters."},
		domain.ErrWeakPassword:    {http.StatusUnprocessableEntity, "Enter a password of 12 to 128 characters."},
		app.ErrInvalidToken:       {http.StatusUnprocessableEntity, "This link has expired or was already used. Sign up again for a new one."},
		app.ErrInvalidCredentials: {http.StatusUnauthorized, "The email or the password is wrong."},
	}
	for err, want := range cases {
		h := web.New(&useCases{err: err}, admins{})
		res, gotErr := h.SignUp(context.Background(), &oas.SignUp{})
		wantProblem(t, res, gotErr, want.status, want.detail)
	}
}

func TestAnUnexpectedFailurePassesOnForA500(t *testing.T) {
	boom := errors.New("boom")
	h := web.New(&useCases{err: boom}, admins{})
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
	h := web.New(&useCases{token: "tok"}, admins{})
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
	h := web.New(uc, admins{})
	res, err := h.SignOut(signedIn(t, h))
	if err != nil || uc.signedOut != "tok" {
		t.Fatalf("SignOut = %v, ended %q", err, uc.signedOut)
	}
	if got := res.(*oas.SignOutNoContent).SetCookie; !strings.Contains(got, "__Host-identity_session=;") || !strings.Contains(got, "Max-Age=0") {
		t.Fatalf("Set-Cookie %q does not clear the cookie", got)
	}
}

func TestHandleSessionRefusesWhatAuthenticateRefuses(t *testing.T) {
	uc := &useCases{err: app.ErrNoSession}
	if _, err := web.New(uc, admins{}).HandleSession(context.Background(), oas.GetProfileOperation, oas.Session{APIKey: "x"}); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("err = %v", err)
	}
	if uc.authorised != "x" {
		t.Fatalf("authenticated %q", uc.authorised)
	}
}

func TestProfile(t *testing.T) {
	u := ada(t)
	uc := &useCases{user: u}
	h := web.New(uc, admins{})
	ctx := signedIn(t, h)

	res, err := h.GetProfile(ctx)
	p, ok := res.(*oas.Profile)
	if err != nil || !ok || p.Email != "ada@example.org" || p.Name != "Ada" || !p.CreatedAt.Equal(u.CreatedAt) || p.Admin {
		t.Fatalf("GetProfile = %#v, %v", res, err)
	}
	if p := principal.From(ctx); p.UserID != u.ID || p.SecondFactor {
		t.Fatalf("the session handler left %+v as the principal, want %v without a Second Factor", p, u.ID)
	}
	admin := web.New(uc, admins{ids: map[uuid.UUID]bool{u.ID: true}})
	if res, err := admin.GetProfile(ctx); err != nil || !res.(*oas.Profile).Admin {
		t.Fatalf("an Admin's Profile = %#v, %v", res, err)
	}
	boom := errors.New("boom")
	if _, err := web.New(uc, admins{err: boom}).GetProfile(ctx); !errors.Is(err, boom) {
		t.Fatalf("a failing admin check: %v", err)
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
	wantProblem(t, res2, err, http.StatusUnprocessableEntity, "Enter a name of 1 to 100 characters.")
}

func TestConfirmWithTheWrongPasswordSaysSo(t *testing.T) {
	h := web.New(&useCases{err: app.ErrInvalidCredentials}, admins{})
	res, err := h.ConfirmEmail(context.Background(), &oas.Confirmation{Token: "t", Password: "wrong"})
	wantProblem(t, res, err, http.StatusUnprocessableEntity, "That is not the password you chose when you signed up. Try again.")
}

func TestChangePassword(t *testing.T) {
	u := ada(t)
	uc := &useCases{user: u}
	h := web.New(uc, admins{})
	ctx := signedIn(t, h)

	res, err := h.ChangePassword(ctx, &oas.PasswordChange{Current: "old", Next: "new"})
	if _, ok := res.(*oas.ChangePasswordNoContent); err != nil || !ok {
		t.Fatalf("ChangePassword = %#v, %v", res, err)
	}
	if uc.changed != [4]string{u.ID.String(), "tok", "old", "new"} {
		t.Fatalf("asked to change %v", uc.changed)
	}
	uc.err = app.ErrInvalidCredentials
	res, err = h.ChangePassword(ctx, &oas.PasswordChange{})
	wantProblem(t, res, err, http.StatusUnprocessableEntity, "Your current password is wrong. Enter it again.")
	uc.err = domain.ErrWeakPassword
	res, err = h.ChangePassword(ctx, &oas.PasswordChange{})
	wantProblem(t, res, err, http.StatusUnprocessableEntity, "Enter a password of 12 to 128 characters.")
}

func TestABusyServerSaysToTryAgain(t *testing.T) {
	h := web.New(&useCases{err: app.ErrBusy}, admins{})
	res, err := h.SignIn(context.Background(), &oas.SignIn{})
	wantProblem(t, res, err, http.StatusServiceUnavailable, "Too many people are signing in right now. Try again in a moment.")
}

func TestSignInWithASecondFactorSetsAChallengeCookie(t *testing.T) {
	h := web.New(&useCases{token: "chal", err: app.ErrSecondFactorNeeded}, admins{})
	res, err := h.SignIn(context.Background(), &oas.SignIn{Email: "a@b.c", Password: "pw"})
	got, ok := res.(*oas.SecondFactorNeededHeaders)
	if err != nil || !ok || got.Response.SecondFactor != oas.SecondFactorNeededSecondFactorTotp {
		t.Fatalf("SignIn = %#v, %v", res, err)
	}
	for _, part := range []string{"__Host-identity_challenge=chal", "Max-Age=300", "HttpOnly", "Secure", "SameSite=Lax"} {
		if !strings.Contains(got.SetCookie, part) {
			t.Errorf("Set-Cookie %q lacks %q", got.SetCookie, part)
		}
	}
}

func TestCompleteSignInTakesTheChallengeFromItsCookie(t *testing.T) {
	uc := &useCases{token: "sess"}
	h := web.New(uc, admins{})
	ctx, _ := h.HandleChallenge(context.Background(), oas.CompleteSignInOperation, oas.Challenge{APIKey: "chal"})
	res, err := h.CompleteSignIn(ctx, &oas.SecondFactorCode{Code: "123456"})
	if err != nil || !strings.HasPrefix(res.(*oas.CompleteSignInNoContent).SetCookie, "__Host-identity_session=sess;") ||
		uc.asked[0] != "chal" || uc.asked[1] != "123456" {
		t.Fatalf("CompleteSignIn = %#v, %v, asked %v", res, err, uc.asked)
	}
	for _, tc := range []struct {
		err    error
		status int
		detail string
	}{
		{app.ErrInvalidToken, http.StatusUnauthorized, "This sign-in has expired, or too many wrong codes were entered. Sign in again, and after five wrong codes wait 15 minutes first."},
		{app.ErrInvalidCode, http.StatusUnprocessableEntity, "That code is wrong or was already used. Enter the code your authenticator app shows now."},
	} {
		res, err := web.New(&useCases{err: tc.err}, admins{}).CompleteSignIn(ctx, &oas.SecondFactorCode{})
		wantProblem(t, res, err, tc.status, tc.detail)
	}
}

func TestSecondFactorsInTheProfile(t *testing.T) {
	used := time.Date(2026, 10, 16, 18, 0, 0, 0, time.UTC)
	label, _ := domain.NewFactorLabel("Phone")
	id := uuid.New()
	uc := &useCases{user: ada(t), methods: []string{"pwd", "otp"}, factors: []app.TOTPFactor{
		{ID: id, Label: label, CreatedAt: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC), LastUsedAt: &used},
		{ID: uuid.New(), Label: label},
	}}
	h := web.New(uc, admins{})
	ctx := signedIn(t, h)
	if p := principal.From(ctx); !p.SecondFactor {
		t.Fatal("a session proved by otp is not marked so")
	}
	if res, _ := h.GetProfile(ctx); !res.(*oas.Profile).SecondFactor {
		t.Fatal("the Profile does not say the session proved a Second Factor")
	}

	res, err := h.ListSecondFactors(ctx)
	list, ok := res.(*oas.SecondFactors)
	if err != nil || !ok || len(list.Factors) != 2 || uuid.UUID(list.Factors[0].ID) != id || list.Factors[0].Label != "Phone" ||
		list.Factors[0].Kind != oas.SecondFactorKindTotp || !list.Factors[0].LastUsedAt.Value.Equal(used) || list.Factors[1].LastUsedAt.Set {
		t.Fatalf("ListSecondFactors = %#v, %v", res, err)
	}
	if res, err := h.StartTOTP(ctx); err != nil || res.(*oas.TOTPSetup).Secret != "SECRET" {
		t.Fatalf("StartTOTP = %#v, %v", res, err)
	}
	if res, err := h.ConfirmTOTP(ctx, &oas.TOTPConfirmation{Label: "Phone", Code: "123456", Password: "pw"}); err != nil {
		t.Fatal(err)
	} else if _, ok := res.(*oas.ConfirmTOTPNoContent); !ok || uc.asked[0] != "tok" || uc.asked[1] != "Phone" || uc.asked[3] != "pw" {
		t.Fatalf("ConfirmTOTP = %#v, asked %v", res, uc.asked)
	}
	if res, err := h.RemoveSecondFactor(ctx, &oas.PasswordCheck{Password: "pw"}, oas.RemoveSecondFactorParams{FactorId: oas.ID(id)}); err != nil {
		t.Fatal(err)
	} else if _, ok := res.(*oas.RemoveSecondFactorNoContent); !ok || uc.asked[4] != id.String() || uc.asked[5] != "tok" || uc.asked[6] != "pw" {
		t.Fatalf("RemoveSecondFactor = %#v, asked %v", res, uc.asked)
	}
}

func TestSecondFactorRefusals(t *testing.T) {
	boom := errors.New("boom")
	ctx := func(h *web.Handler) context.Context { return signedIn(t, h) }
	for _, tc := range []struct {
		err    error
		status int
		detail string
	}{
		{app.ErrInvalidToken, http.StatusUnprocessableEntity, "This setup has expired. Choose Add an authenticator app to get a new QR code."},
		{app.ErrInvalidCredentials, http.StatusUnprocessableEntity, "Your password is wrong. Enter it again."},
		{app.ErrSecondFactorNeeded, http.StatusForbidden, "Sign out, then sign in again with an authenticator app to add another."},
		{domain.ErrInvalidLabel, http.StatusUnprocessableEntity, "Enter a name of 1 to 64 characters for this authenticator app."},
		{app.ErrInvalidCode, http.StatusUnprocessableEntity, "That code is wrong or was already used. Enter the code your authenticator app shows now."},
	} {
		h := web.New(&useCases{user: ada(t)}, admins{})
		c := ctx(h)
		h = web.New(&useCases{user: ada(t), err: tc.err}, admins{})
		res, err := h.ConfirmTOTP(c, &oas.TOTPConfirmation{})
		wantProblem(t, res, err, tc.status, tc.detail)
	}
	for _, tc := range []struct {
		err    error
		status int
		detail string
	}{
		{app.ErrInvalidCredentials, http.StatusUnprocessableEntity, "Your password is wrong. Enter it again."},
		{app.ErrNotFound, http.StatusNotFound, "That authenticator app is not on your account any more. Reload the page."},
		{app.ErrSecondFactorNeeded, http.StatusForbidden, "Sign out, then sign in again with an authenticator app to remove one."},
	} {
		h := web.New(&useCases{user: ada(t)}, admins{})
		c := ctx(h)
		res, err := web.New(&useCases{err: tc.err}, admins{}).RemoveSecondFactor(c, &oas.PasswordCheck{}, oas.RemoveSecondFactorParams{})
		wantProblem(t, res, err, tc.status, tc.detail)
	}
	h := web.New(&useCases{user: ada(t)}, admins{})
	c := ctx(h)
	failing := web.New(&useCases{err: boom}, admins{})
	if _, err := failing.ListSecondFactors(c); !errors.Is(err, boom) {
		t.Errorf("ListSecondFactors = %v", err)
	}
	if _, err := failing.StartTOTP(c); !errors.Is(err, boom) {
		t.Errorf("StartTOTP = %v", err)
	}
	res, err := web.New(&useCases{err: app.ErrNoSealer}, admins{}).StartTOTP(c)
	wantProblem(t, res, err, http.StatusServiceUnavailable, "Authenticator apps can't be used right now. Try again later.")
	if _, err := failing.ConfirmTOTP(c, &oas.TOTPConfirmation{}); !errors.Is(err, boom) {
		t.Errorf("ConfirmTOTP = %v", err)
	}
	if _, err := failing.RemoveSecondFactor(c, &oas.PasswordCheck{}, oas.RemoveSecondFactorParams{}); !errors.Is(err, boom) {
		t.Errorf("RemoveSecondFactor = %v", err)
	}
	if _, err := failing.CompleteSignIn(c, &oas.SecondFactorCode{}); !errors.Is(err, boom) {
		t.Errorf("CompleteSignIn = %v", err)
	}
}
