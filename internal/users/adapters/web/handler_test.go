package web_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
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
	factors    []app.Factor
	kinds      []string
	kindsErr   error
	devices    []app.Device
	pending    string
	pendingErr error
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

func (u *useCases) SecondFactors(context.Context, uuid.UUID) ([]app.Factor, error) {
	return u.factors, u.err
}

func (u *useCases) ChallengeKinds(context.Context, string) ([]string, error) {
	return u.kinds, u.kindsErr
}

func (u *useCases) PasskeyOptions(_ context.Context, challenge string) ([]byte, error) {
	u.asked = append(u.asked, challenge)
	return []byte(`{"publicKey":{}}`), u.err
}

func (u *useCases) CompleteSignInWithPasskey(_ context.Context, challenge string, response []byte) (string, error) {
	u.asked = append(u.asked, challenge, string(response))
	return u.token, u.err
}

func (u *useCases) StartStepUp(_ context.Context, token string) ([]string, error) {
	u.asked = append(u.asked, token)
	return u.kinds, u.err
}

func (u *useCases) StepUpWithCode(_ context.Context, token, code string) error {
	u.asked = append(u.asked, token, code)
	return u.err
}

func (u *useCases) StepUpPasskeyOptions(_ context.Context, token string) ([]byte, error) {
	u.asked = append(u.asked, token)
	return []byte(`{"publicKey":{}}`), u.err
}

func (u *useCases) StepUpWithPasskey(_ context.Context, token string, response []byte) error {
	u.asked = append(u.asked, token, string(response))
	return u.err
}

func (u *useCases) AskEmailChange(_ context.Context, _ uuid.UUID, email, password string) error {
	u.asked = append(u.asked, email, password)
	return u.err
}

func (u *useCases) PendingEmail(context.Context, uuid.UUID) (string, error) {
	return u.pending, u.pendingErr
}

func (u *useCases) CancelEmailChange(context.Context, uuid.UUID) error {
	u.asked = append(u.asked, "cancel")
	return u.err
}

func (u *useCases) ConfirmEmailChange(_ context.Context, token string) error {
	u.asked = append(u.asked, token)
	return u.err
}

func (u *useCases) AskPasswordReset(_ context.Context, email string) error {
	u.asked = append(u.asked, email)
	return u.err
}

func (u *useCases) CheckPasswordReset(_ context.Context, token string) ([]string, error) {
	u.asked = append(u.asked, token)
	return u.kinds, u.err
}

func (u *useCases) ResetWithCode(_ context.Context, token, code string) error {
	u.asked = append(u.asked, token, code)
	return u.err
}

func (u *useCases) ResetPasskeyOptions(_ context.Context, token string) ([]byte, error) {
	u.asked = append(u.asked, token)
	return []byte(`{"publicKey":{}}`), u.err
}

func (u *useCases) ResetWithPasskey(_ context.Context, token string, response []byte) error {
	u.asked = append(u.asked, token, string(response))
	return u.err
}

func (u *useCases) CompletePasswordReset(_ context.Context, token, password string) error {
	u.asked = append(u.asked, token, password)
	return u.err
}

func (u *useCases) RenameAs(_ context.Context, actor app.Actor, userID uuid.UUID, name string) error {
	u.asked = append(u.asked, actor.ID.String(), userID.String(), name)
	return u.err
}

func (u *useCases) AskEmailChangeAs(_ context.Context, actor app.Actor, userID uuid.UUID, email string) error {
	u.asked = append(u.asked, actor.ID.String(), userID.String(), email)
	return u.err
}

func (u *useCases) DeleteAs(_ context.Context, actor app.Actor, userID uuid.UUID) error {
	u.asked = append(u.asked, actor.ID.String(), userID.String())
	return u.err
}

func (u *useCases) DeleteAccount(_ context.Context, _ uuid.UUID, token, password string) error {
	u.asked = append(u.asked, token, password)
	return u.err
}

func (u *useCases) Devices(_ context.Context, _ uuid.UUID, token string) ([]app.Device, error) {
	u.asked = append(u.asked, token)
	return u.devices, u.err
}

func (u *useCases) SignOutDevice(_ context.Context, _, deviceID uuid.UUID) error {
	u.asked = append(u.asked, deviceID.String())
	return u.err
}

func (u *useCases) SignOutOtherDevices(_ context.Context, _ uuid.UUID, token string) error {
	u.asked = append(u.asked, token)
	return u.err
}

func (u *useCases) StartPasskey(context.Context, uuid.UUID) ([]byte, error) {
	return []byte(`{"publicKey":{"rp":{}}}`), u.err
}

func (u *useCases) ConfirmPasskey(_ context.Context, _ uuid.UUID, token, label string, response []byte, password string) error {
	u.asked = append(u.asked, token, label, string(response), password)
	return u.err
}

func (u *useCases) StartTOTP(context.Context, uuid.UUID) (app.TOTPSetup, error) {
	return app.TOTPSetup{Secret: "SECRET", URI: "otpauth://totp/x"}, u.err
}

