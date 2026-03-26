package campaigns

import (
	"fmt"
	"strings"
)

// ParseShopifyExportRows parses CSV records from a Shopify product export.
// The first row must be the header row. Returns parsed rows, any parse errors,
// and a fatal error if the CSV structure is invalid (e.g., missing required columns).
func ParseShopifyExportRows(records [][]string) ([]ShopifyExportRow, []ParseError, error) {
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("CSV must have a header row and at least one data row")
	}

	headerMap := BuildHeaderMap(records[0])
	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	// Validate required Shopify columns exist.
	if _, hasHandle := headerMap["handle"]; !hasHandle {
		return nil, nil, fmt.Errorf("CSV is missing required column: handle")
	}
	if _, hasTitle := headerMap["title"]; !hasTitle {
		return nil, nil, fmt.Errorf("CSV is missing required column: title")
	}

	// Consolidate multi-row products by Handle+CertNumber so rows with
	// the same handle but different certs are preserved. Image-only rows
	// (empty title) are tracked separately by handle for back-image merging.
	type product struct {
		row ShopifyExportRow
	}
	products := make(map[string]*product) // keyed by handle+"|"+certNumber
	backImages := make(map[string]string) // back images keyed by handle
	var order []string
	var parseErrors []ParseError

	for rowIdx, rec := range records[1:] {
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
			// Variant-only row -- capture back image by handle for later merging.
			if imageURL != "" {
				if _, exists := backImages[handle]; !exists {
					backImages[handle] = imageURL
				}
			}
			continue
		}

		// Extract PSA cert number from explicit column or SKU pattern.
		// Only PSA-graded cards with valid cert numbers are imported.
		certNumber := NormalizePSACert(getField(colIdx("cert number")))
		if certNumber == "" {
			certNumber = NormalizePSACert(getField(colIdx("cert")))
		}
		if certNumber == "" {
			// Fallback: extract from SKU pattern PSA-XXXXX
			certNumber = NormalizePSACert(getField(colIdx("sku")))
		}
		if certNumber == "" {
			// No PSA cert number -- skip this row (CGC, raw, etc.)
			continue
		}

		// Use composite key so rows with same handle but different certs are preserved.
		productKey := handle + "|" + certNumber
		if _, exists := products[productKey]; exists {
			// Duplicate handle+cert row -- capture additional image by handle.
			if imageURL != "" {
				if _, hasBack := backImages[handle]; !hasBack {
					backImages[handle] = imageURL
				}
			}
			continue
		}

		tags := getField(colIdx("tags"))
		cardName, cardNumber, setName, _, tagErr := ParseShopifyTags(tags)
		if tagErr != nil {
			// Tags parse failed -- will fall back to title-based extraction below.
			_ = tagErr
		}

		// Fall back to title-based extraction if tags don't have card name
		if cardName == "" {
			cardName = ExtractCardNameFromTitle(title)
		}

		grader, gradeValue := ExtractGraderAndGrade(title)
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
			v, err := ParseCurrencyString(priceField)
			if err != nil {
				parseErrors = append(parseErrors, ParseError{
					Row:     rowIdx + 2,
					Field:   "variant price",
					Message: fmt.Sprintf("invalid price %q for handle %s: %v", priceField, handle, err),
				})
				continue
			}
			variantPrice = v
		}

		var costPerItem float64
		if cp := getField(colIdx("cost per item")); cp != "" {
			v, err := ParseCurrencyString(cp)
			if err != nil {
				parseErrors = append(parseErrors, ParseError{
					Row:     rowIdx + 2,
					Field:   "cost per item",
					Message: fmt.Sprintf("invalid cost per item %q for handle %s: %v", cp, handle, err),
				})
				continue
			}
			costPerItem = v
		}

		products[productKey] = &product{
			row: ShopifyExportRow{
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

	var shopifyRows []ShopifyExportRow
	for _, key := range order {
		p := products[key]
		if img, ok := backImages[p.row.Handle]; ok {
			p.row.BackImageURL = img
		}
		shopifyRows = append(shopifyRows, p.row)
	}

	return shopifyRows, parseErrors, nil
}
