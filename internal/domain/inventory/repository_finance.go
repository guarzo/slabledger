package inventory

import (
	"context"
	"time"
)

// FinanceRepository handles finance-related persistence and queries.
type FinanceRepository interface {
	// Invoices
	CreateInvoice(ctx context.Context, inv *Invoice) error
	GetInvoice(ctx context.Context, id string) (*Invoice, error)
	ListInvoices(ctx context.Context) ([]Invoice, error)
	UpdateInvoice(ctx context.Context, inv *Invoice) error
	SumPurchaseCostByInvoiceDate(ctx context.Context, invoiceDate string) (int, error)
	GetPendingReceiptByInvoiceDate(ctx context.Context, invoiceDates []string) (map[string]int, error)
	GetInvoiceSellThrough(ctx context.Context, invoiceDate string) (InvoiceSellThrough, error)

	// Cashflow config
	GetCashflowConfig(ctx context.Context) (*CashflowConfig, error)
	UpdateCashflowConfig(ctx context.Context, cfg *CashflowConfig) error
	GetCapitalRawData(ctx context.Context) (*CapitalRawData, error)

	// Revocation flags
	CreateRevocationFlag(ctx context.Context, flag *RevocationFlag) error
	ListRevocationFlags(ctx context.Context) ([]RevocationFlag, error)
	GetLatestRevocationFlag(ctx context.Context) (*RevocationFlag, error)
	UpdateRevocationFlagStatus(ctx context.Context, id string, status string, sentAt *time.Time) error
}
