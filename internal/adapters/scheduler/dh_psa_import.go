package scheduler

import (
	"context"
	"strconv"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// psaImportMaxAttempts caps total psa_import attempts per purchase per cycle,
// including the original call. This is a safety ceiling — the rotator itself
// returns false once keys are exhausted, so the loop typically exits earlier.
// 8 is well above any realistic PSA_ACCESS_TOKEN count.
const psaImportMaxAttempts = 8

// DHPushPSAImporter submits PSA-graded certs via DH's psa_import endpoint.
// This is the scheduler's primary cert intake path — psa_import does PSA
// lookup, catalog match, and inventory creation in one request.
type DHPushPSAImporter interface {
	PSAImport(ctx context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error)
}

// pushViaPSAImport submits a single pending purchase to DH's psa_import
// endpoint. Rotates PSA keys on rate-limit and returns a processResult that
// matches processPurchase's expectations.
//
// Resolution handling:
//
//	matched / unmatched_created / override_corrected / already_listed
//	  → persist dh_card_id + dh_inventory_id, flip dh_push_status to matched.
//
//	psa_error with RateLimited
//	  → rotate to the next PSA key and retry the same item. If all keys are
//	    exhausted, leave pending so the next cycle can retry after rotation
//	    resets.
//
//	psa_error without RateLimited (bad cert, not found, network)
//	  → log the error from DH and leave pending.
//
//	partner_card_error
//	  → DH resolved the cert but couldn't persist the partner card (usually
//	    an invalid overrides.language / overrides.rarity). Log and leave
//	    pending so the operator can repair overrides.
func (s *DHPushScheduler) pushViaPSAImport(ctx context.Context, p inventory.Purchase) processResult {
	item := buildPSAImportItem(p)

	for range psaImportMaxAttempts {
		resp, err := s.psaImporter.PSAImport(ctx, []dh.PSAImportItem{item})
		if err != nil {
			s.logger.Warn(ctx, "dh push: psa_import api error, leaving pending",
				observability.String("purchaseID", p.ID),
				observability.String("cert", p.CertNumber),
				observability.Err(err))
			return processSkipped
		}

		// Batch-level rejection (DH returns 422 with {success:false, error:"..."}
		// when the batch itself is invalid — >50 items, missing vendor profile,
		// blank cert_number, etc.). Check before Results so we surface DH's
		// reason instead of a generic "no results" message.
		if !resp.Success {
			s.logger.Warn(ctx, "dh push: psa_import batch rejected",
				observability.String("purchaseID", p.ID),
				observability.String("cert", p.CertNumber),
				observability.String("batchError", resp.Error))
			return processSkipped
		}

		if len(resp.Results) == 0 {
			s.logger.Warn(ctx, "dh push: psa_import returned no results",
				observability.String("purchaseID", p.ID),
				observability.String("cert", p.CertNumber))
			return processSkipped
		}

		result := resp.Results[0]

		if result.RateLimited {
			rotator, ok := s.psaImporter.(dh.PSAKeyRotator)
			if ok && rotator.RotatePSAKey() {
				s.logger.Info(ctx, "dh push: psa_import rate-limited, rotating PSA key",
					observability.String("purchaseID", p.ID),
					observability.String("cert", p.CertNumber),
					observability.String("psaError", result.Error))
				continue
			}
			s.logger.Warn(ctx, "dh push: psa_import rate-limited, no PSA keys left to rotate",
				observability.String("purchaseID", p.ID),
				observability.String("cert", p.CertNumber),
				observability.String("psaError", result.Error))
			return processSkipped
		}

		switch result.Resolution {
		case dh.PSAImportStatusMatched,
			dh.PSAImportStatusUnmatchedCreated,
			dh.PSAImportStatusOverrideCorrected,
			dh.PSAImportStatusAlreadyListed:
			return s.applyPSAImportSuccess(ctx, p, result)

		case dh.PSAImportStatusPSAError:
			s.logger.Warn(ctx, "dh push: psa_import psa_error, leaving pending",
				observability.String("purchaseID", p.ID),
				observability.String("cert", p.CertNumber),
				observability.String("psaError", result.Error))
			return processSkipped

		case dh.PSAImportStatusPartnerCardError:
			s.logger.Warn(ctx, "dh push: psa_import partner_card_error (check overrides)",
				observability.String("purchaseID", p.ID),
				observability.String("cert", p.CertNumber),
				observability.String("dhError", result.Error))
			return processSkipped

		default:
			s.logger.Warn(ctx, "dh push: psa_import unknown resolution",
				observability.String("purchaseID", p.ID),
				observability.String("cert", p.CertNumber),
				observability.String("resolution", result.Resolution),
				observability.String("dhError", result.Error))
			return processSkipped
		}
	}

	s.logger.Warn(ctx, "dh push: psa_import key rotation cap reached, leaving pending",
		observability.String("purchaseID", p.ID),
		observability.String("cert", p.CertNumber))
	return processSkipped
}

// applyPSAImportSuccess persists DH IDs + status for any successful psa_import
// resolution (matched / unmatched_created / override_corrected / already_listed).
// No state event is emitted from this scheduler — dh_state_events records
// exceptional transitions (held, unmatched) here. The inline push path in the
// dhlisting service does emit TypePushed because it runs in the user-visible
// "List" flow where the extra audit trail is useful.
func (s *DHPushScheduler) applyPSAImportSuccess(ctx context.Context, p inventory.Purchase, result dh.PSAImportResult) processResult {
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
		// purchase next cycle. Log at Error; the already-pushed branch at the
		// top of processPurchase will repair the status.
		s.logger.Error(ctx, "dh push: psa_import inventory persisted but status update failed",
			observability.String("purchaseID", p.ID),
			observability.Err(updateErr))
	}

	s.saveCardIDMapping(ctx, p, result.DHCardID)

	s.logger.Info(ctx, "dh push: psa_import succeeded",
		observability.String("purchaseID", p.ID),
		observability.String("cert", p.CertNumber),
		observability.String("resolution", result.Resolution),
		observability.Int("dhCardID", result.DHCardID),
		observability.Int("dhInventoryID", result.DHInventoryID))

	return processMatchedComplete
}

// saveCardIDMapping persists a DH card ID mapping to the card_id_mappings
// table so other subsystems (price sync, CL refresh) can reuse it.
func (s *DHPushScheduler) saveCardIDMapping(ctx context.Context, p inventory.Purchase, dhCardID int) {
	externalID := strconv.Itoa(dhCardID)
	if saveErr := s.cardIDSaver.SaveExternalID(ctx, p.CardName, p.SetName, p.CardNumber, pricing.SourceDH, externalID); saveErr != nil {
		s.logger.Warn(ctx, "dh push: failed to save external ID mapping",
			observability.String("purchaseID", p.ID), observability.Err(saveErr))
	}
}

// buildPSAImportItem assembles a PSA import item from existing purchase columns.
// Overrides come from fields we already populate via PSA cert lookup and CL
// metadata. Rarity is intentionally omitted because DH rejects unknown enum
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
