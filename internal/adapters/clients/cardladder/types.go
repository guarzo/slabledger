package cardladder

import "time"

// SearchResponse is the envelope returned by the Cloud Run search API.
type SearchResponse[T any] struct {
	Hits      []T `json:"hits"`
	TotalHits int `json:"totalHits"`
}

// CollectionCard represents one card from the collectioncards index.
type CollectionCard struct {
	CollectionCardID string  `json:"collectionCardId"`
	CollectionID     string  `json:"collectionId"`
	Category         string  `json:"category"`
	Condition        string  `json:"condition"`
	Year             string  `json:"year"`
	Number           string  `json:"number"`
	Set              string  `json:"set"`
	Variation        string  `json:"variation"`
	Label            string  `json:"label"`
	Player           string  `json:"player"`
	Image            string  `json:"image"`
	ImageBack        string  `json:"imageBack"`
	CurrentValue     float64 `json:"currentValue"`
	Investment       float64 `json:"investment"`
	Profit           float64 `json:"profit"`
	WeeklyPctChange  float64 `json:"weeklyPercentChange"`
	MonthlyPctChange float64 `json:"monthlyPercentChange"`
	DateAdded        string  `json:"dateAdded"`
	HasQuantityAvail bool    `json:"hasQuantityAvailable"`
	Sold             bool    `json:"sold"`
}

// SaleComp represents one sold listing from the salesarchive index.
type SaleComp struct {
	ItemID          string  `json:"itemId"`
	Date            string  `json:"date"`
	Price           float64 `json:"price"`
	Platform        string  `json:"platform"`
	ListingType     string  `json:"listingType"`
	Seller          string  `json:"seller"`
	Feedback        int     `json:"feedback"`
	URL             string  `json:"url"`
	SlabSerial      string  `json:"slabSerial"`
	CardDescription string  `json:"cardDescription"`
	GemRateID       string  `json:"gemRateId"`
	Condition       string  `json:"condition"`
	GradingCompany  string  `json:"gradingCompany"`
}

// CatalogCard represents one card from the CL cards index (full catalog).
// gemRateID is grade-agnostic: it identifies the card variant (card + set + year + number + parallel),
// not the specific grade. The same gemRateID appears for PSA 10, PSA 9, etc.
type CatalogCard struct {
	ID                     string   `json:"id"`
	GemRateID              string   `json:"gemRateId"`
	PSASpecID              int      `json:"psaSpecId"`
	Label                  string   `json:"label"`
	Player                 string   `json:"player"`
	PlayerIndexID          string   `json:"playerIndexId"`
	Set                    string   `json:"set"`
	Year                   string   `json:"year"`
	Number                 string   `json:"number"`
	Variation              string   `json:"variation"`
	Category               string   `json:"category"`
	Condition              string   `json:"condition"`
	GradingCompany         string   `json:"gradingCompany"`
	CurrentValue           float64  `json:"currentValue"`
	MarketValue            float64  `json:"marketValue"`
	Pop                    *int     `json:"pop"`
	NumSales               int      `json:"numSales"`
	MarketCap              *float64 `json:"marketCap"`
	Score                  float64  `json:"score"`
	WeeklyPercentChange    float64  `json:"weeklyPercentChange"`
	MonthlyPercentChange   float64  `json:"monthlyPercentChange"`
	QuarterlyPercentChange float64  `json:"quarterlyPercentChange"`
	AnnualPercentChange    float64  `json:"annualPercentChange"`
	PriceMovement          float64  `json:"priceMovement"`
	LastSoldDate           string   `json:"lastSoldDate"`
	Slug                   string   `json:"slug"`
	KeyCard                bool     `json:"keyCard"`
	Image                  string   `json:"image"`
	EbayQuery              string   `json:"ebayQuery"`
}

// FirebaseAuthResponse is returned by the Firebase signInWithPassword endpoint.
type FirebaseAuthResponse struct {
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    string `json:"expiresIn"`
	LocalID      string `json:"localId"`
}

