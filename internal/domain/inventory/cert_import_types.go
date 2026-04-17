package inventory

// CertImportRequest is the input for POST /api/purchases/import-certs.
type CertImportRequest struct {
	CertNumbers []string `json:"certNumbers"`
}

// CertImportResult tallies imported, already-existing, sold, and failed certs.
type CertImportResult struct {
	Imported       int                  `json:"imported"`
	AlreadyExisted int                  `json:"alreadyExisted"`
	SoldExisting   int                  `json:"soldExisting"`
	Failed         int                  `json:"failed"`
	Errors         []CertImportError    `json:"errors"`
	SoldItems      []CertImportSoldItem `json:"soldItems,omitempty"`
}

// CertImportSoldItem represents a cert that exists but is currently sold.
type CertImportSoldItem struct {
	CertNumber string `json:"certNumber"`
	PurchaseID string `json:"purchaseId"`
	CardName   string `json:"cardName"`
	CampaignID string `json:"campaignId"`
}

type CertImportError struct {
	CertNumber string `json:"certNumber"`
	Error      string `json:"error"`
}

// ScanCertRequest is the input for POST /api/purchases/scan-cert.
type ScanCertRequest struct {
	CertNumber string `json:"certNumber"`
}

// ScanCertResult is the response from POST /api/purchases/scan-cert.
type ScanCertResult struct {
	Status       string          `json:"status"` // "existing", "sold", "new"
	CardName     string          `json:"cardName,omitempty"`
	PurchaseID   string          `json:"purchaseId,omitempty"`
	CampaignID   string          `json:"campaignId,omitempty"`
	BuyCostCents int             `json:"buyCostCents,omitempty"`
	Market       *MarketSnapshot `json:"market,omitempty"`
}

// ResolveCertRequest is the input for POST /api/purchases/resolve-cert.
type ResolveCertRequest struct {
	CertNumber string `json:"certNumber"`
}

// ResolveCertResult is the response from POST /api/purchases/resolve-cert.
type ResolveCertResult struct {
	CertNumber string  `json:"certNumber"`
	CardName   string  `json:"cardName"`
	Grade      float64 `json:"grade"`
	Year       string  `json:"year"`
	Category   string  `json:"category"`
	Subject    string  `json:"subject"`
}
