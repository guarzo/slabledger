package dh

// --- Cert Resolution Status Constants ---

const (
	CertStatusMatched   = "matched"
	CertStatusAmbiguous = "ambiguous"
	CertStatusNotFound  = "not_found"
)

// --- Cert Resolution Types ---

// CertResolveRequest is a single cert to resolve.
type CertResolveRequest struct {
	CertNumber string `json:"cert_number"`
	CardName   string `json:"card_name,omitempty"`
	SetName    string `json:"set_name,omitempty"`
	CardNumber string `json:"card_number,omitempty"`
	Year       string `json:"year,omitempty"`
	Variant    string `json:"variant,omitempty"`
	Language   string `json:"language,omitempty"`
}

// CertResolution is the result of resolving a single cert.
type CertResolution struct {
	CertNumber              string                    `json:"cert_number"`
	Status                  string                    `json:"status"` // "matched", "ambiguous", "not_found"
	DHCardID                int                       `json:"dh_card_id,omitempty"`
	CardName                string                    `json:"card_name,omitempty"`
	SetName                 string                    `json:"set_name,omitempty"`
	CardNumber              string                    `json:"card_number,omitempty"`
	Grade                   float64                   `json:"grade,omitempty"`
	ImageURL                string                    `json:"image_url,omitempty"`
	CurrentMarketPriceCents int                       `json:"current_market_price_cents,omitempty"`
	Candidates              []CertResolutionCandidate `json:"candidates,omitempty"`
}

// CertResolutionCandidate is one possible match for an ambiguous cert.
type CertResolutionCandidate struct {
	DHCardID   int    `json:"dh_card_id"`
	CardName   string `json:"card_name"`
	SetName    string `json:"set_name"`
	CardNumber string `json:"card_number"`
	ImageURL   string `json:"image_url"`
}

// CertResolveBody is the request body for POST /enterprise/certs/resolve (single cert).
type CertResolveBody struct {
	Cert CertResolveRequest `json:"cert"`
}

// CertResolveBatchRequest is the request body for POST /enterprise/certs/resolve_batch.
type CertResolveBatchRequest struct {
	Certs []CertResolveRequest `json:"certs"`
}

// CertResolveBatchResponse is the response from POST /enterprise/certs/resolve_batch.
type CertResolveBatchResponse struct {
	JobID      string `json:"job_id"`
	Status     string `json:"status"` // "queued"
	TotalCerts int    `json:"total_certs"`
}

// CertResolutionJobStatus is the response from GET /enterprise/certs/resolve_batch/:job_id.
type CertResolutionJobStatus struct {
	JobID         string           `json:"job_id"`
	Status        string           `json:"status"` // "queued", "processing", "completed", "failed"
	TotalCerts    int              `json:"total_certs"`
	ResolvedCount int              `json:"resolved_count"`
	Results       []CertResolution `json:"results,omitempty"`
}

// --- Inventory Types ---

// InventoryItem is a single item to push to DH inventory.
type InventoryItem struct {
	DHCardID       int     `json:"dh_card_id"`
	CertNumber     string  `json:"cert_number"`
	GradingCompany string  `json:"grading_company"`
	Grade          float64 `json:"grade"`
	CostBasisCents int     `json:"cost_basis_cents"`
}

// InventoryPushRequest is the request body for POST /inventory.
type InventoryPushRequest struct {
	Items []InventoryItem `json:"items"`
}

// InventoryChannelStatus is the per-channel sync status.
type InventoryChannelStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "pending", "active", "error"
}

// InventoryResult is the per-item response from inventory push.
type InventoryResult struct {
	DHInventoryID      int                      `json:"dh_inventory_id"`
	CertNumber         string                   `json:"cert_number"`
	Status             string                   `json:"status"` // "active", "pending", "failed"
	AssignedPriceCents int                      `json:"assigned_price_cents"`
	Channels           []InventoryChannelStatus `json:"channels,omitempty"`
	Error              string                   `json:"error,omitempty"`
}

// InventoryPushResponse is the response from POST /inventory.
type InventoryPushResponse struct {
	Results []InventoryResult `json:"results"`
}

// InventoryListItem is a single item in the inventory list response.
type InventoryListItem struct {
	DHInventoryID     int                      `json:"dh_inventory_id"`
	DHCardID          int                      `json:"dh_card_id"`
	CertNumber        string                   `json:"cert_number"`
	CardName          string                   `json:"card_name"`
	SetName           string                   `json:"set_name"`
	CardNumber        string                   `json:"card_number"`
	GradingCompany    string                   `json:"grading_company"`
	Grade             float64                  `json:"grade"`
	Status            string                   `json:"status"`
	ListingPriceCents int                      `json:"listing_price_cents"`
	CostBasisCents    int                      `json:"cost_basis_cents"`
	Channels          []InventoryChannelStatus `json:"channels,omitempty"`
	CreatedAt         string                   `json:"created_at"`
	UpdatedAt         string                   `json:"updated_at"`
}

// InventoryListResponse is the response from GET /inventory.
type InventoryListResponse struct {
	Items []InventoryListItem `json:"items"`
	Meta  PaginationMeta      `json:"meta"`
}

// PaginationMeta holds pagination metadata.
type PaginationMeta struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalCount int `json:"total_count"`
}

// InventoryUpdate is the request body for PATCH /inventory/:id.
type InventoryUpdate struct {
	CostBasisCents int `json:"cost_basis_cents"`
}

// --- Orders Types ---

// Order is a single completed sale from DH.
type Order struct {
	OrderID        string    `json:"order_id"`
	CertNumber     string    `json:"cert_number"`
	DHCardID       int       `json:"dh_card_id"`
	CardName       string    `json:"card_name"`
	SetName        string    `json:"set_name"`
	Grade          float64   `json:"grade"`
	SalePriceCents int       `json:"sale_price_cents"`
	Channel        string    `json:"channel"` // "dh", "ebay", "shopify"
	Fees           OrderFees `json:"fees"`
	NetAmountCents *int      `json:"net_amount_cents"` // nullable
	SoldAt         string    `json:"sold_at"`          // ISO 8601
}

// OrderFees is the fee breakdown for an order.
type OrderFees struct {
	ChannelFeeCents *int `json:"channel_fee_cents"` // nullable
	CommissionCents *int `json:"commission_cents"`  // nullable
}

// OrdersResponse is the response from GET /orders.
type OrdersResponse struct {
	Orders []Order        `json:"orders"`
	Meta   PaginationMeta `json:"meta"`
}
