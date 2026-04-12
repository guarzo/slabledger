package inventory

import (
	"context"
	"fmt"
	"time"
)

// --- Capital & Invoice ---

func (s *service) GetCapitalSummary(ctx context.Context) (*CapitalSummary, error) {
	raw, err := s.finance.GetCapitalRawData(ctx)
	if err != nil {
		return nil, err
	}
	summary := ComputeCapitalSummary(raw)

	invoices, err := s.finance.ListInvoices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list invoices for projection: %w", err)
	}
	projection := ComputeInvoiceProjection(invoices, time.Now())
	summary.NextInvoiceDate = projection.NextInvoiceDate
	summary.NextInvoiceDueDate = projection.NextInvoiceDueDate
	summary.NextInvoiceAmountCents = projection.NextInvoiceAmountCents
	summary.DaysUntilInvoiceDue = projection.DaysUntilInvoiceDue

	if projection.NextInvoiceDate != "" {
		// Pending receipt for this invoice's purchases
		pendingMap, err := s.finance.GetPendingReceiptByInvoiceDate(ctx, []string{projection.NextInvoiceDate})
		if err != nil {
			return nil, fmt.Errorf("get pending receipt for invoice: %w", err)
		}
		summary.NextInvoicePendingReceiptCents = pendingMap[projection.NextInvoiceDate]

		// Sell-through for returned purchases on this invoice
		sellThrough, err := s.finance.GetInvoiceSellThrough(ctx, projection.NextInvoiceDate)
		if err != nil {
			return nil, fmt.Errorf("get invoice sell-through: %w", err)
		}
		summary.NextInvoiceSellThrough = sellThrough
	}

	return summary, nil
}

func (s *service) GetCashflowConfig(ctx context.Context) (*CashflowConfig, error) {
	return s.finance.GetCashflowConfig(ctx)
}

// UpdateCashflowConfig persists operator-set capital budget and cash buffer values.
// Both values are validated as non-negative; nil config is rejected.
func (s *service) UpdateCashflowConfig(ctx context.Context, cfg *CashflowConfig) error {
	if cfg == nil {
		return fmt.Errorf("cashflow config: %w", ErrInvalidCashflowConfig)
	}
	if cfg.CapitalBudgetCents < 0 || cfg.CashBufferCents < 0 {
		return ErrInvalidCashflowConfig
	}
	cfg.UpdatedAt = time.Now()
	return s.finance.UpdateCashflowConfig(ctx, cfg)
}

func (s *service) ListInvoices(ctx context.Context) ([]Invoice, error) {
	invoices, err := s.finance.ListInvoices(ctx)
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

	pending, err := s.finance.GetPendingReceiptByInvoiceDate(ctx, dates)
	if err != nil {
		return nil, fmt.Errorf("get pending receipt: %w", err)
	}

	for i := range invoices {
		invoices[i].PendingReceiptCents = pending[invoices[i].InvoiceDate]
	}
	return invoices, nil
}

func (s *service) UpdateInvoice(ctx context.Context, inv *Invoice) error {
	inv.UpdatedAt = time.Now()
	return s.finance.UpdateInvoice(ctx, inv)
}

// --- Revocation Flags ---

func (s *service) FlagForRevocation(ctx context.Context, segmentLabel, segmentDimension, reason string) (*RevocationFlag, error) {
	// Check one-per-week constraint
	latest, err := s.finance.GetLatestRevocationFlag(ctx)
	if err != nil {
		return nil, fmt.Errorf("check latest revocation flag: %w", err)
	}
	if latest != nil && time.Since(latest.CreatedAt) < 7*24*time.Hour {
		return nil, ErrRevocationTooSoon
	}

	flag := &RevocationFlag{
		ID:               s.idGen(),
		SegmentLabel:     segmentLabel,
		SegmentDimension: segmentDimension,
		Reason:           reason,
		Status:           "pending",
		CreatedAt:        time.Now(),
	}
	if err := s.finance.CreateRevocationFlag(ctx, flag); err != nil {
		return nil, fmt.Errorf("create revocation flag: %w", err)
	}
	return flag, nil
}

func (s *service) ListRevocationFlags(ctx context.Context) ([]RevocationFlag, error) {
	return s.finance.ListRevocationFlags(ctx)
}

func (s *service) GenerateRevocationEmail(ctx context.Context, flagID string) (string, error) {
	flags, err := s.finance.ListRevocationFlags(ctx)
	if err != nil {
		return "", fmt.Errorf("list revocation flags: %w", err)
	}

	var flag *RevocationFlag
	for i := range flags {
		if flags[i].ID == flagID {
			flag = &flags[i]
			break
		}
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
