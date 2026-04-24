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

	// Metadata for the intake-row DH-search helper. Populated only for
	// "existing" and "sold" statuses from the underlying Purchase record.
	FrontImageURL string  `json:"frontImageUrl,omitempty"`
	SetName       string  `json:"setName,omitempty"`
	CardNumber    string  `json:"cardNumber,omitempty"`
	CardYear      string  `json:"cardYear,omitempty"`
	GradeValue    float64 `json:"gradeValue,omitempty"`
	Population    int     `json:"population,omitempty"`

	// DHSearchQuery is a pre-normalized query string for the DH marketplace
	// search UI — built with the same cardutil pipeline used for backend DH
	// card matching so the operator's "Search on DH" link lands on the same
	// candidates DH's own matcher would consider.
	DHSearchQuery string `json:"dhSearchQuery,omitempty"`

	// DH pipeline state — lets the intake screen unblock once the card is
	// pushable on DH, even if the market snapshot hasn't landed yet.
	DHCardID      int    `json:"dhCardId,omitempty"`
	DHInventoryID int    `json:"dhInventoryId,omitempty"`
	DHPushStatus  string `json:"dhPushStatus,omitempty"`
	DHStatus      string `json:"dhStatus,omitempty"`

	// Sale-mode fields — pricing context + in-hand status
	DHListingPriceCents int    `json:"dhListingPriceCents,omitempty"` // Current DH listing price
	ReceivedAt          string `json:"receivedAt,omitempty"`          // Non-empty = in-hand
}

// ScanCertsRequest is the input for POST /api/purchases/scan-certs, the batch
// variant used by the cert-intake polling loop to avoid rate-limiting itself.
type ScanCertsRequest struct {
	CertNumbers []string `json:"certNumbers"`
}

// ScanCertsResult partitions batch scan responses: successful cert scans go in
// Results (keyed by cert number), per-cert failures go in Errors. A cert
// appears in one or the other, never both. Callers that need to reconcile
// against their input list should check both maps.
type ScanCertsResult struct {
	Results map[string]*ScanCertResult `json:"results"`
	Errors  []CertImportError          `json:"errors,omitempty"`
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
