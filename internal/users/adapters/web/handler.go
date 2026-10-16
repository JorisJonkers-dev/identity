// Package web is the users context's inbound HTTP adapter: it implements the account and profile
// operations of the generated ogen Handler, and the session security handler, over the use cases.
package web

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/platform/httpx"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	"github.com/JorisJonkers-dev/identity/internal/platform/principal"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// CookieName is the Single sign-on session cookie. The __Host- prefix makes the browser keep it
// to this host, over HTTPS, with Path=/.
const CookieName = "__Host-identity_session"

// challengeCookie holds a sign-in whose password was right while it waits for a Second Factor.
const challengeCookie = "__Host-identity_challenge"

// challengeMaxAge is the challenge's five minutes, in seconds.
const challengeMaxAge = 5 * 60

// sessionMaxAge is the cookie's lifetime in seconds, the session's 30 days (docs/adr/0007).
const sessionMaxAge = int(30 * 24 * time.Hour / time.Second)

// expireNow is net/http's MaxAge for "Max-Age=0"; a MaxAge of 0 would send no Max-Age at all.
const expireNow = -1

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	SignUp(ctx context.Context, email, name, password string) error
	Confirm(ctx context.Context, token, password string) error
	SignIn(ctx context.Context, email, password string) (string, error)
	CompleteSignIn(ctx context.Context, challenge, code string) (string, error)
	SignOut(ctx context.Context, token string) error
	SignedIn(ctx context.Context, token string) (domain.User, app.Session, error)
	ChallengeKinds(ctx context.Context, challenge string) ([]string, error)
	PasskeyOptions(ctx context.Context, challenge string) ([]byte, error)
	CompleteSignInWithPasskey(ctx context.Context, challenge string, response []byte) (string, error)
	SecondFactors(ctx context.Context, userID uuid.UUID) ([]app.Factor, error)
	StartTOTP(ctx context.Context, userID uuid.UUID) (app.TOTPSetup, error)
	ConfirmTOTP(ctx context.Context, userID uuid.UUID, sessionToken, label, code, password string) error
	StartPasskey(ctx context.Context, userID uuid.UUID) ([]byte, error)
	ConfirmPasskey(ctx context.Context, userID uuid.UUID, sessionToken, label string, response []byte, password string) error
	RemoveFactor(ctx context.Context, userID, factorID uuid.UUID, sessionToken, password string) error
	Rename(ctx context.Context, id uuid.UUID, name string) error
	ChangePassword(ctx context.Context, id uuid.UUID, sessionToken, current, next string) error
	User(ctx context.Context, id uuid.UUID) (domain.User, error)
}

// Admins tells whether a User is an Admin, for the Profile to say so.
type Admins interface {
	IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error)
}

// Handler implements the users operations and the session security scheme.
type Handler struct {
	uc     UseCases
	admins Admins
}

// New returns a Handler calling uc, asking admins who is an Admin.
func New(uc UseCases, admins Admins) *Handler { return &Handler{uc: uc, admins: admins} }

type signedIn struct {
	user         domain.User
	token        string
	secondFactor bool
}

type signedInKey struct{}

// HandleSession resolves the session cookie to its User; ogen answers 401 when it fails.
func (h *Handler) HandleSession(ctx context.Context, _ oas.OperationName, t oas.Session) (context.Context, error) {
	u, sess, err := h.uc.SignedIn(ctx, t.APIKey)
	if err != nil {
		return ctx, err
	}
	proved := app.ProvedSecondFactor(sess.Methods)
	ctx = principal.With(ctx, principal.Principal{UserID: u.ID, SecondFactor: proved})
	return context.WithValue(ctx, signedInKey{}, signedIn{user: u, token: t.APIKey, secondFactor: proved}), nil
}

type challengeKey struct{}

// HandleChallenge passes the challenge cookie to completeSignIn, which checks it.
func (h *Handler) HandleChallenge(ctx context.Context, _ oas.OperationName, t oas.Challenge) (context.Context, error) {
	return context.WithValue(ctx, challengeKey{}, t.APIKey), nil
}

// current is the signed-in User; ogen only calls an operation that needs one after HandleSession.
func current(ctx context.Context) signedIn {
	s, _ := ctx.Value(signedInKey{}).(signedIn)
	return s
}

