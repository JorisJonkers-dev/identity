package domain_test

import (
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

func TestOnlyAPKCEClientIsPublic(t *testing.T) {
	if !(domain.Client{Credential: domain.PKCE}).Public() || (domain.Client{Credential: domain.Secret}).Public() || (domain.Client{}).Public() {
		t.Fatal("Public reads the credential wrong")
	}
}
