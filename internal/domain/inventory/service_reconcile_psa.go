package inventory

import (
	"context"
	"errors"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ReconcileResult counts the outcomes of one reconciliation pass.
type ReconcileResult struct {
	Agreed      int // PSA agrees with our current attribution
	Moved       int // campaign corrected from PSA
	SoldSkipped int // PSA disagrees but the purchase has a linked sale
	Unresolved  int // PSA named a campaign we cannot resolve
	Failed      int // per-row error; logged and skipped
}

// ReconcilePSAAttribution corrects existing purchases against PSA's own campaign
// attribution. PSA is authoritative: where it resolves, it wins.
//
// Sold purchases are skipped rather than corrected. Sales freeze campaign-derived
// values (sale_fee_cents, channel_fee_pct_at_sale, and net_profit_cents, which
// also depends on the purchase's psa_sourcing_fee_cents) and analytics reads the
// stored net_profit_cents rather than recomputing it. Moving a sold purchase's
// campaign without repairing all of that would leave the ledger inconsistent.
func (s *service) ReconcilePSAAttribution(ctx context.Context, rows []PSAExportRow) (ReconcileResult, error) {
	var res ReconcileResult
	if s.psaResolver == nil {
		return res, nil
	}

	certs := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.CertNumber != "" {
			certs = append(certs, r.CertNumber)
		}
	}
	purchases, err := s.purchases.GetPurchasesByGraderAndCertNumbers(ctx, "PSA", certs)
	if err != nil {
		return res, err
	}

	for _, row := range rows {
		p := purchases[row.CertNumber]
		if p == nil {
			continue // cert unknown to us; not eligible for reconciliation
		}
		if row.PSACampaignName == "" {
			continue
		}

		campaignID, ok, rErr := s.psaResolver.ResolveCampaignID(ctx, row.PSACampaignName)
		if rErr != nil {
			// A lookup that could not run at all is a hard stop: continuing would
			// mark every remaining row unresolved and enqueue spurious pending items.
			return res, rErr
		}

		if !ok {
			res.Unresolved++
			if err := s.recordUnresolvedAttribution(ctx, p, row.PSACampaignName); err != nil {
				res.Failed++
				s.logReconcileFailure(ctx, row.CertNumber, err)
			}
			continue
		}

		if p.CampaignID == campaignID {
			res.Agreed++
			if err := s.purchases.UpdatePurchaseAttributionName(ctx, p.ID, row.PSACampaignName, AttributionSourcePSA); err != nil {
				res.Failed++
				s.logReconcileFailure(ctx, row.CertNumber, err)
			}
			s.resolveStalePendingItem(ctx, row.CertNumber, campaignID)
			continue
		}

		campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
		if err != nil || campaign == nil {
			res.Failed++
			s.logReconcileFailure(ctx, row.CertNumber, err)
			continue
		}

		err = s.purchases.ReattributePurchase(ctx, p.ID, Reattribution{
			CampaignID:             campaignID,
			PSACampaignName:        row.PSACampaignName,
			PSASourcingFeeCents:    campaign.PSASourcingFeeCents,
			CLConfidenceAtPurchase: clConfidenceForReattribution(p.PurchaseDate, campaign),
		})
		switch {
		case errors.Is(err, ErrPurchaseHasSale):
			res.SoldSkipped++
			if nErr := s.purchases.UpdatePurchaseAttributionName(ctx, p.ID, row.PSACampaignName, p.AttributionSource); nErr != nil {
				res.Failed++
				s.logReconcileFailure(ctx, row.CertNumber, nErr)
			}
			s.logSoldSkip(ctx, row.CertNumber, row.PSACampaignName)
		case err != nil:
			res.Failed++
			s.logReconcileFailure(ctx, row.CertNumber, err)
		default:
			res.Moved++
			s.resolveStalePendingItem(ctx, row.CertNumber, campaignID)
		}
	}

	// s.logger is optional — NewService installs no default (service.go:337), and
	// every other file in this package guards it. An unguarded call panics in any
	// service constructed without WithLogger, which includes most domain tests.
	if s.logger != nil {
		s.logger.Info(ctx, "PSA attribution reconciled",
			observability.Int("agreed", res.Agreed),
			observability.Int("moved", res.Moved),
			observability.Int("soldSkipped", res.SoldSkipped),
			observability.Int("unresolved", res.Unresolved),
			observability.Int("failed", res.Failed))
	}
	return res, nil
}