// SignUp implements signUp.
func (h *Handler) SignUp(ctx context.Context, req *oas.SignUp) (oas.SignUpRes, error) {
	if err := h.uc.SignUp(ctx, string(req.Email), req.Name, string(req.Password)); err != nil {
		return refusal(err)
	}
	return &oas.SignUpAccepted{}, nil
}

// ConfirmEmail implements confirmEmail.
func (h *Handler) ConfirmEmail(ctx context.Context, req *oas.Confirmation) (oas.ConfirmEmailRes, error) {
	err := h.uc.Confirm(ctx, req.Token, string(req.Password))
	if errors.Is(err, app.ErrInvalidCredentials) {
		return problem(http.StatusUnprocessableEntity, "That is not the password you chose when you signed up. Try again."), nil
	}
	if err != nil {
		return refusal(err)
	}
	return &oas.ConfirmEmailNoContent{}, nil
}

// SignIn implements signIn.
func (h *Handler) SignIn(ctx context.Context, req *oas.SignIn) (oas.SignInRes, error) {
	token, err := h.uc.SignIn(ctx, req.Email, string(req.Password))
	if errors.Is(err, app.ErrSecondFactorNeeded) {
		kinds, err := h.uc.ChallengeKinds(ctx, token)
		if err != nil {
			return nil, err
		}
		methods := make([]oas.SecondFactorKind, 0, len(kinds))
		for _, k := range kinds {
			methods = append(methods, oas.SecondFactorKind(k))
		}
		return &oas.SecondFactorNeededHeaders{
			SetCookie: named(challengeCookie, token, challengeMaxAge).String(),
			Response:  oas.SecondFactorNeeded{Methods: methods},
		}, nil
	}
	if err != nil {
		return refusal(err)
	}
	return &oas.SignInNoContent{SetCookie: cookie(token, sessionMaxAge).String()}, nil
}

// CompleteSignIn implements completeSignIn.
func (h *Handler) CompleteSignIn(ctx context.Context, req *oas.SecondFactorCode) (oas.CompleteSignInRes, error) {
	challenge, _ := ctx.Value(challengeKey{}).(string)
	token, err := h.uc.CompleteSignIn(ctx, challenge, req.Code)
	if errors.Is(err, app.ErrInvalidToken) {
		return expiredSignIn(), nil
	}
	if err != nil {
		return refusal(err)
	}
	return &oas.CompleteSignInNoContent{SetCookie: cookie(token, sessionMaxAge).String()}, nil
}

func expiredSignIn() *oas.ProblemStatusCode {
	return problem(http.StatusUnauthorized, "This sign-in has expired, or too many wrong codes were entered. Sign in again, and after five wrong codes wait 15 minutes first.")
}

// PasskeyOptions implements passkeyOptions.
func (h *Handler) PasskeyOptions(ctx context.Context) (oas.PasskeyOptionsRes, error) {
	challenge, _ := ctx.Value(challengeKey{}).(string)
	options, err := h.uc.PasskeyOptions(ctx, challenge)
	switch {
	case errors.Is(err, app.ErrInvalidToken):
		return expiredSignIn(), nil
	case err != nil:
		return refusal(err)
	}
	return &oas.PasskeyOptions{Options: string(options)}, nil
}

// CompleteSignInWithPasskey implements completeSignInWithPasskey.
func (h *Handler) CompleteSignInWithPasskey(ctx context.Context, req *oas.PasskeyAssertion) (oas.CompleteSignInWithPasskeyRes, error) {
	challenge, _ := ctx.Value(challengeKey{}).(string)
	token, err := h.uc.CompleteSignInWithPasskey(ctx, challenge, []byte(req.Response))
	switch {
	case errors.Is(err, app.ErrInvalidToken):
		return expiredSignIn(), nil
	case err != nil:
		return refusal(err)
	}
	return &oas.CompleteSignInWithPasskeyNoContent{SetCookie: cookie(token, sessionMaxAge).String()}, nil
}

// StartPasskey implements startPasskey.
func (h *Handler) StartPasskey(ctx context.Context) (oas.StartPasskeyRes, error) {
	options, err := h.uc.StartPasskey(ctx, current(ctx).user.ID)
	if err != nil {
		return refusal(err)
	}
	return &oas.PasskeyOptions{Options: string(options)}, nil
}

