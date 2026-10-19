//go:build dev

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"sync"
	"time"

	usersapp "github.com/JorisJonkers-dev/identity/internal/users/app"
)

// base is where the seed's browsers reach identity, in process; https so the session cookies,
// which are Secure, are sent back.
const base = "https://localhost:5173"

// inProcess serves requests with identity's own handler, with no network in between.
type inProcess struct{ h http.Handler }

func (p inProcess) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	p.h.ServeHTTP(rec, r)
	return rec.Result(), nil
}

// browser is one person's browser: their cookies, against identity's API.
type browser struct {
	client *http.Client
}

func newBrowser(h http.Handler) *browser {
	jar, _ := cookiejar.New(nil) // cookiejar.New never fails without options
	return &browser{client: &http.Client{Transport: inProcess{h}, Jar: jar}}
}

// call sends body as JSON and fails unless the answer has status want; out, when not nil, gets
// the answer's JSON.
func (b *browser) call(ctx context.Context, method, path string, body any, want int, out any) error {
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, payload)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		return fmt.Errorf("%s %s = %d %s, want %d", method, path, res.StatusCode, raw, want)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// mailbox keeps the last mail to each address, for the confirmation links.
type mailbox struct {
	mu   sync.Mutex
	last map[string]string
}

func (m *mailbox) Send(_ context.Context, mail usersapp.Mail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.last[mail.To] = mail.Text
	return nil
}

var confirmLink = regexp.MustCompile(`/confirm\?token=([^\s"<>]+)`)

func (m *mailbox) confirmation(email string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := confirmLink.FindStringSubmatch(m.last[email])
	if found == nil {
		return "", fmt.Errorf("no confirmation mail to %s", email)
	}
	return found[1], nil
}

// clock is the time the use cases see: set to a moment in the past, and a millisecond later at
// every reading so events keep their order.
type clock struct {
	mu sync.Mutex
	at time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(time.Millisecond)
	return c.at
}

func (c *clock) set(at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = at
}
