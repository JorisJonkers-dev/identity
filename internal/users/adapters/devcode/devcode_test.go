//go:build !dev

package devcode_test

import (
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/users/adapters/devcode"
)

func TestAReleaseBuildTakesNoFixedSecondFactorCode(t *testing.T) {
	if devcode.TOTPMatch() != nil {
		t.Fatal("a build without the dev tag replaces the authenticator code check")
	}
}
