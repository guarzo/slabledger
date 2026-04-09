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

// CLSyncUpdater sets cl_synced_at on a purchase after it is pushed to Card Ladder.
type CLSyncUpdater interface {
	UpdatePurchaseCLSyncedAt(ctx context.Context, purchaseID string, syncedAt string) error
}

// SetPurchaseLister injects the purchase lister for sync operations.
func (h *CardLadderHandler) SetPurchaseLister(lister CLPurchaseLister) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.purchaseLister = lister
}

// SetSyncUpdater injects the sync-timestamp updater for CL push.
func (h *CardLadderHandler) SetSyncUpdater(u CLSyncUpdater) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.syncUpdater = u
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
	Status     string  `json:"status"` // "synced", "skipped", "error"
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
	} else {
		req.Grader = strings.ToLower(req.Grader)
	}

	cfg, err := h.store.GetConfig(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get CL config", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get Card Ladder config")
		return
	}
	if cfg == nil {
		writeError(w, http.StatusPreconditionFailed, "Card Ladder not configured")
		return
	}

	result, err := h.addCardToCollection(r.Context(), cfg.FirebaseUID, cfg.CollectionID, "", req)
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
	if err != nil {
		h.logger.Error(r.Context(), "failed to get CL config", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get Card Ladder config")
		return
	}
	if cfg == nil {
		writeError(w, http.StatusPreconditionFailed, "Card Ladder not configured")
		return
	}

	purchases, err := lister.ListAllUnsoldPurchases(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "list unsold purchases failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to list purchases")
		return
	}

	// Load all existing mappings once to avoid N+1 queries
	allMappings, err := h.store.ListMappings(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "list CL mappings failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to list mappings")
		return
	}
	mappedCerts := make(map[string]bool, len(allMappings))
	for _, m := range allMappings {
		mappedCerts[m.SlabSerial] = true
	}

	// Filter to purchases with cert numbers and check existing mappings
	type syncEntry struct {
		purchaseID string
		req        addCardRequest
	}
	var toSync []syncEntry
	for _, p := range purchases {
		if p.CertNumber == "" {
			continue
		}
		if mappedCerts[p.CertNumber] {
			continue // already synced
		}
		grader := strings.ToLower(p.Grader)
		if grader == "" {
			grader = "psa"
		}
		toSync = append(toSync, syncEntry{
			purchaseID: p.ID,
			req: addCardRequest{
				CertNumber:    p.CertNumber,
				Grader:        grader,
				InvestmentUSD: float64(p.BuyCostCents) / 100.0,
				DatePurchased: p.PurchaseDate,
			},
		})
		mappedCerts[p.CertNumber] = true
	}

	var results []addCardResult
	var synced, skipped, errCount int
	for _, entry := range toSync {
		result, err := h.addCardToCollection(r.Context(), cfg.FirebaseUID, cfg.CollectionID, entry.purchaseID, entry.req)
		if err != nil {
			results = append(results, addCardResult{
				CertNumber: entry.req.CertNumber,
				Status:     "error",
				Error:      err.Error(),
			})
			errCount++
			continue
		}
		results = append(results, *result)
		if result.Status == "synced" {
			synced++
		} else {
			skipped++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"synced":  synced,
		"skipped": skipped + len(purchases) - len(toSync),
		"failed":  errCount,
		"total":   len(purchases),
		"results": results,
	})
}

// addCardToCollection resolves a cert number via Cloud Functions and writes the card
// to the CardLadder Firestore collection.
func (h *CardLadderHandler) addCardToCollection(ctx context.Context, uid, collectionID, purchaseID string, req addCardRequest) (*addCardResult, error) {
	result, err := h.client.ResolveAndCreateCard(ctx, uid, collectionID, cardladder.CardPushParams{
		CertNumber:    req.CertNumber,
		Grader:        req.Grader,
		InvestmentUSD: req.InvestmentUSD,
		DatePurchased: req.DatePurchased,
	})
	if err != nil {
		return nil, err
	}

	// Save the local mapping
	if err := h.store.SaveMapping(ctx, req.CertNumber, result.DocumentName, result.GemRateID, result.GemRateCondition); err != nil {
		h.logger.Error(ctx, "failed to save CL mapping after Firestore write",
			observability.String("cert", req.CertNumber), observability.Err(err))
		// Best-effort compensating delete to avoid orphaned remote document.
		if delErr := h.client.DeleteCollectionCard(ctx, result.DocumentName); delErr != nil {
			h.logger.Error(ctx, "compensating delete of remote CL doc failed",
				observability.String("doc", result.DocumentName), observability.Err(delErr))
		}
		return nil, fmt.Errorf("save mapping for cert %s: %w", req.CertNumber, err)
	}

	// Update cl_synced_at timestamp
	if h.syncUpdater != nil && purchaseID != "" {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := h.syncUpdater.UpdatePurchaseCLSyncedAt(ctx, purchaseID, now); err != nil {
			h.logger.Error(ctx, "failed to update cl_synced_at",
				observability.String("cert", req.CertNumber), observability.Err(err))
		}
	}

	return &addCardResult{
		CertNumber: req.CertNumber,
		Player:     result.Player,
		Set:        result.Set,
		Condition:  result.Condition,
		Value:      result.EstimatedValue,
		Status:     "synced",
	}, nil
}
