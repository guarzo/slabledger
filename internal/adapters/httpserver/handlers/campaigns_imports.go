package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// psaCertFromSKU extracts a PSA cert number from a SKU like "PSA-192060238".
var psaCertFromSKU = regexp.MustCompile(`(?i)^PSA-(\d+)$`)

// digitsOnly matches a string that is entirely digits.
var digitsOnly = regexp.MustCompile(`^\d+$`)

// normalizePSACert returns a digits-only cert number from a raw field value.
// It handles plain digits, "PSA-XXXXX" format, and trims whitespace.
func normalizePSACert(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if digitsOnly.MatchString(s) {
		return s
	}
	if m := psaCertFromSKU.FindStringSubmatch(s); len(m) == 2 {
		return m[1]
	}
	return ""
}

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

	// Parse CL refresh rows
	headerMap := buildHeaderMap(rows[0])

	if _, exists := headerMap["slab serial #"]; !exists {
		writeError(w, http.StatusBadRequest, `Missing required column: "slab serial #"`)
		return
	}

	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	var clRows []campaigns.CLExportRow
	for i, rec := range rows[1:] {
		getField := func(idx int) string {
			if idx >= 0 && idx < len(rec) {
				return strings.TrimSpace(rec[idx])
			}
			return ""
		}

		slabSerial := getField(colIdx("slab serial #"))
		if slabSerial == "" {
			continue
		}

		cvStr := getField(colIdx("current value"))
		if cvStr == "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Row %d: missing 'current value' for slab serial %s", i+2, slabSerial))
			return
		}
		currentValue, err := strconv.ParseFloat(cvStr, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Row %d: invalid 'current value' %q for slab serial %s", i+2, cvStr, slabSerial))
			return
		}

		var population int
		if pop := getField(colIdx("population")); pop != "" {
			population, _ = strconv.Atoi(pop) //nolint:errcheck
		}

		clRows = append(clRows, campaigns.CLExportRow{
			SlabSerial:   slabSerial,
			Card:         getField(colIdx("card")),
			Set:          getField(colIdx("set")),
			Number:       getField(colIdx("number")),
			CurrentValue: currentValue,
			Population:   population,
		})
	}

	result, err := h.service.RefreshCLValuesGlobal(r.Context(), clRows)
	if err != nil {
		h.logger.Error(r.Context(), "global CL refresh failed", observability.Err(err))
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

	headerMap := buildHeaderMap(rows[0])

	requiredHeaders := []string{"slab serial #", "investment", "current value"}
	for _, hdr := range requiredHeaders {
		if _, ok := headerMap[hdr]; !ok {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Missing required column: %q", hdr))
			return
		}
	}

	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	var clRows []campaigns.CLExportRow
	for i, rec := range rows[1:] {
		rowNum := i + 2

		getField := func(idx int) string {
			if idx >= 0 && idx < len(rec) {
				return strings.TrimSpace(rec[idx])
			}
			return ""
		}

		slabSerial := getField(colIdx("slab serial #"))
		if slabSerial == "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Row %d: missing Slab Serial #", rowNum))
			return
		}

		investment, err := strconv.ParseFloat(getField(colIdx("investment")), 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Row %d: invalid Investment %q", rowNum, getField(colIdx("investment"))))
			return
		}

		cvStr := getField(colIdx("current value"))
		if cvStr == "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Row %d: missing Current Value", rowNum))
			return
		}
		currentValue, err := strconv.ParseFloat(cvStr, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Row %d: invalid Current Value %q", rowNum, cvStr))
			return
		}

		var population int
		if pop := getField(colIdx("population")); pop != "" {
			population, _ = strconv.Atoi(pop) //nolint:errcheck
		}

		datePurchased := getField(colIdx("date purchased"))
		if datePurchased != "" {
			converted, err := campaigns.ParseCLDate(datePurchased)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("Row %d: invalid Date Purchased %q: expected M/D/YYYY", rowNum, datePurchased))
				return
			}
			datePurchased = converted
		}

		clRows = append(clRows, campaigns.CLExportRow{
			DatePurchased: datePurchased,
			Card:          getField(colIdx("card")),
			Player:        getField(colIdx("player")),
			Set:           getField(colIdx("set")),
			Number:        getField(colIdx("number")),
			Condition:     getField(colIdx("condition")),
			Investment:    investment,
			CurrentValue:  currentValue,
			SlabSerial:    slabSerial,
			Population:    population,
		})
	}

	result, err := h.service.ImportCLExportGlobal(r.Context(), clRows)
	if err != nil {
		h.logger.Error(r.Context(), "global CL import failed", observability.Err(err))
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

	// Dynamically find the header row by scanning for known PSA column names.
	headerIdx := findHeaderRow(rows)
	if headerIdx < 0 {
		writeError(w, http.StatusBadRequest,
			"Could not find PSA header row (expected columns: cert number, listing title, grade)")
		return
	}

	headerMap := buildHeaderMap(rows[headerIdx])
	dataRows := rows[headerIdx+1:]

	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	var psaRows []campaigns.PSAExportRow
	for i, rec := range dataRows {
		rowNum := headerIdx + 2 + i // 1-based row number for error reporting

		getField := func(idx int) string {
			if idx >= 0 && idx < len(rec) {
				return strings.TrimSpace(rec[idx])
			}
			return ""
		}

		certNumber := getField(colIdx("cert number"))
		if certNumber == "" {
			continue // Skip empty template rows
		}

		var pricePaid float64
		if pp := getField(colIdx("price paid")); pp != "" {
			var parseErr error
			pricePaid, parseErr = parseCurrencyString(pp)
			if parseErr != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("Row %d: invalid price paid %q: %v", rowNum, pp, parseErr))
				return
			}
		}

		var grade float64
		if g := getField(colIdx("grade")); g != "" {
			var parseErr error
			grade, parseErr = strconv.ParseFloat(g, 64)
			if parseErr != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("Row %d: invalid grade %q: %v", rowNum, g, parseErr))
				return
			}
		}

		dateStr := getField(colIdx("date"))
		purchaseDate := ""
		if dateStr != "" {
			converted, dateErr := campaigns.ParsePSADate(dateStr)
			if dateErr != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("Row %d: invalid date %q: %v", rowNum, dateStr, dateErr))
				return
			}
			purchaseDate = converted
		}

		invoiceDateStr := getField(colIdx("invoice date"))
		invoiceDate := ""
		if invoiceDateStr != "" {
			converted, dateErr := campaigns.ParsePSADate(invoiceDateStr)
			if dateErr != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("Row %d: invalid invoice date %q: %v", rowNum, invoiceDateStr, dateErr))
				return
			}
			invoiceDate = converted
		}

		wasRefunded := false
		refundedStr := strings.ToLower(getField(colIdx("was refunded?")))
		if refundedStr == "yes" || refundedStr == "true" || refundedStr == "1" {
			wasRefunded = true
		}

		psaRows = append(psaRows, campaigns.PSAExportRow{
			Date:           purchaseDate,
			Category:       getField(colIdx("category")),
			CertNumber:     certNumber,
			ListingTitle:   getField(colIdx("listing title")),
			Grade:          grade,
			PricePaid:      pricePaid,
			PurchaseSource: getField(colIdx("purchase source")),
			VaultStatus:    getField(colIdx("vault status")),
			InvoiceDate:    invoiceDate,
			WasRefunded:    wasRefunded,
			FrontImageURL:  getField(colIdx("front image url")),
			BackImageURL:   getField(colIdx("back image url")),
		})
	}

	if len(psaRows) == 0 {
		writeError(w, http.StatusBadRequest, "No valid PSA data rows found in CSV")
		return
	}

	result, err := h.service.ImportPSAExportGlobal(r.Context(), psaRows)
	if err != nil {
		h.logger.Error(r.Context(), "global PSA import failed", observability.Err(err))
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

	headerMap := buildHeaderMap(rows[0])
	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	// Validate required Shopify columns exist.
	if _, hasHandle := headerMap["handle"]; !hasHandle {
		writeError(w, http.StatusBadRequest, "CSV is missing required column: handle")
		return
	}
	if _, hasTitle := headerMap["title"]; !hasTitle {
		writeError(w, http.StatusBadRequest, "CSV is missing required column: title")
		return
	}

	// Consolidate multi-row products by Handle+CertNumber so rows with
	// the same handle but different certs are preserved. Image-only rows
	// (empty title) are tracked separately by handle for back-image merging.
	type product struct {
		row campaigns.ShopifyExportRow
	}
	products := make(map[string]*product) // keyed by handle+"|"+certNumber
	backImages := make(map[string]string) // back images keyed by handle
	var order []string
	var parseErrors []campaigns.ImportError

	for rowIdx, rec := range rows[1:] {
		getField := func(idx int) string {
			if idx >= 0 && idx < len(rec) {
				return strings.TrimSpace(rec[idx])
			}
			return ""
		}

		handle := getField(colIdx("handle"))
		if handle == "" {
			continue
		}

		title := getField(colIdx("title"))
		imageURL := getField(colIdx("image src"))

		if title == "" {
			// Variant-only row — capture back image by handle for later merging.
			if imageURL != "" {
				if _, exists := backImages[handle]; !exists {
					backImages[handle] = imageURL
				}
			}
			continue
		}

		// Extract PSA cert number from explicit column or SKU pattern.
		// Only PSA-graded cards with valid cert numbers are imported.
		certNumber := normalizePSACert(getField(colIdx("cert number")))
		if certNumber == "" {
			certNumber = normalizePSACert(getField(colIdx("cert")))
		}
		if certNumber == "" {
			// Fallback: extract from SKU pattern PSA-XXXXX
			certNumber = normalizePSACert(getField(colIdx("sku")))
		}
		if certNumber == "" {
			// No PSA cert number — skip this row (CGC, raw, etc.)
			continue
		}

		// Use composite key so rows with same handle but different certs are preserved.
		productKey := handle + "|" + certNumber
		if _, exists := products[productKey]; exists {
			// Duplicate handle+cert row — capture additional image by handle.
			if imageURL != "" {
				if _, hasBack := backImages[handle]; !hasBack {
					backImages[handle] = imageURL
				}
			}
			continue
		}

		tags := getField(colIdx("tags"))
		cardName, cardNumber, setName, _, tagErr := campaigns.ParseShopifyTags(tags)
		if tagErr != nil {
			h.logger.Debug(r.Context(), "shopify import: tags parse failed, falling back to title",
				observability.String("handle", handle),
				observability.String("tags", tags),
				observability.Err(tagErr))
		}

		// Fall back to title-based extraction if tags don't have card name
		if cardName == "" {
			cardName = campaigns.ExtractCardNameFromTitle(title)
		}

		grader, gradeValue := campaigns.ExtractGraderAndGrade(title)
		if grader == "" {
			grader = "PSA" // cert number implies PSA
		}

		var variantPrice float64
		// Try "variant price" first (standard Shopify export), fall back to "price"
		priceField := getField(colIdx("variant price"))
		if priceField == "" {
			priceField = getField(colIdx("price"))
		}
		if priceField != "" {
			v, err := parseCurrencyString(priceField)
			if err != nil {
				h.logger.Warn(r.Context(), "shopify import: invalid price",
					observability.String("handle", handle),
					observability.String("value", priceField),
					observability.Int("row", rowIdx+2))
				parseErrors = append(parseErrors, campaigns.ImportError{
					Row:   rowIdx + 2,
					Error: fmt.Sprintf("invalid price %q for handle %s: %v", priceField, handle, err),
				})
				continue
			}
			variantPrice = v
		}

		var costPerItem float64
		if cp := getField(colIdx("cost per item")); cp != "" {
			v, err := parseCurrencyString(cp)
			if err != nil {
				h.logger.Warn(r.Context(), "shopify import: invalid cost per item",
					observability.String("handle", handle),
					observability.String("value", cp),
					observability.Int("row", rowIdx+2))
				parseErrors = append(parseErrors, campaigns.ImportError{
					Row:   rowIdx + 2,
					Error: fmt.Sprintf("invalid cost per item %q for handle %s: %v", cp, handle, err),
				})
				continue
			}
			costPerItem = v
		}

		products[productKey] = &product{
			row: campaigns.ShopifyExportRow{
				Handle:        handle,
				CertNumber:    certNumber,
				Title:         title,
				CardName:      cardName,
				CardNumber:    cardNumber,
				SetName:       setName,
				Grader:        grader,
				GradeValue:    gradeValue,
				VariantPrice:  variantPrice,
				CostPerItem:   costPerItem,
				FrontImageURL: imageURL,
			},
		}
		order = append(order, productKey)
	}

	var shopifyRows []campaigns.ShopifyExportRow
	for _, key := range order {
		p := products[key]
		if img, ok := backImages[p.row.Handle]; ok {
			p.row.BackImageURL = img
		}
		shopifyRows = append(shopifyRows, p.row)
	}

	if len(shopifyRows) == 0 {
		if len(parseErrors) > 0 {
			writeJSON(w, http.StatusOK, campaigns.ExternalImportResult{
				Failed: len(parseErrors),
				Errors: parseErrors,
			})
		} else {
			writeJSON(w, http.StatusBadRequest, campaigns.ExternalImportResult{
				Failed: 1,
				Errors: []campaigns.ImportError{{Row: 0, Error: "No valid product rows found in CSV"}},
			})
		}
		return
	}

	result, err := h.service.ImportExternalCSV(r.Context(), shopifyRows)
	if err != nil {
		h.logger.Error(r.Context(), "external import failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if len(parseErrors) > 0 {
		result.Errors = append(result.Errors, parseErrors...)
		result.Failed += len(parseErrors)
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

// buildHeaderMap creates a lowercase header name → column index map.
func buildHeaderMap(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, col := range header {
		m[strings.TrimSpace(strings.ToLower(col))] = i
	}
	return m
}

// findHeaderRow scans the first few rows for known PSA column names.
// Returns the header row index, or -1 if not found.
func findHeaderRow(rows [][]string) int {
	knownColumns := map[string]bool{
		"cert number":   true,
		"listing title": true,
		"grade":         true,
		"price paid":    true,
	}
	for i, row := range rows {
		if i > 5 { // Don't scan more than 6 rows
			break
		}
		headerMap := buildHeaderMap(row)
		matches := 0
		for col := range knownColumns {
			if _, ok := headerMap[col]; ok {
				matches++
			}
		}
		if matches >= 3 { // At least 3 known columns found
			return i
		}
	}
	return -1
}
