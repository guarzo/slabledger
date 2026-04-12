package inventory

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// --- Global Operations ---

func (s *service) RefreshCLValuesGlobal(ctx context.Context, rows []CLExportRow) (*GlobalCLRefreshResult, error) {
	// Build campaign name lookup for the response
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	campaignNames := make(map[string]string, len(allCampaigns))
	for _, c := range allCampaigns {
		campaignNames[c.ID] = c.Name
	}

	// Batch pre-load all purchases by cert number to avoid per-row DB calls
	certNumbers := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.SlabSerial != "" {
			certNumbers = append(certNumbers, row.SlabSerial)
		}
	}
	existingMap, err := s.purchases.GetPurchasesByGraderAndCertNumbers(ctx, "PSA", certNumbers)
	if err != nil {
		return nil, fmt.Errorf("batch load purchases: %w", err)
	}

	result := &GlobalCLRefreshResult{
		ByCampaign: make(map[string]CampaignRefreshSummary),
	}

	for i, row := range rows {
		rowNum := i + 2

		if row.SlabSerial == "" {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{Row: rowNum, Error: "missing cert number"})
			result.Results = append(result.Results, CLRefreshItemResult{
				Status: "failed", Error: "missing cert number",
			})
			continue
		}

		purchase, found := existingMap[row.SlabSerial]
		if !found {
			result.NotFound++
			result.Results = append(result.Results, CLRefreshItemResult{
				CertNumber: row.SlabSerial, Status: "skipped", Error: "not found in inventory",
			})
			continue
		}

		newCLCents := mathutil.ToCentsInt(row.CurrentValue)
		oldCLCents := purchase.CLValueCents
		newPop := row.Population
		if newPop == 0 {
			newPop = purchase.Population
		}

		if err := s.purchases.UpdatePurchaseCLValue(ctx, purchase.ID, newCLCents, newPop); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{Row: rowNum, Error: err.Error()})
			result.Results = append(result.Results, CLRefreshItemResult{
				CertNumber: row.SlabSerial, CardName: purchase.CardName,
				OldValueCents: oldCLCents, NewValueCents: newCLCents,
				Status: "failed", Error: err.Error(),
			})
			continue
		}
		purchase.CLValueCents = newCLCents

		// Flag for DH push if eligible (first-time push).
		if purchase.NeedsDHPush() {
			if err := s.purchases.UpdatePurchaseDHPushStatus(ctx, purchase.ID, DHPushStatusPending); err != nil && s.logger != nil {
				s.logger.Warn(ctx, "cl refresh: failed to set dh push status",
					observability.String("purchaseID", purchase.ID),
					observability.Err(err))
			}
		}

		// Re-push to DH when market value changes on an already-pushed item.
		if purchase.DHInventoryID != 0 && newCLCents != oldCLCents {
			if err := s.purchases.UpdatePurchaseDHPushStatus(ctx, purchase.ID, DHPushStatusPending); err != nil && s.logger != nil {
				s.logger.Warn(ctx, "cl refresh: failed to re-enroll for dh push",
					observability.String("purchaseID", purchase.ID),
					observability.Err(err))
			}
		}

		// Backfill card metadata from CL data if needed (DB-only, no external calls).
		// Must run before history recording so history uses repaired identity.
		s.backfillMetadataFromCL(ctx, purchase, row)

		// Record history entries (best-effort — never block the import).
		s.recordCLHistory(ctx, purchase, newCLCents)
		s.recordPopHistory(ctx, purchase, newPop)

		// Defer snapshot recovery to background worker instead of blocking the CSV upload.
		snapshotDeferred := false
		if s.priceProv != nil && needsSnapshotRecovery(purchase) {
			if err := s.purchases.UpdatePurchaseSnapshotStatus(ctx, purchase.ID, SnapshotStatusPending, 0); err != nil {
				if s.logger != nil {
					s.logger.Warn(ctx, "failed to set pending snapshot status",
						observability.String("purchaseID", purchase.ID),
						observability.Err(err))
				}
			} else {
				snapshotDeferred = true
			}
		}

		result.Updated++
		result.Results = append(result.Results, CLRefreshItemResult{
			CertNumber: row.SlabSerial, CardName: purchase.CardName,
			OldValueCents: oldCLCents, NewValueCents: newCLCents,
			Status: "updated", SnapshotQueued: snapshotDeferred,
		})

		// Track per-campaign summary
		summary := result.ByCampaign[purchase.CampaignID]
		summary.CampaignName = campaignNames[purchase.CampaignID]
		summary.Updated++
		result.ByCampaign[purchase.CampaignID] = summary
	}
	// Kick off background cert→card_id resolution for successfully updated purchases.
	if s.cardIDResolver != nil {
		var certs []string
		seen := make(map[string]struct{})
		for _, res := range result.Results {
			if res.Status != "updated" || res.CertNumber == "" {
				continue
			}
			if _, ok := seen[res.CertNumber]; !ok {
				seen[res.CertNumber] = struct{}{}
				certs = append(certs, res.CertNumber)
			}
		}
		if len(certs) > 0 {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				s.batchResolveCardIDs(ctx, certs)
			}()
		}
	}

	return result, nil
}

