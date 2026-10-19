//go:build dev

package main

import (
	"github.com/JorisJonkers-dev/identity/internal/users/adapters/devcode"
	usersapp "github.com/JorisJonkers-dev/identity/internal/users/app"
)

// totpMatch takes the fixed code of a local run; only `task dev` builds with the dev tag.
func totpMatch() usersapp.TOTPMatch { return devcode.Match }
