package passwords

import (
	"errors"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

func TestAHashWaitsForAFreeSlot(t *testing.T) {
	a := NewArgon2id(1, time.Minute)
	a.slots <- struct{}{}
	p, _ := domain.NewPassword("correct horse battery")
	done := make(chan string, 1)
	go func() {
		hash, _ := a.Hash(p)
		done <- hash
	}()
	select {
	case <-done:
		t.Fatal("a hash ran while every slot was taken")
	case <-time.After(300 * time.Millisecond):
	}
	<-a.slots
	if ok, err := a.Verify(<-done, "correct horse battery"); !ok || err != nil {
		t.Fatalf("verify through the bound: %v, %v", ok, err)
	}
	if len(a.slots) != 0 {
		t.Fatalf("%d hashes still hold a slot", len(a.slots))
	}
}

func TestAHashGivesUpWhenNoSlotFreesInTime(t *testing.T) {
	a := NewArgon2id(1, 20*time.Millisecond)
	a.slots <- struct{}{}
	p, _ := domain.NewPassword("correct horse battery")
	if _, err := a.Hash(p); !errors.Is(err, app.ErrBusy) {
		t.Fatalf("Hash = %v, want ErrBusy", err)
	}
	const h = "$argon2id$v=19$m=8,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$FdL8IRWh6nXNKrRyTIByxITX3JWvjmHo2f6R8bjmFjc"
	if _, err := a.Verify(h, "x"); !errors.Is(err, app.ErrBusy) {
		t.Fatalf("Verify = %v, want ErrBusy", err)
	}
}

func TestABcryptCheckGivesUpWhenNoSlotFreesInTime(t *testing.T) {
	a := NewArgon2id(1, 20*time.Millisecond)
	a.slots <- struct{}{}
	const h = "$2a$04$abcdefghijklmnopqrstuuJp1mFhvM2tVYV3Ut7Yy6V9oKz7wq3bW"
	if _, err := a.Verify(h, "x"); !errors.Is(err, app.ErrBusy) {
		t.Fatalf("Verify = %v, want ErrBusy", err)
	}
}
