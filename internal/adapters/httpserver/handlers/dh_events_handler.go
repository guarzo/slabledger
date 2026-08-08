package handlers

import (
	"net/http"
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// dhEventResponse is one row of the DH state-change trail.
//
// Monetary values keep the explicit `_cents` suffix rather than converting to
// USD: this is a diagnostic view of what was written to dh_state_events, and a
// reader comparing it against the row wants the stored value, not a derived one.
type dhEventResponse struct {
	ID             int64  `json:"id"`
	EventAt        string `json:"event_at"`
	PurchaseID     string `json:"purchase_id,omitempty"`
	CertNumber     string `json:"cert_number,omitempty"`
	Type           string `json:"type"`
	Source         string `json:"source"`
	PrevPushStatus string `json:"prev_push_status,omitempty"`
	NewPushStatus  string `json:"new_push_status,omitempty"`
	PrevDHStatus   string `json:"prev_dh_status,omitempty"`
	NewDHStatus    string `json:"new_dh_status,omitempty"`
	DHInventoryID  int    `json:"dh_inventory_id,omitempty"`
	DHCardID       int    `json:"dh_card_id,omitempty"`
	DHOrderID      string `json:"dh_order_id,omitempty"`
	SalePriceCents int    `json:"sale_price_cents,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

// HandleGetEvents handles GET /api/dh/events?purchaseId=...|cert=...&limit=N.
//
// Exactly one subject must be supplied. The table is append-only and nothing
// else reads it per-row, so an unfiltered listing is deliberately not offered:
// the question this answers is always "what happened to this one".
func (h *DHHandler) HandleGetEvents(w http.ResponseWriter, r *http.Request) {
	if h.eventHistoryStore == nil {
		writeError(w, http.StatusServiceUnavailable, "DH event history not configured")
		return
	}

	ctx := r.Context()
	purchaseID := r.URL.Query().Get("purchaseId")
	cert := r.URL.Query().Get("cert")

	switch {
	case purchaseID == "" && cert == "":
		writeError(w, http.StatusBadRequest, "one of purchaseId or cert is required")
		return
	case purchaseID != "" && cert != "":
		writeError(w, http.StatusBadRequest, "specify only one of purchaseId or cert")
		return
	}

	limit, ok := parseEventLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeError(w, http.StatusBadRequest, "limit must be a positive integer")
		return
	}

	var (
		events []dhevents.StoredEvent
		err    error
	)
	if purchaseID != "" {
		events, err = h.eventHistoryStore.ByPurchase(ctx, purchaseID, limit)
	} else {
		events, err = h.eventHistoryStore.ByCert(ctx, cert, limit)
	}
	if err != nil {
		h.logger.Error(ctx, "dh event history lookup failed",
			observability.String("purchase_id", purchaseID),
			observability.String("cert_number", cert),
			observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to load DH events")
		return
	}

	out := make([]dhEventResponse, 0, len(events))
	for _, e := range events {
		out = append(out, toDHEventResponse(e))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"events": out,
		"count":  len(out),
		"limit":  dhevents.ClampHistoryLimit(limit),
	})
}

// parseEventLimit reads the optional limit parameter. An empty value means
// "use the default", which ClampHistoryLimit supplies for 0.
func parseEventLimit(raw string) (int, bool) {
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func toDHEventResponse(e dhevents.StoredEvent) dhEventResponse {
	return dhEventResponse{
		ID:             e.ID,
		EventAt:        e.EventAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		PurchaseID:     e.PurchaseID,
		CertNumber:     e.CertNumber,
		Type:           string(e.Type),
		Source:         string(e.Source),
		PrevPushStatus: e.PrevPushStatus,
		NewPushStatus:  e.NewPushStatus,
		PrevDHStatus:   e.PrevDHStatus,
		NewDHStatus:    e.NewDHStatus,
		DHInventoryID:  e.DHInventoryID,
		DHCardID:       e.DHCardID,
		DHOrderID:      e.DHOrderID,
		SalePriceCents: e.SalePriceCents,
		Notes:          e.Notes,
	}
}
