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
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/JorisJonkers-dev/identity/internal/users/app"
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

	minHashBytes = 16
	maxHashBytes = 64
)

// Argon2id is the app.Hasher. Random defaults to crypto/rand. The zero value runs any number of
// hashes at once; NewArgon2id bounds them.
type Argon2id struct {
	Random io.Reader
	slots  chan struct{}
	wait   time.Duration
}

// NewArgon2id returns an Argon2id running at most n hashes at once. A hash waits up to wait for a
// slot and then fails with app.ErrBusy, so a burst of sign-ins neither asks for n times 46 MiB nor
// queues without end.
func NewArgon2id(n int, wait time.Duration) Argon2id {
	return Argon2id{slots: make(chan struct{}, n), wait: wait}
}

func (a Argon2id) key(password, salt []byte, passes, memory uint32, threads uint8, size uint32) ([]byte, error) {
	if a.slots != nil {
		timer := time.NewTimer(a.wait)
		defer timer.Stop()
		select {
		case a.slots <- struct{}{}:
			defer func() { <-a.slots }()
		case <-timer.C:
			return nil, app.ErrBusy
		}
	}
	return argon2.IDKey(password, salt, passes, memory, threads, size), nil
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
	key, err := a.key([]byte(p.String()), salt, passes, memoryKiB, lanes, hashBytes)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", phcVersion, memoryKiB, passes, lanes,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify reports whether password matches hash, using the parameters hash records.
func (a Argon2id) Verify(hash, password string) (bool, error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != phcVersion {
		return false, ErrMalformedHash
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil ||
		!affordable(memory, iterations, threads) {
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
	if len(want) < minHashBytes || len(want) > maxHashBytes {
		return false, ErrMalformedHash
	}
	got, err := a.key([]byte(password), salt, iterations, memory, threads, uint32(len(want))) //nolint:gosec // at most maxHashBytes.
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// affordable reports whether a hash's recorded costs are no dearer than Hash's own: no more memory,
// and no more memory times passes, so OWASP's lower-memory, more-pass settings still verify.
func affordable(memory, iterations uint32, threads uint8) bool {
	return memory <= memoryKiB && iterations >= 1 && uint64(memory)*uint64(iterations) <= memoryKiB*passes &&
		threads >= 1 && threads <= lanes
}
