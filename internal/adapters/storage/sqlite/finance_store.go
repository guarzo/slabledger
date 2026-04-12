package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"strings"
	"time"
)

// FinanceStore implements inventory.FinanceRepository operations.
type FinanceStore struct {
	base
}

// NewFinanceStore creates a new Finance store.
func NewFinanceStore(db *sql.DB, logger observability.Logger) *FinanceStore {
	return &FinanceStore{base{db: db, logger: logger}}
}

var _ inventory.FinanceRepository = (*FinanceStore)(nil)

func (fs *FinanceStore) CreateInvoice(ctx context.Context, inv *inventory.Invoice) error {
	query := `INSERT INTO invoices (id, invoice_date, total_cents, paid_cents, due_date, paid_date, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := fs.db.ExecContext(ctx, query,
		inv.ID, inv.InvoiceDate, inv.TotalCents, inv.PaidCents,
		inv.DueDate, inv.PaidDate, inv.Status, inv.CreatedAt, inv.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create invoice: %w", err)
	}
	return nil
}

func (fs *FinanceStore) GetInvoice(ctx context.Context, id string) (*inventory.Invoice, error) {
	query := `SELECT id, invoice_date, total_cents, paid_cents, due_date, paid_date, status, created_at, updated_at
		FROM invoices WHERE id = ?`
	var inv inventory.Invoice
	err := fs.db.QueryRowContext(ctx, query, id).Scan(
		&inv.ID, &inv.InvoiceDate, &inv.TotalCents, &inv.PaidCents,
		&inv.DueDate, &inv.PaidDate, &inv.Status, &inv.CreatedAt, &inv.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, inventory.ErrInvoiceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (fs *FinanceStore) ListInvoices(ctx context.Context) (result []inventory.Invoice, err error) {
	query := `SELECT id, invoice_date, total_cents, paid_cents, due_date, paid_date, status, created_at, updated_at
		FROM invoices ORDER BY invoice_date DESC`
	rows, err := fs.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	for rows.Next() {
		var inv inventory.Invoice
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

func (fs *FinanceStore) UpdateInvoice(ctx context.Context, inv *inventory.Invoice) error {
	result, err := fs.db.ExecContext(ctx,
		`UPDATE invoices SET total_cents = ?, paid_cents = ?, due_date = ?, paid_date = ?, status = ?, updated_at = ? WHERE id = ?`,
		inv.TotalCents, inv.PaidCents, inv.DueDate, inv.PaidDate, inv.Status, inv.UpdatedAt, inv.ID,
	)
	if err != nil {
		return fmt.Errorf("update invoice: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected for update invoice: %w", err)
	}
	if n == 0 {
		return inventory.ErrInvoiceNotFound
	}
	return nil
}

// SumPurchaseCostByInvoiceDate returns the total (buy_cost_cents + psa_sourcing_fee_cents)
// for all non-refunded purchases with the given invoice date.
func (fs *FinanceStore) SumPurchaseCostByInvoiceDate(ctx context.Context, invoiceDate string) (int, error) {
	var total int
	err := fs.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(buy_cost_cents + psa_sourcing_fee_cents), 0)
		 FROM campaign_purchases
		 WHERE invoice_date = ? AND was_refunded = 0`,
		invoiceDate,
	).Scan(&total)
	return total, err
}

// GetPendingReceiptByInvoiceDate returns the sum of buy_cost_cents for purchases
// that are not yet in-hand, grouped by invoice date. A purchase is considered
// in-hand when it has either been scanned via cert intake (received_at IS NOT NULL)
// or already sold (a row exists in campaign_sales). Refunded purchases are excluded.
// Returns an empty map if invoiceDates is empty.
func (fs *FinanceStore) GetPendingReceiptByInvoiceDate(ctx context.Context, invoiceDates []string) (_ map[string]int, result_err error) {
	if len(invoiceDates) == 0 {
		return map[string]int{}, nil
	}

	placeholders := make([]string, len(invoiceDates))
	args := make([]any, len(invoiceDates))
	for i, d := range invoiceDates {
		placeholders[i] = "?"
		args[i] = d
	}

	query := fmt.Sprintf(
		`SELECT p.invoice_date, COALESCE(SUM(p.buy_cost_cents), 0)
		 FROM campaign_purchases p
		 LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		 WHERE p.invoice_date IN (%s)
		   AND p.received_at IS NULL
		   AND s.id IS NULL
		   AND p.was_refunded = 0
		 GROUP BY p.invoice_date`,
		strings.Join(placeholders, ", "),
	)

	rows, err := fs.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); result_err == nil && cerr != nil {
			result_err = cerr
		}
	}()

	result := make(map[string]int)
	for rows.Next() {
		var date string
		var pending int
		if err := rows.Scan(&date, &pending); err != nil {
			return nil, err
		}
		result[date] = pending
	}
	return result, rows.Err()
}

// --- Cashflow Config ---

