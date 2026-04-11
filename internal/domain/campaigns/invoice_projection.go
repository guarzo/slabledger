package campaigns

import (
	"sort"
	"time"
)

// InvoiceProjection holds derived next-invoice fields produced by
// ComputeInvoiceProjection. All cent fields are non-negative.
type InvoiceProjection struct {
	NextInvoiceDate        string
	NextInvoiceDueDate     string
	NextInvoiceAmountCents int
	DaysUntilInvoiceDue    int
	ProjectedRecoveryCents int
	ProjectedCashGapCents  int
}

// ComputeInvoiceProjection picks the earliest unpaid invoice with a parseable
// DueDate, then projects sale recovery between now and that due date at the
// caller-supplied 30-day daily rate. Returns a zero-valued projection when no
// qualifying invoice exists.
//
// Edge cases:
//   - Empty invoice list / all invoices paid -> zero projection.
//   - Invoice with empty or unparseable DueDate -> skipped (next candidate wins).
//   - recoveryRate30dCents == 0 -> projected recovery is 0; gap = amount - buffer.
//   - Overdue invoice (due date in the past) -> picked, daysUntilDue and
//     projected recovery reported as 0; gap = amount - buffer.
//
// Parameters:
//
//	invoices             - full invoice list (any order; function sorts internally)
//	recoveryRate30dCents - 30-day rolling sale revenue (cents)
//	cashBufferCents      - operator-set cash buffer (cents)
//	now                  - current time, injected for testability
func ComputeInvoiceProjection(
	invoices []Invoice,
	recoveryRate30dCents int,
	cashBufferCents int,
	now time.Time,
) InvoiceProjection {
	const dueDateLayout = "2006-01-02"

	type candidate struct {
		invoice Invoice
		dueDate time.Time
	}

	// Truncate "now" to a date-only value so daysUntilDue is whole days.
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	candidates := make([]candidate, 0, len(invoices))
	for _, inv := range invoices {
		if inv.Status == "paid" {
			continue
		}
		if inv.DueDate == "" {
			continue
		}
		due, err := time.ParseInLocation(dueDateLayout, inv.DueDate, now.Location())
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{invoice: inv, dueDate: due})
	}

	if len(candidates) == 0 {
		return InvoiceProjection{}
	}

	// Sort ascending by DueDate; ties broken by InvoiceDate then ID for determinism.
	sort.Slice(candidates, func(i, j int) bool {
		if !candidates[i].dueDate.Equal(candidates[j].dueDate) {
			return candidates[i].dueDate.Before(candidates[j].dueDate)
		}
		if candidates[i].invoice.InvoiceDate != candidates[j].invoice.InvoiceDate {
			return candidates[i].invoice.InvoiceDate < candidates[j].invoice.InvoiceDate
		}
		return candidates[i].invoice.ID < candidates[j].invoice.ID
	})

	picked := candidates[0]

	amountOwed := picked.invoice.TotalCents - picked.invoice.PaidCents
	if amountOwed < 0 {
		amountOwed = 0
	}

	daysUntilDue := int(picked.dueDate.Sub(today).Hours() / 24)
	if daysUntilDue < 0 {
		daysUntilDue = 0
	}

	projectedRecovery := 0
	if recoveryRate30dCents > 0 && daysUntilDue > 0 {
		// Use integer math to avoid float drift on cent values.
		projectedRecovery = (recoveryRate30dCents * daysUntilDue) / 30
	}

	gap := amountOwed - projectedRecovery - cashBufferCents
	if gap < 0 {
		gap = 0
	}

	return InvoiceProjection{
		NextInvoiceDate:        picked.invoice.InvoiceDate,
		NextInvoiceDueDate:     picked.invoice.DueDate,
		NextInvoiceAmountCents: amountOwed,
		DaysUntilInvoiceDue:    daysUntilDue,
		ProjectedRecoveryCents: projectedRecovery,
		ProjectedCashGapCents:  gap,
	}
}