// ConfirmPasskey implements confirmPasskey.
func (h *Handler) ConfirmPasskey(ctx context.Context, req *oas.PasskeyConfirmation) (oas.ConfirmPasskeyRes, error) {
	s := current(ctx)
	err := h.uc.ConfirmPasskey(ctx, s.user.ID, s.token, req.Label, []byte(req.Response), string(req.Password))
	if err != nil {
		return enrolRefusal(err)
	}
	return &oas.ConfirmPasskeyNoContent{}, nil
}

// enrolRefusal answers a refused ConfirmTOTP or ConfirmPasskey.
func enrolRefusal(err error) (*oas.ProblemStatusCode, error) {
	switch {
	case errors.Is(err, app.ErrInvalidToken):
		return problem(http.StatusUnprocessableEntity, "This setup has expired. Start adding it again."), nil
	case errors.Is(err, app.ErrInvalidCredentials):
		return problem(http.StatusUnprocessableEntity, "Your password is wrong. Enter it again."), nil
	case errors.Is(err, app.ErrSecondFactorNeeded):
		return problem(http.StatusForbidden, "Sign out, then sign in again with a passkey or authenticator app to add another."), nil
	default:
		return refusal(err)
	}
}

// ListSecondFactors implements listSecondFactors.
func (h *Handler) ListSecondFactors(ctx context.Context) (oas.ListSecondFactorsRes, error) {
	factors, err := h.uc.SecondFactors(ctx, current(ctx).user.ID)
	if err != nil {
		return nil, err
	}
	res := &oas.SecondFactors{Factors: make([]oas.SecondFactor, 0, len(factors))}
	for _, f := range factors {
		sf := oas.SecondFactor{ID: oas.ID(f.ID), Kind: oas.SecondFactorKind(f.Kind), Label: f.Label.String(), CreatedAt: f.CreatedAt}
		if f.LastUsedAt != nil {
			sf.LastUsedAt = oas.NewOptDateTime(*f.LastUsedAt)
		}
		res.Factors = append(res.Factors, sf)
	}
	return res, nil
}

// StartTOTP implements startTOTP.
func (h *Handler) StartTOTP(ctx context.Context) (oas.StartTOTPRes, error) {
	setup, err := h.uc.StartTOTP(ctx, current(ctx).user.ID)
	if err != nil {
		return refusal(err)
	}
	return &oas.TOTPSetup{Secret: setup.Secret, URI: setup.URI}, nil
}

// ConfirmTOTP implements confirmTOTP.
func (h *Handler) ConfirmTOTP(ctx context.Context, req *oas.TOTPConfirmation) (oas.ConfirmTOTPRes, error) {
	s := current(ctx)
	err := h.uc.ConfirmTOTP(ctx, s.user.ID, s.token, req.Label, req.Code, string(req.Password))
	if err != nil {
		return enrolRefusal(err)
	}
	return &oas.ConfirmTOTPNoContent{}, nil
}

// RemoveSecondFactor implements removeSecondFactor.
func (h *Handler) RemoveSecondFactor(ctx context.Context, req *oas.PasswordCheck, p oas.RemoveSecondFactorParams) (oas.RemoveSecondFactorRes, error) {
	s := current(ctx)
	err := h.uc.RemoveFactor(ctx, s.user.ID, uuid.UUID(p.FactorId), s.token, string(req.Password))
	switch {
	case errors.Is(err, app.ErrSecondFactorNeeded):
		return problem(http.StatusForbidden, "Sign out, then sign in again with a passkey or authenticator app to remove one."), nil
	case errors.Is(err, app.ErrInvalidCredentials):
		return problem(http.StatusUnprocessableEntity, "Your password is wrong. Enter it again."), nil
	case errors.Is(err, app.ErrNotFound):
		return problem(http.StatusNotFound, "That passkey or authenticator app is no longer on your account. Reload the page."), nil
	case err != nil:
		return nil, err
	}
	return &oas.RemoveSecondFactorNoContent{}, nil
}

