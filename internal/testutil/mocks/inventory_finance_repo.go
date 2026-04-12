package mocks

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// FinanceRepositoryMock implements inventory.FinanceRepository with Fn-field pattern.
type FinanceRepositoryMock struct {
	CreateInvoiceFn                  func(ctx context.Context, inv *inventory.Invoice) error
	GetInvoiceFn                     func(ctx context.Context, id string) (*inventory.Invoice, error)
	ListInvoicesFn                   func(ctx context.Context) ([]inventory.Invoice, error)
	UpdateInvoiceFn                  func(ctx context.Context, inv *inventory.Invoice) error
	SumPurchaseCostByInvoiceDateFn   func(ctx context.Context, invoiceDate string) (int, error)
	GetPendingReceiptByInvoiceDateFn func(ctx context.Context, invoiceDates []string) (map[string]int, error)
	GetInvoiceSellThroughFn          func(ctx context.Context, invoiceDate string) (inventory.InvoiceSellThrough, error)
	GetCashflowConfigFn              func(ctx context.Context) (*inventory.CashflowConfig, error)
	UpdateCashflowConfigFn           func(ctx context.Context, cfg *inventory.CashflowConfig) error
	GetCapitalRawDataFn              func(ctx context.Context) (*inventory.CapitalRawData, error)
	CreateRevocationFlagFn           func(ctx context.Context, flag *inventory.RevocationFlag) error
	ListRevocationFlagsFn            func(ctx context.Context) ([]inventory.RevocationFlag, error)
	GetLatestRevocationFlagFn        func(ctx context.Context) (*inventory.RevocationFlag, error)
	UpdateRevocationFlagStatusFn     func(ctx context.Context, id string, status string, sentAt *time.Time) error
}

var _ inventory.FinanceRepository = (*FinanceRepositoryMock)(nil)

func (m *FinanceRepositoryMock) CreateInvoice(ctx context.Context, inv *inventory.Invoice) error {
	if m.CreateInvoiceFn != nil {
		return m.CreateInvoiceFn(ctx, inv)
	}
	return nil
}

func (m *FinanceRepositoryMock) GetInvoice(ctx context.Context, id string) (*inventory.Invoice, error) {
	if m.GetInvoiceFn != nil {
		return m.GetInvoiceFn(ctx, id)
	}
	return nil, inventory.ErrInvoiceNotFound
}

func (m *FinanceRepositoryMock) ListInvoices(ctx context.Context) ([]inventory.Invoice, error) {
	if m.ListInvoicesFn != nil {
		return m.ListInvoicesFn(ctx)
	}
	return []inventory.Invoice{}, nil
}

func (m *FinanceRepositoryMock) UpdateInvoice(ctx context.Context, inv *inventory.Invoice) error {
	if m.UpdateInvoiceFn != nil {
		return m.UpdateInvoiceFn(ctx, inv)
	}
	return nil
}

func (m *FinanceRepositoryMock) SumPurchaseCostByInvoiceDate(ctx context.Context, invoiceDate string) (int, error) {
	if m.SumPurchaseCostByInvoiceDateFn != nil {
		return m.SumPurchaseCostByInvoiceDateFn(ctx, invoiceDate)
	}
	return 0, nil
}

func (m *FinanceRepositoryMock) GetPendingReceiptByInvoiceDate(ctx context.Context, invoiceDates []string) (map[string]int, error) {
	if m.GetPendingReceiptByInvoiceDateFn != nil {
		return m.GetPendingReceiptByInvoiceDateFn(ctx, invoiceDates)
	}
	return map[string]int{}, nil
}

func (m *FinanceRepositoryMock) GetInvoiceSellThrough(ctx context.Context, invoiceDate string) (inventory.InvoiceSellThrough, error) {
	if m.GetInvoiceSellThroughFn != nil {
		return m.GetInvoiceSellThroughFn(ctx, invoiceDate)
	}
	return inventory.InvoiceSellThrough{}, nil
}

func (m *FinanceRepositoryMock) GetCashflowConfig(ctx context.Context) (*inventory.CashflowConfig, error) {
	if m.GetCashflowConfigFn != nil {
		return m.GetCashflowConfigFn(ctx)
	}
	return nil, nil
}

func (m *FinanceRepositoryMock) UpdateCashflowConfig(ctx context.Context, cfg *inventory.CashflowConfig) error {
	if m.UpdateCashflowConfigFn != nil {
		return m.UpdateCashflowConfigFn(ctx, cfg)
	}
	return nil
}

func (m *FinanceRepositoryMock) GetCapitalRawData(ctx context.Context) (*inventory.CapitalRawData, error) {
	if m.GetCapitalRawDataFn != nil {
		return m.GetCapitalRawDataFn(ctx)
	}
	return nil, nil
}

func (m *FinanceRepositoryMock) CreateRevocationFlag(ctx context.Context, flag *inventory.RevocationFlag) error {
	if m.CreateRevocationFlagFn != nil {
		return m.CreateRevocationFlagFn(ctx, flag)
	}
	return nil
}

func (m *FinanceRepositoryMock) ListRevocationFlags(ctx context.Context) ([]inventory.RevocationFlag, error) {
	if m.ListRevocationFlagsFn != nil {
		return m.ListRevocationFlagsFn(ctx)
	}
	return []inventory.RevocationFlag{}, nil
}

func (m *FinanceRepositoryMock) GetLatestRevocationFlag(ctx context.Context) (*inventory.RevocationFlag, error) {
	if m.GetLatestRevocationFlagFn != nil {
		return m.GetLatestRevocationFlagFn(ctx)
	}
	return nil, inventory.ErrRevocationFlagNotFound
}

func (m *FinanceRepositoryMock) UpdateRevocationFlagStatus(ctx context.Context, id string, status string, sentAt *time.Time) error {
	if m.UpdateRevocationFlagStatusFn != nil {
		return m.UpdateRevocationFlagStatusFn(ctx, id, status, sentAt)
	}
	return nil
}
