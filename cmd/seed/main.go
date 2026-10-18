//go:build dev

// Command seed fills a local identity with the people, Groups, Assignments, Personal Tokens and
// Audit Events the design canvas draws, so each page can be compared with its frame (identity#69,
// identity#70). Only `task dev` runs it, built with the dev tag; a second run changes nothing.
//
// Environment: DATABASE_URL, VAULT_ADDR, VAULT_TOKEN and CLIENTS_FILE, as `task dev` sets them.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := run(context.Background(), os.Getenv, os.Stdout, logger); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}
