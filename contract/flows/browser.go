package flows

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
	"time"
)

// browser is one person's browser: it keeps cookies across identity and the app, and treats
// localhost as secure, as browsers do, so identity's Secure cookies come back over plain HTTP.
type browser struct{ client *http.Client }

// newBrowser is a browser trusting the app's proxy through trust.
func newBrowser(t *testing.T, trust *x509.CertPool) *browser {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: trust, MinVersion: tls.VersionTLS12}
	return &browser{client: &http.Client{Jar: secureLocalhost{jar}, Transport: transport, Timeout: 15 * time.Second}}
}

// secureLocalhost is a cookie jar that reads http://localhost as https.
type secureLocalhost struct{ jar http.CookieJar }

func (j secureLocalhost) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.jar.SetCookies(asSecure(u), cookies)
}

func (j secureLocalhost) Cookies(u *url.URL) []*http.Cookie { return j.jar.Cookies(asSecure(u)) }

func asSecure(u *url.URL) *url.URL {
	if u.Scheme != "http" || u.Hostname() != "localhost" {
		return u
	}
	secure := *u
	secure.Scheme = "https"
	return &secure
}

// answer is where a request ended up after following every redirect, and what it answered there.
type answer struct {
	URL    *url.URL
	Status int
	Body   []byte
}

// get follows target's redirects to the end.
func (b *browser) get(t *testing.T, target string) answer {
	t.Helper()
	return b.do(t, http.MethodGet, target, nil)
}

func (b *browser) do(t *testing.T, method, target string, body any) answer {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, target, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := b.client.Do(req)
	if err != nil {
		t.Fatalf("flows: %s %s: %v", method, target, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	return answer{URL: res.Request.URL, Status: res.StatusCode, Body: raw}
}

// call sends body to identity's API as JSON, wants status, and decodes the answer into out.
func (b *browser) call(t *testing.T, method, target string, body any, status int, out any) {
	t.Helper()
	a := b.do(t, method, target, body)
	if a.Status != status {
		t.Fatalf("flows: %s %s = %d, want %d: %s", method, target, a.Status, status, a.Body)
	}
	if out != nil {
		if err := json.Unmarshal(a.Body, out); err != nil {
			t.Fatalf("flows: %s %s: %v in %s", method, target, err, a.Body)
		}
	}
}
