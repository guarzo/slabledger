package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// PSARowProviderMock is a test double for scheduler.RowProvider.
type PSARowProviderMock struct {
	FetchRowsFn func(ctx context.Context) ([]inventory.PSAExportRow, error)
}

func (m *PSARowProviderMock) FetchRows(ctx context.Context) ([]inventory.PSAExportRow, error) {
	if m.FetchRowsFn != nil {
		return m.FetchRowsFn(ctx)
	}
	return nil, nil
}
