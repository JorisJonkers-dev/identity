package passwords_test

import (
	"errors"
	"strings"
	"testing"

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
