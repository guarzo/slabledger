package campaigns

// CertImportRequest holds the input for cert-based import.
type CertImportRequest struct {
	CertNumbers []string `json:"certNumbers"`
}

// CertImportResult holds the outcome of a cert-based import.
type CertImportResult struct {
	Imported       int               `json:"imported"`
	AlreadyExisted int               `json:"alreadyExisted"`
	Failed         int               `json:"failed"`
	Errors         []CertImportError `json:"errors"`
}

// CertImportError describes a single cert that failed to import.
type CertImportError struct {
	CertNumber string `json:"certNumber"`
	Error      string `json:"error"`
}

// EbayExportItem holds one purchase's data for the eBay export review screen.
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

// EbayExportListResponse is the API response for listing items to export.
type EbayExportListResponse struct {
	Items []EbayExportItem `json:"items"`
}

// EbayExportGenerateItem is one item in the generate request with the user's chosen price.
type EbayExportGenerateItem struct {
	PurchaseID string `json:"purchaseId"`
	PriceCents int    `json:"priceCents"`
}

// EbayExportGenerateRequest is the request body for generating the eBay CSV.
type EbayExportGenerateRequest struct {
	Items []EbayExportGenerateItem `json:"items"`
}
