// Package passwords hashes and verifies passwords with argon2id, in the PHC string format, so each
// hash records the parameters it was made with and a stronger setting never strands an old hash.
package passwords

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// ErrMalformedHash is a stored hash this package cannot read.
var ErrMalformedHash = errors.New("malformed password hash")

// OWASP's minimum for argon2id: 46 MiB, one pass, one lane.
const (
	memoryKiB  = 47104
	passes     = 1
	lanes      = 1
	saltBytes  = 16
	hashBytes  = 32
	phcVersion = 19
)

// Argon2id is the app.Hasher. Random defaults to crypto/rand.
type Argon2id struct {
	Random io.Reader
}

// Hash returns a PHC string for p, under a fresh random salt.
func (a Argon2id) Hash(p domain.Password) (string, error) {
	random := a.Random
	if random == nil {
		random = rand.Reader
	}
	salt := make([]byte, saltBytes)
	if _, err := io.ReadFull(random, salt); err != nil {
		return "", fmt.Errorf("read a salt: %w", err)
	}
	key := argon2.IDKey([]byte(p.String()), salt, passes, memoryKiB, lanes, hashBytes)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", phcVersion, memoryKiB, passes, lanes,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify reports whether password matches hash, using the parameters hash records.
func (Argon2id) Verify(hash, password string) (bool, error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != phcVersion {
		return false, ErrMalformedHash
	}
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, ErrMalformedHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrMalformedHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ErrMalformedHash
	}
	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want))) //nolint:gosec // len of a decoded hash, far below 2^32.
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
