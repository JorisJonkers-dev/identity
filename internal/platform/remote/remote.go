// Package remote names the client a request comes from, for limits counted per client
// (identity#81). Behind Traefik that is the last address in X-Forwarded-For, the one Traefik itself
// added; without it, the connection's own address. It is kept in memory only, never stored.
package remote

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type key struct{}

// Address is the client a request comes from.
func Address(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		parts := strings.Split(f, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Middleware names each request's client for the handlers behind it.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), key{}, Address(r))))
	})
}

// From is the client a request ctx carries came from; "" when none was named.
func From(ctx context.Context) string {
	a, _ := ctx.Value(key{}).(string)
	return a
}