// FirebaseRefreshResponse is returned by the Firebase token refresh endpoint.
type FirebaseRefreshResponse struct {
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    string `json:"expires_in"`
}

// TokenState holds the current auth token and its expiry.
type TokenState struct {
	IDToken   string
	ExpiresAt time.Time
}

// callableRequest wraps data for Firebase callable function calls.
type callableRequest struct {
	Data any `json:"data"`
}

// callableResponse wraps the result from Firebase callable function calls.
type callableResponse[T any] struct {
	Result T `json:"result"`
}

// BuildCardRequest is the input for httpbuildcollectioncard.
type BuildCardRequest struct {
	Cert   string `json:"cert"`
	Grader string `json:"grader"`
}

// BuildCardResponse is the result from httpbuildcollectioncard.
type BuildCardResponse struct {
	Pop              int    `json:"pop"`
	Year             string `json:"year"`
	Set              string `json:"set"`
	Category         string `json:"category"`
	Number           string `json:"number"`
	Player           string `json:"player"`
	Variation        string `json:"variation"`
	Condition        string `json:"condition"`
	ImageURL         string `json:"imageUrl"`
	ImageBackURL     string `json:"imageBackUrl"`
	GemRateID        string `json:"gemRateId"`
	GemRateCondition string `json:"gemRateCondition"`
	SlabSerial       string `json:"slabSerial"`
	GradingCompany   string `json:"gradingCompany"`
}

// CardEstimateRequest is the input for httpcardestimate.
type CardEstimateRequest struct {
	GemRateID      string `json:"gemRateId"`
	GradingCompany string `json:"gradingCompany"`
	Condition      string `json:"condition"`
	Description    string `json:"description"`
}

// VelocityData holds sales velocity for a time window.
type VelocityData struct {
	Velocity     int     `json:"velocity"`
	AveragePrice float64 `json:"averagePrice"`
}

// CardEstimateResponse is the result from httpcardestimate.
type CardEstimateResponse struct {
	EstimatedValue     float64      `json:"estimatedValue"`
	LastSaleDate       string       `json:"lastSaleDate"`
	LastSalePrice      float64      `json:"lastSalePrice"`
	Confidence         int          `json:"confidence"`
	Grader             string       `json:"grader"`
	Index              string       `json:"index"`
	IndexID            string       `json:"indexId"`
	Description        string       `json:"description"`
	Grade              string       `json:"grade"`
	TwoWeekData        VelocityData `json:"twoWeekData"`
	OneMonthData       VelocityData `json:"oneMonthData"`
	OneQuarterData     VelocityData `json:"oneQuarterData"`
	OneYearData        VelocityData `json:"oneYearData"`
	IsPlayerIndex      bool         `json:"isPlayerIndex"`
	IndexPercentChange float64      `json:"indexPercentChange"`
}

// AddCollectionCardInput holds everything needed to create a card in a CL collection.
type AddCollectionCardInput struct {
	// Card metadata (from BuildCollectionCard)
	Label            string
	Player           string
	PlayerIndexID    string
	Category         string
	Year             string
	Set              string
	Number           string
	Variation        string
	Condition        string // e.g. "PSA 9"
	GradingCompany   string // e.g. "psa"
	GemRateID        string
	GemRateCondition string // e.g. "g9"
	SlabSerial       string
	Pop              int
	ImageURL         string
	ImageBackURL     string

	// Valuation (from CardEstimate)
	CurrentValue float64
	Investment   float64 // purchase cost in USD (dollars); CL stores whole-dollar integers

	// Purchase date
	DatePurchased time.Time
}

// CardPushParams holds the parameters needed to resolve and create a card in CL.
type CardPushParams struct {
	CertNumber    string
	Grader        string
	InvestmentUSD float64
	DatePurchased string // YYYY-MM-DD, optional
}

// CardPushResult holds the result of resolving and creating a card in CL.
type CardPushResult struct {
	DocumentName     string
	Player           string
	Set              string
	Condition        string
	EstimatedValue   float64
	GemRateID        string
	GemRateCondition string
}
