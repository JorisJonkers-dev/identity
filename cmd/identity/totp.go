//go:build !dev

package main

import usersapp "github.com/JorisJonkers-dev/identity/internal/users/app"

// totpMatch is nil in a release build: nothing replaces the RFC 6238 check.
func totpMatch() usersapp.TOTPMatch { return nil }
