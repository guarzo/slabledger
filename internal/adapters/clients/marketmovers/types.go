package marketmovers

import "time"

// tRPCResponse is the envelope for a successful tRPC query response.
type tRPCResponse[T any] struct {
	Result struct {
		Data T `json:"data"`
	} `json:"result"`
}

// TokenState holds the in-memory access token and its expiry.
type TokenState struct {
	AccessToken string
	ExpiresAt   time.Time
}

// AuthLoginResponse is returned by auth.loginWithBasicAuth.
type AuthLoginResponse struct {
	AccessToken  string   `json:"accessToken"`
	RefreshToken string   `json:"refreshToken"`
	User         UserInfo `json:"user"`
}

// AuthRefreshResponse is returned by auth.getFreshAccessToken.
type AuthRefreshResponse struct {
	AccessToken string   `json:"accessToken"`
	User        UserInfo `json:"user"`
}

// UserInfo contains the user fields embedded in auth responses.
type UserInfo struct {
	UserID         string `json:"userId"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	MembershipTier string `json:"membershipTier"`
}

// ActiveSalesResponse is the data envelope from private.sales.active.
type ActiveSalesResponse struct {
	Items      []ActiveSale `json:"items"`
	TotalCount int          `json:"totalCount"`
}

// ActiveSale represents one active listing from private.sales.active.
type ActiveSale struct {
	SaleID                  int64   `json:"saleId"`
	CollectibleID           int64   `json:"collectibleId"`
	CollectibleType         string  `json:"collectibleType"`
	SaleTitle               string  `json:"saleTitle"`
	LastSalePrice           float64 `json:"lastSalePrice"`
	IsBestOfferEnabled      bool    `json:"isBestOfferEnabled"`
	IsBuyItNowAvailable     bool    `json:"isBuyItNowAvailable"`
	BuyItNowPrice           float64 `json:"buyItNowPrice"`
	SalePrice               float64 `json:"salePrice"`
	FinalPrice              float64 `json:"finalPrice"`
	LastSalePriceDiffAmount float64 `json:"lastSalePriceDiffAmount"`
	LastSalePriceDiffPct    float64 `json:"lastSalePriceDiffPercentage"`
	LastSaleDate            string  `json:"lastSaleDate"`
	ListingType             string  `json:"listingType"`
	SaleURL                 string  `json:"saleUrl"`
	ImageURL                string  `json:"imageUrl"`
	Recommendation          int     `json:"recommendation"`
	Marketplace             string  `json:"marketplace"`
	SalePlatform            string  `json:"salePlatform"`
	EndTime                 string  `json:"endTime"`
}

// CompletedSalesResponse is the data envelope from private.sales.completed.
type CompletedSalesResponse struct {
	Items []CompletedSale `json:"items"`
	Count int             `json:"count"`
}

// CompletedSale represents one sold listing from private.sales.completed.
type CompletedSale struct {
	SaleID               int64   `json:"saleId"`
	CollectibleID        int64   `json:"collectibleId"`
	SaleTitle            string  `json:"saleTitle"`
	ImageURL             string  `json:"imageUrl"`
	SaleURL              string  `json:"saleUrl"`
	SellerName           string  `json:"sellerName"`
	SaleDate             string  `json:"saleDate"`
	FormattedSaleDate    string  `json:"formattedSaleDate"`
	NumberOfBids         *int    `json:"numberOfBids"`
	ListingType          string  `json:"listingType"`
	FinalPrice           float64 `json:"finalPrice"`
	OriginalSellingPrice float64 `json:"originalSellingPrice"`
	SellerFeedbackRating int     `json:"sellerFeedbackRating"`
	ExternalSaleID       string  `json:"externalSaleId"`
	SalePlatform         string  `json:"salePlatform"`
	IsBestOfferAccepted  bool    `json:"isBestOfferAccepted"`
}

// CompletedSummariesResponse is the data envelope from private.sales.completedSummaries.
type CompletedSummariesResponse struct {
	Items      []DailySummary `json:"items"`
	TotalCount int            `json:"totalCount"`
}

// DailySummary represents one day's aggregated sales from private.sales.completedSummaries.
type DailySummary struct {
	CollectibleID    int64   `json:"collectibleId"`
	FormattedDate    string  `json:"formattedDate"`
	TotalSalesCount  int     `json:"totalSalesCount"`
	AverageSalePrice float64 `json:"averageSalePrice"`
}

// DailyStatsResponse is the data envelope from private.collectibles.stats.dailyStatsV2.
type DailyStatsResponse struct {
	DailyStats []DailyStatItem `json:"dailyStats"`
}

// DailyStatItem represents one day's aggregated stats from dailyStatsV2.
// Note: the API has a typo in "averageSalePriceChangeParcentage" — preserved here intentionally.
type DailyStatItem struct {
	CollectibleID                    int64   `json:"collectibleId"`
	FormattedDate                    string  `json:"formattedDate"` // ISO: "2026-03-09"
	TotalSalesAmount                 float64 `json:"totalSalesAmount"`
	TotalSalesCount                  int     `json:"totalSalesCount"`
	AverageSalePrice                 float64 `json:"averageSalePrice"`
	MinSalePrice                     float64 `json:"minSalePrice"`
	MaxSalePrice                     float64 `json:"maxSalePrice"`
	AverageSalePriceChangeAmount     float64 `json:"averageSalePriceChangeAmount"`
	AverageSalePriceChangeParcentage float64 `json:"averageSalePriceChangeParcentage"` //nolint:misspell // API typo
}

// CollectiblesSearchResponse is the data envelope from private.collectibles.search.
type CollectiblesSearchResponse struct {
	Items []CollectibleSearchResult `json:"items"`
}

// CollectibleSearchResult is one result from private.collectibles.search.
type CollectibleSearchResult struct {
	Item CollectibleItem `json:"item"`
}

// CollectibleItem is the card/collectible metadata from the search index.
type CollectibleItem struct {
	ID              int64            `json:"id"`
	MasterID        int64            `json:"masterId"` // Grade-agnostic variant ID shared across all grades of the same card
	SearchTitle     string           `json:"searchTitle"`
	CollectibleType string           `json:"collectibleType"`
	ImageURL        string           `json:"imageUrl"`
	Query           string           `json:"query"`
	Stats           CollectibleStats `json:"stats"`
}

// CollectibleStats holds the pricing stats for a collectible.
type CollectibleStats struct {
	Last30 PeriodStats `json:"last30"`
	Last90 PeriodStats `json:"last90"`
}

// PeriodStats holds aggregated sales stats for a time window.
type PeriodStats struct {
	AvgPrice          float64  `json:"avgPrice"`
	MaxPrice          float64  `json:"maxPrice"`
	MinPrice          float64  `json:"minPrice"`
	TotalSalesCount   int      `json:"totalSalesCount"`
	TotalSalesAmount  float64  `json:"totalSalesAmount"`
	PriceChangeAmount *float64 `json:"priceChangeAmount"`
	PriceChangePct    *float64 `json:"priceChangePercentage"`
}

// ──────────────────────────────────────────────────────────────────────
// Collection mutation types (private.collection.items.add / addMultiple)
// ──────────────────────────────────────────────────────────────────────

// AddCollectionItemInput is the input for the private.collection.items.add mutation.
type AddCollectionItemInput struct {
	Collectible     CollectionCollectible     `json:"collectible"`
	PurchaseDetails CollectionPurchaseDetails `json:"purchaseDetails"`
	CategoryIDs     *[]int64                  `json:"categoryIds"` // null means no category
}

// CollectionCollectible identifies the collectible to add.
type CollectionCollectible struct {
	CollectibleType string `json:"collectibleType"`
	CollectibleID   int64  `json:"collectibleId"`
	ImageURL        string `json:"imageUrl,omitempty"`
}

// CollectionPurchaseDetails holds purchase metadata for a collection item.
type CollectionPurchaseDetails struct {
	Quantity             int     `json:"quantity"`
	PurchasePricePerItem float64 `json:"purchasePricePerItem"` // USD
	ConversionFeePerItem float64 `json:"conversionFeePerItem"`
	PurchaseDateISO      string  `json:"purchaseDateISO"` // "2006-01-02"
	Notes                string  `json:"notes"`
}

// AddCollectionItemResponse is the response from private.collection.items.add.
type AddCollectionItemResponse struct {
	Success             bool  `json:"success"`
	CollectibleID       int64 `json:"collectibleId"`
	IsCustomCollectible bool  `json:"isCustomCollectible"`
	CollectionItemID    int64 `json:"collectionItemId"`
}

// AddMultipleCollectionItemsResponse is the response from private.collection.items.addMultiple.
type AddMultipleCollectionItemsResponse struct {
	Items []AddCollectionItemResponse `json:"items"`
}
