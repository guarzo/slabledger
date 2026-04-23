package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// MockImportService is a test double for inventory.ImportService.
// Each method delegates to a function field, allowing per-test configuration.
//
// Example:
//
//	svc := &MockImportService{
//	    ImportCertsFn: func(ctx context.Context, certNumbers []string) (*inventory.CertImportResult, error) {
//	        return &inventory.CertImportResult{}, nil
//	    },
//	}
type MockImportService struct {
	ImportPSAExportGlobalFn     func(ctx context.Context, rows []inventory.PSAExportRow) (*inventory.PSAImportResult, error)
	ExportMMFormatGlobalFn      func(ctx context.Context, missingMMOnly bool) ([]inventory.MMExportEntry, error)
	RefreshMMValuesGlobalFn     func(ctx context.Context, rows []inventory.MMRefreshRow) (*inventory.MMRefreshResult, error)
	EnsureExternalCampaignFn    func(ctx context.Context) (*inventory.Campaign, error)
	ImportExternalCSVFn         func(ctx context.Context, rows []inventory.ShopifyExportRow) (*inventory.ExternalImportResult, error)
	ImportOrdersSalesFn         func(ctx context.Context, rows []inventory.OrdersExportRow) (*inventory.OrdersImportResult, error)
	ImportEbayOrdersSalesFn     func(ctx context.Context, rows []inventory.EbayOrderRow) (*inventory.OrdersImportResult, error)
	ConfirmOrdersSalesFn        func(ctx context.Context, items []inventory.OrdersConfirmItem) (*inventory.BulkSaleResult, error)
	ImportCertsFn               func(ctx context.Context, certNumbers []string) (*inventory.CertImportResult, error)
	GetPurchasesByCertNumbersFn func(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error)
	ScanCertFn                  func(ctx context.Context, certNumber string) (*inventory.ScanCertResult, error)
	ScanCertsFn                 func(ctx context.Context, certNumbers []string) (*inventory.ScanCertsResult, error)
	ResolveCertFn               func(ctx context.Context, certNumber string) (*inventory.CertInfo, error)
}

var _ inventory.ImportService = (*MockImportService)(nil)

func (m *MockImportService) ImportPSAExportGlobal(ctx context.Context, rows []inventory.PSAExportRow) (*inventory.PSAImportResult, error) {
	if m.ImportPSAExportGlobalFn != nil {
		return m.ImportPSAExportGlobalFn(ctx, rows)
	}
	return &inventory.PSAImportResult{}, nil
}

func (m *MockImportService) ExportMMFormatGlobal(ctx context.Context, missingMMOnly bool) ([]inventory.MMExportEntry, error) {
	if m.ExportMMFormatGlobalFn != nil {
		return m.ExportMMFormatGlobalFn(ctx, missingMMOnly)
	}
	return []inventory.MMExportEntry{}, nil
}

func (m *MockImportService) RefreshMMValuesGlobal(ctx context.Context, rows []inventory.MMRefreshRow) (*inventory.MMRefreshResult, error) {
	if m.RefreshMMValuesGlobalFn != nil {
		return m.RefreshMMValuesGlobalFn(ctx, rows)
	}
	return &inventory.MMRefreshResult{}, nil
}

func (m *MockImportService) EnsureExternalCampaign(ctx context.Context) (*inventory.Campaign, error) {
	if m.EnsureExternalCampaignFn != nil {
		return m.EnsureExternalCampaignFn(ctx)
	}
	return &inventory.Campaign{}, nil
}

func (m *MockImportService) ImportExternalCSV(ctx context.Context, rows []inventory.ShopifyExportRow) (*inventory.ExternalImportResult, error) {
	if m.ImportExternalCSVFn != nil {
		return m.ImportExternalCSVFn(ctx, rows)
	}
	return &inventory.ExternalImportResult{}, nil
}

func (m *MockImportService) ImportOrdersSales(ctx context.Context, rows []inventory.OrdersExportRow) (*inventory.OrdersImportResult, error) {
	if m.ImportOrdersSalesFn != nil {
		return m.ImportOrdersSalesFn(ctx, rows)
	}
	return &inventory.OrdersImportResult{}, nil
}

func (m *MockImportService) ImportEbayOrdersSales(ctx context.Context, rows []inventory.EbayOrderRow) (*inventory.OrdersImportResult, error) {
	if m.ImportEbayOrdersSalesFn != nil {
		return m.ImportEbayOrdersSalesFn(ctx, rows)
	}
	return &inventory.OrdersImportResult{}, nil
}

func (m *MockImportService) ConfirmOrdersSales(ctx context.Context, items []inventory.OrdersConfirmItem) (*inventory.BulkSaleResult, error) {
	if m.ConfirmOrdersSalesFn != nil {
		return m.ConfirmOrdersSalesFn(ctx, items)
	}
	return &inventory.BulkSaleResult{}, nil
}

func (m *MockImportService) ImportCerts(ctx context.Context, certNumbers []string) (*inventory.CertImportResult, error) {
	if m.ImportCertsFn != nil {
		return m.ImportCertsFn(ctx, certNumbers)
	}
	return &inventory.CertImportResult{}, nil
}

func (m *MockImportService) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error) {
	if m.GetPurchasesByCertNumbersFn != nil {
		return m.GetPurchasesByCertNumbersFn(ctx, certNumbers)
	}
	return map[string]*inventory.Purchase{}, nil
}

func (m *MockImportService) ScanCert(ctx context.Context, certNumber string) (*inventory.ScanCertResult, error) {
	if m.ScanCertFn != nil {
		return m.ScanCertFn(ctx, certNumber)
	}
	return &inventory.ScanCertResult{}, nil
}

func (m *MockImportService) ScanCerts(ctx context.Context, certNumbers []string) (*inventory.ScanCertsResult, error) {
	if m.ScanCertsFn != nil {
		return m.ScanCertsFn(ctx, certNumbers)
	}
	return &inventory.ScanCertsResult{Results: map[string]*inventory.ScanCertResult{}}, nil
}

func (m *MockImportService) ResolveCert(ctx context.Context, certNumber string) (*inventory.CertInfo, error) {
	if m.ResolveCertFn != nil {
		return m.ResolveCertFn(ctx, certNumber)
	}
	return nil, nil
}
