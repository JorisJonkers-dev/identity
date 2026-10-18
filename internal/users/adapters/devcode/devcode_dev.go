//go:build dev

// Package devcode is the fixed Second Factor code of a local run: built only with the dev tag, which
// only `task dev` passes. A release build has none (devcode.go).
package devcode

import (
	"time"

	usersapp "github.com/JorisJonkers-dev/identity/internal/users/app"
	usersdomain "github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// Code is every authenticator app's code in a local run, so the Admin pages need no phone.
const Code = "000000"

// TOTPMatch takes Code as any authenticator's code, at a step after the last one used so it
// works every time, and checks every other code as a release build does.
func TOTPMatch() usersapp.TOTPMatch { return match }

func match(secret, code string, now time.Time, after int64) (int64, bool) {
	if code == Code {
		return max(now.Unix()/30, after+1), true
	}
	return usersdomain.MatchTOTP(secret, code, now, after)
}
