package campaigns

import (
	"context"
	"fmt"
	"strings"
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

	// Auto-detect invoices from newly imported purchases with invoice dates
	invoicesCreated := s.autoDetectInvoices(ctx, rows)
	result.InvoicesCreated = invoicesCreated

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

// batchResolveCardIDs resolves cert numbers to CardHedger card_ids in the background.
// This pre-populates the card_id_mappings cache so future pricing lookups can skip
// the search step entirely.
func (s *service) batchResolveCardIDs(ctx context.Context, certs []string) {
	if s.cardIDResolver == nil {
		return
	}
	resolved, err := s.cardIDResolver.ResolveCardIDsByCerts(ctx, certs, "PSA")
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "batch cert→card_id resolution failed",
				observability.Int("certs", len(certs)),
				observability.Err(err))
		}
		return
	}
	if s.logger != nil && len(resolved) > 0 {
		s.logger.Info(ctx, "batch cert→card_id resolution complete",
			observability.Int("requested", len(certs)),
			observability.Int("resolved", len(resolved)))
	}
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
	if err := s.repo.UpdatePurchasePSAFields(ctx, existing.ID, fields); err != nil {
		return PSAImportItemResult{
			CertNumber: row.CertNumber, CardName: existing.CardName, Grade: existing.GradeValue,
			Status: "failed", Error: err.Error(),
		}
	}

	// Repair card metadata if set name is generic (e.g., "TCG Cards")
	// or card number is missing. Fixes records from earlier imports.
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
			} else if s.priceProv != nil && existing.GradeValue > 0 {
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

// certEnrichWorker is a single background goroutine that processes cert numbers
// from the bounded channel. It enriches purchases with authoritative PSA cert
// data, one at a time, respecting the PSA client's built-in rate limiting.
func (s *service) certEnrichWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case certNum, ok := <-s.certEnrichCh:
			if !ok {
				return
			}
			s.enrichSingleCert(ctx, certNum)
		}
	}
}

// enrichSingleCert processes one cert number: looks up the cert, updates
// purchase metadata, persists grade if changed, and recaptures the market snapshot.
func (s *service) enrichSingleCert(ctx context.Context, certNum string) {
	if s.certLookup == nil {
		return
	}

	info, err := s.certLookup.LookupCert(ctx, certNum)
	if err != nil {
		if s.logger != nil {
			s.logger.Debug(ctx, "cert enrichment failed",
				observability.String("cert", certNum),
				observability.Err(err))
		}
		return
	}
	if info == nil {
		return
	}

	purchase, lookupErr := s.repo.GetPurchaseByCertNumber(ctx, "PSA", certNum)
	if lookupErr != nil || purchase == nil {
		return
	}

	cardName := info.CardName
	if cardName == "" {
		cardName = purchase.CardName
	}
	cardNumber := info.CardNumber
	if cardNumber == "" {
		cardNumber = purchase.CardNumber
	}

	setName := purchase.SetName
	if info.Category != "" {
		resolved := resolvePSACategory(info.Category)
		if !isGenericSetName(resolved) {
			setName = resolved
		}
	}

	if info.Variety != "" && !strings.Contains(strings.ToUpper(cardName), strings.ToUpper(info.Variety)) {
		cardName = cardName + " " + info.Variety
	}

	if err := s.repo.UpdatePurchaseCardMetadata(ctx, purchase.ID, cardName, cardNumber, setName); err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "cert enrichment: failed to update purchase",
				observability.String("cert", certNum),
				observability.Err(err))
		}
		return
	}

	if info.Year != "" && purchase.CardYear == "" {
		if err := s.repo.UpdatePurchaseCardYear(ctx, purchase.ID, info.Year); err != nil && s.logger != nil {
			s.logger.Warn(ctx, "cert enrichment: failed to update card year",
				observability.String("cert", certNum),
				observability.Err(err))
		}
	}

	// Persist grade from cert if it differs from the purchase
	grade := info.Grade
	if grade == 0 {
		grade = purchase.GradeValue
	}
	if info.Grade != 0 && info.Grade != purchase.GradeValue {
		if err := s.repo.UpdatePurchaseGrade(ctx, purchase.ID, info.Grade); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "cert enrichment: failed to update grade",
					observability.String("cert", certNum),
					observability.Err(err))
			}
			// Fallback to the persisted grade so the snapshot matches the DB record.
			grade = purchase.GradeValue
		}
	}

	card := CardIdentity{CardName: cardName, CardNumber: cardNumber, SetName: setName, PSAListingTitle: purchase.PSAListingTitle}
	if s.recaptureMarketSnapshot(ctx, purchase.ID, card, grade, purchase.CLValueCents) {
		// Clear pending status so ProcessPendingSnapshots doesn't re-process this purchase
		if purchase.SnapshotStatus != SnapshotStatusNone {
			if err := s.repo.UpdatePurchaseSnapshotStatus(ctx, purchase.ID, SnapshotStatusNone, 0); err != nil && s.logger != nil {
				s.logger.Warn(ctx, "cert enrichment: failed to clear snapshot status",
					observability.String("cert", certNum),
					observability.Err(err))
			}
		}
	}
}

func (s *service) autoDetectInvoices(ctx context.Context, rows []PSAExportRow) int {
	// Group by invoice date, deduplicating by cert number to avoid
	// double-counting when the export contains repeated rows.
	groups := make(map[string]int)    // invoiceDate -> totalCents
	seen := make(map[string]struct{}) // certNumber -> already counted
	for _, row := range rows {
		if row.InvoiceDate == "" || row.PricePaid <= 0 || row.CertNumber == "" {
			continue
		}
		if _, dup := seen[row.CertNumber]; dup {
			continue
		}
		seen[row.CertNumber] = struct{}{}
		cents := mathutil.ToCentsInt(row.PricePaid)
		groups[row.InvoiceDate] += cents
	}

	created := 0
	existingInvoices, err := s.repo.ListInvoices(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "autoDetectInvoices: failed to list existing invoices", observability.Err(err))
		}
		return 0
	}
	existingDates := make(map[string]bool, len(existingInvoices))
	for _, inv := range existingInvoices {
		existingDates[inv.InvoiceDate] = true
	}

	for invoiceDate, totalCents := range groups {
		if existingDates[invoiceDate] {
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
	return created
}
