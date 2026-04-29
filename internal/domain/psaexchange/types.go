// Package psaexchange surfaces PSA-Exchange marketplace listings as ranked
// acquisition opportunities scored by a tiered offer × velocity model.
package psaexchange

import "time"

// Tier captures the offer-percentage policy applied to a listing based on its
// CardLadder velocity and confidence.
type Tier struct {
	Name        string  // "high_liquidity" | "default"
	MaxOfferPct float64 // e.g. 0.75 or 0.65
}

// CatalogCard mirrors the /api/catalog response shape for one listing.
// All money fields are dollars-as-float exactly as returned by the upstream API.
type CatalogCard struct {
	Cert        string   `json:"cert"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Grade       string   `json:"grade"`
	CertIssuer  string   `json:"certIssuer"`
	Price       float64  `json:"price"`
	Front       string   `json:"front"`
	Back        string   `json:"back"`
	IsSpotlight bool     `json:"isSpotlight"`
	DateAdded   string   `json:"dateAdded"`
	IsNew       bool     `json:"isNew"`
	IsAutograph bool     `json:"isAutograph"`
	Tags        []string `json:"tags"`
}

// Catalog is the /api/catalog response.
type Catalog struct {
	Cards []CatalogCard `json:"cards"`
	Count int           `json:"count"`
}

// CardLadderBucket is one velocity/price window from /api/cardladder.
type CardLadderBucket struct {
	Velocity     int     `json:"velocity"`
	AveragePrice float64 `json:"averagePrice"`
}

// CardLadder mirrors the /api/cardladder?cert=… response.
type CardLadder struct {
	EstimatedValue  float64          `json:"estimatedValue"`
	LastSalePrice   float64          `json:"lastSalePrice"`
	LastSaleDate    time.Time        `json:"lastSaleDate"`
	Confidence      int              `json:"confidence"`
	Grader          string           `json:"grader"`
	Index           string           `json:"index"`
	IndexID         string           `json:"indexId"`
	Description     string           `json:"description"`
	Grade           string           `json:"grade"`
	TwoWeekData     CardLadderBucket `json:"twoWeekData"`
	OneMonthData    CardLadderBucket `json:"oneMonthData"`
	OneQuarterData  CardLadderBucket `json:"oneQuarterData"`
	OneYearData     CardLadderBucket `json:"oneYearData"`
	Population      int              `json:"population"`
	IsPlayerIndex   bool             `json:"isPlayerIndex"`
	IndexPctChange  float64          `json:"indexPercentChange"`
}

// Listing is a catalog row enriched with CardLadder data and computed score
// fields. All money is in cents (int64) per project convention.
type Listing struct {
	Cert             string
	Name             string     // raw from catalog (kept for display)
	Description      string     // clean from cardladder
	Grade            string
	ListPriceCents   int64
	TargetOfferCents int64
	MaxOfferPct      float64
	CompCents        int64      // CardLadder estimatedValue
	LastSalePriceCents int64
	LastSaleDate     time.Time
	VelocityMonth    int
	VelocityQuarter  int
	Confidence       int
	Population       int
	EdgeAtOffer      float64    // (comp - targetOffer) / targetOffer
	Score            float64    // edge_at_offer × log(1 + velocity_month)
	ListRunwayPct    float64    // (list - targetOffer) / list
	MayTakeAtList    bool       // list <= targetOffer
	FrontImage       string
	BackImage        string
	IndexID          string
	Tier             string     // tier name applied
}

// OpportunitiesResult is what the service returns to the handler.
type OpportunitiesResult struct {
	Opportunities []Listing
	CategoryURL   string    // empty when token unconfigured
	FetchedAt     time.Time
	TotalCatalog  int       // total Pokemon listings before filter
	AfterFilter   int
	EnrichmentErrors int
}
