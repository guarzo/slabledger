package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CardDiscoverer discovers and prices cards via CardHedger on demand.
type CardDiscoverer interface {
	DiscoverAndPrice(ctx context.Context, cards []campaigns.CardIdentity) (discovered, priced int)
}

// triggerCardDiscovery runs CardHedger discovery for imported cards in a background
// goroutine so it doesn't delay the HTTP response.
func (h *CampaignsHandler) triggerCardDiscovery(cards []campaigns.CardIdentity) {
	if h.discoverer == nil || len(cards) == 0 {
		return
	}
	// Deduplicate
	seen := make(map[string]bool, len(cards))
	var unique []campaigns.CardIdentity
	for _, c := range cards {
		key := c.CardName + "|" + c.SetName + "|" + c.CardNumber
		if !seen[key] {
			seen[key] = true
			unique = append(unique, c)
		}
	}
	h.bgWG.Add(1)
	go func() {
		defer h.bgWG.Done()
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error(h.baseCtx, "panic in triggerCardDiscovery",
					observability.String("panic", fmt.Sprintf("%v", r)))
			}
		}()
		ctx, cancel := context.WithTimeout(h.baseCtx, 5*time.Minute)
		defer cancel()
		discovered, priced := h.discoverer.DiscoverAndPrice(ctx, unique)
		h.logger.Info(ctx, "card discovery completed",
			observability.Int("discovered", discovered),
			observability.Int("priced", priced),
			observability.Int("requested", len(unique)))
	}()
}

