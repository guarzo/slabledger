package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestFinanceStore_InvoiceChargesExcludeNewSourcingFees(t *testing.T) {
	ctx := context.Background()
	prior := requireTestDB(t)
	if _, err := prior.ExecContext(ctx, `INSERT INTO invoices (id, invoice_date, paid_cents, status)
		VALUES ('finance-cutover-prior', '2026-08-15', 200, 'paid')`); err != nil {
		t.Fatal(err)
	}
	db := setupTestDB(t) // campaign cleanup does not cascade to invoices
	if _, err := db.ExecContext(ctx, `TRUNCATE TABLE invoices`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO campaigns (id, name) VALUES ('test', 'Test')`); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		id, date string
		cost     int
		refunded bool
	}{
		{"old", "2026-09-15", 10000, false},
		{"oct", "2026-10-01", 10000, false},
		{"new", "2026-10-15", 35000, false},
		{"refund", "2026-10-15", 5000, true},
	}
	for _, r := range rows {
		_, err := db.ExecContext(ctx, `INSERT INTO campaign_purchases
			(id, campaign_id, card_name, cert_number, purchase_date, invoice_date, buy_cost_cents, psa_sourcing_fee_cents, was_refunded)
			VALUES ($1, 'test', 'Card', $1, $2, $2, $3, 300, $4)`, r.id, r.date, r.cost, r.refunded)
		if err != nil {
			t.Fatal(err)
		}
	}
	store := NewFinanceStore(db.DB, mocks.NewMockLogger())
	for _, tc := range []struct {
		date string
		want int
	}{
		{"2026-09-15", 10300}, {"2026-10-01", 10000}, {"2026-10-15", 35000},
	} {
		got, err := store.SumPurchaseCostByInvoiceDate(ctx, tc.date)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("invoice %s = %d, want %d", tc.date, got, tc.want)
		}
	}
	capital, err := store.GetCapitalRawData(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if capital.OutstandingCents != 55300 {
		t.Errorf("outstanding = %d, want 55300", capital.OutstandingCents)
	}
	if capital.RefundedCents != 5000 {
		t.Errorf("refunded = %d, want 5000", capital.RefundedCents)
	}
}
