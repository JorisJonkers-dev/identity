package httpapi

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ogen-go/ogen/ogenerrors"
)

// NewError is reached only by a handler error or a request the contract refuses, and the only
// operation today does neither, so it is driven directly.
func TestNewErrorMapsCodesAndLogsOnlyServerFailures(t *testing.T) {
	var logs bytes.Buffer
	a := &api{logger: slog.New(slog.NewTextHandler(&logs, nil))}

	refused := a.NewError(t.Context(), &ogenerrors.SecurityError{Err: errors.New("no credential")})
	if refused.StatusCode != http.StatusUnauthorized || refused.Response.Status != http.StatusUnauthorized {
		t.Fatalf("a refused credential = %+v, want a 401 problem", refused)
	}
	if logs.Len() != 0 {
		t.Fatalf("a 401 was logged: %s", logs.String())
	}

	failed := a.NewError(t.Context(), errors.New("connection refused to 10.0.0.7"))
	if failed.StatusCode != http.StatusInternalServerError || failed.Response.Detail.Set {
		t.Fatalf("a handler failure = %+v, want a 500 without detail", failed)
	}
	if !strings.Contains(logs.String(), "connection refused to 10.0.0.7") {
		t.Fatalf("a 500's cause was not logged: %s", logs.String())
	}
}

func TestADecodeFailureIsAProblemWithOgensStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(t.Context(), rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), &ogenerrors.DecodeRequestError{Err: errors.New("bad body")})
	if rec.Code != http.StatusBadRequest || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("decode failure = %d %q, want a 400 problem", rec.Code, rec.Header().Get("Content-Type"))
	}
}
