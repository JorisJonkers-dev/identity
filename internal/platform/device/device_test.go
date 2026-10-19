package device_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/platform/device"
)

func TestADeviceIsNamedByItsBrowserAndOS(t *testing.T) {
	for _, tc := range []struct{ name, agent, want string }{
		{"Chrome on Windows", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36", "Chrome on Windows"},
		{"Edge on Windows", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36 Edg/129.0.0.0", "Edge on Windows"},
		{"Firefox on macOS", "Mozilla/5.0 (Macintosh; Intel Mac OS X 14.6; rv:131.0) Gecko/20100101 Firefox/131.0", "Firefox on macOS"},
		{"Safari on macOS", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15", "Safari on macOS"},
		{"Safari on iPhone", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1", "Safari on iOS"},
		{"Safari on iPad", "Mozilla/5.0 (iPad; CPU OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1", "Safari on iPadOS"},
		{"Chrome on iPhone", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/129.0.6668.69 Mobile/15E148 Safari/604.1", "Chrome on iOS"},
		{"Firefox on iPhone", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/131.0 Mobile/15E148 Safari/605.1.15", "Firefox on iOS"},
		{"Chrome on Android", "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Mobile Safari/537.36", "Chrome on Android"},
		{"Samsung Internet on Android", "Mozilla/5.0 (Linux; Android 14; SM-S921B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/26.0 Chrome/122.0.0.0 Mobile Safari/537.36", "Samsung Internet on Android"},
		{"Firefox on Linux", "Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0", "Firefox on Linux"},
		{"Opera on Windows", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36 OPR/114.0.0.0", "Opera on Windows"},
		{"Chrome on ChromeOS", "Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36", "Chrome on ChromeOS"},
		{"a known browser on an unknown OS", "Mozilla/5.0 (Haiku) Gecko/20100101 Firefox/131.0", "Firefox"},
		{"an unknown browser on a known OS", "Mozilla/5.0 (Windows NT 10.0) Netscape/9.0", "Unknown browser on Windows"},
		{"a script", "curl/8.7.1", "Unknown device"},
		{"nothing", "", "Unknown device"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := device.Label(tc.agent); got != tc.want {
				t.Fatalf("Label = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTheMiddlewareNamesTheRequestsDevice(t *testing.T) {
	var got string
	h := device.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = device.From(r.Context()) }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "Firefox on Linux" {
		t.Fatalf("From = %q", got)
	}
}

func TestAContextWithoutADeviceIsUnknown(t *testing.T) {
	if got := device.From(context.Background()); got != "Unknown device" {
		t.Fatalf("From = %q", got)
	}
}
