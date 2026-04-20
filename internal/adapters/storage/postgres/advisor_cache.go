package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ advisor.CacheStore = (*AdvisorCacheRepository)(nil)

// AdvisorCacheRepository implements advisor.CacheStore using Postgres.
type AdvisorCacheRepository struct {
	db     *sql.DB
	logger observability.Logger
}

// NewAdvisorCacheRepository creates a new advisor cache repository.
func NewAdvisorCacheRepository(db *sql.DB, logger observability.Logger) *AdvisorCacheRepository {
	return &AdvisorCacheRepository{db: db, logger: logger}
}

func (r *AdvisorCacheRepository) Get(ctx context.Context, analysisType advisor.AnalysisType) (*advisor.CachedAnalysis, error) {
	var (
		ca          advisor.CachedAnalysis
		startedAt   sql.NullString
		completedAt sql.NullString
		updatedAt   string
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT analysis_type, status, content, error_message, started_at, completed_at, updated_at
		 FROM advisor_cache WHERE analysis_type = $1`, string(analysisType),
	).Scan(&ca.AnalysisType, &ca.Status, &ca.Content, &ca.ErrorMessage, &startedAt, &completedAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if startedAt.Valid {
		if t, parseErr := parseFlexibleTimestamp(startedAt.String); parseErr != nil {
			r.logParseWarn(ctx, analysisType, "started_at", startedAt.String, parseErr)
		} else {
			ca.StartedAt = t
		}
	}
	if completedAt.Valid {
		if t, parseErr := parseFlexibleTimestamp(completedAt.String); parseErr != nil {
			r.logParseWarn(ctx, analysisType, "completed_at", completedAt.String, parseErr)
		} else {
			ca.CompletedAt = t
		}
	}
	if t, parseErr := parseFlexibleTimestamp(updatedAt); parseErr != nil {
		r.logParseWarn(ctx, analysisType, "updated_at", updatedAt, parseErr)
	} else {
		ca.UpdatedAt = t
	}
	return &ca, nil
}

func (r *AdvisorCacheRepository) AcquireRefresh(ctx context.Context, analysisType advisor.AnalysisType) (string, bool, error) {
	now := time.Now().Format(time.RFC3339)
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO advisor_cache (analysis_type, status, started_at, updated_at)
		VALUES ($1, 'running', $2, $3)
		ON CONFLICT(analysis_type) DO UPDATE SET
			status = 'running',
			content = '',
			error_message = '',
			completed_at = NULL,
			started_at = excluded.started_at,
			updated_at = excluded.updated_at
		WHERE advisor_cache.status != 'running'
	`, string(analysisType), now, now)
	if err != nil {
		return "", false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return "", false, err
	}
	if n > 0 {
		return now, true, nil
	}
	return "", false, nil
}

func (r *AdvisorCacheRepository) MarkRunning(ctx context.Context, analysisType advisor.AnalysisType) (string, error) {
	now := time.Now().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO advisor_cache (analysis_type, status, started_at, updated_at)
		VALUES ($1, 'running', $2, $3)
		ON CONFLICT(analysis_type) DO UPDATE SET
			status = 'running',
			content = '',
			error_message = '',
			completed_at = NULL,
			started_at = excluded.started_at,
			updated_at = excluded.updated_at
	`, string(analysisType), now, now)
	if err != nil {
		return "", err
	}
	return now, nil
}

func (r *AdvisorCacheRepository) ForceAcquireStale(ctx context.Context, analysisType advisor.AnalysisType, staleThreshold time.Duration) (string, bool, error) {
	now := time.Now()
	cutoff := now.Add(-staleThreshold).Format(time.RFC3339)
	nowStr := now.Format(time.RFC3339)
	result, err := r.db.ExecContext(ctx, `
		UPDATE advisor_cache
		SET status = 'running',
			content = '',
			error_message = '',
			completed_at = NULL,
			started_at = $1,
			updated_at = $2
		WHERE analysis_type = $3 AND status = 'running' AND started_at < $4
	`, nowStr, nowStr, string(analysisType), cutoff)
	if err != nil {
		return "", false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return "", false, err
	}
	if n > 0 {
		return nowStr, true, nil
	}
	return "", false, nil
}

func (r *AdvisorCacheRepository) SaveResult(ctx context.Context, analysisType advisor.AnalysisType, lease string, content string, errMsg string) error {
	now := time.Now().Format(time.RFC3339)
	status := advisor.StatusComplete
	if errMsg != "" {
		status = advisor.StatusError
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE advisor_cache
		SET status = $1, content = $2, error_message = $3, completed_at = $4, updated_at = $5
		WHERE analysis_type = $6 AND started_at = $7
	`, string(status), content, errMsg, now, now, string(analysisType), lease)
	if err != nil {
		return fmt.Errorf("update advisor cache: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		r.logSuperseded(ctx, analysisType, lease)
	}
	return nil
}

func (r *AdvisorCacheRepository) logSuperseded(ctx context.Context, analysisType advisor.AnalysisType, lease string) {
	if r.logger != nil {
		r.logger.Warn(ctx, "advisor_cache: save skipped (lease superseded)",
			observability.String("type", string(analysisType)),
			observability.String("lease", lease),
		)
	}
}

// parseFlexibleTimestamp parses timestamps in either RFC3339 ("2006-01-02T15:04:05Z")
// or SQLite/Postgres datetime format ("2006-01-02 15:04:05"). The latter format
// arises from the trg_advisor_cache_updated_at trigger which writes (NOW())::text.
func parseFlexibleTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t, nil
	}
	// Postgres NOW()::text also emits timezone-offset variants e.g.
	// "2026-04-19 10:11:12.123456+00".
	if t, err := time.Parse("2006-01-02 15:04:05.999999-07", s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05-07", s)
}

func (r *AdvisorCacheRepository) logParseWarn(ctx context.Context, analysisType advisor.AnalysisType, field, value string, err error) {
	if r.logger != nil {
		r.logger.Warn(ctx, "advisor_cache: failed to parse timestamp",
			observability.String("type", string(analysisType)),
			observability.String("field", field),
			observability.String("value", value),
			observability.Err(err),
		)
	}
}