func (s *service) ImportCLExportGlobal(ctx context.Context, rows []CLExportRow) (*GlobalImportResult, error) {
	// All campaigns for name lookups (refreshed purchases may belong to non-active campaigns)
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}

	// Only active campaigns for new purchase allocation (exclude external — only Shopify import uses it)
	activeCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	activeCampaigns = filterOutExternal(activeCampaigns)

	campaignMap := make(map[string]*Campaign, len(allCampaigns))
	for i := range allCampaigns {
		campaignMap[allCampaigns[i].ID] = &allCampaigns[i]
	}

	// Batch pre-load all purchases by cert number to avoid per-row DB calls
	certNumbers := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.SlabSerial != "" {
			certNumbers = append(certNumbers, row.SlabSerial)
		}
	}
	existingMap, err := s.purchases.GetPurchasesByGraderAndCertNumbers(ctx, "PSA", certNumbers)
	if err != nil {
		return nil, fmt.Errorf("batch load purchases: %w", err)
	}

	result := &GlobalImportResult{
		ByCampaign: make(map[string]CampaignImportSummary),
	}

	for i, row := range rows {
		rowNum := i + 2

		if row.SlabSerial == "" {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{Row: rowNum, Error: "missing cert number (Slab Serial #)"})
			result.Results = append(result.Results, GlobalImportItemResult{
				Status: "failed", Error: "missing cert number",
			})
			continue
		}

		gradeValue := ExtractGrade(row.Card)
		if gradeValue == 0 {
			gradeValue = ExtractGrade(row.Condition)
		}
		if gradeValue == 0 && s.logger != nil {
			s.logger.Debug(ctx, "ExtractGrade returned 0 during CL import",
				observability.String("cert", row.SlabSerial),
				observability.String("card", row.Card),
				observability.String("condition", row.Condition))
		}
		// Use float64 grade value for result reporting (preserves half-grades)
		resultGrade := gradeValue

		buyCostCents := mathutil.ToCentsInt(row.Investment)

		// Check if cert already exists — if so, refresh
		existing := existingMap[row.SlabSerial]
		if existing != nil {
			// Refresh existing purchase
			newCLCents := mathutil.ToCentsInt(row.CurrentValue)
			newPop := row.Population
			if newPop == 0 {
				newPop = existing.Population
			}
			if err := s.purchases.UpdatePurchaseCLValue(ctx, existing.ID, newCLCents, newPop); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, ImportError{Row: rowNum, Error: err.Error()})
				result.Results = append(result.Results, GlobalImportItemResult{
					CertNumber: row.SlabSerial, CardName: row.Card, Grade: resultGrade,
					Status: "failed", Error: err.Error(),
				})
				continue
			}
			existing.CLValueCents = newCLCents

			// Flag for DH push if eligible
			if existing.NeedsDHPush() {
				if err := s.purchases.UpdatePurchaseDHPushStatus(ctx, existing.ID, DHPushStatusPending); err != nil && s.logger != nil {
					s.logger.Warn(ctx, "cl import: failed to set dh push status",
						observability.String("purchaseID", existing.ID),
						observability.Err(err))
				}
			}

			s.backfillMetadataFromCL(ctx, existing, row)
			s.recordCLHistory(ctx, existing, newCLCents)
			s.recordPopHistory(ctx, existing, newPop)
			// Defer snapshot recovery to background worker instead of blocking import.
			if s.priceProv != nil && needsSnapshotRecovery(existing) {
				if err := s.purchases.UpdatePurchaseSnapshotStatus(ctx, existing.ID, SnapshotStatusPending, 0); err != nil && s.logger != nil {
					s.logger.Warn(ctx, "failed to set pending snapshot status",
						observability.String("purchaseID", existing.ID),
						observability.Err(err))
				}
			}
			result.Refreshed++
			cName := ""
			if c := campaignMap[existing.CampaignID]; c != nil {
				cName = c.Name
			}
			result.Results = append(result.Results, GlobalImportItemResult{
				CertNumber: row.SlabSerial, CardName: existing.CardName, Grade: existing.GradeValue,
				Status: "refreshed", CampaignID: existing.CampaignID, CampaignName: cName,
				SetName: existing.SetName, CardNumber: existing.CardNumber,
			})
			summary := result.ByCampaign[existing.CampaignID]
			summary.CampaignName = cName
			summary.Refreshed++
			result.ByCampaign[existing.CampaignID] = summary
			continue
		}

		// New purchase — find matching campaign (only active campaigns)
		match := FindMatchingCampaign(gradeValue, buyCostCents, row.Card, row.Set, 0, activeCampaigns)

		switch match.Status {
		case "matched":
			campaign := campaignMap[match.CampaignID]
			p := &Purchase{
				CampaignID:          match.CampaignID,
				CardName:            CLCardName(row),
				CertNumber:          row.SlabSerial,
				CardNumber:          row.Number,
				SetName:             row.Set,
				Grader:              "PSA",
				GradeValue:          gradeValue,
				CLValueCents:        mathutil.ToCentsInt(row.CurrentValue),
				BuyCostCents:        buyCostCents,
				PSASourcingFeeCents: campaign.PSASourcingFeeCents,
				Population:          row.Population,
				PurchaseDate:        row.DatePurchased,
			}
			if err := s.CreatePurchase(ctx, p); err != nil {
				if IsDuplicateCertNumber(err) {
					result.Skipped++
					result.Results = append(result.Results, GlobalImportItemResult{
						CertNumber: row.SlabSerial, CardName: row.Card, Grade: resultGrade,
						Status: "skipped",
					})
				} else {
					result.Failed++
					result.Errors = append(result.Errors, ImportError{Row: rowNum, Error: err.Error()})
					result.Results = append(result.Results, GlobalImportItemResult{
						CertNumber: row.SlabSerial, CardName: row.Card, Grade: resultGrade,
						Status: "failed", Error: err.Error(),
					})
				}
				continue
			}
			if err := s.purchases.UpdatePurchaseDHPushStatus(ctx, p.ID, DHPushStatusPending); err != nil && s.logger != nil {
				s.logger.Warn(ctx, "cl import: failed to set dh push status for new purchase",
					observability.String("purchaseID", p.ID),
					observability.Err(err))
			}
			result.Allocated++
			result.Results = append(result.Results, GlobalImportItemResult{
				CertNumber: row.SlabSerial, CardName: row.Card, Grade: resultGrade,
				Status: "allocated", CampaignID: campaign.ID, CampaignName: campaign.Name,
				SetName: row.Set, CardNumber: row.Number,
			})
			summary := result.ByCampaign[campaign.ID]
			summary.CampaignName = campaign.Name
			summary.Allocated++
			result.ByCampaign[campaign.ID] = summary
			// Cache newly created purchase so duplicate cert rows in the same batch
			// are handled as refreshes rather than allocation attempts.
			if created, err := s.purchases.GetPurchaseByCertNumber(ctx, "PSA", row.SlabSerial); err == nil && created != nil {
				existingMap[row.SlabSerial] = created
			}

		case "ambiguous":
			result.Ambiguous++
			result.Results = append(result.Results, GlobalImportItemResult{
				CertNumber: row.SlabSerial, CardName: row.Card, Grade: resultGrade,
				Status: "ambiguous", Candidates: match.Candidates,
				BuyCostCents: buyCostCents, CLValueCents: mathutil.ToCentsInt(row.CurrentValue),
				PurchaseDate: row.DatePurchased, SetName: row.Set, CardNumber: row.Number,
				Population: row.Population,
			})

		default: // unmatched
			result.Unmatched++
			result.Results = append(result.Results, GlobalImportItemResult{
				CertNumber: row.SlabSerial, CardName: row.Card, Grade: resultGrade,
				Status:       "unmatched",
				BuyCostCents: buyCostCents, CLValueCents: mathutil.ToCentsInt(row.CurrentValue),
				PurchaseDate: row.DatePurchased, SetName: row.Set, CardNumber: row.Number,
				Population: row.Population,
			})
		}
	}

	// Kick off background cert→card_id resolution for successfully persisted purchases.
	if s.cardIDResolver != nil {
		var certs []string
		seen := make(map[string]struct{})
		for _, res := range result.Results {
			if res.CertNumber == "" {
				continue
			}
			// Only resolve certs for rows that were actually persisted
			if res.Status != "refreshed" && res.Status != "allocated" {
				continue
			}
			if _, ok := seen[res.CertNumber]; !ok {
				seen[res.CertNumber] = struct{}{}
				certs = append(certs, res.CertNumber)
			}
		}
		if len(certs) > 0 {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				s.batchResolveCardIDs(ctx, certs)
			}()
		}
	}

	return result, nil
}

