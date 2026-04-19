package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SchedulerStatsStore persists the most recent run stats for a named scheduler,
// keyed by the scheduler name (e.g. "card_ladder_refresh"). The in-memory
// `lastRunStats` on each scheduler is ephemeral; this store keeps the admin
// UI's "Last Run" panel populated across server restarts.
type SchedulerStatsStore struct {
	db *sql.DB
}

// NewSchedulerStatsStore constructs a SchedulerStatsStore.
func NewSchedulerStatsStore(db *sql.DB) *SchedulerStatsStore {
	return &SchedulerStatsStore{db: db}
}

// SchedulerRunStats is the persisted row format. StatsJSON carries the
// scheduler-specific counters as opaque JSON so each scheduler can evolve its
// struct without a schema migration.
type SchedulerRunStats struct {
	Name       string
	LastRunAt  time.Time
	DurationMs int64
	StatsJSON  string
}

// Save upserts the latest run for a scheduler. The primary key (name) ensures
// we only keep the most recent row per scheduler — history isn't required.
func (s *SchedulerStatsStore) Save(ctx context.Context, row SchedulerRunStats) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO scheduler_run_stats (name, last_run_at, duration_ms, stats_json, updated_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET
			last_run_at = excluded.last_run_at,
			duration_ms = excluded.duration_ms,
			stats_json = excluded.stats_json,
			updated_at = CURRENT_TIMESTAMP
	`, row.Name, row.LastRunAt.UTC().Format(time.RFC3339), row.DurationMs, row.StatsJSON)
	if err != nil {
		return fmt.Errorf("save scheduler run stats %q: %w", row.Name, err)
	}
	return nil
}

// Get loads the most recent run stats for a scheduler. Returns (nil, nil)
// when no row exists yet — callers treat that as "scheduler hasn't run yet."
func (s *SchedulerStatsStore) Get(ctx context.Context, name string) (*SchedulerRunStats, error) {
	var (
		row       SchedulerRunStats
		lastRunAt string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT name, last_run_at, duration_ms, stats_json
		FROM scheduler_run_stats
		WHERE name = $1
	`, name).Scan(&row.Name, &lastRunAt, &row.DurationMs, &row.StatsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get scheduler run stats %q: %w", name, err)
	}
	t, err := time.Parse(time.RFC3339, lastRunAt)
	if err != nil {
		return nil, fmt.Errorf("parse last_run_at for %q: %w", name, err)
	}
	row.LastRunAt = t
	return &row, nil
}
