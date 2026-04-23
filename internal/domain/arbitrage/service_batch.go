package arbitrage

import (
	"context"
	"fmt"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

type cardKey struct{ name, set, number string }

func buildEbayFeeMap(campaigns []inventory.Campaign) map[string]float64 {
	m := make(map[string]float64, len(campaigns))
	for _, c := range campaigns {
		m[c.ID] = inventory.EffectiveFeePct(&c)
	}
	return m
}

type resolveResult struct {
	cardToDHID map[cardKey]int
	dhCardIDs  []int
}

// resolveCardDHIDs maps unique cards from unsold purchases to DH card IDs via
// the batch pricer. skipHighGrades filters out PSA 9+ (used by crack, not acquisition).
func (s *service) resolveCardDHIDs(
	ctx context.Context,
	unsold []inventory.Purchase,
	ebayFeeMap map[string]float64,
	skipHighGrades bool,
) resolveResult {
	cardToDHID := make(map[cardKey]int)
	var dhCardIDs []int
	for _, p := range unsold {
		if skipHighGrades && p.GradeValue >= 9 {
			continue
		}
		if _, ok := ebayFeeMap[p.CampaignID]; !ok {
			continue
		}
		key := cardKey{p.CardName, p.SetName, p.CardNumber}
		if _, seen := cardToDHID[key]; seen {
			continue
		}
		dhID, err := s.batchPricer.ResolveDHCardID(ctx, p.CardName, p.SetName, p.CardNumber)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "batch: resolve failed",
					observability.String("cardName", p.CardName), observability.Err(err))
			}
			cardToDHID[key] = 0
			continue
		}
		cardToDHID[key] = dhID
		if dhID > 0 {
			dhCardIDs = append(dhCardIDs, dhID)
		}
	}
	return resolveResult{cardToDHID: cardToDHID, dhCardIDs: dhCardIDs}
}

// getCrackOpportunitiesLegacy is the original per-card price lookup path.
// Uses a single ListAllUnsoldPurchases call to avoid N+1 DB queries.
func (s *service) getCrackOpportunitiesLegacy(ctx context.Context) ([]CrackAnalysis, error) {
	if s.priceProv == nil {
		if s.logger != nil {
			s.logger.Info(ctx, "skipping crack opportunities",
				observability.String("reason", "price provider not configured"))
		}
		return []CrackAnalysis{}, nil
	}

	priceProv := s.requestScopedPriceProv()

	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}

	ebayFeeMap := buildEbayFeeMap(allCampaigns)

	allUnsold, err := s.purchases.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all unsold purchases: %w", err)
	}

	var results []CrackAnalysis
	for _, p := range allUnsold {
		if p.GradeValue >= 9 {
			continue
		}
		ebayFee, ok := ebayFeeMap[p.CampaignID]
		if !ok {
			if s.logger != nil {
				s.logger.Debug(ctx, "crack analysis: skipping purchase — campaign not active",
					observability.String("purchaseID", p.ID),
					observability.String("campaignID", p.CampaignID))
			}
			continue
		}
		card := p.ToCardIdentity()

		rawCents := 0
		gradedCents := 0
		if v, err := priceProv.GetLastSoldCents(ctx, card, 0); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "crack analysis: raw price lookup failed",
					observability.String("cardName", p.CardName),
					observability.Err(err))
			}
		} else {
			rawCents = v
		}
		if v, err := priceProv.GetLastSoldCents(ctx, card, p.GradeValue); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "crack analysis: graded price lookup failed",
					observability.String("cardName", p.CardName),
					observability.Float64("grade", p.GradeValue),
					observability.Err(err))
			}
		} else {
			gradedCents = v
		}

		if rawCents == 0 {
			continue
		}
		if gradedCents == 0 {
			gradedCents = p.CLValueCents
		}

		analysis := ComputeCrackAnalysis(
			p.ID, p.CampaignID, p.CardName, p.CertNumber, p.GradeValue,
			p.BuyCostCents, p.PSASourcingFeeCents, rawCents, gradedCents,
			ebayFee,
		)
		if analysis.CrackAdvantage > 0 {
			results = append(results, *analysis)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CrackAdvantage > results[j].CrackAdvantage
	})
	return results, nil
}

// getCrackOpportunitiesBatch resolves all card IDs upfront and calls BatchPriceDistribution
// (2-3 HTTP calls) instead of per-card GetLastSoldCents (~400+ calls).
func (s *service) getCrackOpportunitiesBatch(ctx context.Context) ([]CrackAnalysis, error) {
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	ebayFeeMap := buildEbayFeeMap(allCampaigns)

	allUnsold, err := s.purchases.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all unsold purchases: %w", err)
	}

	resolved := s.resolveCardDHIDs(ctx, allUnsold, ebayFeeMap, true)

	distributions, err := s.batchPricer.BatchPriceDistribution(ctx, resolved.dhCardIDs)
	if err != nil {
		return nil, fmt.Errorf("batch price distribution: %w", err)
	}

	var results []CrackAnalysis
	for _, p := range allUnsold {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if p.GradeValue >= 9 {
			continue
		}
		ebayFee, ok := ebayFeeMap[p.CampaignID]
		if !ok {
			continue
		}
		key := cardKey{p.CardName, p.SetName, p.CardNumber}
		dhID := resolved.cardToDHID[key]
		if dhID == 0 {
			continue
		}
		dist, ok := distributions[dhID]
		if !ok {
			continue
		}

		rawBucket := dist.ByGrade["Raw"]
		gradedBucket := dist.ByGrade[gradeKeyForValue(p.GradeValue)]

		rawCents := rawBucket.MedianCents
		if rawCents == 0 {
			continue
		}
		gradedCents := gradedBucket.MedianCents
		if gradedCents == 0 {
			gradedCents = p.CLValueCents
		}

		analysis := ComputeCrackAnalysis(
			p.ID, p.CampaignID, p.CardName, p.CertNumber, p.GradeValue,
			p.BuyCostCents, p.PSASourcingFeeCents, rawCents, gradedCents,
			ebayFee,
		)
		if analysis.CrackAdvantage > 0 {
			results = append(results, *analysis)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CrackAdvantage > results[j].CrackAdvantage
	})
	return results, nil
}

