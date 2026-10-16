package itest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"testing"

	"github.com/descope/virtualwebauthn"
)

// PasskeyCredential is a virtual authenticator's EC2 credential that a relying party accepts.
// virtualwebauthn writes the key's coordinates without their leading zero bytes, so about one
// key in 128 comes out short and is refused; this one keeps drawing until both are full length.
func PasskeyCredential(t *testing.T) virtualwebauthn.Credential {
	t.Helper()
	for {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if len(key.X.Bytes()) < 32 || len(key.Y.Bytes()) < 32 { //nolint:staticcheck // the short form is the bug
			continue
		}
		data, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return virtualwebauthn.NewCredentialWithImportedKey(virtualwebauthn.KeyTypeEC2, data)
	}
}
