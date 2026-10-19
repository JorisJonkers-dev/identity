//go:build dev

package main

import "testing"

func TestADevBuildTakesTheFixedSecondFactorCode(t *testing.T) {
	if totpMatch() == nil {
		t.Fatal("a dev build checks authenticator codes as a release build does")
	}
}
