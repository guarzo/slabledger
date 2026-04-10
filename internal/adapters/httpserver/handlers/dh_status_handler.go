package handlers

import (
	"net/http"
	"strings"
	"sync"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ordersEpoch is the earliest date for counting DH orders. Set to 2020-01-01
// because DoubleHolo's API requires a "since" parameter and this pre-dates all
// Card Yeti activity on the platform, ensuring we capture every order.
const ordersEpoch = "2020-01-01T00:00:00Z"

// HandleGetIntelligence returns market intelligence for a specific card.
func (h *DHHandler) HandleGetIntelligence(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	cardName := r.URL.Query().Get("card_name")
	setName := r.URL.Query().Get("set_name")
	cardNumber := r.URL.Query().Get("card_number")
	if cardName == "" || setName == "" {
		writeError(w, http.StatusBadRequest, "card_name and set_name are required")
		return
	}

	intel, err := h.intelRepo.GetByCard(ctx, cardName, setName, cardNumber)
	if err != nil {
		h.logger.Error(ctx, "get intelligence", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get intelligence")
		return
	}
	if intel == nil {
		writeError(w, http.StatusNotFound, "no intelligence data found")
		return
	}

	writeJSON(w, http.StatusOK, intel)
}

// HandleGetSuggestions returns the latest DH buy/sell suggestions.
func (h *DHHandler) HandleGetSuggestions(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	suggestions, err := h.suggestionsRepo.GetLatest(ctx)
	if err != nil {
		h.logger.Error(ctx, "get suggestions", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get suggestions")
		return
	}
	if suggestions == nil {
		suggestions = []intelligence.Suggestion{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions, "count": len(suggestions)})
}

// HandleInventoryAlerts cross-references latest DH suggestions against current inventory.
func (h *DHHandler) HandleInventoryAlerts(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	suggestions, err := h.suggestionsRepo.GetLatest(ctx)
	if err != nil {
		h.logger.Error(ctx, "inventory alerts: get suggestions", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get suggestions")
		return
	}

	purchases, err := h.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		h.logger.Error(ctx, "inventory alerts: list purchases", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to list purchases")
		return
	}

	// Build lookup set of inventory cards by name+set+number for efficient matching
	type inventoryKey struct{ name, set, cardNumber string }
	inventorySet := make(map[inventoryKey]bool, len(purchases))
	for _, p := range purchases {
		inventorySet[inventoryKey{
			name:       strings.ToLower(p.CardName),
			set:        strings.ToLower(p.SetName),
			cardNumber: strings.ToLower(p.CardNumber),
		}] = true
	}

	var alerts []intelligence.Suggestion
	for _, s := range suggestions {
		key := inventoryKey{
			name:       strings.ToLower(s.CardName),
			set:        strings.ToLower(s.SetName),
			cardNumber: strings.ToLower(s.CardNumber),
		}
		if inventorySet[key] {
			alerts = append(alerts, s)
		}
	}
	if alerts == nil {
		alerts = []intelligence.Suggestion{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"alerts": alerts, "count": len(alerts)})
}

type dhStatusResponse struct {
	IntelligenceCount     int             `json:"intelligence_count"`
	IntelligenceLastFetch string          `json:"intelligence_last_fetch"`
	SuggestionsCount      int             `json:"suggestions_count"`
	SuggestionsLastFetch  string          `json:"suggestions_last_fetch"`
	UnmatchedCount        int             `json:"unmatched_count"`
	DismissedCount        int             `json:"dismissed_count"`
	PendingCount          int             `json:"pending_count"`
	MappedCount           int             `json:"mapped_count"`
	BulkMatchRunning      bool            `json:"bulk_match_running"`
	BulkMatchError        string          `json:"bulk_match_error,omitempty"`
	APIHealth             *dh.HealthStats `json:"api_health,omitempty"`
	DHInventoryCount      int             `json:"dh_inventory_count,omitempty"`
	DHListingsCount       int             `json:"dh_listings_count,omitempty"`
	DHOrdersCount         int             `json:"dh_orders_count,omitempty"`
}

// HandleGetStatus returns aggregate stats for the DH integration.
func (h *DHHandler) HandleGetStatus(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	var bulkMatchErr string
	if v := h.bulkMatchError.Load(); v != nil {
		bulkMatchErr, _ = v.(string)
	}
	resp := dhStatusResponse{
		BulkMatchRunning: h.bulkMatchRunning.Load(),
		BulkMatchError:   bulkMatchErr,
	}

	if h.intelCounter != nil {
		if n, err := h.intelCounter.CountAll(ctx); err != nil {
			h.logger.Warn(ctx, "dh status: count intelligence", observability.Err(err))
		} else {
			resp.IntelligenceCount = n
		}
		if t, err := h.intelCounter.LatestFetchedAt(ctx); err != nil {
			h.logger.Warn(ctx, "dh status: latest intelligence fetch", observability.Err(err))
		} else {
			resp.IntelligenceLastFetch = t
		}
	}

	if h.suggestCounter != nil {
		if n, err := h.suggestCounter.CountLatest(ctx); err != nil {
			h.logger.Warn(ctx, "dh status: count suggestions", observability.Err(err))
		} else {
			resp.SuggestionsCount = n
		}
		if t, err := h.suggestCounter.LatestFetchedAt(ctx); err != nil {
			h.logger.Warn(ctx, "dh status: latest suggestions fetch", observability.Err(err))
		} else {
			resp.SuggestionsLastFetch = t
		}
	}

	if h.statusCounter != nil {
		counts, err := h.statusCounter.CountUnsoldByDHPushStatus(ctx)
		if err != nil {
			h.logger.Error(ctx, "dh status: count push statuses", observability.Err(err))
			writeError(w, http.StatusInternalServerError, "failed to count push statuses")
			return
		}
		resp.UnmatchedCount = counts[campaigns.DHPushStatusUnmatched]
		resp.DismissedCount = counts[campaigns.DHPushStatusDismissed]
		resp.PendingCount = counts[campaigns.DHPushStatusPending]
		resp.MappedCount = counts[campaigns.DHPushStatusMatched] + counts[campaigns.DHPushStatusManual]
	}

	// API health metrics
	if h.healthReporter != nil {
		if ht := h.healthReporter.Health(); ht != nil {
			stats := ht.Stats()
			resp.APIHealth = &stats
		}
	}

	// DH counts (best-effort, concurrent — don't fail the whole response)
	if h.countsFetcher != nil {
		var wg sync.WaitGroup
		var mu sync.Mutex
		wg.Add(3)
		go func() {
			defer wg.Done()
			if invResp, err := h.countsFetcher.ListInventory(ctx, dh.InventoryFilters{PerPage: 1}); err != nil {
				h.logger.Warn(ctx, "dh status: count inventory", observability.Err(err))
			} else {
				mu.Lock()
				resp.DHInventoryCount = invResp.Meta.TotalCount
				mu.Unlock()
			}
		}()
		go func() {
			defer wg.Done()
			if listResp, err := h.countsFetcher.ListInventory(ctx, dh.InventoryFilters{Status: "listed", PerPage: 1}); err != nil {
				h.logger.Warn(ctx, "dh status: count listings", observability.Err(err))
			} else {
				mu.Lock()
				resp.DHListingsCount = listResp.Meta.TotalCount
				mu.Unlock()
			}
		}()
		go func() {
			defer wg.Done()
			if ordResp, err := h.countsFetcher.GetOrders(ctx, dh.OrderFilters{Since: ordersEpoch, PerPage: 1}); err != nil {
				h.logger.Warn(ctx, "dh status: count orders", observability.Err(err))
			} else {
				mu.Lock()
				resp.DHOrdersCount = ordResp.Meta.TotalCount
				mu.Unlock()
			}
		}()
		wg.Wait()
	}

	writeJSON(w, http.StatusOK, resp)
}
