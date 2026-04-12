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
	ListEbayExportItemsFn       func(ctx context.Context, flaggedOnly bool) (*inventory.EbayExportListResponse, error)
	GenerateEbayCsvFn           func(ctx context.Context, items []inventory.EbayExportGenerateItem) ([]byte, error)
	MatchShopifyPricesFn        func(ctx context.Context, items []inventory.ShopifyPriceSyncItem) (*inventory.ShopifyPriceSyncResponse, error)
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

func (m *MockExportService) ListEbayExportItems(ctx context.Context, flaggedOnly bool) (*inventory.EbayExportListResponse, error) {
	if m.ListEbayExportItemsFn != nil {
		return m.ListEbayExportItemsFn(ctx, flaggedOnly)
	}
	return &inventory.EbayExportListResponse{}, nil
}

func (m *MockExportService) GenerateEbayCSV(ctx context.Context, items []inventory.EbayExportGenerateItem) ([]byte, error) {
	if m.GenerateEbayCsvFn != nil {
		return m.GenerateEbayCsvFn(ctx, items)
	}
	return []byte{}, nil
}

func (m *MockExportService) MatchShopifyPrices(ctx context.Context, items []inventory.ShopifyPriceSyncItem) (*inventory.ShopifyPriceSyncResponse, error) {
	if m.MatchShopifyPricesFn != nil {
		return m.MatchShopifyPricesFn(ctx, items)
	}
	return &inventory.ShopifyPriceSyncResponse{}, nil
}
