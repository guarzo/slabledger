package dhlisting

import (
	"context"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// inlineMatchAndPush submits a single pending purchase to DH via
// /api/v1/enterprise/inventory/psa_import — DH does the PSA lookup, catalog
// match, and inventory creation atomically. Returns the DH inventory ID on
// success, 0 on any failure (logged). The caller skips the list transition
// when 0 is returned.
//
// Resolution handling mirrors the push scheduler:
//
//	matched / unmatched_created / override_corrected / already_listed
//	  → persist IDs, flip dh_push_status to matched, return the inventory ID.
//	psa_error (RateLimited)
//	  → legacy mode rotates PSA keys and retries. Coordinated mode returns 0
//	    after the first call: neither free text nor a rate-limit flag proves
//	    whole-call nonmutation, so the committed attempt remains OPEN.
//	psa_error (other) / partner_card_error
//	  → log the DH-supplied reason, leave dh_push_status pending, return 0.
//	unknown resolution / empty results / missing IDs
//	  → return 0.
func (s *dhListingService) inlineMatchAndPush(ctx context.Context, p *inventory.Purchase) int {
	if p.CertNumber == "" {
		s.logger.Warn(ctx, "inline dh psa_import: purchase has no cert number",
			observability.String("purchaseID", p.ID))
		return 0
	}

	item := DHPSAImportItem{
		CertNumber:     p.CertNumber,
		CostBasisCents: p.BuyCostCents,
		CardName:       p.CardName,
		SetName:        p.SetName,
		CardNumber:     p.CardNumber,
		Year:           p.CardYear,
		Language:       InferDHLanguage(p.SetName, p.CardName),
	}

	// Legacy rotation is capped at 8. Configured mode exits on the first
	// incomplete result, preserving uncertainty instead of redispatching.
	const psaImportMaxAttempts = 8
	for range psaImportMaxAttempts {
		results, err := s.psaImporter.PSAImport(ctx, []DHPSAImportItem{item})
		if err != nil {
			s.logger.Warn(ctx, "inline dh psa_import api error",
				observability.String("cert", p.CertNumber), observability.Err(err))
			return 0
		}
		if len(results) == 0 {
			s.logger.Warn(ctx, "inline dh psa_import returned no results",
				observability.String("cert", p.CertNumber))
			return 0
		}
		r := results[0]

		if r.RateLimited || isPSARateLimitMessage(r.Error) {
			if s.mutationRequired {
				s.logger.Warn(ctx, "inline dh psa_import rate-limited — coordinated attempt remains fenced",
					observability.String("cert", p.CertNumber),
					observability.String("psaError", r.Error))
				return 0
			}
			if rotator, ok := s.psaImporter.(PSAKeyRotator); ok && rotator.RotatePSAKey() {
				s.logger.Info(ctx, "inline dh psa_import rate-limited, rotating PSA key",
					observability.String("cert", p.CertNumber),
					observability.String("psaError", r.Error))
				continue
			}
			s.logger.Warn(ctx, "inline dh psa_import rate-limited, no more PSA keys",
				observability.String("cert", p.CertNumber),
				observability.String("psaError", r.Error))
			return 0
		}

		if !IsPSAImportSuccess(r.Resolution) {
			s.logger.Warn(ctx, "inline dh psa_import did not create inventory",
				observability.String("cert", p.CertNumber),
				observability.String("resolution", r.Resolution),
				observability.String("dhError", r.Error))
			return 0
		}

		if r.DHCardID == 0 || r.DHInventoryID == 0 || (s.mutationRequired && r.CertNumber != p.CertNumber) {
			s.logger.Warn(ctx, "inline dh psa_import success missing IDs",
				observability.String("cert", p.CertNumber),
				observability.String("resolution", r.Resolution),
				observability.Int("dhCardID", r.DHCardID),
				observability.Int("dhInventoryID", r.DHInventoryID))
			return 0
		}

		return s.persistInlinePSAImport(ctx, p, r)
	}

	return 0
}

// persistInlinePSAImport saves the DH card/inventory IDs, flips dh_push_status
// to matched, and emits a pushed event. Returns the DH inventory ID on full
// success, 0 if DH-fields persistence failed (caller skips listing).
func (s *dhListingService) persistInlinePSAImport(ctx context.Context, p *inventory.Purchase, r DHPSAImportResult) int {
	if s.cardIDSaver != nil {
		externalID := strconv.Itoa(r.DHCardID)
		if err := s.cardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, SourceDH, externalID); err != nil {
			s.logger.Error(ctx, "inline dh psa_import: failed to save card mapping",
				observability.String("cert", p.CertNumber),
				observability.String("cardName", p.CardName),
				observability.Err(err))
			if s.mutationRequired {
				return 0
			}
		}
	}

	// Map DH's psa_import status to the value to persist; see
	// inventory.DHStatusForPush for why "skipped" must not become in_stock.
	dhStatus := inventory.DHStatusForPush(r.DHStatus)

	if s.fieldsUpdater != nil {
		if err := s.fieldsUpdater.UpdatePurchaseDHFields(ctx, p.ID, inventory.DHFieldsUpdate{
			CardID:      r.DHCardID,
			InventoryID: r.DHInventoryID,
			CertStatus:  DHCertStatusMatched,
			DHStatus:    dhStatus,
		}); err != nil {
			s.logger.Error(ctx, "inline dh psa_import: failed to persist DH fields — returning 0 to prevent duplicate push",
				observability.String("cert", p.CertNumber), observability.Err(err))
			return 0
		}
	}

	if s.pushStatusUpdater != nil {
		if err := s.pushStatusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, inventory.DHPushStatusMatched); err != nil {
			if s.mutationRequired {
				// Execution rolls back these local writes; the precommitted OPEN
				// attempt blocks the scheduler. Provider-completion evidence or
				// approved reconciliation is required, not another push cycle.
				s.logger.Error(ctx, "inline dh psa_import: failed to set matched status — coordinated attempt needs provider evidence or approved reconciliation",
					observability.String("cert", p.CertNumber), observability.Err(err))
				return 0
			}
			// Legacy fields/ID writes are already committed. Its scheduler can
			// repair the pending push status via the existing inventory-ID guard.
			s.logger.Error(ctx, "inline dh psa_import: failed to set matched status — legacy scheduler will repair next cycle",
				observability.String("cert", p.CertNumber), observability.Err(err))
		}
	}

	p.DHCardID = r.DHCardID
	p.DHInventoryID = r.DHInventoryID

	s.recordEvent(ctx, dhevents.Event{
		PurchaseID:    p.ID,
		CertNumber:    p.CertNumber,
		Type:          dhevents.TypePushed,
		NewPushStatus: string(inventory.DHPushStatusMatched),
		NewDHStatus:   dhStatus,
		DHInventoryID: r.DHInventoryID,
		DHCardID:      r.DHCardID,
		Source:        dhevents.SourceDHListing,
		Notes:         r.Resolution,
	})

	return r.DHInventoryID
}

// isPSARateLimitMessage detects PSA rate-limit reasons in DH's per-cert
// result.error field. DH does not always set rate_limited=true even when the
// reason is a rate limit ("Daily PSA API limit reached" arrives with
// resolution=psa_error and rate_limited=false). Legacy mode uses this to rotate;
// coordinated mode treats it as incomplete evidence, never as retry permission.
// Mirrors dh.IsPSARateLimitMessage in the adapter; duplicated here because
// hexagonal rules forbid the domain importing adapter packages.
func isPSARateLimitMessage(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "psa api rate limit") ||
		strings.Contains(lower, "daily limit reached") ||
		strings.Contains(lower, "daily psa api limit")
}
