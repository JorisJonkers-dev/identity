// Package httpapi assembles the generated ogen server from each context's web adapter, and maps
// every error the contract does not name to an RFC 9457 problem.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/ogen-go/ogen/ogenerrors"

	accessweb "github.com/JorisJonkers-dev/identity/internal/access/adapters/web"
	auditweb "github.com/JorisJonkers-dev/identity/internal/audit/adapters/web"
	launcherweb "github.com/JorisJonkers-dev/identity/internal/launcher/adapters/web"
	"github.com/JorisJonkers-dev/identity/internal/platform/httpx"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	signinweb "github.com/JorisJonkers-dev/identity/internal/signin/adapters/web"
	tokensweb "github.com/JorisJonkers-dev/identity/internal/tokens/adapters/web"
	usersweb "github.com/JorisJonkers-dev/identity/internal/users/adapters/web"
)

// api is the oas.Handler: one embedded web adapter per context, plus the shared error mapping.
type api struct {
	*usersweb.Handler
	*accessweb.Access
	*auditweb.Audit
	*launcherweb.Launcher
	*tokensweb.Tokens
	*signinweb.Apps
	logger  *slog.Logger
	version string
}

var _ oas.Handler = (*api)(nil)

// NewError maps every error the contract does not name to a problem with the status ogen gives
// it; anything a handler returned is a 500. A 500's cause is logged, never sent.
func (a *api) NewError(ctx context.Context, err error) *oas.ProblemStatusCode {
	code := ogenerrors.ErrorCode(err)
	if code >= http.StatusInternalServerError {
		a.logger.ErrorContext(ctx, "request failed", "error", err)
	}
	return &oas.ProblemStatusCode{StatusCode: code, Response: httpx.Problem(code, "")}
}

// GetVersion reports the build that is serving; it needs no sign-in.
func (a *api) GetVersion(context.Context) (oas.GetVersionRes, error) {
	return &oas.Version{Version: a.version}, nil
}

// maxBody caps a request body; the largest the contract accepts is a few hundred bytes.
const maxBody = 64 << 10

// Access is the access context: its use cases, and who is an Admin.
type Access interface {
	accessweb.UseCases
	usersweb.Admins
}

// New returns the API's http.Handler, serving every path the contract declares under /api.
// The users adapter is also the session security handler.
func New(logger *slog.Logger, version string, users usersweb.UseCases, access Access, audit *auditweb.Audit, launcher launcherweb.UseCases, tokens *tokensweb.Tokens, apps *signinweb.Apps) (http.Handler, error) {
	h := usersweb.New(users, access)
	srv, err := oas.NewServer(
		&api{Handler: h, Access: accessweb.New(access), Audit: audit, Launcher: launcherweb.NewLauncher(launcher), Tokens: tokens, Apps: apps, logger: logger, version: version},
		h,
		oas.WithErrorHandler(writeError),
		oas.WithNotFound(func(w http.ResponseWriter, _ *http.Request) {
			httpx.WriteProblem(w, http.StatusNotFound, "")
		}),
		oas.WithMethodNotAllowed(func(w http.ResponseWriter, _ *http.Request, allowed string) {
			w.Header().Set("Allow", allowed)
			httpx.WriteProblem(w, http.StatusMethodNotAllowed, "")
		}),
	)
	return httpx.LimitBody(maxBody, srv), err // NewServer fails only on a bad option, never at run time
}

// writeError answers a request ogen could not decode with a problem carrying ogen's status.
func writeError(_ context.Context, w http.ResponseWriter, _ *http.Request, err error) {
	httpx.WriteProblem(w, ogenerrors.ErrorCode(err), "")
}
