package oidc

import (
	"context"
	"errors"
	"time"

	"github.com/zitadel/oidc/v3/pkg/op"
)

// resolveTimeout bounds a Personal Token's lookup, which the Crypto interface gives no context for
// (docs/adr/0017).
const resolveTimeout = 2 * time.Second

var errNotPersonalToken = errors.New("not a Personal Token")

// resolver turns a Personal Token's secret into the token id and subject a check needs.
type resolver func(ctx context.Context, secret string) (tokenID, subject string, ok bool)

// tokenDecrypter is the Provider's first decrypter: it answers a Personal Token by looking its hash
// up, and leaves every other token to zitadel's own.
type tokenDecrypter struct{ resolve resolver }

func (d tokenDecrypter) Decrypt(token string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), resolveTimeout)
	defer cancel()
	tokenID, subject, ok := d.resolve(ctx, token)
	if !ok {
		return "", errNotPersonalToken
	}
	return tokenID + ":" + subject, nil
}

// newCrypto seals with AES-256-GCM and looks Personal Tokens up first. It has no AES-CFB fallback:
// that cipher is unauthenticated, so it opens random tokens.
func newCrypto(key [32]byte, resolve resolver) op.Crypto {
	sealed := op.NewAES256GCMCrypto(key, "")
	apps := appSealer{sealed: sealed}
	return op.NewCompositeCrypto(apps, []op.Decrypter{tokenDecrypter{resolve: resolve}, apps, sealed})
}