func (s *service) ExportCLFormatGlobal(ctx context.Context, missingCLOnly bool) ([]CLExportEntry, error) {
	unsold, err := s.purchases.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsold: %w", err)
	}

	entries := make([]CLExportEntry, 0, len(unsold))
	for _, p := range unsold {
		if p.Grader != "PSA" {
			continue
		}
		if missingCLOnly && p.CLValueCents > 0 {
			continue
		}
		datePurchased := p.PurchaseDate
		if t, err := time.Parse("2006-01-02", p.PurchaseDate); err == nil {
			datePurchased = fmt.Sprintf("%d/%d/%d", int(t.Month()), t.Day(), t.Year())
		}
		entries = append(entries, CLExportEntry{
			DatePurchased:  datePurchased,
			CertNumber:     p.CertNumber,
			Grader:         p.Grader,
			Investment:     mathutil.ToDollars(int64(p.BuyCostCents)),
			EstimatedValue: mathutil.ToDollars(int64(p.CLValueCents)),
		})
	}
	return entries, nil
}

// filterOutExternal returns a copy of campaigns with the external campaign removed.
func filterOutExternal(campaigns []Campaign) []Campaign {
	filtered := make([]Campaign, 0, len(campaigns))
	for _, c := range campaigns {
		if c.ID != ExternalCampaignID {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// recordCLHistory records a CL value observation if the recorder is configured.
func (s *service) recordCLHistory(ctx context.Context, p *Purchase, clValueCents int) {
	if s.clRecorder == nil || clValueCents == 0 {
		return
	}
	today := time.Now().Format("2006-01-02")
	if err := s.clRecorder.RecordCLValue(ctx, CLValueEntry{
		CertNumber:      p.CertNumber,
		CardName:        p.CardName,
		SetName:         p.SetName,
		CardNumber:      p.CardNumber,
		GradeValue:      p.GradeValue,
		CLValueCents:    clValueCents,
		ObservationDate: today,
		Source:          "csv_import",
	}); err != nil && s.logger != nil {
		s.logger.Warn(ctx, "failed to record CL history", observability.Err(err))
	}
}

// recordPopHistory records a population observation if the recorder is configured.
func (s *service) recordPopHistory(ctx context.Context, p *Purchase, pop int) {
	if s.popRecorder == nil || pop == 0 {
		return
	}
	today := time.Now().Format("2006-01-02")
	if err := s.popRecorder.RecordPopulation(ctx, PopulationEntry{
		CardName:        p.CardName,
		SetName:         p.SetName,
		CardNumber:      p.CardNumber,
		GradeValue:      p.GradeValue,
		Grader:          p.Grader,
		Population:      pop,
		ObservationDate: today,
		Source:          "csv_import",
	}); err != nil && s.logger != nil {
		s.logger.Warn(ctx, "failed to record pop history", observability.Err(err))
	}
}
