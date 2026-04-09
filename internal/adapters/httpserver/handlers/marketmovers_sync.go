package handlers

import (
	"context"
	"net/http"

	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// MMPurchaseLister provides unsold purchase data for collection sync.
type MMPurchaseLister interface {
	ListAllUnsoldPurchases(ctx context.Context) ([]campaigns.Purchase, error)
}

// MMSyncResult is the JSON response for the collection sync endpoint.
type MMSyncResult struct {
	Synced  int           `json:"synced"`
	Skipped int           `json:"skipped"`
	Failed  int           `json:"failed"`
	Errors  []MMSyncError `json:"errors,omitempty"`
}

// MMSyncError describes a single sync failure.
type MMSyncError struct {
	CertNumber string `json:"certNumber"`
	Error      string `json:"error"`
}

// SetPurchaseLister injects the purchase lister after construction.
func (h *MarketMoversHandler) SetPurchaseLister(l MMPurchaseLister) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.purchaseLister = l
}

// HandleSyncCollection pushes unmapped unsold inventory to MM via the API.
// Items must have a resolved MM collectible_id (from the scheduler) and must
// not already have a collection_item_id. Purchase details are sourced from
// the local database.
func (h *MarketMoversHandler) HandleSyncCollection(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	client := h.client
	lister := h.purchaseLister
	h.mu.Unlock()

	if client == nil || !client.Available() {
		writeError(w, http.StatusServiceUnavailable, "Market Movers client not configured")
		return
	}
	if lister == nil {
		writeError(w, http.StatusServiceUnavailable, "purchase lister not available")
		return
	}

	ctx := r.Context()

	// 1. Load unsynced mappings (have collectible_id, no collection_item_id).
	unsynced, err := h.store.ListUnsyncedMappings(ctx)
	if err != nil {
		h.logger.Error(ctx, "failed to load unsynced mappings", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to load mappings")
		return
	}
	if len(unsynced) == 0 {
		writeJSON(w, http.StatusOK, MMSyncResult{})
		return
	}

	// 2. Load unsold purchases and index by cert number.
	purchases, err := lister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		h.logger.Error(ctx, "failed to load unsold purchases", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to load purchases")
		return
	}
	byCert := make(map[string]campaigns.Purchase, len(purchases))
	for _, p := range purchases {
		if p.CertNumber != "" {
			if existing, ok := byCert[p.CertNumber]; ok {
				h.logger.Warn(ctx, "MM sync: duplicate cert number in unsold purchases, keeping first",
					observability.String("cert", p.CertNumber),
					observability.String("keptPurchaseID", existing.ID),
					observability.String("skippedPurchaseID", p.ID))
				continue
			}
			byCert[p.CertNumber] = p
		}
	}

	// 3. Build mutation inputs for each unsynced mapping that has a matching unsold purchase.
	type syncItem struct {
		certNumber string
		input      marketmovers.AddCollectionItemInput
	}
	var items []syncItem
	result := MMSyncResult{}

	for _, m := range unsynced {
		p, ok := byCert[m.SlabSerial]
		if !ok {
			// Mapping exists but purchase is missing or already sold — skip.
			result.Skipped++
			continue
		}

		pricePerItem := mathutil.ToDollars(int64(p.BuyCostCents))
		if pricePerItem <= 0 {
			pricePerItem = 0.01
		}

		items = append(items, syncItem{
			certNumber: m.SlabSerial,
			input: marketmovers.AddCollectionItemInput{
				Collectible: marketmovers.CollectionCollectible{
					CollectibleType: "sports-card",
					CollectibleID:   m.MMCollectibleID,
				},
				PurchaseDetails: marketmovers.CollectionPurchaseDetails{
					Quantity:             1,
					PurchasePricePerItem: pricePerItem,
					ConversionFeePerItem: 0,
					PurchaseDateISO:      p.PurchaseDate,
					Notes:                p.CertNumber,
				},
				CategoryIDs: nil,
			},
		})
	}

	if len(items) == 0 {
		writeJSON(w, http.StatusOK, result)
		return
	}

	// 4. Send items one-by-one to avoid large batch failures.
	//    The rate limiter inside the client handles throttling.
	for _, item := range items {
		resp, err := client.AddCollectionItem(ctx, item.input)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, MMSyncError{
				CertNumber: item.certNumber,
				Error:      err.Error(),
			})
			h.logger.Error(ctx, "MM sync failed for cert",
				observability.String("cert", item.certNumber),
				observability.Err(err))
			continue
		}

		// 5. Persist the collection item ID so we don't re-add on next sync.
		if err := h.store.SaveCollectionItemID(ctx, item.certNumber, resp.CollectionItemID); err != nil {
			h.logger.Error(ctx, "failed to save collection item ID",
				observability.String("cert", item.certNumber),
				observability.Err(err))
			result.Failed++
			result.Errors = append(result.Errors, MMSyncError{
				CertNumber: item.certNumber,
				Error:      "added to MM but failed to save locally: " + err.Error(),
			})
			continue
		}
		result.Synced++
	}

	writeJSON(w, http.StatusOK, result)
}
