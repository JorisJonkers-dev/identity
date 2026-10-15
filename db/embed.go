// Package db embeds the SQL migrations, so the binary migrates its own database at startup.
package db

import "embed"

// Migrations holds the goose migrations, applied in file-name order.
//
//go:embed migrations/*.sql
var Migrations embed.FS
