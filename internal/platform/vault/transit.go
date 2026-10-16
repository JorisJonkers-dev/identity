// Package vault seals small secrets with Vault's transit engine, so the key that protects them
// never leaves Vault (docs/adr/0013). identity keeps only the ciphertext.
package vault

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
)

// ErrSealed is a ciphertext Vault refused to open.
var ErrSealed = errors.New("vault: the ciphertext cannot be opened")

// maxResponse bounds what is read from Vault; a transit answer is a few hundred bytes.
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
		if err := call(ctx, c, addr+"/v1/auth/"+mount+"/login", "", body, &out); err != nil {
			return "", err
		}
		if out.Auth.ClientToken == "" {
			return "", errors.New("vault: the login returned no token")
		}
		return out.Auth.ClientToken, nil
	}
}

// Transit seals and opens with one transit key.
type Transit struct {
	addr, path string
	login      Login
	client     *http.Client

	mu    sync.Mutex
	token string
}

// NewTransit returns a Transit for the transit engine mounted at mount on the Vault at addr.
func NewTransit(addr, mount string, login Login, client *http.Client) *Transit {
	return &Transit{addr: strings.TrimRight(addr, "/"), path: "/v1/" + mount, login: login, client: client}
}

// Seal encrypts plaintext and returns Vault's ciphertext, "vault:v1:…".
func (t *Transit) Seal(ctx context.Context, key string, plaintext []byte) (string, error) {
	var out struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	in := map[string]string{"plaintext": base64.StdEncoding.EncodeToString(plaintext)}
	if err := t.do(ctx, "/encrypt/"+key, in, &out); err != nil {
		return "", err
	}
	return out.Data.Ciphertext, nil
}

// Open decrypts a ciphertext Seal returned.
func (t *Transit) Open(ctx context.Context, key, ciphertext string) ([]byte, error) {
	var out struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if err := t.do(ctx, "/decrypt/"+key, map[string]string{"ciphertext": ciphertext}, &out); err != nil {
		var status statusError
		if errors.As(err, &status) && status == http.StatusBadRequest {
			return nil, ErrSealed
		}
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Data.Plaintext)
}

// do calls the transit engine, signing in first and once more if the token was refused.
func (t *Transit) do(ctx context.Context, op string, in map[string]string, out any) error {
	for attempt := 0; ; attempt++ {
		token, err := t.current(ctx, attempt > 0)
		if err != nil {
			return err
		}
		err = call(ctx, t.client, t.addr+t.path+op, token, in, out)
		var status statusError
		if attempt == 0 && errors.As(err, &status) && status == http.StatusForbidden {
			continue
		}
		return err
	}
}

func (t *Transit) current(ctx context.Context, renew bool) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token != "" && !renew {
		return t.token, nil
	}
	token, err := t.login(ctx, t.client, t.addr)
	if err != nil {
		return "", err
	}
	t.token = token
	return token, nil
}

// statusError is an answer Vault gave with a status other than 200.
type statusError int

func (s statusError) Error() string { return fmt.Sprintf("vault: answered %d", int(s)) }

func call(ctx context.Context, c *http.Client, url, token string, in map[string]string, out any) error {
	body, _ := json.Marshal(in) // a map of strings always marshals
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
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

// Key seals and opens with one named transit key.
type Key struct {
	transit *Transit
	name    string
}

// Key returns the transit key called name.
func (t *Transit) Key(name string) Key { return Key{transit: t, name: name} }

// Seal encrypts plaintext under the key.
func (k Key) Seal(ctx context.Context, plaintext []byte) (string, error) {
	return k.transit.Seal(ctx, k.name, plaintext)
}

// Open decrypts a ciphertext the key sealed.
func (k Key) Open(ctx context.Context, ciphertext string) ([]byte, error) {
	return k.transit.Open(ctx, k.name, ciphertext)
}
