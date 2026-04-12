package finance

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// FinanceReader handles finance-related persistence and queries.
type FinanceReader interface {
	// Invoices
	CreateInvoice(ctx context.Context, inv *inventory.Invoice) error
	GetInvoice(ctx context.Context, id string) (*inventory.Invoice, error)
	ListInvoices(ctx context.Context) ([]inventory.Invoice, error)
	UpdateInvoice(ctx context.Context, inv *inventory.Invoice) error
	SumPurchaseCostByInvoiceDate(ctx context.Context, invoiceDate string) (int, error)
	GetPendingReceiptByInvoiceDate(ctx context.Context, invoiceDates []string) (map[string]int, error)
	GetInvoiceSellThrough(ctx context.Context, invoiceDate string) (inventory.InvoiceSellThrough, error)

	// Cashflow config
	GetCashflowConfig(ctx context.Context) (*inventory.CashflowConfig, error)
	UpdateCashflowConfig(ctx context.Context, cfg *inventory.CashflowConfig) error
	GetCapitalRawData(ctx context.Context) (*inventory.CapitalRawData, error)

	// Revocation flags
	CreateRevocationFlag(ctx context.Context, flag *inventory.RevocationFlag) error
	ListRevocationFlags(ctx context.Context) ([]inventory.RevocationFlag, error)
	GetLatestRevocationFlag(ctx context.Context) (*inventory.RevocationFlag, error)
	GetRevocationFlagByID(ctx context.Context, id string) (*inventory.RevocationFlag, error)
	UpdateRevocationFlagStatus(ctx context.Context, id string, status string, sentAt *time.Time) error
}
