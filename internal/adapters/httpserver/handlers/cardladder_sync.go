package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CLPurchaseLister lists unsold purchases for syncing to Card Ladder.
type CLPurchaseLister interface {
	ListAllUnsoldPurchases(ctx context.Context) ([]campaigns.Purchase, error)
}

// SetPurchaseLister injects the purchase lister for sync operations.
func (h *CardLadderHandler) SetPurchaseLister(lister CLPurchaseLister) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.purchaseLister = lister
}

type addCardRequest struct {
	CertNumber    string  `json:"certNumber"`
	Grader        string  `json:"grader"`
	InvestmentUSD float64 `json:"investment"`    // cost basis in dollars
	DatePurchased string  `json:"datePurchased"` // YYYY-MM-DD, optional
}

type addCardResult struct {
	CertNumber string  `json:"certNumber"`
	Player     string  `json:"player"`
	Set        string  `json:"set"`
	Condition  string  `json:"condition"`
	Value      float64 `json:"estimatedValue"`
	Status     string  `json:"status"` // "added", "skipped", "error"
	Error      string  `json:"error,omitempty"`
}

// HandleAddCard adds a single card to the Card Ladder collection by cert number.
func (h *CardLadderHandler) HandleAddCard(w http.ResponseWriter, r *http.Request) {
	var req addCardRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.CertNumber == "" {
		writeError(w, http.StatusBadRequest, "certNumber is required")
		return
	}
	if req.Grader == "" {
		req.Grader = "psa"
	}

	cfg, err := h.store.GetConfig(r.Context())
	if err != nil || cfg == nil {
		writeError(w, http.StatusPreconditionFailed, "Card Ladder not configured")
		return
	}

	result, err := h.addCardToCollection(r.Context(), cfg.FirebaseUID, cfg.CollectionID, req)
	if err != nil {
		h.logger.Error(r.Context(), "add card to CL failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleSyncToCardLadder pushes unsold purchases with cert numbers to the
// Card Ladder collection. Cards already present (by cert number mapping) are skipped.
func (h *CardLadderHandler) HandleSyncToCardLadder(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	lister := h.purchaseLister
	h.mu.Unlock()
	if lister == nil {
		writeError(w, http.StatusServiceUnavailable, "purchase lister not available")
		return
	}

	cfg, err := h.store.GetConfig(r.Context())
	if err != nil || cfg == nil {
		writeError(w, http.StatusPreconditionFailed, "Card Ladder not configured")
		return
	}

	purchases, err := lister.ListAllUnsoldPurchases(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "list unsold purchases failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to list purchases")
		return
	}

	// Filter to purchases with cert numbers and check existing mappings
	var toSync []addCardRequest
	for _, p := range purchases {
		if p.CertNumber == "" {
			continue
		}
		mapping, err := h.store.GetMapping(r.Context(), p.CertNumber)
		if err != nil {
			continue
		}
		if mapping != nil {
			continue // already synced
		}
		grader := strings.ToLower(p.Grader)
		if grader == "" {
			grader = "psa"
		}
		toSync = append(toSync, addCardRequest{
			CertNumber:    p.CertNumber,
			Grader:        grader,
			InvestmentUSD: float64(p.BuyCostCents) / 100.0,
			DatePurchased: p.PurchaseDate,
		})
	}

	var results []addCardResult
	var added, skipped, errCount int
	for _, req := range toSync {
		result, err := h.addCardToCollection(r.Context(), cfg.FirebaseUID, cfg.CollectionID, req)
		if err != nil {
			results = append(results, addCardResult{
				CertNumber: req.CertNumber,
				Status:     "error",
				Error:      err.Error(),
			})
			errCount++
			continue
		}
		results = append(results, *result)
		if result.Status == "added" {
			added++
		} else {
			skipped++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"added":   added,
		"skipped": skipped,
		"errors":  errCount,
		"total":   len(toSync),
		"results": results,
	})
}

// addCardToCollection resolves a cert number via Cloud Functions and writes the card
// to the CardLadder Firestore collection.
func (h *CardLadderHandler) addCardToCollection(ctx context.Context, uid, collectionID string, req addCardRequest) (*addCardResult, error) {
	// Step 1: Resolve cert to card metadata
	buildResp, err := h.client.BuildCollectionCard(ctx, req.CertNumber, req.Grader)
	if err != nil {
		return nil, fmt.Errorf("resolve cert %s: %w", req.CertNumber, err)
	}

	// Step 2: Get current market estimate
	estimateResp, err := h.client.CardEstimate(ctx, cardladder.CardEstimateRequest{
		GemRateID:      buildResp.GemRateID,
		GradingCompany: buildResp.GradingCompany,
		Condition:      buildResp.GemRateCondition,
		Description:    buildResp.Player,
	})
	if err != nil {
		return nil, fmt.Errorf("estimate cert %s: %w", req.CertNumber, err)
	}

	// Step 3: Build the label (matches CL format)
	label := fmt.Sprintf("%s %s %s %s #%s %s",
		buildResp.Year, buildResp.Set, buildResp.Player,
		buildResp.Variation, buildResp.Number, buildResp.Condition)

	// Parse purchase date
	var datePurchased time.Time
	if req.DatePurchased != "" {
		datePurchased, _ = time.Parse("2006-01-02", req.DatePurchased)
	}

	// Step 4: Write to Firestore
	input := cardladder.AddCollectionCardInput{
		Label:            label,
		Player:           buildResp.Player,
		PlayerIndexID:    estimateResp.IndexID,
		Category:         buildResp.Category,
		Year:             buildResp.Year,
		Set:              buildResp.Set,
		Number:           buildResp.Number,
		Variation:        buildResp.Variation,
		Condition:        buildResp.Condition,
		GradingCompany:   buildResp.GradingCompany,
		GemRateID:        buildResp.GemRateID,
		GemRateCondition: buildResp.GemRateCondition,
		SlabSerial:       buildResp.SlabSerial,
		Pop:              buildResp.Pop,
		ImageURL:         buildResp.ImageURL,
		ImageBackURL:     buildResp.ImageBackURL,
		CurrentValue:     estimateResp.EstimatedValue,
		Investment:       req.InvestmentUSD,
		DatePurchased:    datePurchased,
	}

	docName, err := h.client.CreateCollectionCard(ctx, uid, collectionID, input)
	if err != nil {
		return nil, fmt.Errorf("create card in Firestore: %w", err)
	}

	// Step 5: Save the local mapping
	if err := h.store.SaveMapping(ctx, req.CertNumber, docName, buildResp.GemRateID, buildResp.GemRateCondition); err != nil {
		h.logger.Error(ctx, "failed to save CL mapping after Firestore write",
			observability.String("cert", req.CertNumber), observability.Err(err))
	}

	return &addCardResult{
		CertNumber: req.CertNumber,
		Player:     buildResp.Player,
		Set:        buildResp.Set,
		Condition:  buildResp.Condition,
		Value:      estimateResp.EstimatedValue,
		Status:     "added",
	}, nil
}
