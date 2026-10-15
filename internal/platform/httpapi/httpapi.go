// Package httpapi assembles the generated ogen server from each context's web adapter, and maps
// every error the contract does not name to an RFC 9457 problem.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/ogen-go/ogen/ogenerrors"

	notesweb "github.com/JorisJonkers-dev/template-go-vue/internal/notes/adapters/web"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/httpx"
	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/oas"
)

// api is the oas.Handler: one embedded web adapter per context, plus the shared error mapping.
type api struct {
	*notesweb.Handler
	logger *slog.Logger
}

var _ oas.Handler = (*api)(nil)

// NewError maps every error the contract does not name to a problem: a request without an
// identity to a 401, anything else a handler returned to a 500. A 500's cause is logged, never sent.
func (a *api) NewError(ctx context.Context, err error) *oas.ProblemStatusCode {
	code := ogenerrors.ErrorCode(err)
	if code >= http.StatusInternalServerError {
		a.logger.ErrorContext(ctx, "request failed", "error", err)
	}
	return &oas.ProblemStatusCode{StatusCode: code, Response: httpx.Problem(code, "")}
}

// identity accepts every request the edge put an identity on; ogen refuses one without before
// calling it. A service that authorizes per subject reads t.APIKey here and puts it on ctx.
type identity struct{}

func (identity) HandleForwardAuth(ctx context.Context, _ oas.OperationName, _ oas.ForwardAuth) (context.Context, error) {
	return ctx, nil
}

// New returns the API's http.Handler, serving every path the contract declares under /api.
func New(logger *slog.Logger, notes notesweb.UseCases) (http.Handler, error) {
	return oas.NewServer(
		&api{Handler: notesweb.New(notes), logger: logger},
		identity{},
		oas.WithErrorHandler(func(_ context.Context, w http.ResponseWriter, _ *http.Request, err error) {
			httpx.WriteProblem(w, ogenerrors.ErrorCode(err), "")
		}),
		oas.WithNotFound(func(w http.ResponseWriter, _ *http.Request) {
			httpx.WriteProblem(w, http.StatusNotFound, "")
		}),
		oas.WithMethodNotAllowed(func(w http.ResponseWriter, _ *http.Request, allowed string) {
			w.Header().Set("Allow", allowed)
			httpx.WriteProblem(w, http.StatusMethodNotAllowed, "")
		}),
	)
}
