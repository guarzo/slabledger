package campaigns

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
)

// ExportMMFormatGlobal exports all unsold inventory in Market Movers collection import CSV format.
func (s *service) ExportMMFormatGlobal(ctx context.Context) ([]MMExportEntry, error) {
	unsold, err := s.repo.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsold: %w", err)
	}

	entries := make([]MMExportEntry, 0, len(unsold))
	for _, p := range unsold {
		// Grade string: "PSA 10", "BGS 9.5", etc.
		grader := p.Grader
		if grader == "" {
			grader = "PSA"
		}
		grade := fmt.Sprintf("%s %s", grader, formatMMGrade(p.GradeValue))

		// Player name: prefer enriched CardPlayer, fall back to raw CardName.
		playerName := p.CardPlayer
		if playerName == "" {
			playerName = p.CardName
		}

		// Card number: add "#" prefix when non-empty.
		cardNumber := ""
		if p.CardNumber != "" {
			cardNumber = "#" + p.CardNumber
		}

		// Last sale price: prefer MM value, fall back to CL value.
		lastSalePrice := 0.0
		if p.MMValueCents > 0 {
			lastSalePrice = mathutil.ToDollars(int64(p.MMValueCents))
		} else if p.CLValueCents > 0 {
			lastSalePrice = mathutil.ToDollars(int64(p.CLValueCents))
		}

		entries = append(entries, MMExportEntry{
			Sport:                p.CardCategory,
			Grade:                grade,
			PlayerName:           playerName,
			Year:                 p.CardYear,
			Set:                  p.SetName,
			Variation:            p.CardVariation,
			CardNumber:           cardNumber,
			SpecificQualifier:    "",
			Quantity:             "1",
			DatePurchased:        p.PurchaseDate,
			PurchasePricePerCard: mathutil.ToDollars(int64(p.BuyCostCents)),
			Notes:                p.CertNumber,
			Category:             "",
			DateSold:             "",
			SoldPricePerCard:     "",
			LastSalePrice:        lastSalePrice,
			LastSaleDate:         p.SnapshotDate,
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
	existingMap, err := s.repo.GetPurchasesByCertNumbers(ctx, certNumbers)
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

		if err := s.repo.UpdatePurchaseMMValue(ctx, purchase.ID, newMMCents); err != nil {
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

// formatMMGrade formats a numeric grade for MM export: 10 → "10", 9.5 → "9.5".
func formatMMGrade(v float64) string {
	if v == float64(int(v)) {
		return fmt.Sprintf("%d", int(v))
	}
	return fmt.Sprintf("%g", v)
}
