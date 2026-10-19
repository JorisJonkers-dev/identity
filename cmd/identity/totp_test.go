//go:build !dev

package main

import "testing"

func TestAReleaseBuildTakesNoFixedSecondFactorCode(t *testing.T) {
	if totpMatch() != nil {
		t.Fatal("a build without the dev tag replaces the authenticator code check")
	}
}
