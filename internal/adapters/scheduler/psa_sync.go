package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// PSASyncRunStats holds in-memory stats from the last PSA sync run.
type PSASyncRunStats struct {
	LastRunAt   time.Time `json:"lastRunAt"`
	DurationMs  int64     `json:"durationMs"`
	Allocated   int       `json:"allocated"`
	Updated     int       `json:"updated"`
	Refunded    int       `json:"refunded"`
	Unmatched   int       `json:"unmatched"`
	Ambiguous   int       `json:"ambiguous"`
	Skipped     int       `json:"skipped"`
	Failed      int       `json:"failed"`
	TotalRows   int       `json:"totalRows"`
	ParseErrors int       `json:"parseErrors"`
}

// SheetFetcher fetches sheet data as a 2D string grid.
type SheetFetcher interface {
	ReadSheet(ctx context.Context, spreadsheetID, sheetName string) ([][]string, error)
}

// PSAImporter runs the PSA import pipeline.
type PSAImporter interface {
	ImportPSAExportGlobal(ctx context.Context, rows []campaigns.PSAExportRow) (*campaigns.PSAImportResult, error)
}

var _ Scheduler = (*PSASyncScheduler)(nil)

// PSASyncScheduler fetches PSA data from a Google Sheet and imports it daily.
type PSASyncScheduler struct {
	StopHandle
	fetcher       SheetFetcher
	importer      PSAImporter
	logger        observability.Logger
	config        config.PSASyncConfig
	spreadsheetID string
	tabName       string
	lastRunStats  *PSASyncRunStats
	statsMu       sync.RWMutex
}

// NewPSASyncScheduler creates a new PSA sync scheduler.
func NewPSASyncScheduler(
	fetcher SheetFetcher,
	importer PSAImporter,
	logger observability.Logger,
	cfg config.PSASyncConfig,
	spreadsheetID, tabName string,
) *PSASyncScheduler {
	cfg.ApplyDefaults()
	if cfg.SyncHour >= 0 {
		cfg.InitialDelay = timeUntilHour(time.Now(), cfg.SyncHour)
	}
	return &PSASyncScheduler{
		StopHandle:    NewStopHandle(),
		fetcher:       fetcher,
		importer:      importer,
		logger:        logger.With(context.Background(), observability.String("component", "psa-sync")),
		config:        cfg,
		spreadsheetID: spreadsheetID,
		tabName:       tabName,
	}
}

// Start begins the background scheduler.
func (s *PSASyncScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "PSA sync scheduler disabled")
		return
	}

	s.logger.Info(ctx, "PSA sync scheduler starting",
		observability.String("spreadsheet_id", s.spreadsheetID),
		observability.String("tab", s.tabName),
		observability.Int("sync_hour", s.config.SyncHour))

	RunLoop(ctx, LoopConfig{
		Name:         "psa-sync",
		Interval:     s.config.Interval,
		InitialDelay: s.config.InitialDelay,
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
	}, s.tick)
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

func (s *PSASyncScheduler) tick(ctx context.Context) {
	start := time.Now()
	s.logger.Info(ctx, "running PSA Google Sheets sync")

	fetchCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	rows, err := s.fetcher.ReadSheet(fetchCtx, s.spreadsheetID, s.tabName)
	if err != nil {
		s.logger.Error(ctx, "failed to fetch Google Sheet",
			observability.Err(err),
			observability.String("spreadsheet_id", s.spreadsheetID))
		return
	}

	psaRows, parseErrors, err := campaigns.ParsePSAExportRows(rows)
	if err != nil {
		s.logger.Error(ctx, "failed to parse PSA sheet data", observability.Err(err))
		return
	}
	if len(parseErrors) > 0 {
		s.logger.Warn(ctx, "PSA sheet parse failures — rows skipped",
			observability.Int("skipped_rows", len(parseErrors)),
			observability.String("spreadsheet_id", s.spreadsheetID))
	}
	if len(psaRows) == 0 {
		s.logger.Warn(ctx, "no valid PSA rows found in sheet")
		return
	}

	importCtx, importCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer importCancel()
	importCtx = campaigns.WithImportSource(importCtx, "scheduler")

	result, err := s.importer.ImportPSAExportGlobal(importCtx, psaRows)
	if err != nil {
		s.logger.Error(ctx, "PSA import failed", observability.Err(err))
		return
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
		LastRunAt:   start,
		DurationMs:  time.Since(start).Milliseconds(),
		Allocated:   result.Allocated,
		Updated:     result.Updated,
		Refunded:    result.Refunded,
		Unmatched:   result.Unmatched,
		Ambiguous:   result.Ambiguous,
		Skipped:     result.Skipped,
		Failed:      result.Failed,
		TotalRows:   len(psaRows),
		ParseErrors: len(parseErrors),
	}
	s.statsMu.Unlock()
}
