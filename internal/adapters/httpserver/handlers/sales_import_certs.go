package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/csvimport"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// HandleImportCertSales handles POST /api/sales/import-certs.
// Accepts a batch of (certNumber, theirCompCents) pairs captured at a card
// show — see the itemized-sales skill — applies the batch's negotiated
// percentage to compute each sale price, and returns categorized results for
// review. This is a preview only: no sale records are created here.
func (h *CampaignsHandler) HandleImportCertSales(w http.ResponseWriter, r *http.Request) {
	if !h.importConfigured(w) {
		return
	}

	const maxBytes = 1 << 20 // 1MB — a card-show batch is a short JSON array, not a file upload
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	var req csvimport.CertSaleImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "No items provided")
		return
	}

	result, ok := serviceCall(w, r.Context(), h.logger, "cert sale import failed", func() (*csvimport.CertSaleImportResult, error) {
		return h.importSvc.ImportCertSales(r.Context(), req)
	})
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleConfirmCertSales handles POST /api/sales/import-certs/confirm.
// Accepts confirmed matches from the itemized cert-sale preview and creates
// sale records via the same ConfirmOrdersSales path used by CSV order
// confirmation. Kept as a distinct handler/route (rather than reusing
// /api/purchases/import-orders/confirm) because the URL is the API's
// documentation: this batch never touched a CSV, so "import-orders" is the
// wrong noun for it.
func (h *CampaignsHandler) HandleConfirmCertSales(w http.ResponseWriter, r *http.Request) {
	if !h.importConfigured(w) {
		return
	}

	const maxBytes = 1 << 20 // 1MB
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	var items []csvimport.OrdersConfirmItem
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "No items provided")
		return
	}

	result, ok := serviceCall(w, r.Context(), h.logger, "confirm cert sales failed", func() (*inventory.BulkSaleResult, error) {
		return h.importSvc.ConfirmOrdersSales(r.Context(), items)
	})
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, result)
}
