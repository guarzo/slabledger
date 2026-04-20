package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CertPriceLookup is the interface the pricing API handler depends on.
// Defined at the consumer level per idiomatic Go.
// Satisfied by *postgres.PurchaseStore (and test mocks).
type CertPriceLookup interface {
	GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error)
}

// PricingAPIHandler handles pricing API requests.
type PricingAPIHandler struct {
	lookup CertPriceLookup
	logger observability.Logger
}

// NewPricingAPIHandler creates a new pricing API handler.
func NewPricingAPIHandler(lookup CertPriceLookup, logger observability.Logger) *PricingAPIHandler {
	return &PricingAPIHandler{lookup: lookup, logger: logger}
}

// priceResult is the JSON response for a single price lookup.
type priceResult struct {
	CertNumber       string  `json:"certNumber"`
	SuggestedPrice   float64 `json:"suggestedPrice"`
	ComputedPrice    float64 `json:"computedPrice,omitempty"`
	OverridePrice    float64 `json:"overridePrice,omitempty"`
	AISuggestedPrice float64 `json:"aiSuggestedPrice,omitempty"`
	PriceSource      string  `json:"priceSource"`
	Currency         string  `json:"currency"`
}

// writePricingError writes a JSON error in the pricing API format.
func writePricingError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message}) //nolint:errcheck // response already committed; write error unactionable
}

// writePricingJSON writes a JSON success response.
func writePricingJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data) //nolint:errcheck // response already committed; write error unactionable
}

// centsToDollars converts cents to dollars.
// Integer cents always produce at most 2 decimal places.
func centsToDollars(cents int) float64 {
	return float64(cents) / 100
}

// buildPriceResult constructs a priceResult from a purchase, choosing the best price source.
func buildPriceResult(certNumber string, p *inventory.Purchase) priceResult {
	result := priceResult{
		CertNumber:    certNumber,
		ComputedPrice: centsToDollars(p.CLValueCents),
		PriceSource:   "cl_value",
		Currency:      "USD",
	}
	// Use CL value as default suggested price
	result.SuggestedPrice = result.ComputedPrice

	// Override takes priority
	if p.OverridePriceCents > 0 {
		result.OverridePrice = centsToDollars(p.OverridePriceCents)
		result.SuggestedPrice = result.OverridePrice
		result.PriceSource = "override"
	}

	// Surface pending AI suggestion
	if p.AISuggestedPriceCents > 0 {
		result.AISuggestedPrice = centsToDollars(p.AISuggestedPriceCents)
	}

	return result
}

// HandleSinglePrice handles GET /api/v1/prices/{certNumber}.
func (h *PricingAPIHandler) HandleSinglePrice(w http.ResponseWriter, r *http.Request) {
	certNumber := r.PathValue("certNumber")
	if certNumber == "" {
		writePricingError(w, http.StatusBadRequest, "validation_error", "certNumber is required")
		return
	}

	purchases, err := h.lookup.GetPurchasesByCertNumbers(r.Context(), []string{certNumber})
	if err != nil {
		if h.logger != nil {
			h.logger.Error(r.Context(), "pricing API: lookup failed",
				observability.String("certNumber", certNumber), observability.Err(err))
		}
		writePricingError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	p, ok := purchases[certNumber]
	if !ok || (p.CLValueCents == 0 && p.OverridePriceCents == 0 && p.AISuggestedPriceCents == 0) {
		writePricingError(w, http.StatusNotFound, "no_data",
			"No pricing data available for this certification number")
		return
	}

	writePricingJSON(w, http.StatusOK, buildPriceResult(certNumber, p))
}

// batchRequest is the JSON request body for batch price lookups.
type batchRequest struct {
	CertNumbers []string `json:"certNumbers"`
}

// batchResponse is the JSON response for batch price lookups.
type batchResponse struct {
	Results        []priceResult `json:"results"`
	NotFound       []string      `json:"notFound"`
	TotalRequested int           `json:"totalRequested"`
	TotalFound     int           `json:"totalFound"`
}

// HandleBatchPrices handles POST /api/v1/prices/batch.
func (h *PricingAPIHandler) HandleBatchPrices(w http.ResponseWriter, r *http.Request) {
	// Limit request body to 1MB
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req batchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writePricingError(w, http.StatusBadRequest, "validation_error", "Invalid request body")
		return
	}

	totalRequested := len(req.CertNumbers)

	// Validate
	if req.CertNumbers == nil || totalRequested == 0 {
		writePricingError(w, http.StatusBadRequest, "validation_error", "certNumbers array required, max 100 items")
		return
	}
	if totalRequested > 100 {
		writePricingError(w, http.StatusBadRequest, "validation_error", "certNumbers array required, max 100 items")
		return
	}
	if slices.Contains(req.CertNumbers, "") {
		writePricingError(w, http.StatusBadRequest, "validation_error", "Each certNumber must be a non-empty string")
		return
	}

	// Deduplicate while preserving order of first occurrence
	seen := make(map[string]bool, len(req.CertNumbers))
	deduped := make([]string, 0, len(req.CertNumbers))
	for _, cn := range req.CertNumbers {
		if !seen[cn] {
			seen[cn] = true
			deduped = append(deduped, cn)
		}
	}

	purchases, err := h.lookup.GetPurchasesByCertNumbers(r.Context(), deduped)
	if err != nil {
		if h.logger != nil {
			h.logger.Error(r.Context(), "pricing API: batch lookup failed", observability.Err(err))
		}
		writePricingError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	var results []priceResult
	var notFound []string
	for _, cn := range deduped {
		p, ok := purchases[cn]
		if !ok || (p.CLValueCents == 0 && p.OverridePriceCents == 0 && p.AISuggestedPriceCents == 0) {
			notFound = append(notFound, cn)
			continue
		}
		results = append(results, buildPriceResult(cn, p))
	}

	// Ensure non-nil slices in JSON output
	if results == nil {
		results = []priceResult{}
	}
	if notFound == nil {
		notFound = []string{}
	}

	writePricingJSON(w, http.StatusOK, batchResponse{
		Results:        results,
		NotFound:       notFound,
		TotalRequested: totalRequested,
		TotalFound:     len(results),
	})
}

// HandleHealth handles GET /api/v1/health.
func (h *PricingAPIHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	writePricingJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": "1.0.0",
	})
}
