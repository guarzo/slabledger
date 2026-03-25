package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sql.DB
	logger observability.Logger
}

func Open(path string, logger observability.Logger) (*DB, error) {
	start := time.Now()
	ctx := context.Background()

	logger.Info(ctx, "opening database",
		observability.String("path", path))

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Error(ctx, "failed to create database directory",
			observability.Err(err),
			observability.String("dir", dir))
		return nil, apperrors.StorageError("create database directory", err)
	}

	// Verify directory is writable
	testFile := filepath.Join(dir, ".write_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		logger.Error(ctx, "database directory not writable",
			observability.Err(err),
			observability.String("dir", dir))
		return nil, apperrors.StorageError("verify database directory writable", err)
	}
	if err := os.Remove(testFile); err != nil {
		logger.Warn(ctx, "failed to remove write test file",
			observability.Err(err),
			observability.String("file", testFile))
	}

	// Open with WAL mode and foreign keys
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=ON&_busy_timeout=5000", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		logger.Error(ctx, "failed to open database",
			observability.Err(err),
			observability.String("path", path))
		return nil, apperrors.StorageError("open database", err)
	}

	// Configure connection pool for SQLite (single writer)
	db.SetMaxOpenConns(1) // SQLite single-writer constraint
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	// Verify connection
	if err := db.Ping(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			logger.Warn(ctx, "failed to close database after ping failure", observability.Err(closeErr))
		}
		return nil, apperrors.StorageError("ping database", err)
	}

	logger.Info(ctx, "database opened successfully",
		observability.Float64("duration_ms", float64(time.Since(start).Milliseconds())))

	return &DB{
		DB:     db,
		logger: logger,
	}, nil
}

func (db *DB) Close() error {
	db.logger.Info(context.Background(), "closing database")
	return db.DB.Close()
}
