package postgres

import (
	"embed"
)

// MigrationsFS contains the embedded SQL migrations.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
