package inventory

import (
	"sort"
	"time"
)

// InvoiceProjection holds the identifying fields for the earliest unpaid invoice.
// Sell-through and pending-receipt data are fetched separately by the service.
type InvoiceProjection struct {
	NextInvoiceDate        string
	NextInvoiceDueDate     string
	NextInvoiceAmountCents int
	DaysUntilInvoiceDue    int
}

// ComputeInvoiceProjection picks the earliest unpaid invoice with a parseable
// DueDate and returns its identifying fields. Returns a zero-valued projection
// when no qualifying invoice exists.
//
// Edge cases:
//   - Empty invoice list / all invoices paid -> zero projection.
//   - Invoice with empty or unparseable DueDate -> skipped (next candidate wins).
//   - Overdue invoice (due date in the past) -> picked; daysUntilDue is negative.
//
// Parameters:
//
//	invoices - full invoice list (any order; function sorts internally)
//	now      - current time, injected for testability
func ComputeInvoiceProjection(
	invoices []Invoice,
	now time.Time,
) InvoiceProjection {
	const dueDateLayout = "2006-01-02"

	type candidate struct {
		invoice Invoice
		dueDate time.Time
	}

	// Normalize now to UTC so the extracted calendar day matches the UTC
	// midnight used when parsing due dates. Without this, a non-UTC now can
	// yield a different calendar day near day boundaries, skewing DaysUntilDue.
	now = now.UTC()
	todayY, todayM, todayD := now.Date()
	today := time.Date(todayY, todayM, todayD, 0, 0, 0, 0, time.UTC)

	candidates := make([]candidate, 0, len(invoices))
	for _, inv := range invoices {
		if inv.Status == "paid" {
			continue
		}
		if inv.DueDate == "" {
			continue
		}
		due, err := time.ParseInLocation(dueDateLayout, inv.DueDate, time.UTC)
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

	return InvoiceProjection{
		NextInvoiceDate:        picked.invoice.InvoiceDate,
		NextInvoiceDueDate:     picked.invoice.DueDate,
		NextInvoiceAmountCents: amountOwed,
		DaysUntilInvoiceDue:    daysUntilDue,
	}
}
