package vault

import (
	"context"
	"errors"
	"net/http"
)

// ErrNoSecret is a KV path that holds no secret.
var ErrNoSecret = errors.New("vault: no secret at that path")

// KV reads and writes secrets in Vault's KV version 2 engine, each by its API path below /v1/,
// such as "secret/data/monitoring/grafana/client".
type KV struct{ vault *Vault }

// KV is the KV version 2 engine.
func (v *Vault) KV() *KV { return &KV{vault: v} }

// Read is the latest data at path; ErrNoSecret when there is none.
func (k *KV) Read(ctx context.Context, path string) (map[string]string, error) {
	var out struct {
		Data struct {
			Data map[string]string `json:"data"`
		} `json:"data"`
	}
	err := k.vault.do(ctx, http.MethodGet, "/v1/"+path, nil, &out)
	var status statusError
	if errors.As(err, &status) && status == http.StatusNotFound {
		return nil, ErrNoSecret
	}
	return out.Data.Data, err
}

// Create writes data at path only when nothing is there yet; Vault refuses it otherwise.
func (k *KV) Create(ctx context.Context, path string, data map[string]string) error {
	in := map[string]any{"data": data, "options": map[string]int{"cas": 0}}
	return k.vault.do(ctx, http.MethodPost, "/v1/"+path, in, &struct{}{})
}

// Write makes data the latest version at path.
func (k *KV) Write(ctx context.Context, path string, data map[string]string) error {
	return k.vault.do(ctx, http.MethodPost, "/v1/"+path, map[string]any{"data": data}, &struct{}{})
}
