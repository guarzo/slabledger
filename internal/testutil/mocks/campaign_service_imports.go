package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// MockImportService is a test double for campaigns.ImportService.
// Each method delegates to a function field, allowing per-test configuration.
//
// Example:
//
//	svc := &MockImportService{
//	    ImportCertsFn: func(ctx context.Context, certNumbers []string) (*campaigns.CertImportResult, error) {
//	        return &campaigns.CertImportResult{}, nil
//	    },
//	}
type MockImportService struct {
	RefreshCLValuesGlobalFn     func(ctx context.Context, rows []campaigns.CLExportRow) (*campaigns.GlobalCLRefreshResult, error)
	ImportCLExportGlobalFn      func(ctx context.Context, rows []campaigns.CLExportRow) (*campaigns.GlobalImportResult, error)
	ImportPSAExportGlobalFn     func(ctx context.Context, rows []campaigns.PSAExportRow) (*campaigns.PSAImportResult, error)
	ExportCLFormatGlobalFn      func(ctx context.Context, missingCLOnly bool) ([]campaigns.CLExportEntry, error)
	EnsureExternalCampaignFn    func(ctx context.Context) (*campaigns.Campaign, error)
	ImportExternalCSVFn         func(ctx context.Context, rows []campaigns.ShopifyExportRow) (*campaigns.ExternalImportResult, error)
	ImportOrdersSalesFn         func(ctx context.Context, rows []campaigns.OrdersExportRow) (*campaigns.OrdersImportResult, error)
	ConfirmOrdersSalesFn        func(ctx context.Context, items []campaigns.OrdersConfirmItem) (*campaigns.BulkSaleResult, error)
	ImportCertsFn               func(ctx context.Context, certNumbers []string) (*campaigns.CertImportResult, error)
	GetPurchasesByCertNumbersFn func(ctx context.Context, certNumbers []string) (map[string]*campaigns.Purchase, error)
	ScanCertFn                  func(ctx context.Context, certNumber string) (*campaigns.ScanCertResult, error)
	ResolveCertFn               func(ctx context.Context, certNumber string) (*campaigns.CertInfo, error)
}

var _ campaigns.ImportService = (*MockImportService)(nil)

func (m *MockImportService) RefreshCLValuesGlobal(ctx context.Context, rows []campaigns.CLExportRow) (*campaigns.GlobalCLRefreshResult, error) {
	if m.RefreshCLValuesGlobalFn != nil {
		return m.RefreshCLValuesGlobalFn(ctx, rows)
	}
	return &campaigns.GlobalCLRefreshResult{}, nil
}

func (m *MockImportService) ImportCLExportGlobal(ctx context.Context, rows []campaigns.CLExportRow) (*campaigns.GlobalImportResult, error) {
	if m.ImportCLExportGlobalFn != nil {
		return m.ImportCLExportGlobalFn(ctx, rows)
	}
	return &campaigns.GlobalImportResult{}, nil
}

func (m *MockImportService) ImportPSAExportGlobal(ctx context.Context, rows []campaigns.PSAExportRow) (*campaigns.PSAImportResult, error) {
	if m.ImportPSAExportGlobalFn != nil {
		return m.ImportPSAExportGlobalFn(ctx, rows)
	}
	return &campaigns.PSAImportResult{}, nil
}

func (m *MockImportService) ExportCLFormatGlobal(ctx context.Context, missingCLOnly bool) ([]campaigns.CLExportEntry, error) {
	if m.ExportCLFormatGlobalFn != nil {
		return m.ExportCLFormatGlobalFn(ctx, missingCLOnly)
	}
	return []campaigns.CLExportEntry{}, nil
}

func (m *MockImportService) EnsureExternalCampaign(ctx context.Context) (*campaigns.Campaign, error) {
	if m.EnsureExternalCampaignFn != nil {
		return m.EnsureExternalCampaignFn(ctx)
	}
	return &campaigns.Campaign{}, nil
}

func (m *MockImportService) ImportExternalCSV(ctx context.Context, rows []campaigns.ShopifyExportRow) (*campaigns.ExternalImportResult, error) {
	if m.ImportExternalCSVFn != nil {
		return m.ImportExternalCSVFn(ctx, rows)
	}
	return &campaigns.ExternalImportResult{}, nil
}

func (m *MockImportService) ImportOrdersSales(ctx context.Context, rows []campaigns.OrdersExportRow) (*campaigns.OrdersImportResult, error) {
	if m.ImportOrdersSalesFn != nil {
		return m.ImportOrdersSalesFn(ctx, rows)
	}
	return &campaigns.OrdersImportResult{}, nil
}

func (m *MockImportService) ConfirmOrdersSales(ctx context.Context, items []campaigns.OrdersConfirmItem) (*campaigns.BulkSaleResult, error) {
	if m.ConfirmOrdersSalesFn != nil {
		return m.ConfirmOrdersSalesFn(ctx, items)
	}
	return &campaigns.BulkSaleResult{}, nil
}

func (m *MockImportService) ImportCerts(ctx context.Context, certNumbers []string) (*campaigns.CertImportResult, error) {
	if m.ImportCertsFn != nil {
		return m.ImportCertsFn(ctx, certNumbers)
	}
	return &campaigns.CertImportResult{}, nil
}

func (m *MockImportService) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*campaigns.Purchase, error) {
	if m.GetPurchasesByCertNumbersFn != nil {
		return m.GetPurchasesByCertNumbersFn(ctx, certNumbers)
	}
	return map[string]*campaigns.Purchase{}, nil
}

func (m *MockImportService) ScanCert(ctx context.Context, certNumber string) (*campaigns.ScanCertResult, error) {
	if m.ScanCertFn != nil {
		return m.ScanCertFn(ctx, certNumber)
	}
	return &campaigns.ScanCertResult{}, nil
}

func (m *MockImportService) ResolveCert(ctx context.Context, certNumber string) (*campaigns.CertInfo, error) {
	if m.ResolveCertFn != nil {
		return m.ResolveCertFn(ctx, certNumber)
	}
	return nil, nil
}
