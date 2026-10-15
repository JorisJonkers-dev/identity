// Package db embeds the Liquibase changelog, so tests can hand it to liquibase-runner.
package db

import "embed"

// Changelog holds changelog/, the tree the migration image copies to /liquibase/changelog.
//
//go:embed changelog
var Changelog embed.FS
