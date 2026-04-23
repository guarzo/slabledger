package liquidation

import "context"

// ConfidenceLevel indicates how reliable a computed comp price is.
type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceLow    ConfidenceLevel = "low"
	ConfidenceNone   ConfidenceLevel = "none"
)

// SaleComp is a single historical sale record used for comp pricing.
type SaleComp struct {
	SaleDate   string // "2006-01-02"
	PriceCents int
}

// CompPriceResult holds the result of a comp price computation.
type CompPriceResult struct {
	CompPriceCents     int             `json:"compPriceCents"`
	CompCount          int             `json:"compCount"`
	MostRecentCompDate string          `json:"mostRecentCompDate"`
	ConfidenceLevel    ConfidenceLevel `json:"confidenceLevel"`
	GapPct             float64         `json:"gapPct"`
}

// PreviewRequest carries the discount settings for a liquidation preview.
type PreviewRequest struct {
	BaseDiscountPct   float64 `json:"baseDiscountPct"`
	NoCompDiscountPct float64 `json:"noCompDiscountPct"`
}

// PreviewItem is one card in the preview response.
type PreviewItem struct {
	PurchaseID                string          `json:"purchaseId"`
	CertNumber                string          `json:"certNumber"`
	CardName                  string          `json:"cardName"`
	Grade                     float64         `json:"grade"`
	CampaignName              string          `json:"campaignName"`
	BuyCostCents              int             `json:"buyCostCents"`
	CLValueCents              int             `json:"clValueCents"`
	CompPriceCents            int             `json:"compPriceCents"`
	CompCount                 int             `json:"compCount"`
	MostRecentCompDate        string          `json:"mostRecentCompDate"`
	ConfidenceLevel           ConfidenceLevel `json:"confidenceLevel"`
	GapPct                    float64         `json:"gapPct"`
	CurrentReviewedPriceCents int             `json:"currentReviewedPriceCents"`
	SuggestedPriceCents       int             `json:"suggestedPriceCents"`
	BelowCost                 bool            `json:"belowCost"`
}

// PreviewSummary aggregates the preview results.
type PreviewSummary struct {
	TotalCards               int `json:"totalCards"`
	WithComps                int `json:"withComps"`
	WithoutComps             int `json:"withoutComps"`
	NoData                   int `json:"noData"`
	TotalCurrentValueCents   int `json:"totalCurrentValueCents"`
	TotalSuggestedValueCents int `json:"totalSuggestedValueCents"`
	BelowCostCount           int `json:"belowCostCount"`
}

// PreviewResponse is the full preview payload.
type PreviewResponse struct {
	Items   []PreviewItem  `json:"items"`
	Summary PreviewSummary `json:"summary"`
}

// ApplyItem specifies a new price for one purchase.
type ApplyItem struct {
	PurchaseID    string `json:"purchaseId"`
	NewPriceCents int    `json:"newPriceCents"`
}

// ApplyRequest holds the list of price updates to apply.
type ApplyRequest struct {
	Items []ApplyItem `json:"items"`
}

// ApplyResult summarises the outcome of an Apply call.
type ApplyResult struct {
	Applied int      `json:"applied"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors"`
}

// UnsoldPurchase is a purchase that has not yet been sold.
type UnsoldPurchase struct {
	ID                 string
	CertNumber         string
	CardName           string
	GradeValue         float64
	CampaignName       string
	BuyCostCents       int
	CLValueCents       int
	GemRateID          string
	ReviewedPriceCents int
}

// CompReader fetches historical sale comps for a card.
type CompReader interface {
	GetSaleCompsForCard(ctx context.Context, gemRateID, condition string) ([]SaleComp, error)
}

// PurchaseLister lists unsold purchases eligible for liquidation.
type PurchaseLister interface {
	ListUnsoldForLiquidation(ctx context.Context) ([]UnsoldPurchase, error)
}

// PriceWriter persists a reviewed price for a purchase.
type PriceWriter interface {
	SetReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error
}

// Service is the top-level liquidation domain service.
type Service interface {
	Preview(ctx context.Context, req PreviewRequest) (PreviewResponse, error)
	Apply(ctx context.Context, req ApplyRequest) (ApplyResult, error)
}
