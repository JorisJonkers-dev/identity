//go:build !dev

// Package devcode is the fixed Second Factor code of a local run. A release build, which this file
// is, has none: authenticator codes are checked by RFC 6238 alone.
package devcode

import usersapp "github.com/JorisJonkers-dev/identity/internal/users/app"

// TOTPMatch is nil: nothing replaces the RFC 6238 check. A local run's dev build takes a fixed code
// instead (devcode_dev.go).
func TOTPMatch() usersapp.TOTPMatch { return nil }
