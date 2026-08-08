package finance_test

import (
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/finance"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// now is the injected "current time" for every case below. Deliberately not UTC
// midnight so the UTC normalization inside ComputeInvoiceProjection is exercised.
var projectionNow = time.Date(2026, 3, 10, 18, 30, 0, 0, time.UTC)

func TestComputeInvoiceProjection(t *testing.T) {
	tests := []struct {
		name     string
		invoices []inventory.Invoice
		want     finance.InvoiceProjection
	}{
		{
			name:     "empty list — zero projection",
			invoices: nil,
			want:     finance.InvoiceProjection{},
		},
		{
			name: "all paid — zero projection",
			invoices: []inventory.Invoice{
				{ID: "a", InvoiceDate: "2026-03-01", DueDate: "2026-03-15", Status: "paid", TotalCents: 5000},
			},
			want: finance.InvoiceProjection{},
		},
		{
			name: "empty due date is skipped in favor of the next candidate",
			invoices: []inventory.Invoice{
				{ID: "a", InvoiceDate: "2026-03-01", DueDate: "", TotalCents: 5000},
				{ID: "b", InvoiceDate: "2026-03-02", DueDate: "2026-03-20", TotalCents: 7000},
			},
			want: finance.InvoiceProjection{
				NextInvoiceDate:        "2026-03-02",
				NextInvoiceDueDate:     "2026-03-20",
				NextInvoiceAmountCents: 7000,
				DaysUntilInvoiceDue:    10,
			},
		},
		{
			name: "unparseable due date is skipped",
			invoices: []inventory.Invoice{
				{ID: "a", InvoiceDate: "2026-03-01", DueDate: "not-a-date", TotalCents: 5000},
				{ID: "b", InvoiceDate: "2026-03-02", DueDate: "2026-03-20", TotalCents: 7000},
			},
			want: finance.InvoiceProjection{
				NextInvoiceDate:        "2026-03-02",
				NextInvoiceDueDate:     "2026-03-20",
				NextInvoiceAmountCents: 7000,
				DaysUntilInvoiceDue:    10,
			},
		},
		{
			name: "earliest due date wins regardless of input order",
			invoices: []inventory.Invoice{
				{ID: "late", InvoiceDate: "2026-03-01", DueDate: "2026-04-01", TotalCents: 9000},
				{ID: "early", InvoiceDate: "2026-03-02", DueDate: "2026-03-12", TotalCents: 3000},
			},
			want: finance.InvoiceProjection{
				NextInvoiceDate:        "2026-03-02",
				NextInvoiceDueDate:     "2026-03-12",
				NextInvoiceAmountCents: 3000,
				DaysUntilInvoiceDue:    2,
			},
		},
		{
			name: "overdue invoice is picked with a negative day count",
			invoices: []inventory.Invoice{
				{ID: "a", InvoiceDate: "2026-02-01", DueDate: "2026-03-05", TotalCents: 4000},
			},
			want: finance.InvoiceProjection{
				NextInvoiceDate:        "2026-02-01",
				NextInvoiceDueDate:     "2026-03-05",
				NextInvoiceAmountCents: 4000,
				DaysUntilInvoiceDue:    -5,
			},
		},
		{
			name: "amount owed is total minus paid",
			invoices: []inventory.Invoice{
				{ID: "a", InvoiceDate: "2026-03-01", DueDate: "2026-03-20", TotalCents: 10000, PaidCents: 4000},
			},
			want: finance.InvoiceProjection{
				NextInvoiceDate:        "2026-03-01",
				NextInvoiceDueDate:     "2026-03-20",
				NextInvoiceAmountCents: 6000,
				DaysUntilInvoiceDue:    10,
			},
		},
		{
			name: "overpayment clamps amount owed to zero rather than going negative",
			invoices: []inventory.Invoice{
				{ID: "a", InvoiceDate: "2026-03-01", DueDate: "2026-03-20", TotalCents: 10000, PaidCents: 12000},
			},
			want: finance.InvoiceProjection{
				NextInvoiceDate:        "2026-03-01",
				NextInvoiceDueDate:     "2026-03-20",
				NextInvoiceAmountCents: 0,
				DaysUntilInvoiceDue:    10,
			},
		},
		{
			name: "due-date tie is broken by invoice date, then by ID",
			invoices: []inventory.Invoice{
				{ID: "z", InvoiceDate: "2026-03-01", DueDate: "2026-03-20", TotalCents: 1000},
				{ID: "a", InvoiceDate: "2026-03-01", DueDate: "2026-03-20", TotalCents: 2000},
				{ID: "b", InvoiceDate: "2026-02-01", DueDate: "2026-03-20", TotalCents: 3000},
			},
			want: finance.InvoiceProjection{
				NextInvoiceDate:        "2026-02-01",
				NextInvoiceDueDate:     "2026-03-20",
				NextInvoiceAmountCents: 3000,
				DaysUntilInvoiceDue:    10,
			},
		},
		{
			name: "due date equal to today yields zero days until due",
			invoices: []inventory.Invoice{
				{ID: "a", InvoiceDate: "2026-03-01", DueDate: "2026-03-10", TotalCents: 1000},
			},
			want: finance.InvoiceProjection{
				NextInvoiceDate:        "2026-03-01",
				NextInvoiceDueDate:     "2026-03-10",
				NextInvoiceAmountCents: 1000,
				DaysUntilInvoiceDue:    0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := finance.ComputeInvoiceProjection(tt.invoices, projectionNow)
			if got != tt.want {
				t.Errorf("ComputeInvoiceProjection() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// A non-UTC "now" near a day boundary must resolve to the same calendar day as
// the UTC-midnight due dates, so DaysUntilInvoiceDue does not skew by one.
func TestComputeInvoiceProjection_NonUTCNow(t *testing.T) {
	// 2026-03-10 23:30 in UTC-8 is 2026-03-11 07:30 UTC.
	loc := time.FixedZone("UTC-8", -8*60*60)
	now := time.Date(2026, 3, 10, 23, 30, 0, 0, loc)

	invoices := []inventory.Invoice{
		{ID: "a", InvoiceDate: "2026-03-01", DueDate: "2026-03-20", TotalCents: 1000},
	}

	got := finance.ComputeInvoiceProjection(invoices, now)
	if got.DaysUntilInvoiceDue != 9 {
		t.Errorf("DaysUntilInvoiceDue = %d, want 9 (UTC day is 2026-03-11)", got.DaysUntilInvoiceDue)
	}
}
