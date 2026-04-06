package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

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

	// Surface row-level parse errors in the response so the caller
	// knows which rows were skipped and why.
	for _, pe := range parseErrors {
		result.Errors = append(result.Errors, campaigns.ImportError{
			Row:   pe.Row,
			Error: pe.Message,
		})
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

	// Trigger DH listing only for certs that were successfully processed.
	if len(result.Errors) == 0 {
		h.triggerDHListing(req.CertNumbers)
	} else {
		failedCerts := make(map[string]bool, len(result.Errors))
		for _, e := range result.Errors {
			failedCerts[e.CertNumber] = true
		}
		var successCerts []string
		for _, c := range req.CertNumbers {
			if !failedCerts[c] {
				successCerts = append(successCerts, c)
			}
		}
		h.triggerDHListing(successCerts)
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleScanCert handles POST /api/purchases/scan-cert.
func (h *CampaignsHandler) HandleScanCert(w http.ResponseWriter, r *http.Request) {
	var req campaigns.ScanCertRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.CertNumber == "" {
		writeError(w, http.StatusBadRequest, "certNumber is required")
		return
	}

	result, err := h.service.ScanCert(r.Context(), req.CertNumber)
	if err != nil {
		h.logger.Error(r.Context(), "scan cert failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleResolveCert handles POST /api/purchases/resolve-cert.
func (h *CampaignsHandler) HandleResolveCert(w http.ResponseWriter, r *http.Request) {
	var req campaigns.ResolveCertRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.CertNumber == "" {
		writeError(w, http.StatusBadRequest, "certNumber is required")
		return
	}

	info, err := h.service.ResolveCert(r.Context(), req.CertNumber)
	if err != nil {
		if campaigns.IsCertNotFound(err) {
			writeError(w, http.StatusNotFound, "Cert not found")
			return
		}
		h.logger.Error(r.Context(), "resolve cert failed",
			observability.String("cert", req.CertNumber),
			observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, campaigns.ResolveCertResult{
		CertNumber: info.CertNumber,
		CardName:   info.CardName,
		Grade:      info.Grade,
		Year:       info.Year,
		Category:   info.Category,
		Subject:    info.Subject,
	})
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

// HandleImportOrders handles POST /api/purchases/import-orders.
// Accepts an orders export CSV, matches PSA certs against inventory, and returns
// categorized results for review before confirmation.
func (h *CampaignsHandler) HandleImportOrders(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.parseGlobalCSVUpload(w, r)
	if !ok {
		return
	}

	orderRows, skipped, err := campaigns.ParseOrdersExportRows(rows)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(orderRows) == 0 {
		// No valid PSA rows — return result with only skipped items
		writeJSON(w, http.StatusOK, &campaigns.OrdersImportResult{
			Skipped: skipped,
		})
		return
	}

	result, svcErr := h.service.ImportOrdersSales(r.Context(), orderRows)
	if svcErr != nil {
		h.logger.Error(r.Context(), "orders import failed", observability.Err(svcErr))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Merge parser-level skips into the result
	result.Skipped = append(result.Skipped, skipped...)

	writeJSON(w, http.StatusOK, result)
}

// HandleConfirmOrdersSales handles POST /api/purchases/import-orders/confirm.
// Accepts confirmed matches and creates sale records.
func (h *CampaignsHandler) HandleConfirmOrdersSales(w http.ResponseWriter, r *http.Request) {
	const maxBytes = 1 << 20 // 1MB
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	var items []campaigns.OrdersConfirmItem
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "No items provided")
		return
	}

	result, err := h.service.ConfirmOrdersSales(r.Context(), items)
	if err != nil {
		h.logger.Error(r.Context(), "confirm orders sales failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, result)
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
