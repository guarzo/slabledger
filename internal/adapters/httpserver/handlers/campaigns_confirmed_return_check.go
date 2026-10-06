package handlers

import (
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// HandleDHSaleCheck reads the stored purchase identity; request body and query
// parameters cannot select another DH target.
func (h *CampaignsHandler) HandleDHSaleCheck(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	id, ok := pathID(w, r, "purchaseId", "Purchase ID")
	if !ok {
		return
	}
	if r.URL.RawQuery != "" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		writeError(w, http.StatusBadRequest, "DH sale check does not accept query parameters or a body")
		return
	}
	if !inventory.DHDependenciesPresent(h.confirmedReturns) {
		writeError(w, http.StatusServiceUnavailable, "Confirmed returns not configured")
		return
	}
	check, err := h.confirmedReturns.CheckDHSale(r.Context(), id)
	if err != nil {
		writeConfirmedReturnError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, check)
}
