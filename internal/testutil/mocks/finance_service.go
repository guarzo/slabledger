package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/finance"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

type MockFinanceService struct {
	GetCapitalSummaryFn       func(ctx context.Context) (*inventory.CapitalSummary, error)
	GetCashflowConfigFn       func(ctx context.Context) (*inventory.CashflowConfig, error)
	UpdateCashflowConfigFn    func(ctx context.Context, cfg *inventory.CashflowConfig) error
	ListInvoicesFn            func(ctx context.Context) ([]inventory.Invoice, error)
	UpdateInvoiceFn           func(ctx context.Context, inv *inventory.Invoice) error
	FlagForRevocationFn       func(ctx context.Context, segmentLabel, segmentDimension, reason string) (*inventory.RevocationFlag, error)
	ListRevocationFlagsFn     func(ctx context.Context) ([]inventory.RevocationFlag, error)
	GenerateRevocationEmailFn func(ctx context.Context, flagID string) (string, error)
}

var _ finance.Service = (*MockFinanceService)(nil)

func (m *MockFinanceService) GetCapitalSummary(ctx context.Context) (*inventory.CapitalSummary, error) {
	if m.GetCapitalSummaryFn != nil {
		return m.GetCapitalSummaryFn(ctx)
	}
	return &inventory.CapitalSummary{}, nil
}

func (m *MockFinanceService) GetCashflowConfig(ctx context.Context) (*inventory.CashflowConfig, error) {
	if m.GetCashflowConfigFn != nil {
		return m.GetCashflowConfigFn(ctx)
	}
	return &inventory.CashflowConfig{}, nil
}

func (m *MockFinanceService) UpdateCashflowConfig(ctx context.Context, cfg *inventory.CashflowConfig) error {
	if m.UpdateCashflowConfigFn != nil {
		return m.UpdateCashflowConfigFn(ctx, cfg)
	}
	return nil
}

func (m *MockFinanceService) ListInvoices(ctx context.Context) ([]inventory.Invoice, error) {
	if m.ListInvoicesFn != nil {
		return m.ListInvoicesFn(ctx)
	}
	return []inventory.Invoice{}, nil
}

func (m *MockFinanceService) UpdateInvoice(ctx context.Context, inv *inventory.Invoice) error {
	if m.UpdateInvoiceFn != nil {
		return m.UpdateInvoiceFn(ctx, inv)
	}
	return nil
}

func (m *MockFinanceService) FlagForRevocation(ctx context.Context, segmentLabel, segmentDimension, reason string) (*inventory.RevocationFlag, error) {
	if m.FlagForRevocationFn != nil {
		return m.FlagForRevocationFn(ctx, segmentLabel, segmentDimension, reason)
	}
	return &inventory.RevocationFlag{}, nil
}

func (m *MockFinanceService) ListRevocationFlags(ctx context.Context) ([]inventory.RevocationFlag, error) {
	if m.ListRevocationFlagsFn != nil {
		return m.ListRevocationFlagsFn(ctx)
	}
	return []inventory.RevocationFlag{}, nil
}

func (m *MockFinanceService) GenerateRevocationEmail(ctx context.Context, flagID string) (string, error) {
	if m.GenerateRevocationEmailFn != nil {
		return m.GenerateRevocationEmailFn(ctx, flagID)
	}
	return "", nil
}
