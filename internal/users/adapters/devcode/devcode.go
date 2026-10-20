// Package devcode is the fixed two-factor code of a local run. Only a dev build wires it in
// (cmd/identity/totp_dev.go); a release build checks every code by RFC 6238 alone.
package devcode

import (
	"time"

	usersdomain "github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// Code is every authenticator app's code in a local run, so the Admin pages need no phone.
const Code = "000000"

// Match takes Code as any authenticator's code, at a step after the last one used so it works
// every time, and checks every other code as a release build does.
func Match(secret, code string, now time.Time, after int64) (int64, bool) {
	if code == Code {
		return max(now.Unix()/30, after+1), true
	}
	return usersdomain.MatchTOTP(secret, code, now, after)
}
