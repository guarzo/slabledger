package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/export"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

type MockExportService struct {
	GenerateGlobalSellSheetFn func(ctx context.Context) (*inventory.SellSheet, error)
}

var _ export.Service = (*MockExportService)(nil)

func (m *MockExportService) GenerateGlobalSellSheet(ctx context.Context) (*inventory.SellSheet, error) {
	if m.GenerateGlobalSellSheetFn != nil {
		return m.GenerateGlobalSellSheetFn(ctx)
	}
	return &inventory.SellSheet{}, nil
}
