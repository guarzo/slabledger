package dh

// --- PSA Import Resolution Constants ---

const (
	// PSAImportStatusMatched means DH matched the cert to an existing catalog card.
	PSAImportStatusMatched = "matched"
	// PSAImportStatusUnmatchedCreated means DH created a private "partner-submitted"
	// card tied to the account and linked inventory to it.
	PSAImportStatusUnmatchedCreated = "unmatched_created"
	// PSAImportStatusOverrideCorrected means our overrides conflicted with DH's
	// resolver match; DH created a partner_submitted card from the overrides
	// instead. Response includes dh_card_id and dh_inventory_id. Rescue with
	// POST /enterprise/certs/confirm_match once the operator decides which
	// side is right.
	PSAImportStatusOverrideCorrected = "override_corrected"
	// PSAImportStatusAlreadyListed means the cert already had an active
	// market_orders row on DH that agreed with the resolver (or DH left the
	// existing listing alone because the resolver was uncertain). The response
	// includes dh_card_id + dh_inventory_id so we sync local state.
	PSAImportStatusAlreadyListed = "already_listed"
	// PSAImportStatusPSAError means PSA itself couldn't resolve the cert, or
	// a rate limit was hit. Check RateLimited on the result and the Error
	// string to decide whether to retry, rotate the PSA key, or give up.
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
// Resolution is one of: matched, unmatched_created, override_corrected,
// already_listed, psa_error, partner_card_error. See the PSAImportStatus*
// constants above for per-value semantics.
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
	// RateLimited is true when DH's psa_import rejected the cert because the
	// per-user daily PSA quota was exhausted OR PSA's own per-key rate limit
	// fired. Pair with Resolution=="psa_error". The scheduler rotates PSA
	// keys on this signal (comma-separated PSA_ACCESS_TOKEN entries) before
	// giving up for the cycle.
	RateLimited bool `json:"rate_limited,omitempty"`
}

// PSAImportSummary is the per-resolution count block returned with the response.
// Field names mirror DH's summary keys (matched_count, already_listed_count, etc.)
// with the _count suffix stripped so the JSON tag carries the full name.
type PSAImportSummary struct {
	Total             int `json:"total,omitempty"`
	Matched           int `json:"matched_count,omitempty"`
	UnmatchedCreated  int `json:"unmatched_created_count,omitempty"`
	OverrideCorrected int `json:"override_corrected_count,omitempty"`
	AlreadyListed     int `json:"already_listed_count,omitempty"`
	PSAError          int `json:"psa_error_count,omitempty"`
	PartnerCardError  int `json:"partner_card_error_count,omitempty"`
}

// PSAImportRateLimit reflects PSA rate-limit info DH returns with the response.
type PSAImportRateLimit struct {
	Limit     int    `json:"limit,omitempty"`
	Remaining int    `json:"remaining,omitempty"`
	ResetAt   string `json:"reset_at,omitempty"`
}

// PSAImportResponse is the response from POST /enterprise/inventory/psa_import.
// Success+Error are top-level batch-level status; if Success=false the batch
// was rejected before any cert was processed (e.g. >50 items, missing vendor
// profile, blank cert numbers). Per-cert outcomes live in Results.
type PSAImportResponse struct {
	Success   bool                `json:"success,omitempty"`
	Error     string              `json:"error,omitempty"`
	Results   []PSAImportResult   `json:"results"`
	Summary   PSAImportSummary    `json:"summary"`
	RateLimit *PSAImportRateLimit `json:"rate_limit,omitempty"`
}

// PSAImportMaxItems is the per-request item cap DH enforces.
const PSAImportMaxItems = 50
