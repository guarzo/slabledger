package campaigns

import (
	"encoding/json"
	"time"
)

// Phase represents the lifecycle state of a campaign.
type Phase string

const (
	PhasePending Phase = "pending"
	PhaseActive  Phase = "active"
	PhaseClosed  Phase = "closed"
)

// DHStatus represents the DoubleHolo inventory status.
type DHStatus = string

const (
	DHStatusInStock DHStatus = "in_stock"
	DHStatusListed  DHStatus = "listed"
)

// DHPushStatus represents the DH inventory push pipeline status.
type DHPushStatus = string

const (
	DHPushStatusPending   DHPushStatus = "pending"
	DHPushStatusMatched   DHPushStatus = "matched"
	DHPushStatusUnmatched DHPushStatus = "unmatched"
	DHPushStatusManual    DHPushStatus = "manual"
)

// SaleChannel represents where a card was sold.
type SaleChannel string

const (
	SaleChannelEbay     SaleChannel = "ebay"
	SaleChannelWebsite  SaleChannel = "website"
	SaleChannelInPerson SaleChannel = "inperson"
)

// Legacy channel values — kept for backward-compatible DB reads.
const (
	SaleChannelTCGPlayer  SaleChannel = "tcgplayer"
	SaleChannelLocal      SaleChannel = "local"
	SaleChannelOther      SaleChannel = "other"
	SaleChannelGameStop   SaleChannel = "gamestop"
	SaleChannelCardShow   SaleChannel = "cardshow"
	SaleChannelDoubleHolo SaleChannel = "doubleholo"
)

const (
	ExternalCampaignID   = "external"
	ExternalCampaignName = "External"
)

// OverrideSource identifies how a price override was set.
type OverrideSource string

const (
	OverrideSourceNone       OverrideSource = ""
	OverrideSourceManual     OverrideSource = "manual"
	OverrideSourceCostMarkup OverrideSource = "cost_markup"
	OverrideSourceAIAccepted OverrideSource = "ai_accepted"
)

// MarketSnapshotData holds market snapshot fields shared by Purchase and Sale.
// Core fields are stored as individual columns for SQL queries.
// The full MarketSnapshot (including SourcePrices, velocity, etc.) is stored
// as JSON in SnapshotJSON for the frontend without adding many DB columns.
type MarketSnapshotData struct {
	LastSoldCents     int     `json:"lastSoldCents,omitempty"`
	LowestListCents   int     `json:"lowestListCents,omitempty"`
	ConservativeCents int     `json:"conservativeCents,omitempty"`
	MedianCents       int     `json:"medianCents,omitempty"`
	ActiveListings    int     `json:"activeListings,omitempty"`
	SalesLast30d      int     `json:"salesLast30d,omitempty"`
	Trend30d          float64 `json:"trend30d,omitempty"`
	SnapshotDate      string  `json:"snapshotDate,omitempty"`
	SnapshotJSON      string  `json:"-"` // Full MarketSnapshot serialized as JSON (DB column, not in API)
}

func (d *MarketSnapshotData) applySnapshot(snapshot *MarketSnapshot, date string) {
	d.LastSoldCents = snapshot.LastSoldCents
	d.LowestListCents = snapshot.LowestListCents
	d.ConservativeCents = snapshot.ConservativeCents
	d.MedianCents = snapshot.MedianCents
	d.ActiveListings = snapshot.ActiveListings
	d.SalesLast30d = snapshot.SalesLast30d
	d.Trend30d = snapshot.Trend30d
	d.SnapshotDate = date

	// Persist the full snapshot as JSON for frontend consumption
	if b, err := json.Marshal(snapshot); err == nil {
		d.SnapshotJSON = string(b)
	} else {
		d.SnapshotJSON = ""
	}
}

// Campaign represents a PSA Direct Buy campaign with buy parameters and fee configuration.
type Campaign struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Sport               string    `json:"sport"`
	YearRange           string    `json:"yearRange"`          // e.g. "1999-2003"
	GradeRange          string    `json:"gradeRange"`         // e.g. "9-10"
	PriceRange          string    `json:"priceRange"`         // e.g. "50-500"
	CLConfidence        string    `json:"clConfidence"`       // CL confidence range, e.g. "2.5-4"
	BuyTermsCLPct       float64   `json:"buyTermsCLPct"`      // Buy at X% of CL value (0-1)
	DailySpendCapCents  int       `json:"dailySpendCapCents"` // Max daily spend in cents
	InclusionList       string    `json:"inclusionList"`      // Comma-separated card names/sets
	ExclusionMode       bool      `json:"exclusionMode"`      // If true, inclusionList acts as exclusion list
	Phase               Phase     `json:"phase"`
	PSASourcingFeeCents int       `json:"psaSourcingFeeCents"` // Default 300 ($3)
	EbayFeePct          float64   `json:"ebayFeePct"`          // Default 0.1235 (12.35%)
	ExpectedFillRate    float64   `json:"expectedFillRate"`    // Target fill rate as percentage (0-100)
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// SnapshotStatus represents the state of background market snapshot enrichment.
type SnapshotStatus string

