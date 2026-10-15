package domain_test

import (
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

func TestAClientWithoutASecretIsPublic(t *testing.T) {
	if !(domain.Client{}).Public() || (domain.Client{SecretHash: []byte{1}}).Public() {
		t.Fatal("Public reads the secret wrong")
	}
}