// clConfidenceForReattribution returns the campaign's current CL confidence only
// when it is provably the value that was in force at purchase time.
//
// The campaigns table carries no parameter history — only updated_at, which bumps
// on any write. The one sound predicate is "the campaign has not been written at
// all since the purchase". Everything else returns nil (stored as NULL): the 8/15
// buy-terms experiment turns on this column, and a fabricated anachronistic value
// corrupts it silently, whereas a NULL is visible.
func clConfidenceForReattribution(purchaseDate string, campaign *Campaign) *int {
	d, err := time.Parse("2006-01-02", purchaseDate)
	if err != nil {
		return nil
	}
	// Compare against end-of-day so a same-day write does not falsely disqualify.
	if campaign.UpdatedAt.After(d.AddDate(0, 0, 1)) {
		return nil
	}
	c, ok := ParseCLConfidenceMin(campaign.CLConfidence)
	if !ok {
		return nil
	}
	return &c
}

func (s *service) recordUnresolvedAttribution(ctx context.Context, p *Purchase, psaName string) error {
	// Keep the inferred campaign; record PSA's name so a portal-side deletion
	// never loses it again.
	if err := s.purchases.UpdatePurchaseAttributionName(ctx, p.ID, psaName, AttributionSourceInferred); err != nil {
		return err
	}
	return s.enqueueUnresolvedPendingItem(ctx, p, psaName)
}

func (s *service) logReconcileFailure(ctx context.Context, cert string, err error) {
	if s.logger == nil {
		return
	}
	s.logger.Error(ctx, "PSA reconcile: row failed",
		observability.String("certNumber", cert), observability.Err(err))
}

func (s *service) logSoldSkip(ctx context.Context, cert, psaName string) {
	if s.logger == nil {
		return
	}
	s.logger.Info(ctx, "PSA reconcile: skipped sold purchase",
		observability.String("certNumber", cert),
		observability.String("psaCampaign", psaName))
}

// enqueueUnresolvedPendingItem records a work-queue entry for a purchase whose
// PSA campaign name did not resolve. PendingItem has no field for a PSA campaign
// name, so the unresolvable name goes in Candidates — the authoritative record
// is the psa_campaign_name column just written on the purchase; this is only
// the operator work queue. SavePendingItems upserts by cert_number and skips
// already-resolved items, so re-running a harvest is idempotent.
func (s *service) enqueueUnresolvedPendingItem(ctx context.Context, p *Purchase, psaName string) error {
	if s.pendingItemRepo == nil {
		return nil
	}
	item := PendingItem{
		ID:           s.idGen(),
		CertNumber:   p.CertNumber,
		CardName:     p.CardName,
		SetName:      p.SetName,
		CardNumber:   p.CardNumber,
		Grade:        p.GradeValue,
		BuyCostCents: p.BuyCostCents,
		PurchaseDate: p.PurchaseDate,
		Status:       "unmatched",
		Candidates:   []string{psaName},
		Source:       "scheduler",
	}
	return s.pendingItemRepo.SavePendingItems(ctx, []PendingItem{item})
}

// resolveStalePendingItem clears any pending item left over from a previous
// unresolved reconciliation of this cert, now that PSA's name resolves. It is
// best-effort: failures are logged, never propagated, since the purchase's own
// attribution has already been corrected by the time this runs.
func (s *service) resolveStalePendingItem(ctx context.Context, cert, campaignID string) {
	if s.pendingItemRepo == nil {
		return
	}
	items, err := s.pendingItemRepo.ListPendingItems(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "PSA reconcile: failed to list pending items",
				observability.String("certNumber", cert), observability.Err(err))
		}
		return
	}
	for _, item := range items {
		if item.CertNumber != cert {
			continue
		}
		if err := s.pendingItemRepo.ResolvePendingItem(ctx, item.ID, campaignID); err != nil && s.logger != nil {
			s.logger.Warn(ctx, "PSA reconcile: failed to resolve pending item",
				observability.String("certNumber", cert), observability.Err(err))
		}
		return
	}
}
