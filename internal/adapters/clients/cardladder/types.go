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
