package inventory

import (
	"context"
	"fmt"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// crackCandidatesForCampaign computes crack candidates using an already-loaded campaign,
// avoiding a redundant GetCampaign call when the caller already has the campaign.
func (s *service) crackCandidatesForCampaign(ctx context.Context, campaign *Campaign) ([]CrackAnalysis, error) {
	unsold, err := s.purchases.ListUnsoldPurchases(ctx, campaign.ID)
	if err != nil {
		return nil, err
	}

	ebayFee := campaign.EbayFeePct
	if ebayFee == 0 {
		ebayFee = DefaultMarketplaceFeePct
	}

	var results []CrackAnalysis
	for _, p := range unsold {
		if p.GradeValue > 8 {
			continue
		}

		card := p.ToCardIdentity()

		rawCents := 0
		gradedCents := 0
		if s.priceProv != nil {
			if v, err := s.priceProv.GetLastSoldCents(ctx, card, 0); err != nil {
				if s.logger != nil {
					s.logger.Debug(ctx, "price lookup failed for crack candidate",
						observability.String("purchaseID", p.ID),
						observability.String("cardName", p.CardName),
						observability.Err(err))
				}
			} else {
				rawCents = v
			}
			if v, err := s.priceProv.GetLastSoldCents(ctx, card, p.GradeValue); err != nil {
				if s.logger != nil {
					s.logger.Debug(ctx, "graded price lookup failed for crack candidate",
						observability.String("purchaseID", p.ID),
						observability.String("cardName", p.CardName),
						observability.Err(err))
				}
			} else {
				gradedCents = v
			}
		}

		if rawCents == 0 {
			continue
		}
		if gradedCents == 0 {
			gradedCents = p.CLValueCents
		}

		analysis := computeCrackAnalysis(
			p.ID, campaign.ID, p.CardName, p.CertNumber, p.GradeValue,
			p.BuyCostCents, p.PSASourcingFeeCents, rawCents, gradedCents,
			ebayFee,
		)
		results = append(results, *analysis)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CrackAdvantage > results[j].CrackAdvantage
	})

	return results, nil
}

// computeCrackOpportunitiesLive computes crack opportunities by making live API calls.
// This is called ONLY by the background cache worker — never during page loads.
func (s *service) computeCrackOpportunitiesLive(ctx context.Context) ([]CrackAnalysis, error) {
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	var allResults []CrackAnalysis
	for _, campaign := range allCampaigns {
		results, err := s.crackCandidatesForCampaign(ctx, &campaign)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "crack candidates failed for campaign",
					observability.String("campaignId", campaign.ID),
					observability.Err(err))
			}
			continue
		}
		allResults = append(allResults, results...)
	}
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].CrackAdvantage > allResults[j].CrackAdvantage
	})
	return allResults, nil
}
