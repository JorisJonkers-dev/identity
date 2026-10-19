package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JorisJonkers-dev/identity/internal/legacy/adapters/persistence"
	"github.com/JorisJonkers-dev/identity/internal/legacy/app"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
)

const importUsage = "identity import auth-api [--dry-run] [--client SERVICE=client-id]..."

// importOptions are the import's flags: whether to change nothing, and which Client a service's
// grant becomes when it is not the one named like the service.
type importOptions struct {
	dryRun  bool
	clients map[string]string
}

// importCommand runs `identity import auth-api`, the one-off Job that moves Legacy Auth's people
// into identity (docs/adr/0006). It reads LEGACY_DATABASE_URL and writes DATABASE_URL, sealing
// authenticator secrets through Vault as the server does. Running it again changes nothing.
func importCommand(ctx context.Context, getenv func(string) string, args []string, stdout, stderr io.Writer) int {
	opts, ok := parseImport(args)
	if !ok {
		_, _ = fmt.Fprintln(stderr, "usage: "+importUsage)
		return 2
	}
	if err := importAuthAPI(ctx, getenv, opts, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, "identity:", err)
		return 1
	}
	return 0
}

func parseImport(args []string) (importOptions, bool) {
	opts := importOptions{clients: map[string]string{}}
	if len(args) == 0 || args[0] != "auth-api" {
		return opts, false
	}
	for rest := args[1:]; len(rest) > 0; rest = rest[1:] {
		switch rest[0] {
		case "--dry-run":
			opts.dryRun = true
		case "--client":
			if len(rest) < 2 {
				return opts, false
			}
			service, client, found := strings.Cut(rest[1], "=")
			if !found || service == "" || client == "" {
				return opts, false
			}
			opts.clients[service] = client
			rest = rest[1:]
		default:
			return opts, false
		}
	}
	return opts, true
}

func importAuthAPI(ctx context.Context, getenv func(string) string, opts importOptions, stdout io.Writer) error {
	var missing []string
	for _, name := range []string{"DATABASE_URL", "LEGACY_DATABASE_URL", "VAULT_ADDR"} {
		if getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return refusal(strings.Join(missing, ", ") + " not set. Set them and run again.")
	}
	store, err := pg.Open(ctx, getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer store.Close()
	legacy, err := pgxpool.New(ctx, getenv("LEGACY_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer legacy.Close()
	v := readVault(getenv, getenv("VAULT_ADDR"))
	importer := app.New(persistence.NewAuthAPI(legacy), persistence.NewIdentity(store.Queries(), store, time.Now),
		v.vault().Transit(v.transitMount).Key(v.totpKey), opts.clients)
	outcomes, err := importer.Run(ctx, opts.dryRun)
	report(stdout, outcomes, opts.dryRun)
	return err
}

// report prints a line per account and a total, in the dry run's future tense or the run's past.
func report(w io.Writer, outcomes []app.Outcome, dryRun bool) {
	verb := "imported"
	if dryRun {
		verb = "would import"
	}
	imported := 0
	for _, o := range outcomes {
		if o.Skipped != "" {
			_, _ = fmt.Fprintf(w, "skip %s: %s\n", o.Email, o.Skipped)
			continue
		}
		imported++
		var what []string
		if o.Admin {
			what = append(what, "Admin")
		}
		if o.Authenticator {
			what = append(what, "authenticator app")
		}
		what = append(what, o.Clients...)
		line := verb + " " + o.Email
		if len(what) > 0 {
			line += ": " + strings.Join(what, ", ")
		}
		_, _ = fmt.Fprintln(w, line)
	}
	skipped := len(outcomes) - imported
	if dryRun {
		_, _ = fmt.Fprintf(w, "Would import %d and skip %d. Nothing was changed.\n", imported, skipped)
		return
	}
	_, _ = fmt.Fprintf(w, "Imported %d and skipped %d.\n", imported, skipped)
}
