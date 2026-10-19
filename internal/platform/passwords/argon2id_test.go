package passwords_test

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/JorisJonkers-dev/identity/internal/platform/passwords"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

const secret = "correct horse battery"

func password(t *testing.T) domain.Password {
	t.Helper()
	p, err := domain.NewPassword(secret)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHashThenVerify(t *testing.T) {
	h := passwords.Argon2id{}
	hash, err := h.Hash(password(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=47104,t=1,p=1$") {
		t.Fatalf("hash %q does not record OWASP's parameters", hash)
	}
	if ok, err := h.Verify(hash, "correct horse battery"); !ok || err != nil {
		t.Fatalf("the right password: %v, %v", ok, err)
	}
	if ok, err := h.Verify(hash, "wrong horse battery!"); ok || err != nil {
		t.Fatalf("a wrong password: %v, %v", ok, err)
	}
	again, _ := h.Hash(password(t))
	if again == hash {
		t.Fatal("two hashes of one password share a salt")
	}
}

func TestVerifyReadsTheParametersTheHashRecords(t *testing.T) {
	// Hashed with m=8192,t=2,p=1, so a hash from older, weaker settings still verifies.
	const old = "$argon2id$v=19$m=8192,t=2,p=1$c2FsdHNhbHRzYWx0c2FsdA$FdL8IRWh6nXNKrRyTIByxITX3JWvjmHo2f6R8bjmFjc"
	ok, err := passwords.Argon2id{}.Verify(old, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("a hash with its own parameters did not verify")
	}
}

func TestVerifyRefusesAMalformedHash(t *testing.T) {
	for _, bad := range []string{
		"",
		"$bcrypt$x",
		"$argon2i$v=19$m=8,t=1,p=1$c2FsdA$aGFzaGFzaGFzaGFzaA",
		"$argon2id$v=18$m=1,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=x,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=1,t=1,p=1$!!$aGFzaA",
		"$argon2id$v=19$m=1,t=1,p=1$c2FsdA$!!",
		"$argon2id$vx$m=1,t=1,p=1$c2FsdA$aGFzaA",
		// Costs above what Hash uses would let one stored row make every check slow or huge.
		"$argon2id$v=19$m=47105,t=1,p=1$c2FsdA$aGFzaGFzaGFzaGFzaA",
		"$argon2id$v=19$m=23553,t=2,p=1$c2FsdA$aGFzaGFzaGFzaGFzaA",
		"$argon2id$v=19$m=8,t=1,p=0$c2FsdA$aGFzaGFzaGFzaGFzaA",
		"$argon2id$v=19$m=8,t=1,p=2$c2FsdA$aGFzaGFzaGFzaGFzaA",
		"$argon2id$v=19$m=8,t=0,p=1$c2FsdA$aGFzaGFzaGFzaGFzaA",
		"$argon2id$v=19$m=8,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=8,t=1,p=1$c2FsdA$" + strings.Repeat("aGFz", 30),
	} {
		if ok, err := (passwords.Argon2id{}).Verify(bad, "anything at all"); ok || !errors.Is(err, passwords.ErrMalformedHash) {
			t.Errorf("Verify(%q) = %v, %v; want ErrMalformedHash", bad, ok, err)
		}
	}
}

func TestHashFailsWithoutEntropy(t *testing.T) {
	h := passwords.Argon2id{Random: failing{}}
	if _, err := h.Hash(password(t)); err == nil {
		t.Fatal("hashed without a salt")
	}
}

type failing struct{}

func (failing) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

// legacyHash is "correct horse battery" under bcrypt at cost 4, as auth-api stored passwords.
func legacyHash(t *testing.T) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}

func TestVerifyAcceptsTheRightPasswordForAnImportedBcryptHash(t *testing.T) {
	for _, version := range []string{"$2a$", "$2b$", "$2y$"} {
		t.Run(version, func(t *testing.T) {
			hash := version + strings.TrimPrefix(legacyHash(t), "$2a$")
			ok, err := passwords.Argon2id{}.Verify(hash, secret)
			if !ok || err != nil {
				t.Fatalf("Verify = %v, %v", ok, err)
			}
		})
	}
}

func TestVerifyRefusesAWrongPasswordForAnImportedBcryptHash(t *testing.T) {
	ok, err := passwords.Argon2id{}.Verify(legacyHash(t), "wrong horse battery")
	if ok || err != nil {
		t.Fatalf("Verify = %v, %v", ok, err)
	}
}

func TestVerifyRefusesAMalformedBcryptHash(t *testing.T) {
	for _, bad := range []string{
		"$2a$x",
		// A cost above 12 would let one stored row make every check slow.
		"$2a$13$" + strings.Repeat("a", 53),
	} {
		if ok, err := (passwords.Argon2id{}).Verify(bad, secret); ok || !errors.Is(err, passwords.ErrMalformedHash) {
			t.Errorf("Verify(%q) = %v, %v; want ErrMalformedHash", bad, ok, err)
		}
	}
}

func TestABcryptHashIsOutdatedAndAnArgon2idHashIsNot(t *testing.T) {
	h := passwords.Argon2id{}
	current, err := h.Hash(password(t))
	if err != nil {
		t.Fatal(err)
	}
	if !h.Outdated(legacyHash(t)) || h.Outdated(current) {
		t.Fatalf("Outdated(bcrypt) = %v, Outdated(argon2id) = %v", h.Outdated(legacyHash(t)), h.Outdated(current))
	}
}
