package sqlite

import (
	"embed"
)

// MigrationsFS contains the embedded SQL migrations.
// This allows the binary to be run from any directory without
// needing access to the filesystem migrations.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
