package mocks

import "context"

// MockSheetFetcher is a test double for scheduler.SheetFetcher.
// Each method delegates to a function field, allowing per-test configuration.
type MockSheetFetcher struct {
	ReadSheetFn func(ctx context.Context, spreadsheetID, sheetName string) ([][]string, error)
}

func (m *MockSheetFetcher) ReadSheet(ctx context.Context, spreadsheetID, sheetName string) ([][]string, error) {
	if m.ReadSheetFn != nil {
		return m.ReadSheetFn(ctx, spreadsheetID, sheetName)
	}
	return nil, nil
}
