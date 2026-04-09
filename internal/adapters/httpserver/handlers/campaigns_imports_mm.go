package handlers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// HandleGlobalExportMM handles GET /api/purchases/export-mm.
// Returns a CSV file of all unsold inventory in Market Movers collection import format (13 columns).
// Supports ?missing_mm_only=true to export only items lacking MM value data.
//
// Note: headers are committed before row iteration begins, so any mid-stream write error
// produces a truncated 200 OK response with no error signal to the client. This matches
// the pre-existing behaviour of HandleGlobalExportCL.
func (h *CampaignsHandler) HandleGlobalExportMM(w http.ResponseWriter, r *http.Request) {
	missingMMOnly, _ := strconv.ParseBool(r.URL.Query().Get("missing_mm_only"))
	entries, err := h.service.ExportMMFormatGlobal(r.Context(), missingMMOnly)
	if err != nil {
		h.logger.Error(r.Context(), "global MM export failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="market-movers-export.csv"`)

	writer := csv.NewWriter(w)
	header := []string{
		"Sport", "Grade", "Player Name", "Year", "Set", "Variation",
		"Card Number", "Specific Qualifier", "Quantity",
		"Date Purchased", "Purchase Price per Card", "Notes", "Category",
	}
	if err := writer.Write(header); err != nil {
		h.logger.Error(r.Context(), "mm csv header write failed", observability.Err(err))
		return
	}
	for _, e := range entries {
		// Market Movers requires a positive purchase price; default zero-cost items to $0.01.
		purchasePrice := e.PurchasePricePerCard
		if purchasePrice <= 0 {
			purchasePrice = 0.01
		}
		if err := writer.Write([]string{
			e.Sport,
			e.Grade,
			e.PlayerName,
			e.Year,
			e.Set,
			e.Variation,
			e.CardNumber,
			e.SpecificQualifier,
			e.Quantity,
			e.DatePurchased,
			fmt.Sprintf("%.2f", purchasePrice),
			e.Notes,
			e.Category,
		}); err != nil {
			h.logger.Error(r.Context(), "mm csv row write failed", observability.Err(err))
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		h.logger.Error(r.Context(), "mm csv flush failed", observability.Err(err))
	}
}

// HandleGlobalRefreshMM handles POST /api/purchases/refresh-mm.
// Accepts a Market Movers collection export CSV and updates mm_value_cents on matching purchases.
func (h *CampaignsHandler) HandleGlobalRefreshMM(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.parseGlobalCSVUpload(w, r)
	if !ok {
		return
	}

	mmRows, parseErrors, err := campaigns.ParseMMRefreshRows(rows)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(mmRows) == 0 {
		if len(parseErrors) > 0 {
			writeError(w, http.StatusBadRequest, parseErrors[0].Message)
		} else {
			writeError(w, http.StatusBadRequest, "No valid data rows found in Market Movers CSV")
		}
		return
	}

	result, ok := serviceCall(w, r.Context(), h.logger, "global MM refresh failed", func() (*campaigns.MMRefreshResult, error) {
		return h.service.RefreshMMValuesGlobal(r.Context(), mmRows)
	})
	if !ok {
		return
	}

	// Surface row-level parse errors in the response and count them as failures.
	for _, pe := range parseErrors {
		result.Errors = append(result.Errors, campaigns.ImportError{Row: pe.Row, Error: pe.Message})
	}
	result.Failed += len(parseErrors)

	writeJSON(w, http.StatusOK, result)
}