// HandleGlobalRefreshCL handles POST /api/purchases/refresh-cl.
// Accepts a full CL export CSV and refreshes CL values across all campaigns.
func (h *CampaignsHandler) HandleGlobalRefreshCL(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.parseGlobalCSVUpload(w, r)
	if !ok {
		return
	}

	clRows, parseErrors, err := campaigns.ParseCLRefreshRows(rows)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(parseErrors) > 0 {
		writeError(w, http.StatusBadRequest, parseErrors[0].Message)
		return
	}

	result, svcErr := h.service.RefreshCLValuesGlobal(r.Context(), clRows)
	if svcErr != nil {
		h.logger.Error(r.Context(), "global CL refresh failed", observability.Err(svcErr))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// HandleGlobalImportCL handles POST /api/purchases/import-cl.
// Accepts a full CL export CSV, auto-allocates new purchases to campaigns, and refreshes existing ones.
func (h *CampaignsHandler) HandleGlobalImportCL(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.parseGlobalCSVUpload(w, r)
	if !ok {
		return
	}

	clRows, parseErrors, err := campaigns.ParseCLImportRows(rows)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(parseErrors) > 0 {
		writeError(w, http.StatusBadRequest, parseErrors[0].Message)
		return
	}

	result, svcErr := h.service.ImportCLExportGlobal(r.Context(), clRows)
	if svcErr != nil {
		h.logger.Error(r.Context(), "global CL import failed", observability.Err(svcErr))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Trigger CardHedger discovery for imported cards in background
	if result.Results != nil {
		var cards []campaigns.CardIdentity
		for _, res := range result.Results {
			if res.CardName != "" && res.SetName != "" && (res.Status == "allocated" || res.Status == "refreshed") {
				cards = append(cards, campaigns.CardIdentity{CardName: res.CardName, SetName: res.SetName, CardNumber: res.CardNumber})
			}
		}
		h.triggerCardDiscovery(cards)
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleGlobalExportCL handles GET /api/purchases/export-cl.
// Returns a CSV file of all unsold inventory in Card Ladder import format.
func (h *CampaignsHandler) HandleGlobalExportCL(w http.ResponseWriter, r *http.Request) {
	missingCLOnly := r.URL.Query().Get("missing_cl_only") == "true"
	entries, err := h.service.ExportCLFormatGlobal(r.Context(), missingCLOnly)
	if err != nil {
		h.logger.Error(r.Context(), "global CL export failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="card_ladder_import.csv"`)

	writer := csv.NewWriter(w)
	if err := writer.Write([]string{"Date Purchased", "Cert #", "Grader", "Investment", "Estimated Value", "Notes", "Date Sold", "Sold Price"}); err != nil {
		h.logger.Error(r.Context(), "csv header write failed", observability.Err(err))
		return
	}
	for _, e := range entries {
		if err := writer.Write([]string{
			e.DatePurchased,
			e.CertNumber,
			e.Grader,
			fmt.Sprintf("%.2f", e.Investment),
			fmt.Sprintf("%.2f", e.EstimatedValue),
			"", "", "",
		}); err != nil {
			h.logger.Error(r.Context(), "csv row write failed", observability.Err(err))
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		h.logger.Error(r.Context(), "csv flush failed", observability.Err(err))
	}
}

// HandleGlobalImportPSA handles POST /api/purchases/import-psa.
// Accepts a PSA communication spreadsheet CSV and auto-allocates purchases.
func (h *CampaignsHandler) HandleGlobalImportPSA(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.parseGlobalCSVUpload(w, r)
	if !ok {
		return
	}

	psaRows, parseErrors, err := campaigns.ParsePSAExportRows(rows)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// For PSA: continue with valid rows even if there are parse errors.
	// Only fail if no valid rows at all.
	if len(psaRows) == 0 {
		if len(parseErrors) > 0 {
			writeError(w, http.StatusBadRequest, parseErrors[0].Message)
		} else {
			writeError(w, http.StatusBadRequest, "No valid PSA data rows found in CSV")
		}
		return
	}

	result, svcErr := h.service.ImportPSAExportGlobal(r.Context(), psaRows)
	if svcErr != nil {
		h.logger.Error(r.Context(), "global PSA import failed", observability.Err(svcErr))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Trigger CardHedger discovery for imported cards in background
	if result.Results != nil {
		var cards []campaigns.CardIdentity
		for _, res := range result.Results {
			if res.CardName != "" && res.SetName != "" && (res.Status == "allocated" || res.Status == "updated") {
				cards = append(cards, campaigns.CardIdentity{CardName: res.CardName, SetName: res.SetName, CardNumber: res.CardNumber})
			}
		}
		h.triggerCardDiscovery(cards)
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleGlobalImportExternal handles POST /api/purchases/import-external.
// Accepts a Shopify product export CSV file.
func (h *CampaignsHandler) HandleGlobalImportExternal(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.parseGlobalCSVUpload(w, r)
	if !ok {
		return
	}

	shopifyRows, parseErrors, err := campaigns.ParseShopifyExportRows(rows)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Convert ParseErrors to ImportErrors for the response.
	var importErrors []campaigns.ImportError
	for _, pe := range parseErrors {
		importErrors = append(importErrors, campaigns.ImportError{
			Row:   pe.Row,
			Error: pe.Message,
		})
	}

	if len(shopifyRows) == 0 {
		if len(importErrors) > 0 {
			writeJSON(w, http.StatusOK, campaigns.ExternalImportResult{
				Failed: len(importErrors),
				Errors: importErrors,
			})
		} else {
			writeJSON(w, http.StatusBadRequest, campaigns.ExternalImportResult{
				Failed: 1,
				Errors: []campaigns.ImportError{{Row: 0, Error: "No valid product rows found in CSV"}},
			})
		}
		return
	}

	result, svcErr := h.service.ImportExternalCSV(r.Context(), shopifyRows)
	if svcErr != nil {
		h.logger.Error(r.Context(), "external import failed", observability.Err(svcErr))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if len(importErrors) > 0 {
		result.Errors = append(result.Errors, importErrors...)
		result.Failed += len(importErrors)
	}

	// Trigger CardHedger discovery for imported cards in background
	if result.Results != nil {
		var cards []campaigns.CardIdentity
		for _, res := range result.Results {
			if res.CardName != "" && res.SetName != "" && (res.Status == "imported" || res.Status == "updated") {
				cards = append(cards, campaigns.CardIdentity{CardName: res.CardName, SetName: res.SetName, CardNumber: res.CardNumber})
			}
		}
		h.triggerCardDiscovery(cards)
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleImportCerts handles POST /api/purchases/import-certs.
func (h *CampaignsHandler) HandleImportCerts(w http.ResponseWriter, r *http.Request) {
	const maxBytes = 1 << 20 // 1MB — cert numbers are short strings
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	var req campaigns.CertImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if len(req.CertNumbers) == 0 {
		writeError(w, http.StatusBadRequest, "No certificate numbers provided")
		return
	}

	result, err := h.service.ImportCerts(r.Context(), req.CertNumbers)
	if err != nil {
		h.logger.Error(r.Context(), "cert import failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleListEbayExport handles GET /api/purchases/export-ebay.
func (h *CampaignsHandler) HandleListEbayExport(w http.ResponseWriter, r *http.Request) {
	flaggedOnly := r.URL.Query().Get("flagged_only") == "true"
	resp, err := h.service.ListEbayExportItems(r.Context(), flaggedOnly)
	if err != nil {
		h.logger.Error(r.Context(), "list ebay export items failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// HandleGenerateEbayCSV handles POST /api/purchases/export-ebay/generate.
func (h *CampaignsHandler) HandleGenerateEbayCSV(w http.ResponseWriter, r *http.Request) {
	const maxBytes = 1 << 20 // 1MB
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	var req campaigns.EbayExportGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "No items provided")
		return
	}

	csvBytes, err := h.service.GenerateEbayCSV(r.Context(), req.Items)
	if err != nil {
		h.logger.Error(r.Context(), "generate ebay CSV failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=ebay_import.csv")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(csvBytes) //nolint:errcheck // response already committed; write error unactionable
}

// parseGlobalCSVUpload reads and validates an uploaded CSV file (no campaign ID in path).
func (h *CampaignsHandler) parseGlobalCSVUpload(w http.ResponseWriter, r *http.Request) (records [][]string, ok bool) {
	ctx := r.Context()
	const maxBytes = 10 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "File upload required")
		return nil, false
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			h.logger.Error(ctx, "failed to close uploaded file", observability.Err(cerr))
		}
	}()

	buf, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Failed to read file")
		return nil, false
	}
	if len(buf) > maxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "File too large (max 10MB)")
		return nil, false
	}
	reader := csv.NewReader(bytes.NewReader(buf))
	records, err = reader.ReadAll()
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid CSV file")
		return nil, false
	}

	if len(records) < 2 {
		writeError(w, http.StatusBadRequest, "CSV must have a header row and at least one data row")
		return nil, false
	}

	return records, true
}
