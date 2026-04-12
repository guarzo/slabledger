package inventory

import "time"

// PriceFlagReason enumerates valid flag reasons.
type PriceFlagReason string

const (
	PriceFlagWrongMatch         PriceFlagReason = "wrong_match"
	PriceFlagStaleData          PriceFlagReason = "stale_data"
	PriceFlagWrongGrade         PriceFlagReason = "wrong_grade"
	PriceFlagSourceDisagreement PriceFlagReason = "source_disagreement"
	PriceFlagOther              PriceFlagReason = "other"
)

// validPriceFlagReasons is the set of valid flag reasons.
var validPriceFlagReasons = map[PriceFlagReason]bool{
	PriceFlagWrongMatch:         true,
	PriceFlagStaleData:          true,
	PriceFlagWrongGrade:         true,
	PriceFlagSourceDisagreement: true,
	PriceFlagOther:              true,
}

// Valid reports whether r is a recognized price-flag reason.
func (r PriceFlagReason) Valid() bool {
	return validPriceFlagReasons[r]
}

// ReviewSource identifies how a reviewed price was chosen.
type ReviewSource string

const (
	ReviewSourceManual     ReviewSource = "manual"
	ReviewSourceCL         ReviewSource = "cl"
	ReviewSourceMarket     ReviewSource = "market"
	ReviewSourceLastSold   ReviewSource = "last_sold"
	ReviewSourceCostMarkup ReviewSource = "cost_markup"
	ReviewSourceMM         ReviewSource = "mm"
)

// PriceFlag represents a user-reported data quality issue on a purchase's pricing.
type PriceFlag struct {
	ID         int64           `json:"id"`
	PurchaseID string          `json:"purchaseId"`
	FlaggedBy  int64           `json:"flaggedBy"`
	FlaggedAt  time.Time       `json:"flaggedAt"`
	Reason     PriceFlagReason `json:"reason"`
	ResolvedAt *time.Time      `json:"resolvedAt,omitempty"`
	ResolvedBy *int64          `json:"resolvedBy,omitempty"`
}

// PriceFlagWithContext enriches a PriceFlag with card and pricing details for admin review.
type PriceFlagWithContext struct {
	PriceFlag
	CardName           string        `json:"cardName"`
	SetName            string        `json:"setName,omitempty"`
	CardNumber         string        `json:"cardNumber,omitempty"`
	Grade              float64       `json:"grade"`
	CertNumber         string        `json:"certNumber"`
	FlaggedByEmail     string        `json:"flaggedByEmail"`
	MarketPriceCents   int           `json:"marketPriceCents"`
	CLValueCents       int           `json:"clValueCents"`
	ReviewedPriceCents int           `json:"reviewedPriceCents"`
	SourcePrices       []SourcePrice `json:"sourcePrices,omitempty"`
}

// ReviewStats contains aggregate review progress metrics.
type ReviewStats struct {
	Total       int `json:"total"`
	NeedsReview int `json:"needsReview"`
	Reviewed    int `json:"reviewed"`
	Flagged     int `json:"flagged"`
}