// getAcquisitionTargetsLegacy is the original per-card price lookup path.
func (s *service) getAcquisitionTargetsLegacy(ctx context.Context) ([]AcquisitionOpportunity, error) {
	if s.priceProv == nil {
		if s.logger != nil {
			s.logger.Info(ctx, "skipping acquisition targets",
				observability.String("reason", "price provider not configured"))
		}
		return []AcquisitionOpportunity{}, nil
	}

	priceProv := s.requestScopedPriceProv()

	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}

	ebayFeeMap := buildEbayFeeMap(allCampaigns)

	allUnsold, err := s.purchases.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all unsold purchases: %w", err)
	}

	opportunities := []AcquisitionOpportunity{}
	seen := make(map[string]bool)
	for _, p := range allUnsold {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		ebayFee, ok := ebayFeeMap[p.CampaignID]
		if !ok {
			if s.logger != nil {
				s.logger.Debug(ctx, "acquisition targets: skipping purchase — campaign not active",
					observability.String("purchaseID", p.ID),
					observability.String("campaignID", p.CampaignID))
			}
			continue
		}
		key := p.CardName + "|" + p.SetName + "|" + p.CardNumber
		if seen[key] {
			continue
		}
		seen[key] = true
		card := p.ToCardIdentity()
		rawNMCents := 0
		if v, err := priceProv.GetLastSoldCents(ctx, card, 0); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "acquisition targets: raw price lookup failed",
					observability.String("cardName", p.CardName),
					observability.Err(err))
			}
		} else if v > 0 {
			rawNMCents = v
		}
		if rawNMCents == 0 {
			continue
		}
		gradedEstimates := make(map[string]int)
		for _, grade := range []float64{8, 9, 10} {
			v, err := priceProv.GetLastSoldCents(ctx, card, grade)
			if err != nil {
				if s.logger != nil {
					s.logger.Warn(ctx, "acquisition targets: graded price lookup failed",
						observability.String("cardName", p.CardName),
						observability.Float64("grade", grade),
						observability.Err(err))
				}
				continue
			}
			if v > 0 {
				gradedEstimates[fmt.Sprintf("PSA %g", grade)] = v
			}
		}
		opp := computeAcquisitionOpportunity(
			p.CardName, p.SetName, p.CardNumber, p.CertNumber,
			rawNMCents, gradedEstimates, ebayFee, "inventory",
		)
		if opp != nil {
			opportunities = append(opportunities, *opp)
		}
	}
	sortAcquisitionByProfit(opportunities)
	return opportunities, nil
}

// getAcquisitionTargetsBatch resolves all card IDs upfront and calls BatchPriceDistribution
// instead of per-card GetLastSoldCents.
func (s *service) getAcquisitionTargetsBatch(ctx context.Context) ([]AcquisitionOpportunity, error) {
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	ebayFeeMap := buildEbayFeeMap(allCampaigns)

	allUnsold, err := s.purchases.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all unsold purchases: %w", err)
	}

	resolved := s.resolveCardDHIDs(ctx, allUnsold, ebayFeeMap, false)

	distributions, err := s.batchPricer.BatchPriceDistribution(ctx, resolved.dhCardIDs)
	if err != nil {
		return nil, fmt.Errorf("batch price distribution: %w", err)
	}

	seen := make(map[cardKey]bool)
	var opportunities []AcquisitionOpportunity
	for _, p := range allUnsold {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		ebayFee, ok := ebayFeeMap[p.CampaignID]
		if !ok {
			continue
		}
		key := cardKey{p.CardName, p.SetName, p.CardNumber}
		if seen[key] {
			continue
		}
		seen[key] = true

		dhID := resolved.cardToDHID[key]
		if dhID == 0 {
			continue
		}
		dist, ok := distributions[dhID]
		if !ok {
			continue
		}

		rawBucket := dist.ByGrade["Raw"]
		if rawBucket.MedianCents == 0 {
			continue
		}

		gradedEstimates := make(map[string]int)
		for _, grade := range []float64{8, 9, 10} {
			bucket := dist.ByGrade[gradeKeyForValue(grade)]
			if bucket.MedianCents > 0 {
				gradedEstimates[fmt.Sprintf("PSA %g", grade)] = bucket.MedianCents
			}
		}

		opp := computeAcquisitionOpportunity(
			p.CardName, p.SetName, p.CardNumber, p.CertNumber,
			rawBucket.MedianCents, gradedEstimates, ebayFee, "inventory",
		)
		if opp != nil {
			opportunities = append(opportunities, *opp)
		}
	}

	sortAcquisitionByProfit(opportunities)
	return opportunities, nil
}
