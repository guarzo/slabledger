package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestPSASyncScheduler_Tick(t *testing.T) {
	tests := []struct {
		name               string
		providerFn         func(ctx context.Context) ([]inventory.PSAExportRow, error)
		importerFn         func(ctx context.Context, rows []inventory.PSAExportRow) (*inventory.PSAImportResult, error)
		wantImporterCalled bool
	}{
		{
			name: "success",
			providerFn: func(_ context.Context) ([]inventory.PSAExportRow, error) {
				return []inventory.PSAExportRow{
					{CertNumber: "12345678", Grade: 10, PricePaid: 125.00},
				}, nil
			},
			importerFn: func(_ context.Context, _ []inventory.PSAExportRow) (*inventory.PSAImportResult, error) {
				return &inventory.PSAImportResult{Allocated: 1}, nil
			},
			wantImporterCalled: true,
		},
		{
			name: "fetch error",
			providerFn: func(_ context.Context) ([]inventory.PSAExportRow, error) {
				return nil, errors.New("network error")
			},
			wantImporterCalled: false,
		},
		{
			name: "empty rows",
			providerFn: func(_ context.Context) ([]inventory.PSAExportRow, error) {
				return []inventory.PSAExportRow{}, nil
			},
			wantImporterCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mocks.PSARowProviderMock{FetchRowsFn: tt.providerFn}

			importerCalled := false
			importer := &mocks.MockImportService{
				ImportPSAExportGlobalFn: func(ctx context.Context, rows []inventory.PSAExportRow) (*inventory.PSAImportResult, error) {
					importerCalled = true
					if tt.importerFn != nil {
						return tt.importerFn(ctx, rows)
					}
					return &inventory.PSAImportResult{}, nil
				},
			}

			s := NewPSASyncScheduler(
				provider, importer,
				observability.NewNoopLogger(),
				config.PSASyncConfig{Enabled: true, Interval: 24 * time.Hour, SyncHour: -1},
			)

			s.runOnce(context.Background()) //nolint:errcheck

			if importerCalled != tt.wantImporterCalled {
				t.Errorf("importer called = %v, want %v", importerCalled, tt.wantImporterCalled)
			}
		})
	}
}

func TestPSASyncScheduler_Start_Disabled(t *testing.T) {
	provider := &mocks.PSARowProviderMock{
		FetchRowsFn: func(_ context.Context) ([]inventory.PSAExportRow, error) { return nil, nil },
	}
	s := NewPSASyncScheduler(
		provider, &mocks.MockImportService{},
		observability.NewNoopLogger(),
		config.PSASyncConfig{Enabled: false},
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
	provider := &mocks.PSARowProviderMock{
		FetchRowsFn: func(_ context.Context) ([]inventory.PSAExportRow, error) {
			return []inventory.PSAExportRow{
				{CertNumber: "12345", Grade: 10, PricePaid: 15.00},
			}, nil
		},
	}
	s := NewPSASyncScheduler(
		provider,
		&mocks.MockImportService{
			ImportPSAExportGlobalFn: func(ctx context.Context, rows []inventory.PSAExportRow) (*inventory.PSAImportResult, error) {
				return &inventory.PSAImportResult{
					Allocated: 1, Updated: 0, Refunded: 0,
					Unmatched: 2, Ambiguous: 1, Skipped: 0, Failed: 0,
				}, nil
			},
		},
		mocks.NewMockLogger(),
		config.PSASyncConfig{Enabled: true, SyncHour: -1},
	)

	// Before any run, stats should be nil
	if stats := s.GetLastRunStats(); stats != nil {
		t.Fatal("expected nil stats before first run")
	}

	// Run tick
	s.runOnce(context.Background()) //nolint:errcheck

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
