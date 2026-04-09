package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// mockSheetFetcher implements SheetFetcher for testing.
type mockSheetFetcher struct {
	rows [][]string
	err  error
}

func (m *mockSheetFetcher) ReadSheet(_ context.Context, _, _ string) ([][]string, error) {
	return m.rows, m.err
}

// mockPSAImporter implements PSAImporter for testing.
type mockPSAImporter struct {
	result *campaigns.PSAImportResult
	err    error
	called bool
}

func (m *mockPSAImporter) ImportPSAExportGlobal(_ context.Context, _ []campaigns.PSAExportRow) (*campaigns.PSAImportResult, error) {
	m.called = true
	return m.result, m.err
}

func TestPSASyncScheduler_Tick_Success(t *testing.T) {
	fetcher := &mockSheetFetcher{
		rows: [][]string{
			{"Cert Number", "Listing Title", "Grade", "Price Paid"},
			{"12345678", "2023 Pokemon Charizard PSA 10", "10", "$125.00"},
		},
	}
	importer := &mockPSAImporter{
		result: &campaigns.PSAImportResult{Allocated: 1},
	}

	s := NewPSASyncScheduler(
		fetcher, importer,
		observability.NewNoopLogger(),
		config.PSASyncConfig{Enabled: true, Interval: 24 * time.Hour, SyncHour: -1},
		"spreadsheet-id", "Sheet1",
	)

	s.tick(context.Background())

	if !importer.called {
		t.Error("importer was not called")
	}
}

func TestPSASyncScheduler_Tick_FetchError(t *testing.T) {
	fetcher := &mockSheetFetcher{err: errors.New("network error")}
	importer := &mockPSAImporter{}

	s := NewPSASyncScheduler(
		fetcher, importer,
		observability.NewNoopLogger(),
		config.PSASyncConfig{Enabled: true, Interval: 24 * time.Hour, SyncHour: -1},
		"spreadsheet-id", "Sheet1",
	)

	s.tick(context.Background())

	if importer.called {
		t.Error("importer should not be called on fetch error")
	}
}

func TestPSASyncScheduler_Tick_ParseError(t *testing.T) {
	// No valid header row → ParsePSAExportRows should return an error
	fetcher := &mockSheetFetcher{
		rows: [][]string{
			{"random", "columns", "here"},
			{"data", "rows", "no cert"},
		},
	}
	importer := &mockPSAImporter{}

	s := NewPSASyncScheduler(
		fetcher, importer,
		observability.NewNoopLogger(),
		config.PSASyncConfig{Enabled: true, Interval: 24 * time.Hour, SyncHour: -1},
		"spreadsheet-id", "Sheet1",
	)

	s.tick(context.Background())

	if importer.called {
		t.Error("importer should not be called when parsing fails")
	}
}

func TestPSASyncScheduler_Start_Disabled(t *testing.T) {
	s := NewPSASyncScheduler(
		&mockSheetFetcher{}, &mockPSAImporter{},
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
