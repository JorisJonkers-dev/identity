package vault

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
)

// ErrSealed is a ciphertext Vault refused to open.
var ErrSealed = errors.New("vault: the ciphertext cannot be opened")

// Transit seals and opens with one transit key.
type Transit struct {
	vault *Vault
	path  string
}

// Transit is the transit engine mounted at mount.
func (v *Vault) Transit(mount string) *Transit { return &Transit{vault: v, path: "/v1/" + mount} }

// Seal encrypts plaintext and returns Vault's ciphertext, "vault:v1:…".
func (t *Transit) Seal(ctx context.Context, key string, plaintext []byte) (string, error) {
	var out struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	in := map[string]string{"plaintext": base64.StdEncoding.EncodeToString(plaintext)}
	if err := t.vault.do(ctx, http.MethodPost, t.path+"/encrypt/"+key, in, &out); err != nil {
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
	if err := t.vault.do(ctx, http.MethodPost, t.path+"/decrypt/"+key, map[string]string{"ciphertext": ciphertext}, &out); err != nil {
		var status statusError
		if errors.As(err, &status) && status == http.StatusBadRequest {
			return nil, ErrSealed
		}
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Data.Plaintext)
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
