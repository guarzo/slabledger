package campaigns

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// handleExistingPSAPurchase updates an existing purchase with PSA-specific fields
// and repairs card metadata if the set name is generic or the card number is missing.
func (s *service) handleExistingPSAPurchase(ctx context.Context, existing *Purchase, row PSAExportRow) PSAImportItemResult {
	fields := PSAUpdateFields{
		PSAShipDate:    row.ShipDate,
		InvoiceDate:    row.InvoiceDate,
		WasRefunded:    row.WasRefunded,
		FrontImageURL:  row.FrontImageURL,
		BackImageURL:   row.BackImageURL,
		PurchaseSource: row.PurchaseSource,
	}
	// Only update PSAListingTitle when the import provides one — avoid wiping
	// a previously stored title that the pricing pipeline uses as a fallback.
	if row.ListingTitle != "" {
		fields.PSAListingTitle = row.ListingTitle
	} else {
		fields.PSAListingTitle = existing.PSAListingTitle
	}

	// Skip the SQL UPDATE if nothing changed — prevents bumping updated_at
	// and inflating the "updated" count on idempotent re-syncs.
	psaChanged := existing.PSAShipDate != fields.PSAShipDate ||
		existing.InvoiceDate != fields.InvoiceDate ||
		existing.WasRefunded != fields.WasRefunded ||
		existing.FrontImageURL != fields.FrontImageURL ||
		existing.BackImageURL != fields.BackImageURL ||
		existing.PurchaseSource != fields.PurchaseSource ||
		existing.PSAListingTitle != fields.PSAListingTitle

	buyCostChanged := existing.BuyCostCents == 0 && row.PricePaid > 0

	metadataChanged := false
	if isGenericSetName(existing.SetName) || existing.CardNumber == "" {
		parsed := parseCardMetadataFromTitle(row.ListingTitle, row.Category)
		metadataChanged = (isGenericSetName(existing.SetName) && !isGenericSetName(parsed.SetName)) ||
			(existing.CardNumber == "" && parsed.CardNumber != "")
	}

	if !psaChanged && !buyCostChanged && !metadataChanged {
		return PSAImportItemResult{
			CertNumber: row.CertNumber, CardName: existing.CardName, Grade: existing.GradeValue,
			Status: "unchanged", CampaignID: existing.CampaignID,
			SetName: existing.SetName, CardNumber: existing.CardNumber,
		}
	}

	if psaChanged {
		if err := s.repo.UpdatePurchasePSAFields(ctx, existing.ID, fields); err != nil {
			return PSAImportItemResult{
				CertNumber: row.CertNumber, CardName: existing.CardName, Grade: existing.GradeValue,
				Status: "failed", Error: err.Error(),
			}
		}
	}

	// Backfill buy cost when existing purchase has $0 and PSA CSV has a real price.
	if existing.BuyCostCents == 0 && row.PricePaid > 0 {
		buyCostCents := mathutil.ToCentsInt(row.PricePaid)
		if err := s.repo.UpdatePurchaseBuyCost(ctx, existing.ID, buyCostCents); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "failed to backfill buy cost",
					observability.String("purchaseID", existing.ID),
					observability.String("cert", row.CertNumber),
					observability.Err(err))
			}
			return PSAImportItemResult{
				CertNumber: row.CertNumber, CardName: existing.CardName, Grade: existing.GradeValue,
				Status: "failed", Error: fmt.Sprintf("backfill buy cost for cert %s: %v", row.CertNumber, err),
			}
		}
	}

	// Repair card metadata if set name is generic (e.g., "TCG Cards")
	// or card number is missing. Fixes records from earlier imports.
	metadataWritten := false
	metadataFailed := false
	newName, newNum, newSet := existing.CardName, existing.CardNumber, existing.SetName
	if isGenericSetName(existing.SetName) || existing.CardNumber == "" {
		parsed := parseCardMetadataFromTitle(row.ListingTitle, row.Category)
		needsUpdate := false
		if isGenericSetName(existing.SetName) && !isGenericSetName(parsed.SetName) {
			newSet = parsed.SetName
			needsUpdate = true
		}
		if existing.CardNumber == "" && parsed.CardNumber != "" {
			newNum = parsed.CardNumber
			needsUpdate = true
		}
		if needsUpdate {
			if parsed.CardName != "" && parsed.CardName != row.ListingTitle {
				newName = parsed.CardName
			}
			if err := s.repo.UpdatePurchaseCardMetadata(ctx, existing.ID, newName, newNum, newSet); err != nil {
				metadataFailed = true
				if s.logger != nil {
					s.logger.Warn(ctx, "failed to update purchase card metadata",
						observability.String("purchaseID", existing.ID),
						observability.Err(err))
				}
			} else {
				metadataWritten = true
				if s.priceProv != nil && existing.GradeValue > 0 {
					// Defer snapshot recapture to background worker instead of blocking import
					if err := s.repo.UpdatePurchaseSnapshotStatus(ctx, existing.ID, SnapshotStatusPending, 0); err != nil {
						if s.logger != nil {
							s.logger.Warn(ctx, "failed to set pending snapshot status",
								observability.String("purchaseID", existing.ID),
								observability.Err(err))
						}
					}
				}
			}
		}
	}

	// Return "updated" only if at least one write succeeded.
	if !psaChanged && !buyCostChanged && !metadataWritten {
		if metadataFailed {
			return PSAImportItemResult{
				CertNumber: row.CertNumber, CardName: existing.CardName, Grade: existing.GradeValue,
				Status: "failed", Error: fmt.Sprintf("update card metadata for cert %s", row.CertNumber),
				CampaignID: existing.CampaignID,
				SetName:    existing.SetName, CardNumber: existing.CardNumber,
			}
		}
		return PSAImportItemResult{
			CertNumber: row.CertNumber, CardName: existing.CardName, Grade: existing.GradeValue,
			Status: "unchanged", CampaignID: existing.CampaignID,
			SetName: existing.SetName, CardNumber: existing.CardNumber,
		}
	}

	// Use updated metadata values if metadata was written, otherwise fall back to existing.
	resultName, resultNum, resultSet := existing.CardName, existing.CardNumber, existing.SetName
	if metadataWritten {
		resultName, resultNum, resultSet = newName, newNum, newSet
	}

	return PSAImportItemResult{
		CertNumber: row.CertNumber, CardName: resultName, Grade: existing.GradeValue,
		Status: "updated", CampaignID: existing.CampaignID,
		SetName: resultSet, CardNumber: resultNum,
	}
}
