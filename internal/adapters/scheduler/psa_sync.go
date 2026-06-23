package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// ErrSyncInProgress is returned when RunOnce is called while a sync cycle is already running.
var ErrSyncInProgress = fmt.Errorf("PSA sync already in progress")

// PSASyncRunStats holds in-memory stats from the last PSA sync run.
type PSASyncRunStats struct {
	LastRunAt  time.Time `json:"lastRunAt"`
	DurationMs int64     `json:"durationMs"`
	LastError  string    `json:"lastError,omitempty"` // non-empty if the run failed
	Allocated  int       `json:"allocated"`
	Updated    int       `json:"updated"`
	Refunded   int       `json:"refunded"`
	Unmatched  int       `json:"unmatched"`
	Ambiguous  int       `json:"ambiguous"`
	Skipped    int       `json:"skipped"`
	Failed     int       `json:"failed"`
	TotalRows  int       `json:"totalRows"`
}

// RowProvider fetches PSA export rows from the portal.
type RowProvider interface {
	FetchRows(ctx context.Context) ([]inventory.PSAExportRow, error)
}

// TokenRefresher ensures the stored access token is fresh before each fetch.
type TokenRefresher interface {
	EnsureFreshToken(ctx context.Context) error
}

// PSAImporter runs the PSA import pipeline.
type PSAImporter interface {
	ImportPSAExportGlobal(ctx context.Context, rows []inventory.PSAExportRow) (*inventory.PSAImportResult, error)
}

var _ Scheduler = (*PSASyncScheduler)(nil)

// PSASyncScheduler fetches PSA data from the portal and imports it daily.
type PSASyncScheduler struct {
	StopHandle
	provider     RowProvider
	refresher    TokenRefresher
	importer     PSAImporter
	logger       observability.Logger
	config       config.PSASyncConfig
	running      sync.Mutex // ensures only one runOnce executes at a time
	lastRunStats *PSASyncRunStats
	statsMu      sync.RWMutex
}

// NewPSASyncScheduler creates a new PSA sync scheduler.
func NewPSASyncScheduler(
	provider RowProvider,
	refresher TokenRefresher,
	importer PSAImporter,
	logger observability.Logger,
	cfg config.PSASyncConfig,
) *PSASyncScheduler {
	cfg.ApplyDefaults()
	if cfg.SyncHour >= 0 {
		cfg.InitialDelay = timeUntilHour(time.Now(), cfg.SyncHour)
	}
	return &PSASyncScheduler{
		StopHandle: NewStopHandle(),
		provider:   provider,
		refresher:  refresher,
		importer:   importer,
		logger:     logger.With(context.Background(), observability.String("component", "psa-sync")),
		config:     cfg,
	}
}

// Start begins the background scheduler.
func (s *PSASyncScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "PSA sync scheduler disabled")
		return
	}

	s.logger.Info(ctx, "PSA sync scheduler starting",
		observability.Int("sync_hour", s.config.SyncHour))

	RunLoop(ctx, LoopConfig{
		Name:         "psa-sync",
		Interval:     s.config.Interval,
		InitialDelay: s.config.InitialDelay,
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
	}, func(ctx context.Context) {
		if !s.running.TryLock() {
			s.logger.Info(ctx, "PSA sync skipping tick: previous run still in progress")
			return
		}
		defer s.running.Unlock()
		s.runOnce(ctx) //nolint:errcheck
	})
}

// RunOnce runs a single sync cycle. Exported for manual trigger via HTTP handler.
// Returns ErrSyncInProgress if a cycle is already running (background or prior manual trigger).
func (s *PSASyncScheduler) RunOnce(ctx context.Context) error {
	if !s.running.TryLock() {
		return ErrSyncInProgress
	}
	defer s.running.Unlock()
	return s.runOnce(ctx)
}

// GetLastRunStats returns a copy of the last run stats, or nil if no run has completed.
func (s *PSASyncScheduler) GetLastRunStats() *PSASyncRunStats {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()
	if s.lastRunStats == nil {
		return nil
	}
	cp := *s.lastRunStats
	return &cp
}

func (s *PSASyncScheduler) runOnce(ctx context.Context) error {
	start := time.Now()
	s.logger.Info(ctx, "running PSA portal sync")

	fetchCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Refresh the access token before fetching, if a refresher is wired.
	if s.refresher != nil {
		if err := s.refresher.EnsureFreshToken(fetchCtx); err != nil {
			s.logger.Warn(ctx, "token refresh failed", observability.Err(err))
		}
	}

	psaRows, err := s.provider.FetchRows(fetchCtx)
	if err != nil {
		s.logger.Error(ctx, "failed to fetch PSA rows", observability.Err(err))
		s.recordFailure(start, err)
		return err
	}
	if len(psaRows) == 0 {
		s.logger.Warn(ctx, "no PSA rows returned from portal")
		return nil
	}

	importCtx, importCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer importCancel()
	importCtx = inventory.WithImportSource(importCtx, "scheduler")

	result, err := s.importer.ImportPSAExportGlobal(importCtx, psaRows)
	if err != nil {
		s.logger.Error(ctx, "PSA import failed", observability.Err(err))
		s.recordFailure(start, err)
		return err
	}

	s.logger.Info(ctx, "PSA sync completed",
		observability.Int("allocated", result.Allocated),
		observability.Int("updated", result.Updated),
		observability.Int("refunded", result.Refunded),
		observability.Int("unmatched", result.Unmatched),
		observability.Int("skipped", result.Skipped),
		observability.Int("failed", result.Failed),
		observability.Int("import_errors", len(result.Errors)))

	s.statsMu.Lock()
	s.lastRunStats = &PSASyncRunStats{
		LastRunAt:  start,
		DurationMs: time.Since(start).Milliseconds(),
		Allocated:  result.Allocated,
		Updated:    result.Updated,
		Refunded:   result.Refunded,
		Unmatched:  result.Unmatched,
		Ambiguous:  result.Ambiguous,
		Skipped:    result.Skipped,
		Failed:     result.Failed,
		TotalRows:  len(psaRows),
	}
	s.statsMu.Unlock()

	return nil
}

// recordFailure writes a failure record to lastRunStats so the status endpoint
// reflects the failed run rather than showing stale data from a previous success.
func (s *PSASyncScheduler) recordFailure(start time.Time, err error) {
	s.statsMu.Lock()
	s.lastRunStats = &PSASyncRunStats{
		LastRunAt:  start,
		DurationMs: time.Since(start).Milliseconds(),
		LastError:  err.Error(),
	}
	s.statsMu.Unlock()
}
