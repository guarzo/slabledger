package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

type DHInventoryRecovery interface {
	RecoverDHInventory(context.Context, string, string, string) error
}

func WithDHInventoryRecovery(s DHInventoryRecovery) CampaignsHandlerOption {
	return func(h *CampaignsHandler) { h.dhRecovery = s }
}

func (h *CampaignsHandler) HandleDHInventoryRecovery(w http.ResponseWriter, r *http.Request) {
	user := requireUser(w, r)
	if user == nil {
		return
	}
	if !user.IsAdmin {
		writeError(w, 403, "Admin access required")
		return
	}
	id, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	dec.DisallowUnknownFields()
	var extra any
	if err := dec.Decode(&body); err != nil || !body.Confirm || dec.Decode(&extra) != io.EOF {
		writeError(w, 400, "confirm must be true; no other fields are accepted")
		return
	}
	if !inventory.DHDependenciesPresent(h.dhRecovery) {
		writeError(w, 503, "DH inventory recovery not configured")
		return
	}
	if err := h.dhRecovery.RecoverDHInventory(r.Context(), id, r.PathValue("action"), strconv.FormatInt(user.ID, 10)); err != nil {
		if writeReturnConflictIfPresent(w, err) || inventory.IsPurchaseNotFound(err) {
			if inventory.IsPurchaseNotFound(err) {
				writeError(w, 404, "Purchase not found")
			}
			return
		}
		h.logger.Error(r.Context(), "DH inventory recovery failed", observability.Err(err))
		writeError(w, 500, "DH inventory recovery failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
