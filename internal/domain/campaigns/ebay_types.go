package campaigns

// CertImportRequest is the input for POST /api/purchases/import-certs.
type CertImportRequest struct {
	CertNumbers []string `json:"certNumbers"`
}

// CertImportResult tallies imported, already-existing, and failed certs.
type CertImportResult struct {
	Imported       int               `json:"imported"`
	AlreadyExisted int               `json:"alreadyExisted"`
	Failed         int               `json:"failed"`
	Errors         []CertImportError `json:"errors"`
}

type CertImportError struct {
	CertNumber string `json:"certNumber"`
	Error      string `json:"error"`
}

// EbayExportItem is one row in the eBay export review screen.
// SuggestedPriceCents defaults to CLValueCents, falling back to MedianCents.
type EbayExportItem struct {
	PurchaseID          string  `json:"purchaseId"`
	CertNumber          string  `json:"certNumber"`
	CardName            string  `json:"cardName"`
	SetName             string  `json:"setName"`
	CardNumber          string  `json:"cardNumber"`
	CardYear            string  `json:"cardYear"`
	GradeValue          float64 `json:"gradeValue"`
	Grader              string  `json:"grader"`
	CLValueCents        int     `json:"clValueCents"`
	MarketMedianCents   int     `json:"marketMedianCents"`
	SuggestedPriceCents int     `json:"suggestedPriceCents"`
	HasCLValue          bool    `json:"hasCLValue"`
	HasMarketData       bool    `json:"hasMarketData"`
	FrontImageURL       string  `json:"frontImageUrl,omitempty"`
	BackImageURL        string  `json:"backImageUrl,omitempty"`
}

type EbayExportListResponse struct {
	Items []EbayExportItem `json:"items"`
}

// EbayExportGenerateItem pairs a purchase with the user's chosen listing price.
type EbayExportGenerateItem struct {
	PurchaseID string `json:"purchaseId"`
	PriceCents int    `json:"priceCents"`
}

type EbayExportGenerateRequest struct {
	Items []EbayExportGenerateItem `json:"items"`
}
