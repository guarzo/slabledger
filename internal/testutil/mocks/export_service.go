package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/export"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

type MockExportService struct {
	GenerateSellSheetFn         func(ctx context.Context, campaignID string, purchaseIDs []string) (*inventory.SellSheet, error)
	GenerateGlobalSellSheetFn   func(ctx context.Context) (*inventory.SellSheet, error)
	GenerateSelectedSellSheetFn func(ctx context.Context, purchaseIDs []string) (*inventory.SellSheet, error)
}

var _ export.Service = (*MockExportService)(nil)

func (m *MockExportService) GenerateSellSheet(ctx context.Context, campaignID string, purchaseIDs []string) (*inventory.SellSheet, error) {
	if m.GenerateSellSheetFn != nil {
		return m.GenerateSellSheetFn(ctx, campaignID, purchaseIDs)
	}
	return &inventory.SellSheet{}, nil
}

func (m *MockExportService) GenerateGlobalSellSheet(ctx context.Context) (*inventory.SellSheet, error) {
	if m.GenerateGlobalSellSheetFn != nil {
		return m.GenerateGlobalSellSheetFn(ctx)
	}
	return &inventory.SellSheet{}, nil
}

func (m *MockExportService) GenerateSelectedSellSheet(ctx context.Context, purchaseIDs []string) (*inventory.SellSheet, error) {
	if m.GenerateSelectedSellSheetFn != nil {
		return m.GenerateSelectedSellSheetFn(ctx, purchaseIDs)
	}
	return &inventory.SellSheet{}, nil
}
