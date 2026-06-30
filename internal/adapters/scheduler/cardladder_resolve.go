package scheduler

import (
	"context"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/constants"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// resolveGemRate returns the (gemRateID, condition) pair for a purchase, using
// the cached cl_card_mappings entry when present and resolving fresh via
// BuildCollectionCard otherwise. On a fresh resolve it also persists the
// mapping, gemRateID, CL card metadata, and (when the local set_name is
// generic) the real set name from CL. Returns ok=false when the cert can't be
// resolved; the per-purchase failure reason is recorded for the admin UI.
// quotaHit is true when the resolve failed because CL's daily request quota was
// exhausted — the caller should stop resolving further certs for this cycle.
func (s *CardLadderRefreshScheduler) resolveGemRate(
	ctx context.Context,
	client *cardladder.Client,
	p *inventory.Purchase,
	mappingByCert map[string]postgres.CLCardMapping,
) (gemRateID, condition string, ok, quotaHit bool) {
	// Bypass the cache when set_name is generic so BuildCollectionCard fires
	// and the repair block below (which needs resp.Set) can run. Without this,
	// rows with pre-existing cached mappings (typical for certs whose previous
	// CL resolution only populated gemRateID+condition) would keep their
	// generic set_name forever.
	if m, cached := mappingByCert[p.CertNumber]; cached && m.CLGemRateID != "" && m.CLCondition != "" && !constants.IsGenericSetName(p.SetName) {
		return m.CLGemRateID, m.CLCondition, true, false
	}

	grader := strings.ToLower(p.Grader)
	if grader == "" {
		grader = "psa"
	}

	resp, err := client.BuildCollectionCard(ctx, p.CertNumber, grader)
	if err != nil {
		quota := apperrors.HasErrorCode(err, apperrors.ErrCodeProviderRateLimit)
		reason := CLReasonAPIError
		if quota {
			reason = CLReasonQuotaExhausted
		}
		s.logger.Warn(ctx, "CL refresh: BuildCollectionCard failed",
			observability.String("cert", p.CertNumber),
			observability.Bool("quotaExhausted", quota),
			observability.Err(err))
		s.recordCLError(ctx, p.ID, reason)
		return "", "", false, quota
	}
	if resp.GemRateID == "" || resp.Condition == "" {
		s.logger.Warn(ctx, "CL refresh: BuildCollectionCard returned no gemRateID or condition",
			observability.String("cert", p.CertNumber),
			observability.String("gemRateId", resp.GemRateID),
			observability.String("condition", resp.Condition))
		s.recordCLError(ctx, p.ID, CLReasonCertResolveFailed)
		return "", "", false, false
	}

	if err := s.store.SaveMappingPricing(ctx, p.CertNumber, resp.GemRateID, resp.Condition); err != nil {
		s.logger.Warn(ctx, "CL refresh: failed to save pricing mapping",
			observability.String("cert", p.CertNumber),
			observability.Err(err))
		// Soft failure — we can still price this run, but the mapping won't
		// be cached next run. Return the resolved values anyway.
	} else {
		mappingByCert[p.CertNumber] = postgres.CLCardMapping{
			SlabSerial:  p.CertNumber,
			CLGemRateID: resp.GemRateID,
			CLCondition: resp.Condition,
		}
	}
	if s.gemRateUpdater != nil {
		if p.GemRateID == "" {
			if err := s.gemRateUpdater.UpdatePurchaseGemRateID(ctx, p.ID, resp.GemRateID); err != nil {
				s.logger.Warn(ctx, "CL refresh: failed to persist gemRateID on purchase",
					observability.String("cert", p.CertNumber),
					observability.Err(err))
			} else {
				p.GemRateID = resp.GemRateID
			}
		}
		if resp.Player != "" || resp.Variation != "" || resp.Category != "" {
			if err := s.gemRateUpdater.UpdatePurchaseCLCardMetadata(ctx, p.ID, resp.Player, resp.Variation, resp.Category); err != nil {
				s.logger.Warn(ctx, "CL refresh: failed to persist card metadata",
					observability.String("cert", p.CertNumber),
					observability.Err(err))
			}
		}
		// Repair set_name when PSA returned a generic value (e.g. "TCG Cards"
		// for older certs). CL's Set field carries the real set for any cert
		// CL can resolve, so adopt it only when the current value is generic.
		if constants.IsGenericSetName(p.SetName) && !constants.IsGenericSetName(resp.Set) {
			if err := s.gemRateUpdater.UpdatePurchaseSetName(ctx, p.ID, resp.Set); err != nil {
				s.logger.Warn(ctx, "CL refresh: failed to persist set name from CL",
					observability.String("cert", p.CertNumber),
					observability.String("clSet", resp.Set),
					observability.Err(err))
			} else {
				p.SetName = resp.Set
			}
		}
	}
	return resp.GemRateID, resp.Condition, true, false
}

// shouldReenrollForCLChange returns true when a CL value change should
// trigger DH push-pipeline re-enrollment. Two qualifying cases:
//  1. Already-pushed rows (DHInventoryID != 0) — re-enrolled so DH picks up
//     the new price.
//  2. Received-but-unmatched rows — re-enrolled so a fresh cert resolve is
//     attempted with the new market value, which may push it above a floor.
func shouldReenrollForCLChange(p *inventory.Purchase) bool {
	if p.DHInventoryID != 0 {
		return true
	}
	if p.ReceivedAt != nil && (p.DHPushStatus == inventory.DHPushStatusUnmatched || p.DHPushStatus == "") {
		return true
	}
	return false
}
