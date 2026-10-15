// Package httpapi assembles the generated ogen server from each context's web adapter, and maps
// every error the contract does not name to an RFC 9457 problem.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/ogen-go/ogen/ogenerrors"

	"github.com/JorisJonkers-dev/identity/internal/platform/httpx"
	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
	usersweb "github.com/JorisJonkers-dev/identity/internal/users/adapters/web"
)

// api is the oas.Handler: one embedded web adapter per context, plus the shared error mapping.
type api struct {
	*usersweb.Handler
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

// New returns the API's http.Handler, serving every path the contract declares under /api.
// The users adapter is also the session security handler.
func New(logger *slog.Logger, version string, users usersweb.UseCases) (http.Handler, error) {
	h := usersweb.New(users)
	srv, err := oas.NewServer(
		&api{Handler: h, logger: logger, version: version},
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
