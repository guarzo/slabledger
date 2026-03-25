package sqlite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// RunMigrations runs database migrations from the specified path or embedded migrations.
// If migrationsPath is empty, embedded migrations are used.
// If migrationsPath is provided, it must exist and contain valid migration files.
func RunMigrations(db *DB, migrationsPath string) error {
	ctx := context.Background()

	// Determine source type and validate
	useEmbedded := migrationsPath == ""
	var sourceURL string
	var resolvedPath string

	if useEmbedded {
		db.logger.Info(ctx, "running database migrations from embedded source")
	} else {
		// Resolve to absolute path for clarity in logs
		absPath, err := filepath.Abs(migrationsPath)
		if err != nil {
			return apperrors.StorageError(fmt.Sprintf("resolve migrations path %q", migrationsPath), err)
		}
		resolvedPath = absPath

		// Validate migrations directory exists
		info, err := os.Stat(resolvedPath)
		if err != nil {
			if os.IsNotExist(err) {
				return apperrors.StorageError("migrations directory does not exist: "+resolvedPath, err)
			}
			return apperrors.StorageError("check migrations directory "+resolvedPath, err)
		}
		if !info.IsDir() {
			return apperrors.StorageError("migrations path is not a directory: "+resolvedPath, nil)
		}

		sourceURL = fmt.Sprintf("file://%s", resolvedPath)
		db.logger.Info(ctx, "running database migrations from file system",
			observability.String("path", resolvedPath))
	}

	driver, err := sqlite3.WithInstance(db.DB, &sqlite3.Config{})
	if err != nil {
		return apperrors.StorageError("create migration driver", err)
	}

	var m *migrate.Migrate
	if useEmbedded {
		// Use embedded migrations
		iofsDriver, err := iofs.New(MigrationsFS, "migrations")
		if err != nil {
			return apperrors.StorageError("create embedded migrations source", err)
		}

		m, err = migrate.NewWithInstance("iofs", iofsDriver, "sqlite3", driver)
		if err != nil {
			return apperrors.StorageError("create migration instance with embedded source", err)
		}
	} else {
		// Use file system migrations
		m, err = migrate.NewWithDatabaseInstance(sourceURL, "sqlite3", driver)
		if err != nil {
			return apperrors.StorageError("create migration instance with file source "+resolvedPath, err)
		}
	}
	// Don't close the migrate instance as it closes the underlying DB connection
	// The caller owns the DB and will close it when appropriate

	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return apperrors.StorageError("get current migration version", err)
	}

	db.logger.Info(ctx, "current migration version",
		observability.Int("version", int(version)),
		observability.Bool("dirty", dirty))

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		if useEmbedded {
			return apperrors.StorageError("run embedded migrations", err)
		}
		return apperrors.StorageError("run migrations from "+resolvedPath, err)
	}

	newVersion, _, versionErr := m.Version()
	source := "embedded"
	if !useEmbedded {
		source = resolvedPath
	}
	if versionErr != nil && versionErr != migrate.ErrNilVersion {
		db.logger.Warn(ctx, "failed to read migration version after Up",
			observability.String("error", versionErr.Error()),
			observability.String("source", source))
	} else {
		db.logger.Info(ctx, "migrations completed",
			observability.Int("version", int(newVersion)),
			observability.String("source", source))
	}
	return nil
}