func (fs *FinanceStore) GetCashflowConfig(ctx context.Context) (*inventory.CashflowConfig, error) {
	query := `SELECT credit_limit_cents, cash_buffer_cents, updated_at FROM cashflow_config WHERE id = 1`
	var cfg inventory.CashflowConfig
	err := fs.db.QueryRowContext(ctx, query).Scan(&cfg.CapitalBudgetCents, &cfg.CashBufferCents, &cfg.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return &inventory.CashflowConfig{CapitalBudgetCents: 0, CashBufferCents: 1000000}, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (fs *FinanceStore) UpdateCashflowConfig(ctx context.Context, cfg *inventory.CashflowConfig) error {
	_, err := fs.db.ExecContext(ctx,
		`INSERT INTO cashflow_config (id, credit_limit_cents, cash_buffer_cents, updated_at) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET credit_limit_cents = ?, cash_buffer_cents = ?, updated_at = ?`,
		cfg.CapitalBudgetCents, cfg.CashBufferCents, cfg.UpdatedAt,
		cfg.CapitalBudgetCents, cfg.CashBufferCents, cfg.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update cashflow config: %w", err)
	}
	return nil
}

// --- Capital Summary ---

func (fs *FinanceStore) GetCapitalRawData(ctx context.Context) (*inventory.CapitalRawData, error) {
	// Outstanding: invoiced non-refunded purchases minus payments
	var outstanding, refunded int
	err := fs.db.QueryRowContext(ctx,
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
	err = fs.db.QueryRowContext(ctx,
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
	err = fs.db.QueryRowContext(ctx,
		`SELECT
			COALESCE(SUM(CASE WHEN sale_date >= date('now', '-30 days') THEN sale_price_cents ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN sale_date >= date('now', '-60 days') AND sale_date < date('now', '-30 days') THEN sale_price_cents ELSE 0 END), 0)
		FROM campaign_sales`,
	).Scan(&recovery30d, &recoveryPrior30d)
	if err != nil {
		return nil, err
	}

	return &inventory.CapitalRawData{
		OutstandingCents:          outstanding,
		RecoveryRate30dCents:      recovery30d,
		RecoveryRate30dPriorCents: recoveryPrior30d,
		RefundedCents:             refunded,
		PaidCents:                 paidTotal,
		UnpaidInvoiceCount:        unpaidCount,
	}, nil
}

// GetInvoiceSellThrough returns sell-through metrics for all non-refunded,
// returned purchases belonging to the given invoiceDate. Only purchases where
// received_at IS NOT NULL are included (cards still at PSA are excluded).
func (fs *FinanceStore) GetInvoiceSellThrough(ctx context.Context, invoiceDate string) (inventory.InvoiceSellThrough, error) {
	query := `
		SELECT
			COUNT(*) AS total_count,
			COALESCE(SUM(p.buy_cost_cents), 0) AS total_cost_cents,
			COUNT(s.id) AS sold_count,
			COALESCE(SUM(CASE WHEN s.id IS NOT NULL THEN s.sale_price_cents ELSE 0 END), 0) AS sale_revenue_cents
		FROM campaign_purchases p
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE p.invoice_date = ?
		  AND p.was_refunded = 0
		  AND p.received_at IS NOT NULL
	`
	var result inventory.InvoiceSellThrough
	err := fs.db.QueryRowContext(ctx, query, invoiceDate).Scan(
		&result.TotalPurchaseCount,
		&result.TotalCostCents,
		&result.SoldCount,
		&result.SaleRevenueCents,
	)
	if err != nil {
		return inventory.InvoiceSellThrough{}, err
	}
	return result, nil
}

// --- Revocation Flags ---

func (fs *FinanceStore) CreateRevocationFlag(ctx context.Context, flag *inventory.RevocationFlag) error {
	query := `
		INSERT INTO revocation_flags (id, segment_label, segment_dimension, reason, status, email_text, created_at, sent_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := fs.db.ExecContext(ctx, query,
		flag.ID, flag.SegmentLabel, flag.SegmentDimension, flag.Reason,
		flag.Status, flag.EmailText, flag.CreatedAt, flag.SentAt,
	)
	if err != nil {
		return fmt.Errorf("create revocation flag: %w", err)
	}
	return nil
}

func (fs *FinanceStore) ListRevocationFlags(ctx context.Context) ([]inventory.RevocationFlag, error) {
	query := `
		SELECT id, segment_label, segment_dimension, reason, status, email_text, created_at, sent_at
		FROM revocation_flags
		ORDER BY created_at DESC
	`
	rows, err := fs.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	var result []inventory.RevocationFlag
	for rows.Next() {
		var f inventory.RevocationFlag
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

func (fs *FinanceStore) GetLatestRevocationFlag(ctx context.Context) (*inventory.RevocationFlag, error) {
	query := `
		SELECT id, segment_label, segment_dimension, reason, status, email_text, created_at, sent_at
		FROM revocation_flags
		ORDER BY created_at DESC
		LIMIT 1
	`
	var f inventory.RevocationFlag
	err := fs.db.QueryRowContext(ctx, query).Scan(
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

func (fs *FinanceStore) GetRevocationFlagByID(ctx context.Context, id string) (*inventory.RevocationFlag, error) {
	query := `
		SELECT id, segment_label, segment_dimension, reason, status, email_text, created_at, sent_at
		FROM revocation_flags
		WHERE id = ?
	`
	var f inventory.RevocationFlag
	err := fs.db.QueryRowContext(ctx, query, id).Scan(
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

func (fs *FinanceStore) UpdateRevocationFlagStatus(ctx context.Context, id string, status string, sentAt *time.Time) error {
	query := `UPDATE revocation_flags SET status = ?, sent_at = ? WHERE id = ?`
	result, err := fs.db.ExecContext(ctx, query, status, sentAt, id)
	if err != nil {
		return fmt.Errorf("update revocation flag status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected for update revocation flag status: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("revocation flag %q: %w", id, inventory.ErrRevocationFlagNotFound)
	}
	return nil
}
