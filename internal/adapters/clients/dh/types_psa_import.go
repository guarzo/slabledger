package dh

// --- PSA Import Resolution Constants ---

const (
	// PSAImportStatusMatched means DH matched the cert to an existing catalog card.
	PSAImportStatusMatched = "matched"
	// PSAImportStatusUnmatchedCreated means DH created a private "partner-submitted"
	// card tied to the account and linked inventory to it.
	PSAImportStatusUnmatchedCreated = "unmatched_created"
	// PSAImportStatusPSAError means PSA itself couldn't resolve the cert.
	PSAImportStatusPSAError = "psa_error"
	// PSAImportStatusPartnerCardError means PSA resolved but an override was invalid
	// or persistence failed. The Error field on the row explains why.
	PSAImportStatusPartnerCardError = "partner_card_error"
)

// --- PSA Import Language/Rarity Enum Values ---

const (
	PSAImportLanguageEnglish  = "english"
	PSAImportLanguageJapanese = "japanese"
	PSAImportLanguageGerman   = "german"
	PSAImportLanguageFrench   = "french"
	PSAImportLanguageItalian  = "italian"
	PSAImportLanguageSpanish  = "spanish"
	PSAImportLanguageKorean   = "korean"
	PSAImportLanguageChinese  = "chinese"
)

// PSAImportOverrides are merged per-field over PSA's metadata (override wins).
// Omit fields you don't want to override. Language and Rarity must match DH's
// enum keys or the row fails as partner_card_error.
type PSAImportOverrides struct {
	Name       string `json:"name,omitempty"`
	SetName    string `json:"set_name,omitempty"`
	CardNumber string `json:"card_number,omitempty"`
	Language   string `json:"language,omitempty"`
	Year       string `json:"year,omitempty"`
	Rarity     string `json:"rarity,omitempty"`
}

// PSAImportItem is a single cert to import via POST /enterprise/inventory/psa_import.
type PSAImportItem struct {
	CertNumber         string              `json:"cert_number"`
	CostBasisCents     int                 `json:"cost_basis_cents,omitempty"`
	ListingPriceCents  *int                `json:"listing_price_cents,omitempty"`
	ShippingPriceCents *int                `json:"shipping_price_cents,omitempty"`
	Status             string              `json:"status,omitempty"` // "in_stock" (default) or "listed"
	Overrides          *PSAImportOverrides `json:"overrides,omitempty"`
}

// PSAImportRequest is the request body for POST /enterprise/inventory/psa_import.
type PSAImportRequest struct {
	PSAAPIKey string          `json:"psa_api_key"`
	Items     []PSAImportItem `json:"items"`
}

// PSAImportResult is a per-cert result from the psa_import endpoint.
// Resolution is one of: matched, unmatched_created, psa_error, partner_card_error.
type PSAImportResult struct {
	CertNumber    string `json:"cert_number"`
	Resolution    string `json:"resolution"`
	DHCardID      int    `json:"dh_card_id,omitempty"`
	DHInventoryID int    `json:"dh_inventory_id,omitempty"`
	CardName      string `json:"card_name,omitempty"`
	SetName       string `json:"set_name,omitempty"`
	CardNumber    string `json:"card_number,omitempty"`
	Status        string `json:"status,omitempty"` // inventory status ("in_stock" | "listed")
	Error         string `json:"error,omitempty"`
}

// PSAImportSummary is the per-resolution count block returned with the response.
type PSAImportSummary struct {
	Matched          int `json:"matched"`
	UnmatchedCreated int `json:"unmatched_created"`
	PSAError         int `json:"psa_error"`
	PartnerCardError int `json:"partner_card_error"`
}

// PSAImportRateLimit reflects PSA rate-limit info DH returns with the response.
type PSAImportRateLimit struct {
	Limit     int    `json:"limit,omitempty"`
	Remaining int    `json:"remaining,omitempty"`
	ResetAt   string `json:"reset_at,omitempty"`
}

// PSAImportResponse is the response from POST /enterprise/inventory/psa_import.
type PSAImportResponse struct {
	Results   []PSAImportResult   `json:"results"`
	Summary   PSAImportSummary    `json:"summary"`
	RateLimit *PSAImportRateLimit `json:"rate_limit,omitempty"`
}

// PSAImportMaxItems is the per-request item cap DH enforces.
const PSAImportMaxItems = 50
