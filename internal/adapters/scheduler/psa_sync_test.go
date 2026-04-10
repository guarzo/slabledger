package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestPSASyncScheduler_Tick(t *testing.T) {
	tests := []struct {
		name               string
		fetcherFn          func(ctx context.Context, spreadsheetID, sheetName string) ([][]string, error)
		importerFn         func(ctx context.Context, rows []campaigns.PSAExportRow) (*campaigns.PSAImportResult, error)
		wantImporterCalled bool
	}{
		{
			name: "success",
			fetcherFn: func(_ context.Context, _, _ string) ([][]string, error) {
				return [][]string{
					{"Cert Number", "Listing Title", "Grade", "Price Paid"},
					{"12345678", "2023 Pokemon Charizard PSA 10", "10", "$125.00"},
				}, nil
			},
			importerFn: func(_ context.Context, _ []campaigns.PSAExportRow) (*campaigns.PSAImportResult, error) {
				return &campaigns.PSAImportResult{Allocated: 1}, nil
			},
			wantImporterCalled: true,
		},
		{
			name: "fetch error",
			fetcherFn: func(_ context.Context, _, _ string) ([][]string, error) {
				return nil, errors.New("network error")
			},
			wantImporterCalled: false,
		},
		{
			name: "parse error",
			fetcherFn: func(_ context.Context, _, _ string) ([][]string, error) {
				return [][]string{
					{"random", "columns", "here"},
					{"data", "rows", "no cert"},
				}, nil
			},
			wantImporterCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := &mocks.MockSheetFetcher{ReadSheetFn: tt.fetcherFn}

			importerCalled := false
			importer := &mocks.MockImportService{
				ImportPSAExportGlobalFn: func(ctx context.Context, rows []campaigns.PSAExportRow) (*campaigns.PSAImportResult, error) {
					importerCalled = true
					if tt.importerFn != nil {
						return tt.importerFn(ctx, rows)
					}
					return &campaigns.PSAImportResult{}, nil
				},
			}

			s := NewPSASyncScheduler(
				fetcher, importer,
				observability.NewNoopLogger(),
				config.PSASyncConfig{Enabled: true, Interval: 24 * time.Hour, SyncHour: -1},
				"spreadsheet-id", "Sheet1",
			)

			s.tick(context.Background())

			if importerCalled != tt.wantImporterCalled {
				t.Errorf("importer called = %v, want %v", importerCalled, tt.wantImporterCalled)
			}
		})
	}
}

func TestPSASyncScheduler_Start_Disabled(t *testing.T) {
	s := NewPSASyncScheduler(
		&mocks.MockSheetFetcher{}, &mocks.MockImportService{},
		observability.NewNoopLogger(),
		config.PSASyncConfig{Enabled: false},
		"id", "tab",
	)
	// Start should return immediately when disabled
	done := make(chan struct{})
	go func() {
		s.Start(context.Background())
		close(done)
	}()
	select {
	case <-done:
		// Good — returned immediately
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return when disabled")
	}
}

func TestPSASyncScheduler_GetLastRunStats(t *testing.T) {
	s := NewPSASyncScheduler(
		&mocks.MockSheetFetcher{
			ReadSheetFn: func(ctx context.Context, sid, sn string) ([][]string, error) {
				return [][]string{
					{"Cert Number", "Listing Title", "Grade", "Price Paid", "Date", "Vault Status", "Invoice Date", "Image URLs", "Purchase Source", "Category", "Was Refunded"},
					{"12345", "2023 Pokemon Charizard #1", "10", "15.00", "2026-01-01", "In Vault", "", "", "", "Pokemon", "No"},
				}, nil
			},
		},
		&mocks.MockImportService{
			ImportPSAExportGlobalFn: func(ctx context.Context, rows []campaigns.PSAExportRow) (*campaigns.PSAImportResult, error) {
				return &campaigns.PSAImportResult{
					Allocated: 1, Updated: 0, Refunded: 0,
					Unmatched: 2, Ambiguous: 1, Skipped: 0, Failed: 0,
				}, nil
			},
		},
		mocks.NewMockLogger(),
		config.PSASyncConfig{Enabled: true, SyncHour: -1},
		"sheet-id", "Sheet1",
	)

	// Before any run, stats should be nil
	if stats := s.GetLastRunStats(); stats != nil {
		t.Fatal("expected nil stats before first run")
	}

	// Run tick
	s.tick(context.Background())

	stats := s.GetLastRunStats()
	if stats == nil {
		t.Fatal("expected non-nil stats after tick")
	}
	if stats.Allocated != 1 {
		t.Errorf("expected Allocated=1, got %d", stats.Allocated)
	}
	if stats.Unmatched != 2 {
		t.Errorf("expected Unmatched=2, got %d", stats.Unmatched)
	}
	if stats.Ambiguous != 1 {
		t.Errorf("expected Ambiguous=1, got %d", stats.Ambiguous)
	}
	if stats.TotalRows != 1 {
		t.Errorf("expected TotalRows=1, got %d", stats.TotalRows)
	}
	if stats.DurationMs < 0 {
		t.Error("expected non-negative DurationMs")
	}
}
