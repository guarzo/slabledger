package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/csvimport"
)

// PSARowProviderMock is a test double for scheduler.RowProvider.
type PSARowProviderMock struct {
	FetchRowsFn func(ctx context.Context) ([]csvimport.PSAExportRow, error)
}

func (m *PSARowProviderMock) FetchRows(ctx context.Context) ([]csvimport.PSAExportRow, error) {
	if m.FetchRowsFn != nil {
		return m.FetchRowsFn(ctx)
	}
	return nil, nil
}
