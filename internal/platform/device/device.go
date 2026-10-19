// Package device names the browser and operating system a request comes from, from its user
// agent, for the Devices list (identity#78). Only the name is kept: no user agent, no IP address
// and no location.
package device

import (
	"context"
	"net/http"
	"strings"
)

const unknown = "Unknown device"

type known = []struct{ token, name string }

// browsers are matched in order: Edge, Opera and Samsung Internet also say Chrome, and Chrome
// says Safari.
func browsers() known {
	return known{
		{"Edg/", "Edge"},
		{"OPR/", "Opera"},
		{"SamsungBrowser/", "Samsung Internet"},
		{"CriOS/", "Chrome"},
		{"FxiOS/", "Firefox"},
		{"Firefox/", "Firefox"},
		{"Chrome/", "Chrome"},
		{"Safari/", "Safari"},
	}
}

// systems are matched in order: an iPhone also says "like Mac OS X", and Android also says Linux.
func systems() known {
	return known{
		{"iPhone", "iOS"},
		{"iPad", "iPadOS"},
		{"Android", "Android"},
		{"CrOS", "ChromeOS"},
		{"Windows", "Windows"},
		{"Mac OS X", "macOS"},
		{"Linux", "Linux"},
	}
}

// Label names a user agent's browser and operating system, as "Firefox on macOS".
func Label(userAgent string) string {
	browser := first(userAgent, browsers())
	system := first(userAgent, systems())
	switch {
	case browser != "" && system != "":
		return browser + " on " + system
	case browser != "":
		return browser
	case system != "":
		return "Unknown browser on " + system
	default:
		return unknown
	}
}

func first(userAgent string, names known) string {
	for _, k := range names {
		if strings.Contains(userAgent, k.token) {
			return k.name
		}
	}
	return ""
}

type key struct{}

// Middleware names each request's device for the handlers behind it.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), key{}, Label(r.UserAgent()))))
	})
}

// From is the device the request ctx carries came from; "Unknown device" when none was named.
func From(ctx context.Context) string {
	if label, ok := ctx.Value(key{}).(string); ok {
		return label
	}
	return unknown
}
