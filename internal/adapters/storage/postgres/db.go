package postgres

import (
	"context"
	"database/sql"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

type DB struct {
	*sql.DB
	logger observability.Logger
}

// Open parses a Postgres connection URL and returns a verified *DB.
//
// QueryExecModeExec is used so that the driver does not rely on session-level
// prepared statements. That keeps us compatible with both local Postgres and
// Supabase's PgBouncer transaction pooling (port 6543) without extra tuning.
func Open(url string, logger observability.Logger) (*DB, error) {
	start := time.Now()
	ctx := context.Background()

	logger.Info(ctx, "opening database", observability.String("driver", "pgx/v5"))

	connConfig, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, apperrors.StorageError("parse database URL", err)
	}
	connConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	name := stdlib.RegisterConnConfig(connConfig)
	db, err := sql.Open("pgx", name)
	if err != nil {
		return nil, apperrors.StorageError("open database", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	pingCfg := retryConfig{
		MaxAttempts:  10,
		InitialDelay: time.Second,
		MaxDelay:     30 * time.Second,
	}
	pingErr := retryWithBackoff(ctx, pingCfg, func(ctx context.Context) error {
		if err := db.PingContext(ctx); err != nil {
			logger.Warn(ctx, "database ping failed, will retry",
				observability.String("error", err.Error()))
			return err
		}
		return nil
	})
	if pingErr != nil {
		if closeErr := db.Close(); closeErr != nil {
			logger.Warn(ctx, "failed to close database after ping failure", observability.Err(closeErr))
		}
		return nil, apperrors.StorageError("ping database after retries", pingErr)
	}

	logger.Info(ctx, "database opened successfully",
		observability.Float64("duration_ms", float64(time.Since(start).Milliseconds())))

	return &DB{DB: db, logger: logger}, nil
}

func (db *DB) Close() error {
	db.logger.Info(context.Background(), "closing database")
	return db.DB.Close()
}