// Snapshot status constants for Purchase.SnapshotStatus.
const (
	SnapshotStatusNone      SnapshotStatus = ""          // snapshot captured or not needed
	SnapshotStatusPending   SnapshotStatus = "pending"   // awaiting background enrichment
	SnapshotStatusFailed    SnapshotStatus = "failed"    // enrichment attempt failed, will retry
	SnapshotStatusExhausted SnapshotStatus = "exhausted" // max retries reached, requires manual fix
)

// Purchase represents a single card purchased through a campaign.
type Purchase struct {
	ID                    string         `json:"id"`
	CampaignID            string         `json:"campaignId"`
	CardName              string         `json:"cardName"`
	CertNumber            string         `json:"certNumber"`           // PSA cert number (unique)
	CardNumber            string         `json:"cardNumber,omitempty"` // Card number within set (from PSA)
	SetName               string         `json:"setName,omitempty"`    // Set/category name (from PSA)
	Grader                string         `json:"grader,omitempty"`     // e.g. "PSA", "CGC", "BGS", "SGC"
	GradeValue            float64        `json:"gradeValue"`           // Numeric grade (1-10, supports half-grades like 9.5)
	CLValueCents          int            `json:"clValueCents"`         // CL market value at purchase time
	BuyCostCents          int            `json:"buyCostCents"`         // Actual cost paid
	PSASourcingFeeCents   int            `json:"psaSourcingFeeCents"`  // Fee charged per card
	Population            int            `json:"population,omitempty"` // PSA population count
	PurchaseDate          string         `json:"purchaseDate"`         // YYYY-MM-DD
	VaultStatus           string         `json:"vaultStatus,omitempty"`
	InvoiceDate           string         `json:"invoiceDate,omitempty"`
	WasRefunded           bool           `json:"wasRefunded,omitempty"`
	FrontImageURL         string         `json:"frontImageUrl,omitempty"`
	BackImageURL          string         `json:"backImageUrl,omitempty"`
	PurchaseSource        string         `json:"purchaseSource,omitempty"`
	PSAListingTitle       string         `json:"psaListingTitle,omitempty"` // Raw PSA listing title for pricing fallback
	SnapshotStatus        SnapshotStatus `json:"snapshotStatus,omitempty"`  // see SnapshotStatus* constants
	SnapshotRetryCount    int            `json:"snapshotRetryCount,omitempty"`
	OverridePriceCents    int            `json:"overridePriceCents,omitempty"`
	OverrideSource        OverrideSource `json:"overrideSource,omitempty"`
	OverrideSetAt         string         `json:"overrideSetAt,omitempty"`
	AISuggestedPriceCents int            `json:"aiSuggestedPriceCents,omitempty"`
	AISuggestedAt         string         `json:"aiSuggestedAt,omitempty"`
	CardYear              string         `json:"cardYear,omitempty"`
	EbayExportFlaggedAt   *time.Time     `json:"ebayExportFlaggedAt,omitempty"`
	ReviewedPriceCents    int            `json:"reviewedPriceCents,omitempty"`
	ReviewedAt            string         `json:"reviewedAt,omitempty"`
	ReviewSource          ReviewSource   `json:"reviewSource,omitempty"`
	// DoubleHolo v2 integration fields
	DHCardID            int          `json:"dhCardId,omitempty"`            // DH card identity (from cert resolution)
	DHInventoryID       int          `json:"dhInventoryId,omitempty"`       // DH inventory item ID (from inventory push)
	DHCertStatus        string       `json:"dhCertStatus,omitempty"`        // Resolution state: matched, ambiguous, not_found, unresolved, resolving
	DHListingPriceCents int          `json:"dhListingPriceCents,omitempty"` // Current DH listing price
	DHChannelsJSON      string       `json:"dhChannelsJson,omitempty"`      // Per-channel sync status JSON blob
	DHStatus            DHStatus     `json:"dhStatus,omitempty"`            // DH inventory status
	DHPushStatus        DHPushStatus `json:"dhPushStatus,omitempty"`        // Pipeline status: "", "pending", "matched", "unmatched", "manual"
	DHCandidatesJSON string       `json:"dhCandidatesJson,omitempty"` // Ambiguous cert resolution candidates JSON
	CreatedAt           time.Time    `json:"createdAt"`
	UpdatedAt           time.Time    `json:"updatedAt"`

	// Market snapshot at time of purchase (best-effort, may be zero)
	MarketSnapshotData
}

