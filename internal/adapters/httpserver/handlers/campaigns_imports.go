package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// HandleGlobalImportPSA handles POST /api/purchases/import-psa.
// Accepts a PSA export CSV file and imports graded card data across all inventory.
func (h *CampaignsHandler) HandleGlobalImportPSA(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.parseGlobalCSVUpload(w, r)
	if !ok {
		return
	}

	h.importPSARows(w, r, rows, "CSV", "global PSA import failed")
}

// HandleSyncPSASheets handles POST /api/purchases/sync-psa-sheets.
// Fetches PSA data from a configured Google Sheet and runs the standard import.
func (h *CampaignsHandler) HandleSyncPSASheets(w http.ResponseWriter, r *http.Request) {
	if h.sheetFetcher == nil || h.sheetsSpreadsheet == "" {
		writeError(w, http.StatusServiceUnavailable, "Google Sheets sync not configured")
		return
	}

	fetchCtx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	rows, err := h.sheetFetcher.ReadSheet(fetchCtx, h.sheetsSpreadsheet, h.sheetsTab)
	if err != nil {
		h.logger.Error(r.Context(), "failed to fetch Google Sheet", observability.Err(err))
		writeError(w, http.StatusBadGateway, "Failed to fetch Google Sheet")
		return
	}

	h.importPSARows(w, r, rows, "sheet", "PSA sheets sync failed")
}

// importPSARows parses raw CSV rows as PSA export data, imports them, and
// writes the JSON response. source labels the origin ("CSV" or "sheet") for
// error messages; logLabel identifies the operation in failure logs.
func (h *CampaignsHandler) importPSARows(w http.ResponseWriter, r *http.Request, rows [][]string, source, logLabel string) {
	psaRows, parseErrors, err := inventory.ParsePSAExportRows(rows)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Continue with valid rows even if there are parse errors.
	// Only fail if no valid rows at all.
	if len(psaRows) == 0 {
		if len(parseErrors) > 0 {
			writeError(w, http.StatusBadRequest, parseErrors[0].Message)
		} else {
			writeError(w, http.StatusBadRequest, "No valid PSA data rows found in "+source)
		}
		return
	}

	result, ok := serviceCall(w, r.Context(), h.logger, logLabel, func() (*inventory.PSAImportResult, error) {
		return h.service.ImportPSAExportGlobal(r.Context(), psaRows)
	})
	if !ok {
		return
	}

	// Surface row-level parse errors in the response so the caller
	// knows which rows were skipped and why.
	for _, pe := range parseErrors {
		result.Errors = append(result.Errors, inventory.ImportError{
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

	shopifyRows, parseErrors, err := inventory.ParseShopifyExportRows(rows)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Convert ParseErrors to ImportErrors for the response.
	var importErrors []inventory.ImportError
	for _, pe := range parseErrors {
		importErrors = append(importErrors, inventory.ImportError{
			Row:   pe.Row,
			Error: pe.Message,
		})
	}

	if len(shopifyRows) == 0 {
		if len(importErrors) > 0 {
			writeJSON(w, http.StatusOK, inventory.ExternalImportResult{
				Failed: len(importErrors),
				Errors: importErrors,
			})
		} else {
			writeJSON(w, http.StatusBadRequest, inventory.ExternalImportResult{
				Failed: 1,
				Errors: []inventory.ImportError{{Row: 0, Error: "No valid product rows found in CSV"}},
			})
		}
		return
	}

	result, ok := serviceCall(w, r.Context(), h.logger, "external import failed", func() (*inventory.ExternalImportResult, error) {
		return h.service.ImportExternalCSV(r.Context(), shopifyRows)
	})
	if !ok {
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
	var req inventory.CertImportRequest
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
	var req inventory.ScanCertRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.CertNumber == "" {
		writeError(w, http.StatusBadRequest, "certNumber is required")
		return
	}

	result, ok := serviceCall(w, r.Context(), h.logger, "scan cert failed", func() (*inventory.ScanCertResult, error) {
		return h.service.ScanCert(r.Context(), req.CertNumber)
	})
	if !ok {
		return
	}

	// Existing unsold certs just had received_at set and were enrolled in the
	// DH push pipeline. Trigger a listing run so in-flight items promote from
	// in_stock → listed without waiting for an unrelated import.
	if result.Status == "existing" {
		h.triggerDHListing([]string{req.CertNumber})
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleResolveCert handles POST /api/purchases/resolve-cert.
func (h *CampaignsHandler) HandleResolveCert(w http.ResponseWriter, r *http.Request) {
	var req inventory.ResolveCertRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.CertNumber == "" {
		writeError(w, http.StatusBadRequest, "certNumber is required")
		return
	}

	info, err := h.service.ResolveCert(r.Context(), req.CertNumber)
	if err != nil {
		if inventory.IsCertNotFound(err) {
			writeError(w, http.StatusNotFound, "Cert not found")
			return
		}
		h.logger.Error(r.Context(), "resolve cert failed",
			observability.String("cert", req.CertNumber),
			observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, inventory.ResolveCertResult{
		CertNumber: info.CertNumber,
		CardName:   info.CardName,
		Grade:      info.Grade,
		Year:       info.Year,
		Category:   info.Category,
		Subject:    info.Subject,
	})
}

// HandleImportOrders handles POST /api/purchases/import-orders.
// Accepts an orders export CSV, matches PSA certs against inventory, and returns
// categorized results for review before confirmation.
func (h *CampaignsHandler) HandleImportOrders(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.parseGlobalCSVUpload(w, r)
	if !ok {
		return
	}

	orderRows, skipped, err := inventory.ParseOrdersExportRows(rows)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(orderRows) == 0 {
		// No valid PSA rows — return result with only skipped items
		writeJSON(w, http.StatusOK, &inventory.OrdersImportResult{
			Skipped: skipped,
		})
		return
	}

	result, ok := serviceCall(w, r.Context(), h.logger, "orders import failed", func() (*inventory.OrdersImportResult, error) {
		return h.service.ImportOrdersSales(r.Context(), orderRows)
	})
	if !ok {
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

	var items []inventory.OrdersConfirmItem
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "No items provided")
		return
	}

	result, ok := serviceCall(w, r.Context(), h.logger, "confirm orders sales failed", func() (*inventory.BulkSaleResult, error) {
		return h.service.ConfirmOrdersSales(r.Context(), items)
	})
	if !ok {
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
