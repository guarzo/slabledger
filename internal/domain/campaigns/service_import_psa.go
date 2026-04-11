package campaigns

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// defaultPSAPaymentTermDays is the standard net-payment term for PSA invoices.
const defaultPSAPaymentTermDays = 15

func (s *service) ImportPSAExportGlobal(ctx context.Context, rows []PSAExportRow) (*PSAImportResult, error) {
	allCampaigns, err := s.repo.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	// Only active campaigns for new purchase allocation (exclude external — only Shopify import uses it)
	activeCampaigns, err := s.repo.ListCampaigns(ctx, true)
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
		if row.CertNumber != "" {
			certNumbers = append(certNumbers, row.CertNumber)
		}
	}
	existingMap, err := s.repo.GetPurchasesByGraderAndCertNumbers(ctx, "PSA", certNumbers)
	if err != nil {
		return nil, fmt.Errorf("batch load purchases: %w", err)
	}

	result := &PSAImportResult{
		ByCampaign: make(map[string]CampaignImportSummary),
	}

	for i, row := range rows {
		rowNum := i + 3 // PSA CSV header is row 2, data starts row 3

		if row.CertNumber == "" {
			result.Skipped++
			continue
		}

		// Update existing purchases with PSA-specific fields (invoice date, vault status, etc.)
		existing := existingMap[row.CertNumber]
		if existing != nil {
			itemResult := s.handleExistingPSAPurchase(ctx, existing, row)
			result.Results = append(result.Results, itemResult)
			switch itemResult.Status {
			case "failed":
				result.Failed++
				result.Errors = append(result.Errors, ImportError{Row: rowNum, Error: itemResult.Error})
			case "updated":
				result.Updated++
			}
			continue
		}

		gradeValue := row.Grade
		if gradeValue == 0 {
			gradeValue = ExtractGrade(row.ListingTitle)
		}
		buyCostCents := mathutil.ToCentsInt(row.PricePaid)

		if buyCostCents <= 0 || gradeValue == 0 {
			result.Skipped++
			result.Results = append(result.Results, PSAImportItemResult{
				CertNumber: row.CertNumber, CardName: row.ListingTitle, Grade: gradeValue,
				Status: "skipped", Error: "missing price or grade",
			})
			continue
		}

		// Parse card metadata from listing title (fast, no API calls).
		// Cert lookups happen asynchronously after import completes.
		meta := parseCardMetadataFromTitle(row.ListingTitle, row.Category)
		if meta.ParseWarning != "" && s.logger != nil {
			s.logger.Info(ctx, "PSA title parse fallback",
				observability.String("cert", row.CertNumber),
				observability.String("warning", meta.ParseWarning),
				observability.String("title", row.ListingTitle))
		}

		// New purchase — find matching campaign (use float64 grade for half-grade support)
		match := FindMatchingCampaign(gradeValue, buyCostCents, meta.CardName, meta.SetName, meta.CardYear, activeCampaigns)
		var campaign *Campaign
		if match.Status == "matched" {
			campaign = campaignMap[match.CampaignID]
		}

		itemResult := s.handleNewPSAPurchase(ctx, row, gradeValue, buyCostCents, meta, match, campaign)
		result.Results = append(result.Results, itemResult)
		switch itemResult.Status {
		case "allocated":
			result.Allocated++
			summary := result.ByCampaign[campaign.ID]
			summary.CampaignName = campaign.Name
			summary.Allocated++
			result.ByCampaign[campaign.ID] = summary
			// Cache newly created purchase so duplicate cert rows in the same batch
			// are handled as updates rather than allocation attempts.
			if created, err := s.repo.GetPurchaseByCertNumber(ctx, "PSA", row.CertNumber); err == nil && created != nil {
				existingMap[row.CertNumber] = created
			}
		case "ambiguous":
			result.Ambiguous++
		case "unmatched":
			result.Unmatched++
		case "skipped":
			result.Skipped++
		case "failed":
			result.Failed++
			result.Errors = append(result.Errors, ImportError{Row: rowNum, Error: itemResult.Error})
		}
	}

	// Persist ambiguous and unmatched items for later review.
	if s.pendingItemRepo != nil {
		var pending []PendingItem
		for _, r := range result.Results {
			if r.Status != "ambiguous" && r.Status != "unmatched" {
				continue
			}
			pending = append(pending, PendingItem{
				ID:           s.idGen(),
				CertNumber:   r.CertNumber,
				CardName:     r.CardName,
				SetName:      r.SetName,
				CardNumber:   r.CardNumber,
				Grade:        r.Grade,
				BuyCostCents: r.BuyCostCents,
				PurchaseDate: r.PurchaseDate,
				Status:       r.Status,
				Candidates:   r.Candidates,
				Source:       importSourceFromContext(ctx),
			})
		}
		if len(pending) > 0 {
			if err := s.pendingItemRepo.SavePendingItems(ctx, pending); err != nil && s.logger != nil {
				s.logger.Error(ctx, "failed to save pending items",
					observability.Err(err),
					observability.Int("count", len(pending)))
			}
		}
	}

	// Auto-detect invoices from newly imported purchases with invoice dates
	created, updated := s.autoDetectInvoices(ctx, rows)
	result.InvoicesCreated = created
	result.InvoicesUpdated = updated

	// Submit cert numbers to the bounded enrichment worker.
	// Cert lookups are rate-limited (100/day), so they run in the background
	// to avoid blocking the import response.
	if s.certEnrichCh != nil && result.Allocated > 0 {
		queued := 0
		for _, r := range result.Results {
			if r.Status == "allocated" && r.CertNumber != "" {
				select {
				case s.certEnrichCh <- r.CertNumber:
					queued++
				default:
					if s.logger != nil {
						s.logger.Warn(ctx, "cert enrichment queue full, dropping cert",
							observability.String("cert", r.CertNumber))
					}
				}
			}
		}
		result.CertEnrichmentPending = queued
	}

	// Kick off background batch cert→card_id resolution if resolver is available
	if s.cardIDResolver != nil && result.Allocated > 0 {
		certs := collectAllocatedCerts(result.Results)
		if len(certs) > 0 {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
				defer cancel()
				s.batchResolveCardIDs(ctx, certs)
			}()
		}
	}

	return result, nil
}