// ToCardIdentity returns a CardIdentity populated from this purchase's card metadata.
func (p *Purchase) ToCardIdentity() CardIdentity {
	return CardIdentity{
		CardName:        p.CardName,
		CardNumber:      p.CardNumber,
		SetName:         p.SetName,
		PSAListingTitle: p.PSAListingTitle,
	}
}

// NeedsDHPush returns true if this purchase is eligible for DH push pipeline enrollment.
func (p *Purchase) NeedsDHPush() bool {
	return p.DHInventoryID == 0 &&
		p.DHPushStatus != DHPushStatusPending &&
		p.DHPushStatus != DHPushStatusUnmatched &&
		p.DHPushStatus != DHPushStatusManual
}

// DHCardKey returns the pipe-delimited identity key used for DH card ID mapping lookups.
func (p *Purchase) DHCardKey() string {
	return DHCardKey(p.CardName, p.SetName, p.CardNumber)
}

// DHCardKey builds the pipe-delimited identity key used by DH card ID mapping lookups.
func DHCardKey(cardName, setName, cardNumber string) string {
	return cardName + "|" + setName + "|" + cardNumber
}

// Invoice tracks a PSA invoice cycle for credit limit management.
type Invoice struct {
	ID          string    `json:"id"`
	InvoiceDate string    `json:"invoiceDate"`
	TotalCents  int       `json:"totalCents"`
	PaidCents   int       `json:"paidCents"`
	DueDate     string    `json:"dueDate,omitempty"`
	PaidDate    string    `json:"paidDate,omitempty"`
	Status      string    `json:"status"` // "unpaid", "partial", "paid"
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// CashflowConfig holds credit and cash management settings.
type CashflowConfig struct {
	CreditLimitCents int       `json:"creditLimitCents"`
	CashBufferCents  int       `json:"cashBufferCents"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// CreditSummary provides a snapshot of the current credit position.
type CreditSummary struct {
	CreditLimitCents       int     `json:"creditLimitCents"`
	OutstandingCents       int     `json:"outstandingCents"` // Unpaid purchases
	UtilizationPct         float64 `json:"utilizationPct"`   // (outstanding / limit) * 100
	RefundedCents          int     `json:"refundedCents"`    // Total refunds
	PaidCents              int     `json:"paidCents"`        // Total paid
	UnpaidInvoiceCount     int     `json:"unpaidInvoiceCount"`
	AlertLevel             string  `json:"alertLevel"`             // "ok", "warning", "critical"
	ProjectedExposureCents int     `json:"projectedExposureCents"` // outstanding + avgDailySpend * daysToNextInvoice
	DaysToNextInvoice      int     `json:"daysToNextInvoice"`
}

// PurchaseFilter holds optional filtering criteria for GetAllPurchasesWithSales.
type PurchaseFilter struct {
	SinceDate       string // "2025-01-01" or empty for all
	ExcludeArchived bool
}

// PurchaseFilterOpt is a functional option for configuring PurchaseFilter.
type PurchaseFilterOpt func(*PurchaseFilter)

// WithSinceDate returns an option that filters purchases to those on or after the given date (YYYY-MM-DD).
func WithSinceDate(d string) PurchaseFilterOpt {
	return func(f *PurchaseFilter) { f.SinceDate = d }
}

// WithExcludeArchived returns an option that excludes purchases from archived campaigns.
func WithExcludeArchived() PurchaseFilterOpt {
	return func(f *PurchaseFilter) { f.ExcludeArchived = true }
}

// ChannelVelocity holds cross-campaign recovery velocity stats for a sale channel.
type ChannelVelocity struct {
	Channel       SaleChannel `json:"channel"`
	SaleCount     int         `json:"saleCount"`
	AvgDaysToSell float64     `json:"avgDaysToSell"`
	RevenueCents  int         `json:"revenueCents"`
}

// PortfolioHealth represents cross-campaign health assessment.
type PortfolioHealth struct {
	Campaigns      []CampaignHealth `json:"campaigns"`
	TotalDeployed  int              `json:"totalDeployedCents"`
	TotalRecovered int              `json:"totalRecoveredCents"`
	TotalAtRisk    int              `json:"totalAtRiskCents"`
	OverallROI     float64          `json:"overallROI"`
	RealizedROI    float64          `json:"realizedROI"`
}

// CampaignHealth represents health status for a single campaign.
type CampaignHealth struct {
	CampaignID     string  `json:"campaignId"`
	CampaignName   string  `json:"campaignName"`
	Phase          Phase   `json:"phase"`
	ROI            float64 `json:"roi"`
	SellThroughPct float64 `json:"sellThroughPct"`
	AvgDaysToSell  float64 `json:"avgDaysToSell"`
	TotalPurchases int     `json:"totalPurchases"`
	TotalUnsold    int     `json:"totalUnsold"`
	CapitalAtRisk  int     `json:"capitalAtRiskCents"`
	HealthStatus   string  `json:"healthStatus"` // "healthy", "warning", "critical"
	HealthReason   string  `json:"healthReason"`
}

// Sale represents the sale of a purchased card.
type Sale struct {
	ID             string      `json:"id"`
	PurchaseID     string      `json:"purchaseId"` // FK to Purchase (unique — one sale per purchase)
	SaleChannel    SaleChannel `json:"saleChannel"`
	SalePriceCents int         `json:"salePriceCents"`
	SaleFeeCents   int         `json:"saleFeeCents"`   // Marketplace fees in cents
	SaleDate       string      `json:"saleDate"`       // YYYY-MM-DD
	DaysToSell     int         `json:"daysToSell"`     // Computed: saleDate - purchaseDate
	NetProfitCents int         `json:"netProfitCents"` // Computed: salePrice - buyCost - sourcingFee - saleFee
	CreatedAt      time.Time   `json:"createdAt"`
	UpdatedAt      time.Time   `json:"updatedAt"`
	// DoubleHolo v2 order tracking
	OrderID string `json:"orderId,omitempty"` // DH order_id for idempotency

	// Sale outcome enrichment — captures HOW the card sold
	OriginalListPriceCents int  `json:"originalListPriceCents,omitempty"` // Initial listing price
	PriceReductions        int  `json:"priceReductions,omitempty"`        // Number of price drops before sale
	DaysListed             int  `json:"daysListed,omitempty"`             // Days on the listing platform
	SoldAtAskingPrice      bool `json:"soldAtAskingPrice,omitempty"`      // Sold at the listed price

	// Crack slab tracking — indicates the card was cracked from its slab and sold raw
	WasCracked bool `json:"wasCracked,omitempty"`

	// Market snapshot at time of sale (best-effort, may be zero)
	MarketSnapshotData
}

// BulkSaleInput represents a single item in a bulk sale request.
type BulkSaleInput struct {
	PurchaseID     string `json:"purchaseId"`
	SalePriceCents int    `json:"salePriceCents"`
}

// BulkSaleResult summarizes the outcome of a bulk sale operation.
type BulkSaleResult struct {
	Created int             `json:"created"`
	Failed  int             `json:"failed"`
	Errors  []BulkSaleError `json:"errors,omitempty"`
}

// BulkSaleError describes a failure for a single item in a bulk sale.
type BulkSaleError struct {
	PurchaseID string `json:"purchaseId"`
	Error      string `json:"error"`
}

// ActivationCheck represents a single pre-activation checklist item.
type ActivationCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

// ActivationChecklist contains the full pre-activation advisory.
type ActivationChecklist struct {
	CampaignID   string            `json:"campaignId"`
	CampaignName string            `json:"campaignName"`
	AllPassed    bool              `json:"allPassed"`
	Checks       []ActivationCheck `json:"checks"`
	Warnings     []string          `json:"warnings"`
}

// RevocationFlag represents a segment flagged for PSA revocation.
type RevocationFlag struct {
	ID               string     `json:"id"`
	SegmentLabel     string     `json:"segmentLabel"`
	SegmentDimension string     `json:"segmentDimension"`
	Reason           string     `json:"reason"`
	Status           string     `json:"status"` // "pending", "sent"
	EmailText        string     `json:"emailText"`
	CreatedAt        time.Time  `json:"createdAt"`
	SentAt           *time.Time `json:"sentAt,omitempty"`
}
