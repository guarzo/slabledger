package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// AICallRepository implements ai.AICallTracker using SQLite.
type AICallRepository struct {
	db     *DB
	logger observability.Logger
}

var _ ai.AICallTracker = (*AICallRepository)(nil)

// NewAICallRepository creates a new AI call tracker.
func NewAICallRepository(db *DB) *AICallRepository {
	ctx := context.Background()
	return &AICallRepository{
		db:     db,
		logger: db.logger.With(ctx, observability.String("component", "ai_tracker")),
	}
}

// RecordAICall records a single AI API invocation.
func (r *AICallRepository) RecordAICall(ctx context.Context, call *ai.AICallRecord) error {
	query := `
		INSERT INTO ai_calls (operation, status, error_message, latency_ms, tool_rounds, input_tokens, output_tokens, total_tokens, cost_estimate_cents, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		string(call.Operation),
		string(call.Status),
		call.ErrorMessage,
		call.LatencyMS,
		call.ToolRounds,
		call.InputTokens,
		call.OutputTokens,
		call.TotalTokens,
		call.CostEstimateCents,
		call.Timestamp.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		r.logger.Error(ctx, "failed to record AI call",
			observability.Err(err),
			observability.String("operation", string(call.Operation)))
		return fmt.Errorf("record AI call: %w", err)
	}

	return nil
}

// GetAIUsage retrieves aggregate AI usage statistics (7-day window).
func (r *AICallRepository) GetAIUsage(ctx context.Context) (*ai.AIUsageStats, error) {
	stats := &ai.AIUsageStats{
		ByOperation: make(map[ai.AIOperation]*ai.AIOperationStats),
	}

	// Query summary view.
	summaryQuery := `
		SELECT total_calls, success_calls, error_calls, rate_limit_hits,
		       avg_latency_ms, total_input_tokens, total_output_tokens, total_tokens,
		       total_cost_cents, last_call_at, calls_last_24h
		FROM ai_usage_summary
	`

	var lastCallAt sql.NullString
	err := r.db.QueryRowContext(ctx, summaryQuery).Scan(
		&stats.TotalCalls,
		&stats.SuccessCalls,
		&stats.ErrorCalls,
		&stats.RateLimitHits,
		&stats.AvgLatencyMS,
		&stats.TotalInputTokens,
		&stats.TotalOutputTokens,
		&stats.TotalTokens,
		&stats.TotalCostCents,
		&lastCallAt,
		&stats.CallsLast24h,
	)
	if err != nil && err != sql.ErrNoRows {
		r.logger.Error(ctx, "failed to get AI usage summary", observability.Err(err))
		return nil, fmt.Errorf("get AI usage summary: %w", err)
	}

	if lastCallAt.Valid && lastCallAt.String != "" {
		if t, parseErr := time.Parse("2006-01-02 15:04:05", lastCallAt.String); parseErr == nil {
			stats.LastCallAt = &t
		} else {
			r.logger.Warn(ctx, "failed to parse lastCallAt timestamp",
				observability.String("raw", lastCallAt.String),
				observability.Err(parseErr))
		}
	}

	// Query per-operation breakdown.
	opQuery := `SELECT operation, calls, errors, avg_latency_ms, total_tokens, total_cost_cents FROM ai_usage_by_operation`
	rows, err := r.db.QueryContext(ctx, opQuery)
	if err != nil {
		r.logger.Error(ctx, "failed to get AI usage by operation", observability.Err(err))
		return nil, fmt.Errorf("get AI usage by operation: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Close error is not actionable

	for rows.Next() {
		var op ai.AIOperationStats
		var name string
		if scanErr := rows.Scan(&name, &op.Calls, &op.Errors, &op.AvgLatencyMS, &op.TotalTokens, &op.TotalCostCents); scanErr != nil {
			r.logger.Error(ctx, "failed to scan AI operation stats", observability.Err(scanErr))
			continue
		}
		stats.ByOperation[ai.AIOperation(name)] = &op
	}

	return stats, rows.Err()
}
