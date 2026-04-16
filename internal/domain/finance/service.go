package finance

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// Service defines finance-related operations.
type Service interface {
	GetCapitalSummary(ctx context.Context) (*inventory.CapitalSummary, error)
	GetCashflowConfig(ctx context.Context) (*inventory.CashflowConfig, error)
	UpdateCashflowConfig(ctx context.Context, cfg *inventory.CashflowConfig) error
	ListInvoices(ctx context.Context) ([]inventory.Invoice, error)
	UpdateInvoice(ctx context.Context, inv *inventory.Invoice) error
	FlagForRevocation(ctx context.Context, segmentLabel, segmentDimension, reason string) (*inventory.RevocationFlag, error)
	ListRevocationFlags(ctx context.Context) ([]inventory.RevocationFlag, error)
	GenerateRevocationEmail(ctx context.Context, flagID string) (string, error)
}

type service struct {
	repo  FinanceReader
	idGen func() string
}

// New creates a new finance service.
func New(repo FinanceReader, idGen func() string) Service {
	return &service{
		repo:  repo,
		idGen: idGen,
	}
}

// --- Capital & Invoice ---

func (s *service) GetCapitalSummary(ctx context.Context) (*inventory.CapitalSummary, error) {
	raw, err := s.repo.GetCapitalRawData(ctx)
	if err != nil {
		return nil, err
	}
	summary := inventory.ComputeCapitalSummary(raw)

	invoices, err := s.repo.ListInvoices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list invoices for projection: %w", err)
	}
	projection := inventory.ComputeInvoiceProjection(invoices, time.Now())
	summary.NextInvoiceDate = projection.NextInvoiceDate
	summary.NextInvoiceDueDate = projection.NextInvoiceDueDate
	summary.NextInvoiceAmountCents = projection.NextInvoiceAmountCents
	summary.DaysUntilInvoiceDue = projection.DaysUntilInvoiceDue

	if projection.NextInvoiceDate != "" {
		// Pending receipt for this invoice's purchases
		pendingMap, err := s.repo.GetPendingReceiptByInvoiceDate(ctx, []string{projection.NextInvoiceDate})
		if err != nil {
			return nil, fmt.Errorf("get pending receipt for invoice: %w", err)
		}
		summary.NextInvoicePendingReceiptCents = pendingMap[projection.NextInvoiceDate]

		// Sell-through for returned purchases on this invoice
		sellThrough, err := s.repo.GetInvoiceSellThrough(ctx, projection.NextInvoiceDate)
		if err != nil {
			return nil, fmt.Errorf("get invoice sell-through: %w", err)
		}
		summary.NextInvoiceSellThrough = sellThrough
	}

	return summary, nil
}

func (s *service) GetCashflowConfig(ctx context.Context) (*inventory.CashflowConfig, error) {
	return s.repo.GetCashflowConfig(ctx)
}

// UpdateCashflowConfig persists operator-set capital budget and cash buffer values.
// Both values are validated as non-negative; nil config is rejected.
func (s *service) UpdateCashflowConfig(ctx context.Context, cfg *inventory.CashflowConfig) error {
	if cfg == nil {
		return fmt.Errorf("cashflow config: %w", inventory.ErrInvalidCashflowConfig)
	}
	if cfg.CapitalBudgetCents < 0 || cfg.CashBufferCents < 0 {
		return inventory.ErrInvalidCashflowConfig
	}
	cfg.UpdatedAt = time.Now()
	return s.repo.UpdateCashflowConfig(ctx, cfg)
}

func (s *service) ListInvoices(ctx context.Context) ([]inventory.Invoice, error) {
	invoices, err := s.repo.ListInvoices(ctx)
	if err != nil {
		return nil, err
	}
	if len(invoices) == 0 {
		return invoices, nil
	}

	dates := make([]string, len(invoices))
	for i, inv := range invoices {
		dates[i] = inv.InvoiceDate
	}

	pending, err := s.repo.GetPendingReceiptByInvoiceDate(ctx, dates)
	if err != nil {
		return nil, fmt.Errorf("get pending receipt: %w", err)
	}

	for i := range invoices {
		invoices[i].PendingReceiptCents = pending[invoices[i].InvoiceDate]

		// Backfill TotalCents for legacy/manually-created invoices that were never
		// populated on PSA import. Pure in-memory enrichment — no write-back.
		if invoices[i].TotalCents == 0 && invoices[i].InvoiceDate != "" {
			total, err := s.repo.SumPurchaseCostByInvoiceDate(ctx, invoices[i].InvoiceDate)
			if err != nil {
				return nil, fmt.Errorf("backfill invoice total for %s: %w", invoices[i].InvoiceDate, err)
			}
			invoices[i].TotalCents = total
		}
	}
	return invoices, nil
}

func (s *service) UpdateInvoice(ctx context.Context, inv *inventory.Invoice) error {
	inv.UpdatedAt = time.Now()
	return s.repo.UpdateInvoice(ctx, inv)
}

// --- Revocation Flags ---

func (s *service) FlagForRevocation(ctx context.Context, segmentLabel, segmentDimension, reason string) (*inventory.RevocationFlag, error) {
	// Check one-per-week constraint
	latest, err := s.repo.GetLatestRevocationFlag(ctx)
	if err != nil {
		return nil, fmt.Errorf("check latest revocation flag: %w", err)
	}
	if latest != nil && time.Since(latest.CreatedAt) < 7*24*time.Hour {
		return nil, inventory.ErrRevocationTooSoon
	}

	flag := &inventory.RevocationFlag{
		ID:               s.idGen(),
		SegmentLabel:     segmentLabel,
		SegmentDimension: segmentDimension,
		Reason:           reason,
		Status:           "pending",
		CreatedAt:        time.Now(),
	}
	if err := s.repo.CreateRevocationFlag(ctx, flag); err != nil {
		return nil, fmt.Errorf("create revocation flag: %w", err)
	}
	return flag, nil
}

func (s *service) ListRevocationFlags(ctx context.Context) ([]inventory.RevocationFlag, error) {
	return s.repo.ListRevocationFlags(ctx)
}

func (s *service) GenerateRevocationEmail(ctx context.Context, flagID string) (string, error) {
	flag, err := s.repo.GetRevocationFlagByID(ctx, flagID)
	if err != nil {
		return "", fmt.Errorf("get revocation flag: %w", err)
	}
	if flag == nil {
		return "", fmt.Errorf("revocation flag not found: %s", flagID)
	}

	email := fmt.Sprintf(
		"Subject: PSA Direct Buy Revocation Request\n\nDear PSA Team,\n\nI would like to revoke the following from my buying selections:\n\nSegment: %s (%s)\nReason: %s\n\nPlease process this at your earliest convenience.\n\nThank you.",
		flag.SegmentLabel, flag.SegmentDimension, flag.Reason,
	)
	return email, nil
}
