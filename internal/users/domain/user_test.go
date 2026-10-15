package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

func TestNewEmailNormalisesAndRefuses(t *testing.T) {
	got, err := domain.NewEmail("  Ada@Example.ORG ")
	if err != nil || got.String() != "ada@example.org" {
		t.Fatalf("NewEmail = %q, %v; want the trimmed lower-case address", got, err)
	}
	long := strings.Repeat("a", 250) + "@b.cd"
	for _, bad := range []string{"", "ada", "@example.org", "ada@", "ada@@example.org", "a da@example.org", "ada@example", long} {
		if _, err := domain.NewEmail(bad); !errors.Is(err, domain.ErrInvalidEmail) {
			t.Errorf("NewEmail(%q) err = %v, want ErrInvalidEmail", bad, err)
		}
	}
}

func TestNewNameTrimsAndBoundsByCharacters(t *testing.T) {
	got, err := domain.NewName("  Ada Lovelace ")
	if err != nil || got.String() != "Ada Lovelace" {
		t.Fatalf("NewName = %q, %v", got, err)
	}
	if _, err := domain.NewName(strings.Repeat("é", 100)); err != nil {
		t.Fatalf("100 characters refused: %v", err)
	}
	for _, bad := range []string{"", "   ", strings.Repeat("x", 101)} {
		if _, err := domain.NewName(bad); !errors.Is(err, domain.ErrInvalidName) {
			t.Errorf("NewName(%d chars) err = %v, want ErrInvalidName", len(bad), err)
		}
	}
}

func TestNewPasswordBoundsByCharacters(t *testing.T) {
	if _, err := domain.NewPassword(strings.Repeat("é", 12)); err != nil {
		t.Fatalf("12 characters refused: %v", err)
	}
	if _, err := domain.NewPassword(strings.Repeat("x", 128)); err != nil {
		t.Fatalf("128 characters refused: %v", err)
	}
	for _, bad := range []string{strings.Repeat("x", 11), strings.Repeat("x", 129)} {
		if _, err := domain.NewPassword(bad); !errors.Is(err, domain.ErrWeakPassword) {
			t.Errorf("NewPassword(%d chars) err = %v, want ErrWeakPassword", len(bad), err)
		}
	}
	p, _ := domain.NewPassword("correct horse battery")
	if p.String() != "correct horse battery" {
		t.Fatalf("String() = %q", p.String())
	}
}

func TestUserIsConfirmedOnlyWithAConfirmationTime(t *testing.T) {
	var u domain.User
	if u.Confirmed() {
		t.Fatal("a User with no confirmation time reads as confirmed")
	}
	now := domain.User{}.CreatedAt
	u.ConfirmedAt = &now
	if !u.Confirmed() {
		t.Fatal("a User with a confirmation time reads as unconfirmed")
	}
}
