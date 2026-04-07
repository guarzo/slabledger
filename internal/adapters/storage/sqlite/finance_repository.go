package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// --- Invoices ---

func (r *CampaignsRepository) CreateInvoice(ctx context.Context, inv *campaigns.Invoice) error {
	query := `INSERT INTO invoices (id, invoice_date, total_cents, paid_cents, due_date, paid_date, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query,
		inv.ID, inv.InvoiceDate, inv.TotalCents, inv.PaidCents,
		inv.DueDate, inv.PaidDate, inv.Status, inv.CreatedAt, inv.UpdatedAt,
	)
	return err
}

func (r *CampaignsRepository) GetInvoice(ctx context.Context, id string) (*campaigns.Invoice, error) {
	query := `SELECT id, invoice_date, total_cents, paid_cents, due_date, paid_date, status, created_at, updated_at
		FROM invoices WHERE id = ?`
	var inv campaigns.Invoice
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&inv.ID, &inv.InvoiceDate, &inv.TotalCents, &inv.PaidCents,
		&inv.DueDate, &inv.PaidDate, &inv.Status, &inv.CreatedAt, &inv.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, campaigns.ErrInvoiceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *CampaignsRepository) ListInvoices(ctx context.Context) (result []campaigns.Invoice, err error) {
	query := `SELECT id, invoice_date, total_cents, paid_cents, due_date, paid_date, status, created_at, updated_at
		FROM invoices ORDER BY invoice_date DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	for rows.Next() {
		var inv campaigns.Invoice
		if err := rows.Scan(
			&inv.ID, &inv.InvoiceDate, &inv.TotalCents, &inv.PaidCents,
			&inv.DueDate, &inv.PaidDate, &inv.Status, &inv.CreatedAt, &inv.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, inv)
	}
	return result, rows.Err()
}

func (r *CampaignsRepository) UpdateInvoice(ctx context.Context, inv *campaigns.Invoice) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE invoices SET total_cents = ?, paid_cents = ?, due_date = ?, paid_date = ?, status = ?, updated_at = ? WHERE id = ?`,
		inv.TotalCents, inv.PaidCents, inv.DueDate, inv.PaidDate, inv.Status, inv.UpdatedAt, inv.ID,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return campaigns.ErrInvoiceNotFound
	}
	return nil
}

// SumPurchaseCostByInvoiceDate returns the total (buy_cost_cents + psa_sourcing_fee_cents)
// for all non-refunded purchases with the given invoice date.
func (r *CampaignsRepository) SumPurchaseCostByInvoiceDate(ctx context.Context, invoiceDate string) (int, error) {
	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(buy_cost_cents + psa_sourcing_fee_cents), 0)
		 FROM campaign_purchases
		 WHERE invoice_date = ? AND was_refunded = 0`,
		invoiceDate,
	).Scan(&total)
	return total, err
}

// --- Cashflow Config ---

