// Package httpx holds the HTTP pieces every inbound adapter shares: RFC 9457 problems and browser
// hardening headers, and the request body cap.
package httpx

import (
	"net/http"

	"github.com/JorisJonkers-dev/identity/internal/platform/oas"
)

const contentSecurityPolicy = "default-src 'self'; img-src 'self' data:; style-src 'self'; " +
	"connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

// Problem is the one place an RFC 9457 problem is built: its title is the status text, and detail,
// when not empty, says what the caller can do about it. Causes belong in the logs, never here.
func Problem(status int, detail string) oas.Problem {
	p := oas.Problem{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: int32(status), //nolint:gosec // an HTTP status code fits in an int32
		Detail: oas.OptString{},
	}
	if detail != "" {
		p.Detail = oas.NewOptString(detail)
	}
	return p
}

// WriteProblem writes Problem(status, detail), for the responses the generated server does not encode.
func WriteProblem(w http.ResponseWriter, status int, detail string) {
	p := Problem(status, detail)
	body, _ := p.MarshalJSON() // cannot fail: every field is a string or an integer
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// SecurityHeaders sets the browser hardening headers on every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

// LimitBody refuses a request declaring a body over limit bytes with a 413, and stops reading an
// undeclared one at limit, so no request can make the server buffer more than that.
func LimitBody(limit int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > limit {
			WriteProblem(w, http.StatusRequestEntityTooLarge, "")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}
