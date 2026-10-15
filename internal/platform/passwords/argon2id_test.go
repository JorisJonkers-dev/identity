package passwords_test

import (
	"errors"
	"strings"
	"testing"
	"time"

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
		"$argon2id$v=18$m=1,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=x,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=1,t=1,p=1$!!$aGFzaA",
		"$argon2id$v=19$m=1,t=1,p=1$c2FsdA$!!",
		"$argon2id$vx$m=1,t=1,p=1$c2FsdA$aGFzaA",
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

func TestNewArgon2idMakesAHashWaitForAFreeSlot(t *testing.T) {
	h := passwords.NewArgon2id(1)
	release := passwords.Hold(h)
	done := make(chan string, 1)
	go func() {
		hash, _ := h.Hash(password(t))
		done <- hash
	}()
	select {
	case <-done:
		t.Fatal("a hash ran while every slot was taken")
	case <-time.After(300 * time.Millisecond):
	}
	release()
	hash := <-done
	if ok, err := h.Verify(hash, "correct horse battery"); !ok || err != nil {
		t.Fatalf("verify through the bound: %v, %v", ok, err)
	}
	if got := h.InFlight(); got != 0 {
		t.Fatalf("%d hashes still hold a slot", got)
	}
}
