package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/JorisJonkers-dev/identity/internal/access/adapters/catalog"
	"github.com/JorisJonkers-dev/identity/internal/access/adapters/people"
	accesspersistence "github.com/JorisJonkers-dev/identity/internal/access/adapters/persistence"
	accessapp "github.com/JorisJonkers-dev/identity/internal/access/app"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
	"github.com/JorisJonkers-dev/identity/internal/users/adapters/persistence"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
)

const usage = "usage: identity admin promote <email> [--force]"

// command runs one admin command and returns the exit code: 0 done, 1 refused or failed, 2 misused.
// `identity admin promote <email>` makes the first Admin of a fresh install, and refuses when an
// Admin exists unless --force is given (docs/adr/0005). It needs only DATABASE_URL.
func command(ctx context.Context, getenv func(string) string, args []string, stdout, stderr io.Writer) int {
	force := slices.Contains(args, "--force")
	args = slices.DeleteFunc(slices.Clone(args), func(a string) bool { return a == "--force" })
	if len(args) != 3 || args[0] != "admin" || args[1] != "promote" {
		_, _ = fmt.Fprintln(stderr, usage)
		return 2
	}
	if err := promote(ctx, getenv("DATABASE_URL"), args[2], force); err != nil {
		_, _ = fmt.Fprintln(stderr, "identity:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "%s is now an Admin.\n", args[2])
	return 0
}

func promote(ctx context.Context, databaseURL, email string, force bool) error {
	if databaseURL == "" {
		return errors.New("DATABASE_URL is not set. Set it to the identity database URL and run again.")
	}
	store, err := pg.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	users := app.New(persistence.New(store.Queries()), nil, nil, nil, nil, "")
	u, err := users.UserByEmail(ctx, email)
	if errors.Is(err, app.ErrNotFound) {
		return fmt.Errorf("no User has the email %s. Have them sign up and confirm their email first.", email)
	}
	if err != nil {
		return err
	}
	access := accessapp.New(accesspersistence.New(store.Queries(), store), catalog.New(clients.NewIndex()), people.New(users), rand.Reader)
	err = access.Promote(ctx, u.ID, force)
	if errors.Is(err, accessapp.ErrAdminExists) {
		return errors.New("an Admin already exists. To add another, run again with --force.")
	}
	return err
}