func (r *CampaignsRepository) GetCashflowConfig(ctx context.Context) (*campaigns.CashflowConfig, error) {
	query := `SELECT credit_limit_cents, cash_buffer_cents, updated_at FROM cashflow_config WHERE id = 1`
	var cfg campaigns.CashflowConfig
	err := r.db.QueryRowContext(ctx, query).Scan(&cfg.CapitalBudgetCents, &cfg.CashBufferCents, &cfg.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return &campaigns.CashflowConfig{CapitalBudgetCents: 0, CashBufferCents: 1000000}, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *CampaignsRepository) UpdateCashflowConfig(ctx context.Context, cfg *campaigns.CashflowConfig) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO cashflow_config (id, credit_limit_cents, cash_buffer_cents, updated_at) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET credit_limit_cents = ?, cash_buffer_cents = ?, updated_at = ?`,
		cfg.CapitalBudgetCents, cfg.CashBufferCents, cfg.UpdatedAt,
		cfg.CapitalBudgetCents, cfg.CashBufferCents, cfg.UpdatedAt,
	)
	return err
}

// --- Capital Summary ---

func (r *CampaignsRepository) GetCapitalSummary(ctx context.Context) (*campaigns.CapitalSummary, error) {
	// Outstanding: invoiced non-refunded purchases minus payments
	var outstanding, refunded int
	err := r.db.QueryRowContext(ctx,
		`SELECT
			COALESCE(SUM(CASE WHEN was_refunded = 0 AND invoice_date != '' THEN buy_cost_cents + psa_sourcing_fee_cents ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN was_refunded = 1 THEN buy_cost_cents + psa_sourcing_fee_cents ELSE 0 END), 0)
		FROM campaign_purchases WHERE invoice_date != ''`,
	).Scan(&outstanding, &refunded)
	if err != nil {
		return nil, err
	}

	// Paid total + unpaid count from invoices
	var paidTotal, unpaidCount int
	err = r.db.QueryRowContext(ctx,
		`SELECT
			COALESCE(SUM(CASE WHEN status IN ('paid', 'partial') THEN paid_cents ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status != 'paid' THEN 1 ELSE 0 END), 0)
		FROM invoices`,
	).Scan(&paidTotal, &unpaidCount)
	if err != nil {
		return nil, err
	}

	outstanding -= paidTotal
	if outstanding < 0 {
		outstanding = 0
	}

	// Recovery velocity: 30-day and prior 30-day sale revenue
	var recovery30d, recoveryPrior30d int
	err = r.db.QueryRowContext(ctx,
		`SELECT
			COALESCE(SUM(CASE WHEN sale_date >= date('now', '-30 days') THEN sale_price_cents ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN sale_date >= date('now', '-60 days') AND sale_date < date('now', '-30 days') THEN sale_price_cents ELSE 0 END), 0)
		FROM campaign_sales`,
	).Scan(&recovery30d, &recoveryPrior30d)
	if err != nil {
		return nil, err
	}

	weeksToCover := campaigns.WeeksToCoverNoData
	if recovery30d > 0 {
		weeklyRate := float64(recovery30d) / campaigns.WeeksPerMonth
		weeksToCover = float64(outstanding) / weeklyRate
	}

	trend := campaigns.TrendStable
	if recovery30d > 0 && recoveryPrior30d == 0 {
		trend = campaigns.TrendImproving
	} else if recovery30d > 0 && recoveryPrior30d > 0 {
		ratio := float64(recovery30d) / float64(recoveryPrior30d)
		if ratio > 1+campaigns.TrendChangeThreshold {
			trend = campaigns.TrendImproving
		} else if ratio < 1-campaigns.TrendChangeThreshold {
			trend = campaigns.TrendDeclining
		}
	}

	alertLevel := campaigns.AlertOK
	if recovery30d > 0 {
		if weeksToCover > campaigns.WeeksToCoverCriticalThreshold {
			alertLevel = campaigns.AlertCritical
		} else if weeksToCover >= campaigns.WeeksToCoverWarningThreshold {
			alertLevel = campaigns.AlertWarning
		}
	} else {
		if outstanding > campaigns.FallbackCriticalCents {
			alertLevel = campaigns.AlertCritical
		} else if outstanding > campaigns.FallbackWarningCents {
			alertLevel = campaigns.AlertWarning
		}
	}

	return &campaigns.CapitalSummary{
		OutstandingCents:          outstanding,
		RecoveryRate30dCents:      recovery30d,
		RecoveryRate30dPriorCents: recoveryPrior30d,
		WeeksToCover:              weeksToCover,
		RecoveryTrend:             trend,
		AlertLevel:                alertLevel,
		RefundedCents:             refunded,
		PaidCents:                 paidTotal,
		UnpaidInvoiceCount:        unpaidCount,
	}, nil
}

// --- Revocation Flags ---

func (r *CampaignsRepository) CreateRevocationFlag(ctx context.Context, flag *campaigns.RevocationFlag) error {
	query := `
		INSERT INTO revocation_flags (id, segment_label, segment_dimension, reason, status, email_text, created_at, sent_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		flag.ID, flag.SegmentLabel, flag.SegmentDimension, flag.Reason,
		flag.Status, flag.EmailText, flag.CreatedAt, flag.SentAt,
	)
	return err
}

func (r *CampaignsRepository) ListRevocationFlags(ctx context.Context) ([]campaigns.RevocationFlag, error) {
	query := `
		SELECT id, segment_label, segment_dimension, reason, status, email_text, created_at, sent_at
		FROM revocation_flags
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	result := make([]campaigns.RevocationFlag, 0, 64)
	for rows.Next() {
		var f campaigns.RevocationFlag
		if err := rows.Scan(
			&f.ID, &f.SegmentLabel, &f.SegmentDimension, &f.Reason,
			&f.Status, &f.EmailText, &f.CreatedAt, &f.SentAt,
		); err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

func (r *CampaignsRepository) GetLatestRevocationFlag(ctx context.Context) (*campaigns.RevocationFlag, error) {
	query := `
		SELECT id, segment_label, segment_dimension, reason, status, email_text, created_at, sent_at
		FROM revocation_flags
		ORDER BY created_at DESC
		LIMIT 1
	`
	var f campaigns.RevocationFlag
	err := r.db.QueryRowContext(ctx, query).Scan(
		&f.ID, &f.SegmentLabel, &f.SegmentDimension, &f.Reason,
		&f.Status, &f.EmailText, &f.CreatedAt, &f.SentAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *CampaignsRepository) UpdateRevocationFlagStatus(ctx context.Context, id string, status string, sentAt *time.Time) error {
	query := `UPDATE revocation_flags SET status = ?, sent_at = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, status, sentAt, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("revocation flag %q: %w", id, campaigns.ErrRevocationFlagNotFound)
	}
	return nil
}