// SignOut implements signOut.
func (h *Handler) SignOut(ctx context.Context) (oas.SignOutRes, error) {
	if err := h.uc.SignOut(ctx, current(ctx).token); err != nil {
		return nil, err
	}
	return &oas.SignOutNoContent{SetCookie: cookie("", expireNow).String()}, nil
}

// GetProfile implements getProfile.
func (h *Handler) GetProfile(ctx context.Context) (oas.GetProfileRes, error) {
	return h.profile(ctx, current(ctx).user)
}

// UpdateProfile implements updateProfile.
func (h *Handler) UpdateProfile(ctx context.Context, req *oas.ProfileUpdate) (oas.UpdateProfileRes, error) {
	id := current(ctx).user.ID
	if err := h.uc.Rename(ctx, id, req.Name); err != nil {
		return refusal(err)
	}
	u, err := h.uc.User(ctx, id)
	if err != nil {
		return nil, err
	}
	return h.profile(ctx, u)
}

// ChangePassword implements changePassword.
func (h *Handler) ChangePassword(ctx context.Context, req *oas.PasswordChange) (oas.ChangePasswordRes, error) {
	s := current(ctx)
	err := h.uc.ChangePassword(ctx, s.user.ID, s.token, string(req.Current), string(req.Next))
	if errors.Is(err, app.ErrInvalidCredentials) {
		return problem(http.StatusUnprocessableEntity, "Your current password is wrong. Enter it again."), nil
	}
	if err != nil {
		return refusal(err)
	}
	return &oas.ChangePasswordNoContent{}, nil
}

func (h *Handler) profile(ctx context.Context, u domain.User) (*oas.Profile, error) {
	admin, err := h.admins.IsAdmin(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &oas.Profile{
		Email: oas.Email(u.Email.String()), Name: u.Name.String(), CreatedAt: u.CreatedAt, Admin: admin,
		SecondFactor: current(ctx).secondFactor,
	}, nil
}

// refusal answers a rule the caller broke with a problem saying which, and passes anything else on
// to NewError as a 500. Its result type fits every operation's response union.
func refusal(err error) (*oas.ProblemStatusCode, error) {
	switch {
	case errors.Is(err, domain.ErrInvalidEmail):
		return problem(http.StatusUnprocessableEntity, "Enter a valid email address."), nil
	case errors.Is(err, domain.ErrInvalidName):
		return problem(http.StatusUnprocessableEntity, "Enter a name of 1 to 100 characters."), nil
	case errors.Is(err, domain.ErrWeakPassword):
		return problem(http.StatusUnprocessableEntity, "Enter a password of 12 to 128 characters."), nil
	case errors.Is(err, app.ErrInvalidToken):
		return problem(http.StatusUnprocessableEntity, "This link has expired or was already used. Sign up again for a new one."), nil
	case errors.Is(err, app.ErrInvalidCredentials):
		return problem(http.StatusUnauthorized, "The email or the password is wrong."), nil
	case errors.Is(err, domain.ErrInvalidLabel):
		return problem(http.StatusUnprocessableEntity, "Enter a name of 1 to 64 characters for this authenticator app."), nil
	case errors.Is(err, app.ErrInvalidCode):
		return problem(http.StatusUnprocessableEntity, "That code is wrong or was already used. Enter the code your authenticator app shows now."), nil
	case errors.Is(err, app.ErrInvalidPasskey):
		return problem(http.StatusUnprocessableEntity, "Your passkey was not accepted. Try again, or sign in with another passkey or an authenticator app code."), nil
	case errors.Is(err, app.ErrNoPasskeys):
		return problem(http.StatusUnprocessableEntity, "You have no passkey. Enter the code from your authenticator app instead."), nil
	case errors.Is(err, app.ErrUnavailable):
		return problem(http.StatusServiceUnavailable, "Authenticator apps and passkeys can't be used right now. Try again later."), nil
	case errors.Is(err, app.ErrBusy):
		return problem(http.StatusServiceUnavailable, "Too many people are signing in right now. Try again in a moment."), nil
	default:
		return nil, err
	}
}

func problem(status int, detail string) *oas.ProblemStatusCode {
	return &oas.ProblemStatusCode{StatusCode: status, Response: httpx.Problem(status, detail)}
}

func cookie(value string, maxAge int) *http.Cookie { return named(CookieName, value, maxAge) }

func named(name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	}
}
