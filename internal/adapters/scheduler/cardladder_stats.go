package scheduler

import (
	"context"
	"encoding/json"

	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CLRunStatsName is the scheduler_run_stats.name value used for CL refresh
// snapshots. Exported so handlers can read the same row without duplication.
const CLRunStatsName = "card_ladder_refresh"

// WithCLStatsStore enables persistence of per-run stats so the admin UI's
// "Last Run" panel survives server restarts. Optional — when nil, stats
// remain in-memory only and reset on restart (prior behavior).
func WithCLStatsStore(ss *sqlite.SchedulerStatsStore) CardLadderRefreshOption {
	return func(s *CardLadderRefreshScheduler) { s.statsStore = ss }
}

// GetLastRunStats returns a copy of the stats from the most recent refresh run,
// or nil if no run has completed yet. When in-memory stats are absent (cold
// start after server restart) and a stats store is configured, the most
// recent persisted row is loaded and returned.
func (s *CardLadderRefreshScheduler) GetLastRunStats() *CLRunStats {
	s.statsMu.RLock()
	if s.lastRunStats != nil {
		cp := *s.lastRunStats
		s.statsMu.RUnlock()
		return &cp
	}
	s.statsMu.RUnlock()

	if s.statsStore == nil {
		return nil
	}
	row, err := s.statsStore.Get(context.Background(), CLRunStatsName)
	if err != nil {
		s.logger.Warn(context.Background(), "CL refresh: load persisted stats failed",
			observability.Err(err))
		return nil
	}
	if row == nil {
		return nil
	}
	var stats CLRunStats
	if err := json.Unmarshal([]byte(row.StatsJSON), &stats); err != nil {
		s.logger.Warn(context.Background(), "CL refresh: unmarshal persisted stats failed",
			observability.Err(err))
		return nil
	}
	// Cache the loaded row so subsequent reads skip the DB round-trip until
	// the next runOnce overwrites it.
	s.statsMu.Lock()
	if s.lastRunStats == nil {
		cp := stats
		s.lastRunStats = &cp
	}
	s.statsMu.Unlock()
	return &stats
}

// persistStats writes the most recent run stats to the stats store when one
// is configured. Failures are logged at Warn and never propagated — persistence
// is best-effort observability, never a reason to fail a refresh.
func (s *CardLadderRefreshScheduler) persistStats(ctx context.Context, stats CLRunStats) {
	if s.statsStore == nil {
		return
	}
	payload, err := json.Marshal(stats)
	if err != nil {
		s.logger.Warn(ctx, "CL refresh: marshal stats for persistence failed",
			observability.Err(err))
		return
	}
	if err := s.statsStore.Save(ctx, sqlite.SchedulerRunStats{
		Name:       CLRunStatsName,
		LastRunAt:  stats.LastRunAt,
		DurationMs: stats.DurationMs,
		StatsJSON:  string(payload),
	}); err != nil {
		s.logger.Warn(ctx, "CL refresh: persist stats failed",
			observability.Err(err))
	}
}
