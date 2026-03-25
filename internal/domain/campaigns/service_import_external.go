package campaigns

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
)

// --- External Purchases ---

func (s *service) EnsureExternalCampaign(ctx context.Context) (*Campaign, error) {
	c, err := s.repo.GetCampaign(ctx, ExternalCampaignID)
	if err == nil {
		return c, nil
	}
	if !IsCampaignNotFound(err) {
		return nil, fmt.Errorf("lookup external campaign: %w", err)
	}
	now := time.Now()
	c = &Campaign{
		ID:                  ExternalCampaignID,
		Name:                ExternalCampaignName,
		Phase:               PhaseActive,
		PSASourcingFeeCents: 0,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := s.repo.CreateCampaign(ctx, c); err != nil {
		// Race condition: another goroutine may have created it concurrently
		if existing, getErr := s.repo.GetCampaign(ctx, ExternalCampaignID); getErr == nil {
			return existing, nil
		}
		return nil, fmt.Errorf("create external campaign: %w", err)
	}
	return c, nil
}

func (s *service) ImportExternalCSV(ctx context.Context, rows []ShopifyExportRow) (*ExternalImportResult, error) {
	campaign, err := s.EnsureExternalCampaign(ctx)
	if err != nil {
		return nil, err
	}

	result := &ExternalImportResult{}

	// Dedup by grader+cert number (same cert can exist across different graders)
	seen := make(map[string]bool)

	for i, row := range rows {
		certNumber := row.CertNumber
		if certNumber == "" {
			result.Skipped++
			result.Results = append(result.Results, ExternalImportItemResult{
				Status: "skipped",
				Error:  "empty cert number",
			})
			continue
		}

		dedupKey := row.Grader + "|" + certNumber
		if seen[dedupKey] {
			result.Skipped++
			result.Results = append(result.Results, ExternalImportItemResult{
				CertNumber: certNumber,
				Status:     "skipped",
				Error:      "duplicate cert number",
			})
			continue
		}
		seen[dedupKey] = true

		now := time.Now()
		purchaseDate := now.Format("2006-01-02")

		buyCostCents := mathutil.ToCentsInt(row.CostPerItem)

		p := &Purchase{
			ID:                  s.idGen(),
			CampaignID:          campaign.ID,
			CardName:            row.CardName,
			CertNumber:          certNumber,
			CardNumber:          row.CardNumber,
			SetName:             row.SetName,
			Grader:              row.Grader,
			GradeValue:          row.GradeValue,
			BuyCostCents:        buyCostCents,
			CLValueCents:        mathutil.ToCentsInt(row.VariantPrice),
			PSASourcingFeeCents: 0,
			PurchaseDate:        purchaseDate,
			FrontImageURL:       row.FrontImageURL,
			BackImageURL:        row.BackImageURL,
			CreatedAt:           now,
			UpdatedAt:           now,
		}

		if err := ValidateAndNormalizeExternalPurchase(p); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{Row: i + 1, Error: err.Error()})
			result.Results = append(result.Results, ExternalImportItemResult{
				CertNumber: certNumber,
				CardName:   row.CardName,
				Status:     "failed",
				Error:      err.Error(),
			})
			continue
		}

		// Check if cert already exists — update if so
		existing, existErr := s.repo.GetPurchaseByCertNumber(ctx, row.Grader, certNumber)
		if existErr != nil && !IsPurchaseNotFound(existErr) {
			// Unexpected read failure — treat as failed row
			result.Failed++
			result.Errors = append(result.Errors, ImportError{Row: i + 1, Error: existErr.Error()})
			result.Results = append(result.Results, ExternalImportItemResult{
				CertNumber: certNumber,
				CardName:   row.CardName,
				Status:     "failed",
				Error:      existErr.Error(),
			})
			continue
		}
		if existErr == nil && existing != nil {
			// Update existing purchase with full external payload
			if err := s.repo.UpdateExternalPurchaseFields(ctx, existing.ID, p); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, ImportError{Row: i + 1, Error: err.Error()})
				result.Results = append(result.Results, ExternalImportItemResult{
					CertNumber: certNumber,
					CardName:   row.CardName,
					Status:     "failed",
					Error:      err.Error(),
				})
				continue
			}
			result.Updated++
			if p.GradeValue > 0 && p.CardName != "" && !isGenericSetName(p.SetName) {
				// Use existing for purchase identity (correct ID), p for card identity (updated values)
				s.captureMarketSnapshot(ctx, existing, p.ToCardIdentity(), p.GradeValue, p.CLValueCents)
			}
			result.Results = append(result.Results, ExternalImportItemResult{
				CertNumber: certNumber,
				CardName:   row.CardName,
				SetName:    row.SetName,
				CardNumber: row.CardNumber,
				Status:     "updated",
			})
			continue
		}

		if err := s.repo.CreatePurchase(ctx, p); err != nil {
			if IsDuplicateCertNumber(err) {
				result.Skipped++
				result.Results = append(result.Results, ExternalImportItemResult{
					CertNumber: certNumber,
					CardName:   row.CardName,
					Status:     "skipped",
					Error:      "duplicate cert",
				})
				continue
			}
			result.Failed++
			result.Errors = append(result.Errors, ImportError{Row: i + 1, Error: err.Error()})
			result.Results = append(result.Results, ExternalImportItemResult{
				CertNumber: certNumber,
				CardName:   row.CardName,
				Status:     "failed",
				Error:      err.Error(),
			})
			continue
		}

		result.Imported++
		if p.GradeValue > 0 && p.CardName != "" && !isGenericSetName(p.SetName) {
			s.captureMarketSnapshot(ctx, p, p.ToCardIdentity(), p.GradeValue, p.CLValueCents)
		}
		result.Results = append(result.Results, ExternalImportItemResult{
			CertNumber: certNumber,
			CardName:   row.CardName,
			SetName:    row.SetName,
			CardNumber: row.CardNumber,
			Status:     "imported",
		})
	}

	return result, nil
}
