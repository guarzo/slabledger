package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// RecordAPICall records an API call for tracking
func (r *DBTracker) RecordAPICall(ctx context.Context, call *pricing.APICallRecord) error {

	query := `
		INSERT INTO api_calls (provider, endpoint, status_code, error, latency_ms, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		call.Provider,
		call.Endpoint,
		call.StatusCode,
		call.Error,
		call.LatencyMS,
		call.Timestamp,
	)

	if err != nil {
		r.logger.Error(ctx, "failed to record API call",
			observability.Err(err),
			observability.String("provider", call.Provider))
		return fmt.Errorf("record API call: %w", err)
	}

	return nil
}

// GetAPIUsage retrieves API usage statistics for a provider.
func (r *DBTracker) GetAPIUsage(ctx context.Context, provider string) (*pricing.APIUsageStats, error) {

	query := `
		SELECT
			provider,
			total_calls,
			error_calls,
			rate_limit_hits,
			avg_latency_ms,
			last_call_at,
			calls_last_hour,
			calls_last_5min
		FROM api_usage_summary
		WHERE provider = ?
	`

	var stats pricing.APIUsageStats
	var lastCallAt string

	err := r.db.QueryRowContext(ctx, query, provider).Scan(
		&stats.Provider,
		&stats.TotalCalls,
		&stats.ErrorCalls,
		&stats.RateLimitHits,
		&stats.AvgLatencyMS,
		&lastCallAt,
		&stats.CallsLastHour,
		&stats.CallsLast5Min,
	)

	if errors.Is(err, sql.ErrNoRows) {
		// No API calls recorded for this provider
		return &pricing.APIUsageStats{
			Provider: provider,
		}, nil
	}
	if err != nil {
		r.logger.Error(ctx, "failed to get API usage",
			observability.Err(err),
			observability.String("provider", provider))
		return nil, fmt.Errorf("get API usage: %w", err)
	}

	// Parse timestamp
	if parsedTime, err := time.Parse("2006-01-02 15:04:05", lastCallAt); err == nil {
		stats.LastCallAt = parsedTime
	}

	// Check if provider is blocked
	blocked, until, blockErr := r.IsProviderBlocked(ctx, provider)
	if blockErr != nil {
		// Log the error but don't fail the entire request - block status is supplementary info
		r.logger.Warn(ctx, "failed to check provider block status",
			observability.Err(blockErr),
			observability.String("provider", provider))
	} else if blocked {
		stats.BlockedUntil = &until
	}

	return &stats, nil
}

// UpdateRateLimit updates the rate limit block status for a provider
func (r *DBTracker) UpdateRateLimit(ctx context.Context, provider string, blockedUntil time.Time) error {

	query := `
		INSERT INTO api_rate_limits (provider, blocked_until, last_429_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(provider)
		DO UPDATE SET
			blocked_until = excluded.blocked_until,
			last_429_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
	`

	// Format time as RFC3339 in UTC for consistent storage and parsing
	blockedUntilStr := blockedUntil.UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx, query, provider, blockedUntilStr)
	if err != nil {
		r.logger.Error(ctx, "failed to update rate limit",
			observability.Err(err),
			observability.String("provider", provider))
		return fmt.Errorf("update rate limit: %w", err)
	}

	r.logger.Warn(ctx, "provider rate limit updated",
		observability.String("provider", provider),
		observability.Time("blocked_until", blockedUntil))

	return nil
}

// IsProviderBlocked checks if a provider is currently rate limited
func (r *DBTracker) IsProviderBlocked(ctx context.Context, provider string) (bool, time.Time, error) {
	query := `
		SELECT blocked_until
		FROM api_rate_limits
		WHERE provider = ?
	`

	var blockedUntil sql.NullString
	err := r.db.QueryRowContext(ctx, query, provider).Scan(&blockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, time.Time{}, nil // Not blocked
	}
	if err != nil {
		return false, time.Time{}, fmt.Errorf("check provider block: %w", err)
	}

	if !blockedUntil.Valid {
		return false, time.Time{}, nil // No block time set
	}

	parsedTime, err := time.Parse(time.RFC3339, blockedUntil.String)
	if err != nil {
		// Fall back to legacy SQLite datetime format
		parsedTime, err = time.Parse("2006-01-02 15:04:05", blockedUntil.String)
		if err != nil {
			r.logger.Error(ctx, "failed to parse blocked_until timestamp",
				observability.Err(err),
				observability.String("provider", provider),
				observability.String("blocked_until_raw", blockedUntil.String))
			return true, time.Time{}, fmt.Errorf("parse blocked_until for provider %s: %w", provider, err)
		}
	}

	// Check if block has expired (use UTC for consistent comparison)
	if parsedTime.UTC().Before(time.Now().UTC()) {
		return false, time.Time{}, nil
	}

	return true, parsedTime, nil
}
