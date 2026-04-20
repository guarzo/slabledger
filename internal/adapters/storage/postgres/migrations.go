package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// RunMigrations applies migrations from the specified path (or embedded source
// if migrationsPath is empty) against the given *DB.
func RunMigrations(db *DB, migrationsPath string) error {
	ctx := context.Background()

	useEmbedded := migrationsPath == ""
	var sourceURL, resolvedPath string

	if useEmbedded {
		db.logger.Info(ctx, "running database migrations from embedded source")
	} else {
		absPath, err := filepath.Abs(migrationsPath)
		if err != nil {
			return apperrors.StorageError(fmt.Sprintf("resolve migrations path %q", migrationsPath), err)
		}
		resolvedPath = absPath

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

	driver, err := migratepgx.WithInstance(db.DB, &migratepgx.Config{})
	if err != nil {
		return apperrors.StorageError("create migration driver", err)
	}

	var m *migrate.Migrate
	if useEmbedded {
		iofsDriver, err := iofs.New(MigrationsFS, "migrations")
		if err != nil {
			return apperrors.StorageError("create embedded migrations source", err)
		}
		m, err = migrate.NewWithInstance("iofs", iofsDriver, "pgx5", driver)
		if err != nil {
			return apperrors.StorageError("create migration instance with embedded source", err)
		}
	} else {
		m, err = migrate.NewWithDatabaseInstance(sourceURL, "pgx5", driver)
		if err != nil {
			return apperrors.StorageError("create migration instance with file source "+resolvedPath, err)
		}
	}
	// The caller owns the DB and will close it; don't let migrate close the conn.

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
