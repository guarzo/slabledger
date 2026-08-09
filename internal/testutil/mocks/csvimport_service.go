package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/csvimport"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// MockImportService is a test double for csvimport.Service — the CSV and PSA
// export intake paths. Cert lookup stayed on inventory.Service (SLA-35); use
// MockInventoryService for that half.
//
// Each method delegates to a function field, allowing per-test configuration.
//
// Example:
//
//	svc := &MockImportService{
//	    ImportPSAExportGlobalFn: func(ctx context.Context, rows []csvimport.PSAExportRow) (*csvimport.PSAImportResult, error) {
//	        return &csvimport.PSAImportResult{}, nil
//	    },
//	}
type MockImportService struct {
	ImportPSAExportGlobalFn   func(ctx context.Context, rows []csvimport.PSAExportRow) (*csvimport.PSAImportResult, error)
	ReconcilePSAAttributionFn func(ctx context.Context, rows []csvimport.PSAExportRow) (csvimport.ReconcileResult, error)
	ImportExternalCSVFn       func(ctx context.Context, rows []csvimport.ShopifyExportRow) (*csvimport.ExternalImportResult, error)
	ImportOrdersSalesFn       func(ctx context.Context, rows []csvimport.OrdersExportRow) (*csvimport.OrdersImportResult, error)
	ImportEbayOrdersSalesFn   func(ctx context.Context, rows []csvimport.EbayOrderRow) (*csvimport.OrdersImportResult, error)
	ConfirmOrdersSalesFn      func(ctx context.Context, items []csvimport.OrdersConfirmItem) (*inventory.BulkSaleResult, error)
	ImportCertSalesFn         func(ctx context.Context, req csvimport.CertSaleImportRequest) (*csvimport.CertSaleImportResult, error)
	CloseFn                   func()
}

var _ csvimport.Service = (*MockImportService)(nil)

func (m *MockImportService) ImportPSAExportGlobal(ctx context.Context, rows []csvimport.PSAExportRow) (*csvimport.PSAImportResult, error) {
	if m.ImportPSAExportGlobalFn != nil {
		return m.ImportPSAExportGlobalFn(ctx, rows)
	}
	return &csvimport.PSAImportResult{}, nil
}

func (m *MockImportService) ReconcilePSAAttribution(ctx context.Context, rows []csvimport.PSAExportRow) (csvimport.ReconcileResult, error) {
	if m.ReconcilePSAAttributionFn != nil {
		return m.ReconcilePSAAttributionFn(ctx, rows)
	}
	return csvimport.ReconcileResult{}, nil
}

func (m *MockImportService) ImportExternalCSV(ctx context.Context, rows []csvimport.ShopifyExportRow) (*csvimport.ExternalImportResult, error) {
	if m.ImportExternalCSVFn != nil {
		return m.ImportExternalCSVFn(ctx, rows)
	}
	return &csvimport.ExternalImportResult{}, nil
}

func (m *MockImportService) ImportOrdersSales(ctx context.Context, rows []csvimport.OrdersExportRow) (*csvimport.OrdersImportResult, error) {
	if m.ImportOrdersSalesFn != nil {
		return m.ImportOrdersSalesFn(ctx, rows)
	}
	return &csvimport.OrdersImportResult{}, nil
}

func (m *MockImportService) ImportEbayOrdersSales(ctx context.Context, rows []csvimport.EbayOrderRow) (*csvimport.OrdersImportResult, error) {
	if m.ImportEbayOrdersSalesFn != nil {
		return m.ImportEbayOrdersSalesFn(ctx, rows)
	}
	return &csvimport.OrdersImportResult{}, nil
}

func (m *MockImportService) ConfirmOrdersSales(ctx context.Context, items []csvimport.OrdersConfirmItem) (*inventory.BulkSaleResult, error) {
	if m.ConfirmOrdersSalesFn != nil {
		return m.ConfirmOrdersSalesFn(ctx, items)
	}
	return &inventory.BulkSaleResult{}, nil
}

func (m *MockImportService) ImportCertSales(ctx context.Context, req csvimport.CertSaleImportRequest) (*csvimport.CertSaleImportResult, error) {
	if m.ImportCertSalesFn != nil {
		return m.ImportCertSalesFn(ctx, req)
	}
	return &csvimport.CertSaleImportResult{}, nil
}

func (m *MockImportService) Close() {
	if m.CloseFn != nil {
		m.CloseFn()
	}
}
