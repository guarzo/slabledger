package campaigns

import (
	"context"
	"fmt"
	"time"
)

// --- Capital & Invoice ---

func (s *service) GetCapitalSummary(ctx context.Context) (*CapitalSummary, error) {
	raw, err := s.repo.GetCapitalRawData(ctx)
	if err != nil {
		return nil, err
	}
	return ComputeCapitalSummary(raw), nil
}

func (s *service) GetCashflowConfig(ctx context.Context) (*CashflowConfig, error) {
	return s.repo.GetCashflowConfig(ctx)
}

func (s *service) ListInvoices(ctx context.Context) ([]Invoice, error) {
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
	}
	return invoices, nil
}

func (s *service) UpdateInvoice(ctx context.Context, inv *Invoice) error {
	inv.UpdatedAt = time.Now()
	return s.repo.UpdateInvoice(ctx, inv)
}

// --- Revocation Flags ---

func (s *service) FlagForRevocation(ctx context.Context, segmentLabel, segmentDimension, reason string) (*RevocationFlag, error) {
	// Check one-per-week constraint
	latest, err := s.repo.GetLatestRevocationFlag(ctx)
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
	if err := s.repo.CreateRevocationFlag(ctx, flag); err != nil {
		return nil, fmt.Errorf("create revocation flag: %w", err)
	}
	return flag, nil
}

func (s *service) ListRevocationFlags(ctx context.Context) ([]RevocationFlag, error) {
	return s.repo.ListRevocationFlags(ctx)
}

func (s *service) GenerateRevocationEmail(ctx context.Context, flagID string) (string, error) {
	flags, err := s.repo.ListRevocationFlags(ctx)
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
