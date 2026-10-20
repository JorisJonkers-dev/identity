package domain_test

import (
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/legacy/domain"
)

func TestAnAccountIsNamedByItsFirstAndLastName(t *testing.T) {
	a := domain.Account{Username: "ada", FirstName: "Ada", LastName: "Lovelace"}
	if got := a.FullName(); got != "Ada Lovelace" {
		t.Fatalf("FullName = %q", got)
	}
}

func TestAnAccountWithOnlyAFirstNameIsNamedByIt(t *testing.T) {
	a := domain.Account{Username: "ada", FirstName: "Ada"}
	if got := a.FullName(); got != "Ada" {
		t.Fatalf("FullName = %q", got)
	}
}

func TestAnAccountWithoutANameIsNamedByItsUsername(t *testing.T) {
	a := domain.Account{Username: "ada"}
	if got := a.FullName(); got != "ada" {
		t.Fatalf("FullName = %q", got)
	}
}

func TestAServiceBecomesTheClientNamedLikeIt(t *testing.T) {
	if got := domain.ClientFor("MEMORY_API", nil); got != "memory-api" {
		t.Fatalf("ClientFor = %q", got)
	}
}

func TestAServiceBecomesTheClientItIsMappedTo(t *testing.T) {
	if got := domain.ClientFor("MAIL", map[string]string{"MAIL": "stalwart"}); got != "stalwart" {
		t.Fatalf("ClientFor = %q", got)
	}
}

func TestTheRetiredKnowledgeAPIIsRetired(t *testing.T) {
	if !domain.Retired("KNOWLEDGE_API") {
		t.Fatal("KNOWLEDGE_API is not retired")
	}
}

func TestALiveServiceIsNotRetired(t *testing.T) {
	if domain.Retired("MEMORY_API") {
		t.Fatal("MEMORY_API is retired")
	}
}