// collectAllocatedCerts returns unique cert numbers from allocated PSA import results.
func collectAllocatedCerts(results []PSAImportItemResult) []string {
	seen := make(map[string]struct{})
	var certs []string
	for _, r := range results {
		if r.Status == "allocated" && r.CertNumber != "" {
			if _, ok := seen[r.CertNumber]; !ok {
				seen[r.CertNumber] = struct{}{}
				certs = append(certs, r.CertNumber)
			}
		}
	}
	return certs
}

// handleExistingPSAPurchase updates an existing purchase with PSA-specific fields
// and repairs card metadata if the set name is generic or the card number is missing.
func (s *service) handleExistingPSAPurchase(ctx context.Context, existing *Purchase, row PSAExportRow) PSAImportItemResult {
	fields := PSAUpdateFields{
		VaultStatus:    row.VaultStatus,
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
	psaChanged := existing.VaultStatus != fields.VaultStatus ||
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
	if isGenericSetName(existing.SetName) || existing.CardNumber == "" {
		parsed := parseCardMetadataFromTitle(row.ListingTitle, row.Category)
		newName, newNum, newSet := existing.CardName, existing.CardNumber, existing.SetName
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
		return PSAImportItemResult{
			CertNumber: row.CertNumber, CardName: existing.CardName, Grade: existing.GradeValue,
			Status: "unchanged", CampaignID: existing.CampaignID,
			SetName: existing.SetName, CardNumber: existing.CardNumber,
		}
	}

	return PSAImportItemResult{
		CertNumber: row.CertNumber, CardName: existing.CardName, Grade: existing.GradeValue,
		Status: "updated", CampaignID: existing.CampaignID,
		SetName: existing.SetName, CardNumber: existing.CardNumber,
	}
}

// handleNewPSAPurchase processes a new PSA purchase row that has no existing purchase.
// It uses the pre-computed match result and resolved campaign to create the purchase
// (if matched) or report the row as ambiguous/unmatched.
func (s *service) handleNewPSAPurchase(ctx context.Context, row PSAExportRow, gradeValue float64, buyCostCents int, meta PSACardMetadata, match MatchResult, campaign *Campaign) PSAImportItemResult {
	switch match.Status {
	case "matched":
		purchaseDate := row.Date
		if purchaseDate == "" {
			purchaseDate = time.Now().Format("2006-01-02")
		}
		p := &Purchase{
			CampaignID:          campaign.ID,
			CardName:            meta.CardName,
			CertNumber:          row.CertNumber,
			CardNumber:          meta.CardNumber,
			SetName:             meta.SetName,
			Grader:              "PSA",
			GradeValue:          gradeValue,
			BuyCostCents:        buyCostCents,
			PSASourcingFeeCents: campaign.PSASourcingFeeCents,
			Population:          0,
			PurchaseDate:        purchaseDate,
			VaultStatus:         row.VaultStatus,
			InvoiceDate:         row.InvoiceDate,
			WasRefunded:         row.WasRefunded,
			FrontImageURL:       row.FrontImageURL,
			BackImageURL:        row.BackImageURL,
			PurchaseSource:      row.PurchaseSource,
			PSAListingTitle:     row.ListingTitle,
		}
		// Only defer snapshot to background worker when a price provider is available
		if s.priceProv != nil {
			p.SnapshotStatus = SnapshotStatusPending
		}
		if err := s.CreatePurchase(ctx, p); err != nil {
			if IsDuplicateCertNumber(err) {
				return PSAImportItemResult{
					CertNumber: row.CertNumber, CardName: meta.CardName, Grade: gradeValue,
					Status: "skipped",
				}
			}
			return PSAImportItemResult{
				CertNumber: row.CertNumber, CardName: meta.CardName, Grade: gradeValue,
				Status: "failed", Error: err.Error(),
			}
		}
		return PSAImportItemResult{
			CertNumber: row.CertNumber, CardName: meta.CardName, Grade: gradeValue,
			Status: "allocated", CampaignID: campaign.ID, CampaignName: campaign.Name,
			SetName: meta.SetName, CardNumber: meta.CardNumber,
		}

	case "ambiguous":
		return PSAImportItemResult{
			CertNumber: row.CertNumber, CardName: meta.CardName, Grade: gradeValue,
			Status: "ambiguous", Candidates: match.Candidates,
			BuyCostCents: buyCostCents, PurchaseDate: row.Date, SetName: meta.SetName,
			CardNumber: meta.CardNumber,
		}

	default:
		return PSAImportItemResult{
			CertNumber: row.CertNumber, CardName: meta.CardName, Grade: gradeValue,
			Status:       "unmatched",
			BuyCostCents: buyCostCents, PurchaseDate: row.Date, SetName: meta.SetName,
			CardNumber: meta.CardNumber,
		}
	}
}

func (s *service) autoDetectInvoices(ctx context.Context, rows []PSAExportRow) (int, int) {
	// Collect all unique invoice dates touched by this import so we reconcile
	// totals even when the CSV row has PricePaid == 0 (existing purchase may
	// already have a stored BuyCostCents, or purchases may have been refunded).
	dates := make(map[string]bool)
	for _, row := range rows {
		if row.InvoiceDate != "" {
			dates[row.InvoiceDate] = true
		}
	}

	existingInvoices, err := s.repo.ListInvoices(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "autoDetectInvoices: failed to list existing invoices", observability.Err(err))
		}
		return 0, 0
	}
	// Use a slice-based map so duplicate invoice dates don't silently overwrite.
	existingByDate := make(map[string][]*Invoice, len(existingInvoices))
	for i := range existingInvoices {
		d := existingInvoices[i].InvoiceDate
		existingByDate[d] = append(existingByDate[d], &existingInvoices[i])
	}

	created, updated := 0, 0
	for invoiceDate := range dates {
		// Recalculate total from the DB — the source of truth after purchases
		// have been created/updated earlier in the import.
		totalCents, err := s.repo.SumPurchaseCostByInvoiceDate(ctx, invoiceDate)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "autoDetectInvoices: failed to sum purchases",
					observability.String("invoiceDate", invoiceDate),
					observability.Err(err))
			}
			continue
		}

		if existing := existingByDate[invoiceDate]; len(existing) > 0 {
			// Update existing invoice totals if purchases changed the amount
			// (including zeroing out when all purchases were refunded).
			for _, inv := range existing {
				if inv.TotalCents != totalCents {
					inv.TotalCents = totalCents
					inv.UpdatedAt = time.Now()
					if err := s.repo.UpdateInvoice(ctx, inv); err != nil {
						if s.logger != nil {
							s.logger.Warn(ctx, "autoDetectInvoices: failed to update invoice",
								observability.String("invoiceDate", invoiceDate),
								observability.Err(err))
						}
					} else {
						updated++
					}
				}
			}
			continue
		}

		// No existing invoice for this date — only create one when there is a positive total.
		if totalCents <= 0 {
			continue
		}

		dueDate := ""
		if t, err := time.Parse("2006-01-02", invoiceDate); err == nil {
			dueDate = t.AddDate(0, 0, defaultPSAPaymentTermDays).Format("2006-01-02")
		}
		inv := &Invoice{
			ID:          s.idGen(),
			InvoiceDate: invoiceDate,
			TotalCents:  totalCents,
			DueDate:     dueDate,
			Status:      "unpaid",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := s.repo.CreateInvoice(ctx, inv); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "autoDetectInvoices: failed to create invoice",
					observability.String("invoiceDate", invoiceDate),
					observability.Err(err))
			}
		} else {
			created++
		}
	}
	return created, updated
}
