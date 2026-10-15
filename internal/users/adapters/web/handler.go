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
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// CookieName is the Single sign-on session cookie. The __Host- prefix makes the browser keep it
// to this host, over HTTPS, with Path=/.
const CookieName = "__Host-identity_session"

// sessionMaxAge is the cookie's lifetime in seconds, the session's 30 days (docs/adr/0007).
const sessionMaxAge = int(30 * 24 * time.Hour / time.Second)

// expireNow is net/http's MaxAge for "Max-Age=0"; a MaxAge of 0 would send no Max-Age at all.
const expireNow = -1

// UseCases is what the adapter needs from the application layer.
type UseCases interface {
	SignUp(ctx context.Context, email, name, password string) error
	Confirm(ctx context.Context, token, password string) error
	SignIn(ctx context.Context, email, password string) (string, error)
	SignOut(ctx context.Context, token string) error
	Authenticate(ctx context.Context, token string) (domain.User, error)
	Rename(ctx context.Context, id uuid.UUID, name string) error
	ChangePassword(ctx context.Context, id uuid.UUID, sessionToken, current, next string) error
	User(ctx context.Context, id uuid.UUID) (domain.User, error)
}

// Handler implements the users operations and the session security scheme.
type Handler struct {
	uc UseCases
}

// New returns a Handler calling uc.
func New(uc UseCases) *Handler { return &Handler{uc: uc} }

type signedIn struct {
	user  domain.User
	token string
}

type signedInKey struct{}

// HandleSession resolves the session cookie to its User; ogen answers 401 when it fails.
func (h *Handler) HandleSession(ctx context.Context, _ oas.OperationName, t oas.Session) (context.Context, error) {
	u, err := h.uc.Authenticate(ctx, t.APIKey)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, signedInKey{}, signedIn{user: u, token: t.APIKey}), nil
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
		return problem(http.StatusUnprocessableEntity, "That is not the password you signed up with."), nil
	}
	if err != nil {
		return refusal(err)
	}
	return &oas.ConfirmEmailNoContent{}, nil
}

// SignIn implements signIn.
func (h *Handler) SignIn(ctx context.Context, req *oas.SignIn) (oas.SignInRes, error) {
	token, err := h.uc.SignIn(ctx, req.Email, string(req.Password))
	if err != nil {
		return refusal(err)
	}
	return &oas.SignInNoContent{SetCookie: cookie(token, sessionMaxAge).String()}, nil
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
	return profile(current(ctx).user), nil
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
	return profile(u), nil
}

// ChangePassword implements changePassword.
func (h *Handler) ChangePassword(ctx context.Context, req *oas.PasswordChange) (oas.ChangePasswordRes, error) {
	s := current(ctx)
	err := h.uc.ChangePassword(ctx, s.user.ID, s.token, string(req.Current), string(req.Next))
	if errors.Is(err, app.ErrInvalidCredentials) {
		return problem(http.StatusUnprocessableEntity, "The current password is wrong."), nil
	}
	if err != nil {
		return refusal(err)
	}
	return &oas.ChangePasswordNoContent{}, nil
}

func profile(u domain.User) *oas.Profile {
	return &oas.Profile{Email: oas.Email(u.Email.String()), Name: u.Name.String(), CreatedAt: u.CreatedAt}
}

// refusal answers a rule the caller broke with a problem saying which, and passes anything else on
// to NewError as a 500. Its result type fits every operation's response union.
func refusal(err error) (*oas.ProblemStatusCode, error) {
	switch {
	case errors.Is(err, domain.ErrInvalidEmail):
		return problem(http.StatusUnprocessableEntity, "Enter a valid email address."), nil
	case errors.Is(err, domain.ErrInvalidName):
		return problem(http.StatusUnprocessableEntity, "A name is 1 to 100 characters."), nil
	case errors.Is(err, domain.ErrWeakPassword):
		return problem(http.StatusUnprocessableEntity, "A password is 12 to 128 characters."), nil
	case errors.Is(err, app.ErrInvalidToken):
		return problem(http.StatusUnprocessableEntity, "This link has expired or was already used. Sign up again for a new one."), nil
	case errors.Is(err, app.ErrInvalidCredentials):
		return problem(http.StatusUnauthorized, "The email or the password is wrong."), nil
	case errors.Is(err, app.ErrBusy):
		return problem(http.StatusServiceUnavailable, "Too many sign-ins at once. Try again in a moment."), nil
	default:
		return nil, err
	}
}

func problem(status int, detail string) *oas.ProblemStatusCode {
	return &oas.ProblemStatusCode{StatusCode: status, Response: httpx.Problem(status, detail)}
}

func cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: CookieName, Value: value, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	}
}
