package inventory

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ExportMMFormatGlobal exports all unsold inventory in Market Movers collection import CSV format.
// When missingMMOnly is true, only purchases without MM value data are included.
//
// If an MMMappingProvider is configured, entries for cards that have been resolved by the
// MM scheduler are enriched with canonical field data parsed from the MM SearchTitle.
// Unmapped cards fall back to the cleaned local data.
func (s *service) ExportMMFormatGlobal(ctx context.Context, missingMMOnly bool) ([]MMExportEntry, error) {
	unsold, err := s.purchases.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsold: %w", err)
	}

	// Load MM search titles if available (cert → SearchTitle).
	var searchTitles map[string]string
	if s.mmMappings != nil {
		searchTitles, err = s.mmMappings.ListMMSearchTitles(ctx)
		if err != nil {
			// Non-fatal: fall back to cleaned local data for all entries.
			if s.logger != nil {
				s.logger.Warn(ctx, "MM export: failed to load search titles, using local data",
					observability.String("error", err.Error()))
			}
			searchTitles = nil
		}
	}

	entries := make([]MMExportEntry, 0, len(unsold))
	for _, p := range unsold {
		if missingMMOnly && p.MMValueCents > 0 {
			continue
		}

		// Grade string: "PSA 10", "BGS 9.5", etc.
		grader := p.Grader
		if grader == "" {
			grader = "PSA"
		}
		grade := fmt.Sprintf("%s %s", grader, mathutil.FormatGrade(p.GradeValue))

		// Try enrichment from MM SearchTitle (canonical data from Market Movers).
		var playerName, year, setName, variation, cardNumber string
		if title, ok := searchTitles[p.CertNumber]; ok && title != "" {
			fields := parseMMSearchTitle(title)
			playerName = fields.PlayerName
			year = fields.Year
			setName = fields.Set
			variation = fields.Variation
			cardNumber = fields.CardNumber
		}

		// Fall back to local data for any fields not populated from SearchTitle.
		if playerName == "" {
			playerName = p.CardPlayer
			if playerName == "" {
				playerName = p.CardName
			}
			playerName = cleanMMPlayerName(playerName, p.SetName)
		}
		if year == "" {
			year = p.CardYear
		}
		if setName == "" {
			setName = p.SetName
		}
		if variation == "" {
			variation = p.CardVariation
		}
		if cardNumber == "" {
			if p.CardNumber != "" {
				cardNumber = "#" + p.CardNumber
			}
		}

		sport := p.CardCategory
		if sport == "" {
			sport = "Pokemon"
		}

		entries = append(entries, MMExportEntry{
			Sport:                sport,
			Grade:                grade,
			PlayerName:           playerName,
			Year:                 year,
			Set:                  setName,
			Variation:            variation,
			CardNumber:           cardNumber,
			SpecificQualifier:    "",
			Quantity:             "1",
			DatePurchased:        p.PurchaseDate,
			PurchasePricePerCard: mathutil.ToDollars(int64(p.BuyCostCents)),
			Notes:                p.CertNumber,
			Category:             "",
		})
	}
	return entries, nil
}

// RefreshMMValuesGlobal updates mm_value_cents on purchases matched by cert number
// from a Market Movers collection export CSV. Rows with a zero LastSalePrice are skipped.
func (s *service) RefreshMMValuesGlobal(ctx context.Context, rows []MMRefreshRow) (*MMRefreshResult, error) {
	// Collect cert numbers for a single batch load
	certNumbers := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.CertNumber != "" {
			certNumbers = append(certNumbers, row.CertNumber)
		}
	}
	existingMap, err := s.purchases.GetPurchasesByCertNumbers(ctx, certNumbers)
	if err != nil {
		return nil, fmt.Errorf("batch load purchases: %w", err)
	}

	result := &MMRefreshResult{}

	for i, row := range rows {
		rowNum := i + 2

		if row.CertNumber == "" {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{Row: rowNum, Error: "missing cert number"})
			result.Results = append(result.Results, MMRefreshItemResult{Status: "failed", Error: "missing cert number"})
			continue
		}

		purchase, found := existingMap[row.CertNumber]
		if !found {
			result.NotFound++
			result.Results = append(result.Results, MMRefreshItemResult{
				CertNumber: row.CertNumber, Status: "skipped", Error: "not found in inventory",
			})
			continue
		}

		if row.LastSalePrice <= 0 {
			result.Skipped++
			result.Results = append(result.Results, MMRefreshItemResult{
				CertNumber: row.CertNumber, CardName: purchase.CardName, Status: "skipped",
			})
			continue
		}

		newMMCents := mathutil.ToCentsInt(row.LastSalePrice)
		oldMMCents := purchase.MMValueCents

		if err := s.purchases.UpdatePurchaseMMValue(ctx, purchase.ID, newMMCents); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{Row: rowNum, Error: err.Error()})
			result.Results = append(result.Results, MMRefreshItemResult{
				CertNumber: row.CertNumber, CardName: purchase.CardName,
				OldValueCents: oldMMCents, NewValueCents: newMMCents,
				Status: "failed", Error: err.Error(),
			})
			continue
		}

		result.Updated++
		result.Results = append(result.Results, MMRefreshItemResult{
			CertNumber: row.CertNumber, CardName: purchase.CardName,
			OldValueCents: oldMMCents, NewValueCents: newMMCents,
			Status: "updated",
		})
	}

	return result, nil
}
