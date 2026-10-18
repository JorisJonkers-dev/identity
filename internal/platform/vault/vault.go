// Package vault reaches the Vault engines identity uses: transit, which seals small secrets so the
// key that protects them never leaves Vault (docs/adr/0013), and KV, where Applications read their
// Clients' secrets (identity#32).
package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
)

// maxResponse bounds what is read from Vault; every answer identity asks for is a few hundred bytes.
const maxResponse = 64 << 10

// Login gets a Vault token.
type Login func(ctx context.Context, c *http.Client, addr string) (string, error)

// Token is a fixed token, for a local run against a dev-mode Vault.
func Token(token string) Login {
	return func(context.Context, *http.Client, string) (string, error) { return token, nil }
}

// Kubernetes signs in with the pod's service account token through the Kubernetes auth method.
func Kubernetes(mount, role, jwtPath string) Login {
	return func(ctx context.Context, c *http.Client, addr string) (string, error) {
		jwt, err := os.ReadFile(jwtPath) //nolint:gosec // a path from configuration, not from a request
		if err != nil {
			return "", fmt.Errorf("vault: read the service account token: %w", err)
		}
		var out struct {
			Auth struct {
				ClientToken string `json:"client_token"`
			} `json:"auth"`
		}
		body := map[string]string{"role": role, "jwt": strings.TrimSpace(string(jwt))}
		if err := call(ctx, c, http.MethodPost, addr+"/v1/auth/"+mount+"/login", "", body, &out); err != nil {
			return "", err
		}
		if out.Auth.ClientToken == "" {
			return "", errors.New("vault: the login returned no token")
		}
		return out.Auth.ClientToken, nil
	}
}

// Vault is one Vault, signed in to once for every engine reached through it.
type Vault struct {
	addr   string
	login  Login
	client *http.Client

	mu    sync.Mutex
	token string
}

// New returns the Vault at addr, signed in to by login when first used.
func New(addr string, login Login, client *http.Client) *Vault {
	return &Vault{addr: strings.TrimRight(addr, "/"), login: login, client: client}
}

// do calls Vault at path, signing in first and once more if the token was refused.
func (v *Vault) do(ctx context.Context, method, path string, in, out any) error {
	for attempt := 0; ; attempt++ {
		token, err := v.current(ctx, attempt > 0)
		if err != nil {
			return err
		}
		err = call(ctx, v.client, method, v.addr+path, token, in, out)
		var status statusError
		if attempt == 0 && errors.As(err, &status) && status == http.StatusForbidden {
			continue
		}
		return err
	}
}

func (v *Vault) current(ctx context.Context, renew bool) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.token != "" && !renew {
		return v.token, nil
	}
	token, err := v.login(ctx, v.client, v.addr)
	if err != nil {
		return "", err
	}
	v.token = token
	return token, nil
}

// statusError is an answer Vault gave with a status other than 200 or 204.
type statusError int

func (s statusError) Error() string { return fmt.Sprintf("vault: answered %d", int(s)) }

// call sends in as JSON, or no body when in is nil, and decodes the answer into out.
func call(ctx context.Context, c *http.Client, method, url, token string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, _ := json.Marshal(in) // maps of strings, which always marshal
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	res, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponse))
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return statusError(res.StatusCode)
	}
	return json.Unmarshal(raw, out)
}
