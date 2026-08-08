package inventory

// AlertLevel indicates the severity of capital exposure.
type AlertLevel string

const (
	AlertOK       AlertLevel = "ok"
	AlertWarning  AlertLevel = "warning"
	AlertCritical AlertLevel = "critical"
)

// RecoveryTrend indicates direction of recovery velocity.
type RecoveryTrend string

const (
	TrendImproving RecoveryTrend = "improving"
	TrendDeclining RecoveryTrend = "declining"
	TrendStable    RecoveryTrend = "stable"
)

// Capital velocity thresholds and constants.
const (
	WeeksToCoverNoData            = 99.0    // Sentinel when no recovery data exists
	WeeksPerMonth                 = 4.3     // 30 days / 7
	TrendChangeThreshold          = 0.10    // 10% delta to consider directional
	WeeksToCoverCriticalThreshold = 12.0    // Weeks-to-cover above which alert is critical
	WeeksToCoverWarningThreshold  = 6.0     // Weeks-to-cover above which alert is warning
	FallbackCriticalCents         = 1000000 // $10K outstanding, no recovery data
	FallbackWarningCents          = 500000  // $5K outstanding, no recovery data
)

// InvoiceSellThrough holds sell-through metrics for a single invoice date's purchases.
// Only returned (received_at IS NOT NULL) purchases are counted.
type InvoiceSellThrough struct {
	TotalPurchaseCount int `json:"totalPurchaseCount"` // all non-refunded returned purchases for the invoice_date
	SoldCount          int `json:"soldCount"`          // purchases with a completed sale
	TotalCostCents     int `json:"totalCostCents"`     // sum of buy_cost_cents for returned purchases
	SaleRevenueCents   int `json:"saleRevenueCents"`   // sum of sale_price_cents for sold purchases
}

// CapitalRawData holds the raw SQL-fetched capital data before business logic is applied.
// The repository returns this; domain logic computes derived fields (WeeksToCover, Trend, AlertLevel).
type CapitalRawData struct {
	OutstandingCents          int // Unpaid purchases minus payments
	RecoveryRate30dCents      int // Sale revenue in last 30 days
	RecoveryRate30dPriorCents int // Sale revenue in days 31-60
	RefundedCents             int // Total refunds
	PaidCents                 int // Total paid
	UnpaidInvoiceCount        int // Count of unpaid invoices
}

// CapitalSummary provides a snapshot of current capital exposure with recovery velocity.
type CapitalSummary struct {
	OutstandingCents          int           `json:"outstandingCents"`          // Unpaid purchases minus payments
	RecoveryRate30dCents      int           `json:"recoveryRate30dCents"`      // Sale revenue in last 30 days
	RecoveryRate30dPriorCents int           `json:"recoveryRate30dPriorCents"` // Sale revenue in days 31-60
	WeeksToCover              float64       `json:"weeksToCover"`              // outstanding / weekly recovery rate (99 = no data)
	RecoveryTrend             RecoveryTrend `json:"recoveryTrend"`
	AlertLevel                AlertLevel    `json:"alertLevel"`
	RefundedCents             int           `json:"refundedCents"` // Total refunds
	PaidCents                 int           `json:"paidCents"`     // Total paid
	UnpaidInvoiceCount        int           `json:"unpaidInvoiceCount"`
	// Invoice-cycle actuals (see ComputeInvoiceProjection in invoice_projection.go).
	NextInvoiceDate                string             `json:"nextInvoiceDate,omitempty"`      // YYYY-MM-DD, empty if no unpaid
	NextInvoiceDueDate             string             `json:"nextInvoiceDueDate,omitempty"`   // YYYY-MM-DD
	NextInvoiceAmountCents         int                `json:"nextInvoiceAmountCents"`         // TotalCents - PaidCents of earliest unpaid invoice (amount still owed)
	DaysUntilInvoiceDue            int                `json:"daysUntilInvoiceDue"`            // from now to due date, negative = overdue
	NextInvoicePendingReceiptCents int                `json:"nextInvoicePendingReceiptCents"` // cost of cards still at PSA for this invoice
	NextInvoiceSellThrough         InvoiceSellThrough `json:"nextInvoiceSellThrough"`         // sell-through for returned cards on this invoice
}

// ComputeCapitalSummary applies business logic to raw capital data, computing
// derived fields: WeeksToCover, RecoveryTrend, and AlertLevel.
// Returns a safe default summary if raw is nil.
func ComputeCapitalSummary(raw *CapitalRawData) *CapitalSummary {
	if raw == nil {
		return &CapitalSummary{
			WeeksToCover:  WeeksToCoverNoData,
			RecoveryTrend: TrendStable,
			AlertLevel:    AlertOK,
		}
	}

	weeksToCover := WeeksToCoverNoData
	if raw.RecoveryRate30dCents > 0 {
		weeklyRate := float64(raw.RecoveryRate30dCents) / WeeksPerMonth
		weeksToCover = float64(raw.OutstandingCents) / weeklyRate
	}

	trend := TrendStable
	if raw.RecoveryRate30dCents > 0 && raw.RecoveryRate30dPriorCents == 0 {
		trend = TrendImproving
	} else if raw.RecoveryRate30dCents == 0 && raw.RecoveryRate30dPriorCents > 0 {
		trend = TrendDeclining
	} else if raw.RecoveryRate30dCents > 0 && raw.RecoveryRate30dPriorCents > 0 {
		ratio := float64(raw.RecoveryRate30dCents) / float64(raw.RecoveryRate30dPriorCents)
		if ratio > 1+TrendChangeThreshold {
			trend = TrendImproving
		} else if ratio < 1-TrendChangeThreshold {
			trend = TrendDeclining
		}
	}

	alertLevel := AlertOK
	if raw.RecoveryRate30dCents > 0 {
		if weeksToCover > WeeksToCoverCriticalThreshold {
			alertLevel = AlertCritical
		} else if weeksToCover >= WeeksToCoverWarningThreshold {
			alertLevel = AlertWarning
		}
	} else {
		if raw.OutstandingCents > FallbackCriticalCents {
			alertLevel = AlertCritical
		} else if raw.OutstandingCents > FallbackWarningCents {
			alertLevel = AlertWarning
		}
	}

	return &CapitalSummary{
		OutstandingCents:          raw.OutstandingCents,
		RecoveryRate30dCents:      raw.RecoveryRate30dCents,
		RecoveryRate30dPriorCents: raw.RecoveryRate30dPriorCents,
		WeeksToCover:              weeksToCover,
		RecoveryTrend:             trend,
		AlertLevel:                alertLevel,
		RefundedCents:             raw.RefundedCents,
		PaidCents:                 raw.PaidCents,
		UnpaidInvoiceCount:        raw.UnpaidInvoiceCount,
	}
}
