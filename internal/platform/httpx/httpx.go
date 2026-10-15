// Package httpx holds the HTTP pieces every inbound adapter shares: RFC 9457 problems, browser
// hardening headers, and the local stand-in for the platform's forward-auth.
package httpx

import (
	"net/http"

	"github.com/JorisJonkers-dev/template-go-vue/internal/platform/oas"
)

// IdentityHeader carries the caller's subject. The platform's forward-auth sets it at the edge.
const IdentityHeader = "X-User-Id"

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

// DevIdentity fills in subject when a request carries no identity. Wire it only for local runs,
// where no forward-auth sits in front of the service.
func DevIdentity(subject string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(IdentityHeader) == "" {
			r.Header.Set(IdentityHeader, subject)
		}
		next.ServeHTTP(w, r)
	})
}
