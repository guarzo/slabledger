package scheduler

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DHPushPSAImporter submits PSA-graded certs via DH's psa_import endpoint.
// Used as a fallback when standard cert resolve can't match a catalog card —
// typically for off-catalog items like Japanese promos or non-English WOTC.
// When no importer is wired, the push pipeline falls back to markUnmatched
// (pre-psa_import behavior).
type DHPushPSAImporter interface {
	PSAImport(ctx context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error)
}

// WithDHPushPSAImporter enables fallback PSA import when the standard cert
// resolve path can't match a catalog card.
func WithDHPushPSAImporter(imp DHPushPSAImporter) DHPushOption {
	return func(s *DHPushScheduler) { s.psaImporter = imp }
}

// tryPSAImportOrUnmatch attempts to submit the purchase via DH's psa_import
// endpoint when standard cert resolve couldn't match a catalog card. On
// success the DH card/inventory IDs are persisted inline and the purchase is
// marked matched. On failure (or when no importer is configured), falls back
// to markUnmatched.
//
// Returns processMatchedComplete when PSA import succeeded and inventory was
// persisted (outer processPurchase skips the normal PushInventory step).
// Returns processUnmatched or processSkipped otherwise — matching the
// existing contract of the old markUnmatched call sites.
func (s *DHPushScheduler) tryPSAImportOrUnmatch(ctx context.Context, p inventory.Purchase, mappedSet map[string]string, unmatchReason string) processResult {
	if s.psaImporter == nil {
		return s.finalizeUnmatched(ctx, p, unmatchReason)
	}

	item := buildPSAImportItem(p)
	resp, err := s.psaImporter.PSAImport(ctx, []dh.PSAImportItem{item})
	if err != nil {
		s.logger.Warn(ctx, "dh push: psa_import API error, leaving as pending",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber),
			observability.Err(err))
		return processSkipped
	}

	if len(resp.Results) == 0 {
		s.logger.Warn(ctx, "dh push: psa_import returned no results",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber))
		return s.finalizeUnmatched(ctx, p, "psa_import: empty results")
	}

	result := resp.Results[0]
	switch result.Resolution {
	case dh.PSAImportStatusMatched, dh.PSAImportStatusUnmatchedCreated:
		return s.applyPSAImportSuccess(ctx, p, result, mappedSet)
	default:
		reason := "psa_import " + result.Resolution
		if result.Error != "" {
			reason += ": " + result.Error
		}
		s.logger.Info(ctx, "dh push: psa_import did not create inventory",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber),
			observability.String("resolution", result.Resolution),
			observability.String("error", result.Error))
		return s.finalizeUnmatched(ctx, p, reason)
	}
}

// applyPSAImportSuccess persists DH IDs + status for a successful psa_import
// result. No state event is emitted, matching the existing catalog-match
// happy path in processPurchase — dh_state_events only records exceptional
// transitions (held, unmatched), not routine matches.
func (s *DHPushScheduler) applyPSAImportSuccess(ctx context.Context, p inventory.Purchase, result dh.PSAImportResult, mappedSet map[string]string) processResult {
	if result.DHCardID == 0 || result.DHInventoryID == 0 {
		s.logger.Warn(ctx, "dh push: psa_import success missing IDs, treating as skip",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber),
			observability.String("resolution", result.Resolution),
			observability.Int("dhCardID", result.DHCardID),
			observability.Int("dhInventoryID", result.DHInventoryID))
		return processSkipped
	}

	dhStatus := result.Status
	if dhStatus == "" {
		dhStatus = dh.InventoryStatusInStock
	}

	update := inventory.DHFieldsUpdate{
		CardID:      result.DHCardID,
		InventoryID: result.DHInventoryID,
		CertStatus:  dh.CertStatusMatched,
		DHStatus:    inventory.DHStatus(dhStatus),
	}
	if updateErr := s.fieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, update); updateErr != nil {
		s.logger.Warn(ctx, "dh push: failed to update DH fields after psa_import",
			observability.String("purchaseID", p.ID),
			observability.Err(updateErr))
		return processSkipped
	}

	if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, inventory.DHPushStatusMatched); updateErr != nil {
		// Fields are saved with the inventory ID, so NeedsDHPush will skip this
		// purchase next cycle. Log at Error; status will be repaired by the
		// already-pushed branch at the top of processPurchase.
		s.logger.Error(ctx, "dh push: psa_import inventory persisted but status update failed",
			observability.String("purchaseID", p.ID),
			observability.Err(updateErr))
	}

	s.saveCardIDMapping(ctx, p, result.DHCardID, mappedSet)

	s.logger.Info(ctx, "dh push: psa_import succeeded",
		observability.String("purchaseID", p.ID),
		observability.String("cert", p.CertNumber),
		observability.String("resolution", result.Resolution),
		observability.Int("dhCardID", result.DHCardID),
		observability.Int("dhInventoryID", result.DHInventoryID))

	return processMatchedComplete
}

// finalizeUnmatched wraps markUnmatched with processResult mapping so the
// fallback path stays in sync with the old call-site contract.
func (s *DHPushScheduler) finalizeUnmatched(ctx context.Context, p inventory.Purchase, reason string) processResult {
	if !s.markUnmatched(ctx, p, reason) {
		return processSkipped
	}
	return processUnmatched
}

// buildPSAImportItem assembles a PSA import item from existing purchase columns.
// Overrides come from fields we already populate via PSA cert lookup and CL
// metadata — the only computed field is language, which is inferred from
// set_name. Rarity is intentionally omitted because DH rejects unknown enum
// values as partner_card_error; we let DH's PSA-derived rarity win.
func buildPSAImportItem(p inventory.Purchase) dh.PSAImportItem {
	overrides := &dh.PSAImportOverrides{
		Name:       p.CardName,
		SetName:    p.SetName,
		CardNumber: p.CardNumber,
		Year:       p.CardYear,
	}
	if lang := dhlisting.InferDHLanguage(p.SetName, p.CardName); lang != "" {
		overrides.Language = lang
	}
	return dh.PSAImportItem{
		CertNumber:     p.CertNumber,
		CostBasisCents: p.BuyCostCents,
		Status:         dh.InventoryStatusInStock,
		Overrides:      overrides,
	}
}