func (u *useCases) ConfirmTOTP(_ context.Context, _ uuid.UUID, token, label, code, password string) error {
	u.asked = append(u.asked, token, label, code, password)
	return u.err
}

func (u *useCases) RemoveFactor(_ context.Context, _, factor uuid.UUID, token, password string) error {
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

func TestTheProfileShowsTheSignedInUser(t *testing.T) {
	u := ada(t)
	h := web.New(&useCases{user: u}, admins{})
	res, err := h.GetProfile(signedIn(t, h))
	p, ok := res.(*oas.Profile)
	if err != nil || !ok || p.Email != "ada@example.org" || p.Name != "Ada" || !p.CreatedAt.Equal(u.CreatedAt) || p.Admin {
		t.Fatalf("GetProfile = %#v, %v", res, err)
	}
}

func TestTheSessionCheckSetsThePrincipalWithoutASecondFactor(t *testing.T) {
	u := ada(t)
	h := web.New(&useCases{user: u}, admins{})
	if p := principal.From(signedIn(t, h)); p.UserID != u.ID || p.SecondFactor {
		t.Fatalf("principal = %+v, want %v without a Second Factor", p, u.ID)
	}
}

func TestAnAdminsProfileSaysSo(t *testing.T) {
	u := ada(t)
	uc := &useCases{user: u}
	ctx := signedIn(t, web.New(uc, admins{}))
	res, err := web.New(uc, admins{ids: map[uuid.UUID]bool{u.ID: true}}).GetProfile(ctx)
	if err != nil || !res.(*oas.Profile).Admin {
		t.Fatalf("GetProfile = %#v, %v", res, err)
	}
}

func TestTheProfileFailsWhenTheAdminCheckCannotBeMade(t *testing.T) {
	uc := &useCases{user: ada(t)}
	ctx := signedIn(t, web.New(uc, admins{}))
	boom := errors.New("boom")
	if _, err := web.New(uc, admins{err: boom}).GetProfile(ctx); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestRenamingAnswersTheChangedProfile(t *testing.T) {
	uc := &useCases{user: ada(t)}
	h := web.New(uc, admins{})
	res, err := h.UpdateProfile(signedIn(t, h), &oas.ProfileUpdate{Name: "Ada King"})
	if _, ok := res.(*oas.Profile); err != nil || !ok || uc.renamed != "Ada King" {
		t.Fatalf("UpdateProfile = %#v, %v, renamed %q", res, err, uc.renamed)
	}
}

func TestRenamingFailsWhenTheUserCannotBeReadBack(t *testing.T) {
	uc := &useCases{user: ada(t)}
	h := web.New(uc, admins{})
	ctx := signedIn(t, h)
	uc.userErr = errors.New("boom")
	if _, err := h.UpdateProfile(ctx, &oas.ProfileUpdate{Name: "Ada"}); !errors.Is(err, uc.userErr) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnEmptyNameIsAProblemSayingWhatToEnter(t *testing.T) {
	uc := &useCases{user: ada(t)}
	h := web.New(uc, admins{})
	ctx := signedIn(t, h)
	uc.err = domain.ErrInvalidName
	res, err := h.UpdateProfile(ctx, &oas.ProfileUpdate{})
	wantProblem(t, res, err, http.StatusUnprocessableEntity, "Enter a name of 1 to 100 characters.")
}

func TestConfirmWithTheWrongPasswordSaysSo(t *testing.T) {
	h := web.New(&useCases{err: app.ErrInvalidCredentials}, admins{})
	res, err := h.ConfirmEmail(context.Background(), &oas.Confirmation{Token: "t", Password: "wrong"})
	wantProblem(t, res, err, http.StatusUnprocessableEntity, "That is not the password you chose when you signed up. Try again.")
}

func TestChangingThePasswordSendsTheSessionAndBothPasswords(t *testing.T) {
	u := ada(t)
	uc := &useCases{user: u}
	h := web.New(uc, admins{})
	res, err := h.ChangePassword(signedIn(t, h), &oas.PasswordChange{Current: "old", Next: "new"})
	if _, ok := res.(*oas.ChangePasswordNoContent); err != nil || !ok || uc.changed != [4]string{u.ID.String(), "tok", "old", "new"} {
		t.Fatalf("ChangePassword = %#v, %v, asked %v", res, err, uc.changed)
	}
}

func TestAPasswordChangeIsRefusedWithWhatToDo(t *testing.T) {
	for _, tc := range []struct {
		err    error
		detail string
	}{
		{app.ErrInvalidCredentials, "Your current password is wrong. Enter it again."},
		{domain.ErrWeakPassword, "Enter a password of 12 to 128 characters."},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			uc := &useCases{user: ada(t)}
			h := web.New(uc, admins{})
			ctx := signedIn(t, h)
			uc.err = tc.err
			res, err := h.ChangePassword(ctx, &oas.PasswordChange{})
			wantProblem(t, res, err, http.StatusUnprocessableEntity, tc.detail)
		})
	}
}

func TestABusyServerSaysToTryAgain(t *testing.T) {
	h := web.New(&useCases{err: app.ErrBusy}, admins{})
	res, err := h.SignIn(context.Background(), &oas.SignIn{})
	wantProblem(t, res, err, http.StatusServiceUnavailable, "Too many people are signing in right now. Try again in a moment.")
}

func TestSignInWithASecondFactorSetsAChallengeCookie(t *testing.T) {
	h := web.New(&useCases{token: "chal", err: app.ErrSecondFactorNeeded, kinds: []string{"totp", "passkey"}}, admins{})
	res, err := h.SignIn(context.Background(), &oas.SignIn{Email: "a@b.c", Password: "pw"})
	got, ok := res.(*oas.SecondFactorNeededHeaders)
	if err != nil || !ok || len(got.Response.Methods) != 2 || got.Response.Methods[1] != oas.SecondFactorKindPasskey {
		t.Fatalf("SignIn = %#v, %v", res, err)
	}
	boom := errors.New("boom")
	if _, err := web.New(&useCases{token: "chal", err: app.ErrSecondFactorNeeded, kindsErr: boom}, admins{}).SignIn(context.Background(), &oas.SignIn{}); !errors.Is(err, boom) {
		t.Fatalf("SignIn when the kinds cannot be read = %v", err)
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
	uc := &useCases{user: ada(t), methods: []string{"pwd", "otp"}, factors: []app.Factor{
		{ID: id, Kind: app.KindTOTP, Label: label, CreatedAt: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC), LastUsedAt: &used},
		{ID: uuid.New(), Kind: app.KindPasskey, Label: label},
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
		list.Factors[0].Kind != oas.SecondFactorKindTotp || !list.Factors[0].LastUsedAt.Value.Equal(used) || list.Factors[1].LastUsedAt.Set ||
		list.Factors[1].Kind != oas.SecondFactorKindPasskey {
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
		{app.ErrInvalidToken, http.StatusUnprocessableEntity, "This setup has expired. Start adding it again."},
		{app.ErrInvalidCredentials, http.StatusUnprocessableEntity, "Your password is wrong. Enter it again."},
		{app.ErrSecondFactorNeeded, http.StatusForbidden, "Sign out, then sign in again with a passkey or authenticator app to add another."},
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
		{app.ErrNotFound, http.StatusNotFound, "That passkey or authenticator app is no longer on your account. Reload the page."},
		{app.ErrSecondFactorNeeded, http.StatusForbidden, "Sign out, then sign in again with a passkey or authenticator app to remove one."},
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
	res, err := web.New(&useCases{err: app.ErrUnavailable}, admins{}).StartTOTP(c)
	wantProblem(t, res, err, http.StatusServiceUnavailable, "Authenticator apps and passkeys can't be used right now. Try again later.")
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

func TestPasskeySignIn(t *testing.T) {
	uc := &useCases{token: "sess"}
	h := web.New(uc, admins{})
	ctx, _ := h.HandleChallenge(context.Background(), oas.PasskeyOptionsOperation, oas.Challenge{APIKey: "chal"})
	res, err := h.PasskeyOptions(ctx)
	if err != nil || res.(*oas.PasskeyOptions).Options != `{"publicKey":{}}` || uc.asked[0] != "chal" {
		t.Fatalf("PasskeyOptions = %#v, %v", res, err)
	}
	done, err := h.CompleteSignInWithPasskey(ctx, &oas.PasskeyAssertion{Response: `{"id":"x"}`})
	if err != nil || !strings.HasPrefix(done.(*oas.CompleteSignInWithPasskeyNoContent).SetCookie, "__Host-identity_session=sess;") || uc.asked[2] != `{"id":"x"}` {
		t.Fatalf("CompleteSignInWithPasskey = %#v, %v, asked %v", done, err, uc.asked)
	}
	expired := "This sign-in has expired, or too many wrong codes were entered. Sign in again, and after five wrong codes wait 15 minutes first."
	h = web.New(&useCases{err: app.ErrInvalidToken}, admins{})
	o, err := h.PasskeyOptions(ctx)
	wantProblem(t, o, err, http.StatusUnauthorized, expired)
	c, err := h.CompleteSignInWithPasskey(ctx, &oas.PasskeyAssertion{})
	wantProblem(t, c, err, http.StatusUnauthorized, expired)
	h = web.New(&useCases{err: app.ErrInvalidPasskey}, admins{})
	c, err = h.CompleteSignInWithPasskey(ctx, &oas.PasskeyAssertion{})
	wantProblem(t, c, err, http.StatusUnprocessableEntity, "Your passkey was not accepted. Try again, or sign in with another passkey or an authenticator app code.")
	h = web.New(&useCases{err: app.ErrNoPasskeys}, admins{})
	o, err = h.PasskeyOptions(ctx)
	wantProblem(t, o, err, http.StatusUnprocessableEntity, "You have no passkey. Enter the code from your authenticator app instead.")
}

func TestAddingAPasskeyInTheProfile(t *testing.T) {
	uc := &useCases{user: ada(t)}
	h := web.New(uc, admins{})
	ctx := signedIn(t, h)
	res, err := h.StartPasskey(ctx)
	if err != nil || res.(*oas.PasskeyOptions).Options != `{"publicKey":{"rp":{}}}` {
		t.Fatalf("StartPasskey = %#v, %v", res, err)
	}
	if res, err := h.ConfirmPasskey(ctx, &oas.PasskeyConfirmation{Label: "MacBook", Response: "{}", Password: "pw"}); err != nil {
		t.Fatal(err)
	} else if _, ok := res.(*oas.ConfirmPasskeyNoContent); !ok || uc.asked[1] != "MacBook" || uc.asked[2] != "{}" || uc.asked[3] != "pw" {
		t.Fatalf("ConfirmPasskey = %#v, asked %v", res, uc.asked)
	}
	for err, detail := range map[error]string{
		app.ErrInvalidCredentials: "Your password is wrong. Enter it again.",
		app.ErrInvalidPasskey:     "Your passkey was not accepted. Try again, or sign in with another passkey or an authenticator app code.",
	} {
		res, gotErr := web.New(&useCases{err: err}, admins{}).ConfirmPasskey(ctx, &oas.PasskeyConfirmation{})
		wantProblem(t, res, gotErr, http.StatusUnprocessableEntity, detail)
	}
	boom := errors.New("boom")
	if _, err := web.New(&useCases{err: boom}, admins{}).StartPasskey(ctx); !errors.Is(err, boom) {
		t.Errorf("StartPasskey = %v", err)
	}
	if _, err := web.New(&useCases{err: boom}, admins{}).ConfirmPasskey(ctx, &oas.PasskeyConfirmation{}); !errors.Is(err, boom) {
		t.Errorf("ConfirmPasskey = %v", err)
	}
	if _, err := web.New(&useCases{err: boom}, admins{}).PasskeyOptions(ctx); !errors.Is(err, boom) {
		t.Errorf("PasskeyOptions = %v", err)
	}
	if _, err := web.New(&useCases{err: boom}, admins{}).CompleteSignInWithPasskey(ctx, &oas.PasskeyAssertion{}); !errors.Is(err, boom) {
		t.Errorf("CompleteSignInWithPasskey = %v", err)
	}
}

func TestStartingStepUpSendsTheSessionAndAnswersTheKinds(t *testing.T) {
	uc := &useCases{kinds: []string{"totp", "passkey"}}
	h := web.New(uc, admins{})
	res, err := h.StartStepUp(signedIn(t, h))
	got, ok := res.(*oas.SecondFactorNeeded)
	if err != nil || !ok || len(got.Methods) != 2 || got.Methods[1] != "passkey" || !slices.Equal(uc.asked, []string{"tok"}) {
		t.Fatalf("StartStepUp = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestStepUpWithACodeSendsTheSessionAndTheCode(t *testing.T) {
	uc := &useCases{}
	h := web.New(uc, admins{})
	res, err := h.StepUpWithCode(signedIn(t, h), &oas.SecondFactorCode{Code: "123456"})
	if _, ok := res.(*oas.StepUpWithCodeNoContent); err != nil || !ok || !slices.Equal(uc.asked, []string{"tok", "123456"}) {
		t.Fatalf("StepUpWithCode = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestStepUpPasskeyOptionsSendTheSession(t *testing.T) {
	uc := &useCases{}
	h := web.New(uc, admins{})
	res, err := h.StepUpPasskeyOptions(signedIn(t, h))
	if got, ok := res.(*oas.PasskeyOptions); err != nil || !ok || got.Options != `{"publicKey":{}}` || !slices.Equal(uc.asked, []string{"tok"}) {
		t.Fatalf("StepUpPasskeyOptions = %#v, %v", res, err)
	}
}

func TestStepUpWithAPasskeySendsTheSessionAndTheAnswer(t *testing.T) {
	uc := &useCases{}
	h := web.New(uc, admins{})
	res, err := h.StepUpWithPasskey(signedIn(t, h), &oas.PasskeyAssertion{Response: "signed"})
	if _, ok := res.(*oas.StepUpWithPasskeyNoContent); err != nil || !ok || !slices.Equal(uc.asked, []string{"tok", "signed"}) {
		t.Fatalf("StepUpWithPasskey = %#v, %v", res, err)
	}
}

func TestEachStepUpRefusalSaysWhatToDo(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		detail string
	}{
		{app.ErrNoSecondFactor, http.StatusConflict, "Add a passkey or an authenticator app on the Security page first."},
		{app.ErrInvalidToken, http.StatusUnauthorized, "This check expired, or you entered too many wrong codes. Start again from the app. After five wrong codes, wait 15 minutes before you try again."},
		{app.ErrInvalidCode, http.StatusUnprocessableEntity, "That code is wrong or was already used. Enter the code your authenticator app shows now."},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			res, err := web.New(&useCases{err: tc.err}, admins{}).StepUpWithCode(context.Background(), &oas.SecondFactorCode{})
			p, ok := res.(*oas.ProblemStatusCode)
			if err != nil || !ok || p.StatusCode != tc.status || p.Response.Detail.Value != tc.detail {
				t.Fatalf("StepUpWithCode = %#v, %v", res, err)
			}
		})
	}
}

func TestEveryStepUpOperationAnswersARefusalAsAProblem(t *testing.T) {
	for _, op := range []struct {
		name string
		call func(h *web.Handler) (any, error)
	}{
		{"starting", func(h *web.Handler) (any, error) { return h.StartStepUp(context.Background()) }},
		{"a code", func(h *web.Handler) (any, error) {
			return h.StepUpWithCode(context.Background(), &oas.SecondFactorCode{})
		}},
		{"passkey options", func(h *web.Handler) (any, error) { return h.StepUpPasskeyOptions(context.Background()) }},
		{"a passkey", func(h *web.Handler) (any, error) {
			return h.StepUpWithPasskey(context.Background(), &oas.PasskeyAssertion{})
		}},
	} {
		t.Run(op.name, func(t *testing.T) {
			res, err := op.call(web.New(&useCases{err: app.ErrInvalidToken}, admins{}))
			if p, ok := res.(*oas.ProblemStatusCode); err != nil || !ok || p.StatusCode != http.StatusUnauthorized {
				t.Fatalf("got %#v, %v", res, err)
			}
		})
	}
}

func TestTheDevicesListCarriesEachSessionAndMarksThisOne(t *testing.T) {
	seen := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	id := uuid.New()
	uc := &useCases{devices: []app.Device{{ID: id, Label: "Firefox on macOS", LastSeenAt: seen, Current: true}}}
	h := web.New(uc, admins{})
	res, err := h.ListDevices(signedIn(t, h))
	got, ok := res.(*oas.Devices)
	if err != nil || !ok || len(got.Devices) != 1 || got.Devices[0] != (oas.Device{ID: oas.ID(id), Label: "Firefox on macOS", LastSeenAt: seen, Current: true}) || uc.asked[0] != "tok" {
		t.Fatalf("ListDevices = %#v, %v", res, err)
	}
}

func TestSigningOutADeviceSendsWhichOne(t *testing.T) {
	uc := &useCases{}
	h := web.New(uc, admins{})
	id := uuid.New()
	res, err := h.SignOutDevice(signedIn(t, h), oas.SignOutDeviceParams{DeviceId: oas.ID(id)})
	if _, ok := res.(*oas.SignOutDeviceNoContent); err != nil || !ok || uc.asked[0] != id.String() {
		t.Fatalf("SignOutDevice = %#v, %v", res, err)
	}
}

func TestADeviceAlreadySignedOutIsNotFound(t *testing.T) {
	h := web.New(&useCases{err: app.ErrNotFound}, admins{})
	res, err := h.SignOutDevice(context.Background(), oas.SignOutDeviceParams{})
	if p, ok := res.(*oas.ProblemStatusCode); err != nil || !ok || p.StatusCode != http.StatusNotFound || p.Response.Detail.Value != "That device is already signed out. Reload the page." {
		t.Fatalf("SignOutDevice = %#v, %v", res, err)
	}
}

func TestSigningOutOtherDevicesSendsThisSession(t *testing.T) {
	uc := &useCases{}
	h := web.New(uc, admins{})
	res, err := h.SignOutOtherDevices(signedIn(t, h))
	if _, ok := res.(*oas.SignOutOtherDevicesNoContent); err != nil || !ok || uc.asked[0] != "tok" {
		t.Fatalf("SignOutOtherDevices = %#v, %v", res, err)
	}
}

var errBoom = errors.New("boom")

func TestADevicesFailureIsPassedOn(t *testing.T) {
	for _, op := range []struct {
		name string
		call func(h *web.Handler) (any, error)
	}{
		{"listing", func(h *web.Handler) (any, error) { return h.ListDevices(context.Background()) }},
		{"signing one out", func(h *web.Handler) (any, error) {
			return h.SignOutDevice(context.Background(), oas.SignOutDeviceParams{})
		}},
		{"signing the others out", func(h *web.Handler) (any, error) { return h.SignOutOtherDevices(context.Background()) }},
	} {
		t.Run(op.name, func(t *testing.T) {
			if _, err := op.call(web.New(&useCases{err: errBoom}, admins{})); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAskingForANewEmailSendsTheAddressAndThePassword(t *testing.T) {
	uc := &useCases{}
	h := web.New(uc, admins{})
	res, err := h.AskEmailChange(signedIn(t, h), &oas.EmailChange{Email: "new@example.org", Password: "pw"})
	if _, ok := res.(*oas.AskEmailChangeAccepted); err != nil || !ok || !slices.Equal(uc.asked, []string{"new@example.org", "pw"}) {
		t.Fatalf("AskEmailChange = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestAWrongPasswordForANewEmailSaysSo(t *testing.T) {
	h := web.New(&useCases{err: app.ErrInvalidCredentials}, admins{})
	res, err := h.AskEmailChange(context.Background(), &oas.EmailChange{})
	if p, ok := res.(*oas.ProblemStatusCode); err != nil || !ok || p.StatusCode != http.StatusUnprocessableEntity || p.Response.Detail.Value != "Your password is wrong. Enter it again." {
		t.Fatalf("AskEmailChange = %#v, %v", res, err)
	}
}

func TestAnInvalidNewEmailIsRefused(t *testing.T) {
	h := web.New(&useCases{err: domain.ErrInvalidEmail}, admins{})
	res, err := h.AskEmailChange(context.Background(), &oas.EmailChange{})
	if p, ok := res.(*oas.ProblemStatusCode); err != nil || !ok || p.Response.Detail.Value != "Enter a valid email address." {
		t.Fatalf("AskEmailChange = %#v, %v", res, err)
	}
}

func TestCancellingAnEmailChangeAnswersNoContent(t *testing.T) {
	uc := &useCases{}
	h := web.New(uc, admins{})
	res, err := h.CancelEmailChange(signedIn(t, h))
	if _, ok := res.(*oas.CancelEmailChangeNoContent); err != nil || !ok || uc.asked[0] != "cancel" {
		t.Fatalf("CancelEmailChange = %#v, %v", res, err)
	}
}

func TestConfirmingAnEmailChangeSendsTheToken(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc, admins{}).ConfirmEmailChange(context.Background(), &oas.LinkToken{Token: "tok-1"})
	if _, ok := res.(*oas.ConfirmEmailChangeNoContent); err != nil || !ok || uc.asked[0] != "tok-1" {
		t.Fatalf("ConfirmEmailChange = %#v, %v", res, err)
	}
}

func TestASpentEmailChangeLinkFailsTheSameWayAsAnyOther(t *testing.T) {
	res, err := web.New(&useCases{err: app.ErrInvalidToken}, admins{}).ConfirmEmailChange(context.Background(), &oas.LinkToken{})
	if p, ok := res.(*oas.ProblemStatusCode); err != nil || !ok || p.StatusCode != http.StatusUnprocessableEntity || p.Response.Detail.Value != "This link has expired or was already used. Ask for a new one on the Account page." {
		t.Fatalf("ConfirmEmailChange = %#v, %v", res, err)
	}
}

func TestTheProfileShowsAPendingEmail(t *testing.T) {
	uc := &useCases{pending: "new@example.org"}
	h := web.New(uc, admins{})
	res, err := h.GetProfile(signedIn(t, h))
	if p, ok := res.(*oas.Profile); err != nil || !ok || p.PendingEmail != oas.NewOptEmail("new@example.org") {
		t.Fatalf("GetProfile = %#v, %v", res, err)
	}
}

func TestAnEmailChangeFailureIsPassedOn(t *testing.T) {
	for _, op := range []struct {
		name string
		call func(h *web.Handler, ctx context.Context) (any, error)
	}{
		{"asking", func(h *web.Handler, ctx context.Context) (any, error) {
			return h.AskEmailChange(ctx, &oas.EmailChange{})
		}},
		{"cancelling", func(h *web.Handler, ctx context.Context) (any, error) { return h.CancelEmailChange(ctx) }},
		{"confirming", func(h *web.Handler, ctx context.Context) (any, error) {
			return h.ConfirmEmailChange(ctx, &oas.LinkToken{})
		}},
	} {
		t.Run(op.name, func(t *testing.T) {
			h := web.New(&useCases{err: errBoom}, admins{})
			if _, err := op.call(h, context.Background()); !errors.Is(err, errBoom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestTheProfileFailsWhenThePendingEmailCannotBeRead(t *testing.T) {
	uc := &useCases{}
	h := web.New(uc, admins{})
	ctx := signedIn(t, h)
	uc.pendingErr = errBoom
	if _, err := h.GetProfile(ctx); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeletingTheAccountSendsTheSessionAndPasswordAndClearsTheCookie(t *testing.T) {
	uc := &useCases{}
	h := web.New(uc, admins{})
	res, err := h.DeleteAccount(signedIn(t, h), &oas.PasswordCheck{Password: "pw"})
	got, ok := res.(*oas.DeleteAccountNoContent)
	if err != nil || !ok || !slices.Equal(uc.asked, []string{"tok", "pw"}) || !strings.Contains(got.SetCookie, "Max-Age=0") {
		t.Fatalf("DeleteAccount = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestEachRefusedDeletionSaysWhatToDo(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		detail string
	}{
		{app.ErrInvalidCredentials, http.StatusUnprocessableEntity, "Your password is wrong. Enter it again."},
		{app.ErrFreshTwoFactorNeeded, http.StatusForbidden, "Confirm it is you with two-factor first."},
		{app.ErrLastAdmin, http.StatusUnprocessableEntity, "You are the last Admin. Make someone else an Admin before you delete your account."},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			res, err := web.New(&useCases{err: tc.err}, admins{}).DeleteAccount(context.Background(), &oas.PasswordCheck{})
			if p, ok := res.(*oas.ProblemStatusCode); err != nil || !ok || p.StatusCode != tc.status || p.Response.Detail.Value != tc.detail {
				t.Fatalf("DeleteAccount = %#v, %v", res, err)
			}
		})
	}
}

func TestAFailedDeletionIsPassedOn(t *testing.T) {
	if _, err := web.New(&useCases{err: errBoom}, admins{}).DeleteAccount(context.Background(), &oas.PasswordCheck{}); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAResetIsAskedForByEmail(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc, admins{}).AskPasswordReset(context.Background(), &oas.EmailAddress{Email: "ada@example.org"})
	if _, ok := res.(*oas.AskPasswordResetAccepted); err != nil || !ok || uc.asked[0] != "ada@example.org" {
		t.Fatalf("AskPasswordReset = %#v, %v", res, err)
	}
}

func TestAResetForSomethingThatIsNotAnEmailIsRefused(t *testing.T) {
	res, err := web.New(&useCases{err: domain.ErrInvalidEmail}, admins{}).AskPasswordReset(context.Background(), &oas.EmailAddress{})
	if p, ok := res.(*oas.ProblemStatusCode); err != nil || !ok || p.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("AskPasswordReset = %#v, %v", res, err)
	}
}

func TestCheckingAResetSaysWhatToProveItWith(t *testing.T) {
	uc := &useCases{kinds: []string{"totp"}}
	res, err := web.New(uc, admins{}).CheckPasswordReset(context.Background(), &oas.LinkToken{Token: "tok-1"})
	if got, ok := res.(*oas.ResetCheck); err != nil || !ok || len(got.Methods) != 1 || got.Methods[0] != "totp" || uc.asked[0] != "tok-1" {
		t.Fatalf("CheckPasswordReset = %#v, %v", res, err)
	}
}

func TestCheckingAResetWithNothingToProveSaysNone(t *testing.T) {
	res, err := web.New(&useCases{}, admins{}).CheckPasswordReset(context.Background(), &oas.LinkToken{Token: "tok-1"})
	if got, ok := res.(*oas.ResetCheck); err != nil || !ok || got.Methods == nil || len(got.Methods) != 0 {
		t.Fatalf("CheckPasswordReset = %#v, %v", res, err)
	}
}

func TestAResetCodeSendsTheTokenAndTheCode(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc, admins{}).ResetWithCode(context.Background(), &oas.ResetCode{Token: "tok-1", Code: "123456"})
	if _, ok := res.(*oas.ResetWithCodeNoContent); err != nil || !ok || !slices.Equal(uc.asked, []string{"tok-1", "123456"}) {
		t.Fatalf("ResetWithCode = %#v, %v", res, err)
	}
}

func TestResetPasskeyOptionsSendTheToken(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc, admins{}).ResetPasskeyOptions(context.Background(), &oas.LinkToken{Token: "tok-1"})
	if got, ok := res.(*oas.PasskeyOptions); err != nil || !ok || got.Options != `{"publicKey":{}}` || uc.asked[0] != "tok-1" {
		t.Fatalf("ResetPasskeyOptions = %#v, %v", res, err)
	}
}

func TestAResetPasskeySendsTheTokenAndTheAnswer(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc, admins{}).ResetWithPasskey(context.Background(), &oas.ResetPasskey{Token: "tok-1", Response: "signed"})
	if _, ok := res.(*oas.ResetWithPasskeyNoContent); err != nil || !ok || !slices.Equal(uc.asked, []string{"tok-1", "signed"}) {
		t.Fatalf("ResetWithPasskey = %#v, %v", res, err)
	}
}

func TestCompletingAResetSendsTheTokenAndTheNewPassword(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc, admins{}).CompletePasswordReset(context.Background(), &oas.ResetCompletion{Token: "tok-1", Password: "new"})
	if _, ok := res.(*oas.CompletePasswordResetNoContent); err != nil || !ok || !slices.Equal(uc.asked, []string{"tok-1", "new"}) {
		t.Fatalf("CompletePasswordReset = %#v, %v", res, err)
	}
}

func TestEachRefusedResetSaysWhatToDo(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		detail string
	}{
		{app.ErrInvalidToken, http.StatusUnprocessableEntity, "This link has expired or was already used. Ask for a new one."},
		{app.ErrFreshTwoFactorNeeded, http.StatusForbidden, "Confirm it is you with two-factor first."},
		{domain.ErrWeakPassword, http.StatusUnprocessableEntity, "Enter a password of 12 to 128 characters."},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			res, err := web.New(&useCases{err: tc.err}, admins{}).CompletePasswordReset(context.Background(), &oas.ResetCompletion{})
			if p, ok := res.(*oas.ProblemStatusCode); err != nil || !ok || p.StatusCode != tc.status || p.Response.Detail.Value != tc.detail {
				t.Fatalf("CompletePasswordReset = %#v, %v", res, err)
			}
		})
	}
}

func TestEveryResetStepAnswersARefusalAsAProblem(t *testing.T) {
	for _, op := range []struct {
		name string
		call func(h *web.Handler) (any, error)
	}{
		{"checking", func(h *web.Handler) (any, error) { return h.CheckPasswordReset(context.Background(), &oas.LinkToken{}) }},
		{"a code", func(h *web.Handler) (any, error) { return h.ResetWithCode(context.Background(), &oas.ResetCode{}) }},
		{"passkey options", func(h *web.Handler) (any, error) {
			return h.ResetPasskeyOptions(context.Background(), &oas.LinkToken{})
		}},
		{"a passkey", func(h *web.Handler) (any, error) {
			return h.ResetWithPasskey(context.Background(), &oas.ResetPasskey{})
		}},
	} {
		t.Run(op.name, func(t *testing.T) {
			res, err := op.call(web.New(&useCases{err: app.ErrInvalidToken}, admins{}))
			if p, ok := res.(*oas.ProblemStatusCode); err != nil || !ok || p.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("got %#v, %v", res, err)
			}
		})
	}
}

var (
	root   = uuid.MustParse("00000000-0000-4000-8000-00000000a0a0")
	target = uuid.MustParse("00000000-0000-4000-8000-0000000000a2")
)

func asRoot() context.Context {
	return principal.With(context.Background(), principal.Principal{UserID: root, SecondFactor: true})
}

func TestAnAdminsRenameSendsTheAdminThePersonAndTheName(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc, admins{}).RenameUser(asRoot(), &oas.ProfileUpdate{Name: "Ada King"}, oas.RenameUserParams{UserId: oas.ID(target)})
	if _, ok := res.(*oas.RenameUserNoContent); err != nil || !ok || !slices.Equal(uc.asked, []string{root.String(), target.String(), "Ada King"}) {
		t.Fatalf("RenameUser = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestAnAdminsEmailChangeSendsTheAdminThePersonAndTheAddress(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc, admins{}).AskUserEmailChange(asRoot(), &oas.EmailAddress{Email: "new@example.org"}, oas.AskUserEmailChangeParams{UserId: oas.ID(target)})
	if _, ok := res.(*oas.AskUserEmailChangeAccepted); err != nil || !ok || !slices.Equal(uc.asked, []string{root.String(), target.String(), "new@example.org"}) {
		t.Fatalf("AskUserEmailChange = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestAnAdminsDeletionSendsTheAdminAndThePerson(t *testing.T) {
	uc := &useCases{}
	res, err := web.New(uc, admins{}).DeleteUser(asRoot(), oas.DeleteUserParams{UserId: oas.ID(target)})
	if _, ok := res.(*oas.DeleteUserNoContent); err != nil || !ok || !slices.Equal(uc.asked, []string{root.String(), target.String()}) {
		t.Fatalf("DeleteUser = %#v, %v, asked %v", res, err, uc.asked)
	}
}

func TestEachRefusedAdminActionSaysWhatToDo(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		detail string
	}{
		{app.ErrNotAdmin, http.StatusForbidden, "Only Admins manage people."},
		{app.ErrAdminSecondFactorNeeded, http.StatusForbidden, "Sign out, then sign in again with a passkey or authenticator app to manage people."},
		{app.ErrNotFound, http.StatusNotFound, "That person no longer exists. Reload the page."},
		{app.ErrLastAdmin, http.StatusUnprocessableEntity, "That is the last Admin. Make someone else an Admin first."},
		{domain.ErrInvalidName, http.StatusUnprocessableEntity, "Enter a name of 1 to 100 characters."},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			res, err := web.New(&useCases{err: tc.err}, admins{}).DeleteUser(asRoot(), oas.DeleteUserParams{})
			if p, ok := res.(*oas.ProblemStatusCode); err != nil || !ok || p.StatusCode != tc.status || p.Response.Detail.Value != tc.detail {
				t.Fatalf("DeleteUser = %#v, %v", res, err)
			}
		})
	}
}

func TestEveryAdminActionAnswersARefusalAsAProblem(t *testing.T) {
	for _, op := range []struct {
		name string
		call func(h *web.Handler) (any, error)
	}{
		{"renaming", func(h *web.Handler) (any, error) {
			return h.RenameUser(asRoot(), &oas.ProfileUpdate{}, oas.RenameUserParams{})
		}},
		{"an email change", func(h *web.Handler) (any, error) {
			return h.AskUserEmailChange(asRoot(), &oas.EmailAddress{}, oas.AskUserEmailChangeParams{})
		}},
	} {
		t.Run(op.name, func(t *testing.T) {
			res, err := op.call(web.New(&useCases{err: app.ErrNotAdmin}, admins{}))
			if p, ok := res.(*oas.ProblemStatusCode); err != nil || !ok || p.StatusCode != http.StatusForbidden {
				t.Fatalf("got %#v, %v", res, err)
			}
		})
	}
}
