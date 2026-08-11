package csvimport

// Extracted from ImportPSAExportGlobal (SLA-97) to keep that function under
// the funlen budget; the decomposition is behavior-preserving.

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// resolvePSARowCampaign resolves the campaign for a new PSA purchase row.
// PSA's own attribution (via psaResolver) wins when it resolves; inference
// via FindMatchingCampaign is the fallback.
func (s *service) resolvePSARowCampaign(ctx context.Context, row PSAExportRow, gradeValue float64, buyCostCents int, meta inventory.PSACardMetadata, campaignMap map[string]*inventory.Campaign, matchingCampaigns []inventory.Campaign) (inventory.MatchResult, *inventory.Campaign, string) {
	// PSA's own attribution wins when it resolves; inference is the fallback.
	var match inventory.MatchResult
	var campaign *inventory.Campaign
	attributionSource := inventory.AttributionSourceInferred

	if s.psaResolver != nil && row.PSACampaignName != "" {
		campaignID, ok, rErr := s.psaResolver.ResolveCampaignID(ctx, row.PSACampaignName)
		switch {
		case rErr != nil:
			// Lookup failed outright (e.g. stale snapshot). Fall back to
			// inference rather than dropping the row, and say so.
			if s.logger != nil {
				s.logger.Warn(ctx, "PSA campaign resolve failed, falling back to inference",
					observability.String("certNumber", row.CertNumber),
					observability.String("psaCampaign", row.PSACampaignName),
					observability.Err(rErr))
			}
		case ok:
			if c := campaignMap[campaignID]; c != nil {
				campaign = c
				// handleNewPSAPurchase switches exclusively on match.Status and
				// falls through to "unmatched" by default. A resolved campaign
				// must therefore be expressed as a synthesized matched result —
				// leaving match at its zero value turns every PSA-resolved row
				// into a pending item, i.e. the exact inverse of this feature.
				match = inventory.MatchResult{Status: "matched", CampaignID: campaignID}
				attributionSource = inventory.AttributionSourcePSA
			}
		default:
			if s.logger != nil {
				s.logger.Info(ctx, "PSA campaign name did not resolve",
					observability.String("certNumber", row.CertNumber),
					observability.String("psaCampaign", row.PSACampaignName))
			}
		}
	}

	if campaign == nil {
		// Inference fallback (use float64 grade for half-grade support).
		match = inventory.FindMatchingCampaign(inventory.MatchInput{
			Grade:        gradeValue,
			BuyCostCents: buyCostCents,
			CardName:     meta.CardName,
			SetName:      meta.SetName,
			CardNumber:   meta.CardNumber,
			CardYear:     meta.CardYear,
			// PSASpecID is left at its zero value here: the CSV-title parse
			// path has no CL spec id yet — cert lookups that resolve it run
			// asynchronously after import completes (see the metadata-parse
			// comment above).
		}, matchingCampaigns)
		if match.Status == "matched" {
			campaign = campaignMap[match.CampaignID]
		}
	}

	return match, campaign, attributionSource
}

// recordPSAItemResult appends itemResult to result.Results and updates the
// import counters and per-campaign summary for a new-purchase row. On a
// successful allocation it also writes the newly created purchase back into
// existingMap, keyed by cert number, so a duplicate cert later in the same
// batch is handled as an update rather than a second allocation attempt.
func (s *service) recordPSAItemResult(ctx context.Context, itemResult PSAImportItemResult, row PSAExportRow, campaign *inventory.Campaign, rowNum int, existingMap map[string]*inventory.Purchase, result *PSAImportResult) {
	result.Results = append(result.Results, itemResult)
	switch itemResult.Status {
	case "allocated":
		result.Allocated++
		if campaign == nil {
			if s.logger != nil {
				s.logger.Error(ctx, "allocated status with nil campaign — skipping ByCampaign update",
					observability.String("certNumber", row.CertNumber))
			}
			break
		}
		summary := result.ByCampaign[campaign.ID]
		summary.CampaignName = campaign.Name
		summary.Allocated++
		result.ByCampaign[campaign.ID] = summary
		// Cache newly created purchase so duplicate cert rows in the same batch
		// are handled as updates rather than allocation attempts.
		created, lookupErr := s.purchases.GetPurchaseByCertNumber(ctx, "PSA", row.CertNumber)
		if lookupErr != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "post-allocation cache update failed — duplicate cert risk",
					observability.String("certNumber", row.CertNumber),
					observability.Err(lookupErr))
			}
		} else if created != nil {
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

// savePendingItems persists ambiguous and unmatched items for later review.
func (s *service) savePendingItems(ctx context.Context, result *PSAImportResult) {
	if s.pendingItemRepo == nil {
		return
	}
	var pending []inventory.PendingItem
	for _, r := range result.Results {
		if r.Status != "ambiguous" && r.Status != "unmatched" {
			continue
		}
		pending = append(pending, inventory.PendingItem{
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
			Source:       inventory.ImportSourceFromContext(ctx),
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

// enqueueCertEnrichment submits allocated cert numbers to the cert enrichment
// queue and the pricing queue, and kicks off background batch cert→card_id
// resolution. All three are best-effort background work: cert lookups are
// rate-limited (100/day), pricing is a bounded async queue (D6), and card ID
// resolution is deferred — none of them block the import response.
func (s *service) enqueueCertEnrichment(ctx context.Context, result *PSAImportResult) {
	// Submit cert numbers to the cert enrichment queue.
	// Cert lookups are rate-limited (100/day), so they run in the background
	// to avoid blocking the import response.
	if s.certEnrichQueue != nil && result.Allocated > 0 {
		queued := 0
		for _, r := range result.Results {
			if r.Status == "allocated" && r.CertNumber != "" {
				s.certEnrichQueue.Enqueue(r.CertNumber)
				queued++
			}
		}
		result.CertEnrichmentPending = queued
	}

	// Submit cert numbers for intake-time CL pricing (D6). Without this, a
	// bulk-imported purchase has no CL observation until the next daily refresh
	// sweep — and that sweep only ever touches unsold inventory, so a card that
	// sells quickly gets no CL-at-purchase snapshot at all.
	if s.pricingQueue != nil && result.Allocated > 0 {
		for _, r := range result.Results {
			if r.Status == "allocated" && r.CertNumber != "" {
				s.pricingQueue.Enqueue(r.CertNumber)
			}
		}
	}

	// Kick off background batch cert→card_id resolution if resolver is available
	if s.cardIDResolver != nil && result.Allocated > 0 {
		certs := collectAllocatedCerts(result.Results)
		if len(certs) > 0 {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				s.inv.BatchResolveCardIDs(ctx, certs)
			}()
		}
	}
}
