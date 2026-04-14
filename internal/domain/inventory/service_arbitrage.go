package inventory

import (
	"context"
	"fmt"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/constants"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// crackCandidatesForCampaign returns crack candidate purchase IDs for a campaign.
// A card is a crack candidate when its raw (ungraded) market value exceeds the
// breakeven raw price AND selling raw nets more than selling graded.
func (s *service) crackCandidatesForCampaign(ctx context.Context, campaign *Campaign) ([]string, error) {
	if s.priceProv == nil {
		if s.logger != nil {
			s.logger.Info(ctx, "skipping crack candidates",
				observability.String("reason", "price provider not configured"))
		}
		return nil, nil
	}
	unsold, err := s.purchases.ListUnsoldPurchases(ctx, campaign.ID)
	if err != nil {
		return nil, err
	}

	ebayFee := EffectiveFeePct(campaign)

	var candidates []string
	for _, p := range unsold {
		if p.GradeValue >= 9 {
			continue
		}

		card := p.ToCardIdentity()

		rawCents := 0
		gradedCents := 0
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

		if rawCents == 0 {
			continue
		}
		if gradedCents == 0 {
			gradedCents = p.CLValueCents
		}

		if isCrackCandidate(p.BuyCostCents, p.PSASourcingFeeCents, rawCents, gradedCents, ebayFee) {
			candidates = append(candidates, p.ID)
		}
	}

	return candidates, nil
}

// isCrackCandidate reports whether selling a card raw is more profitable than selling graded.
func isCrackCandidate(buyCostCents, sourcingFeeCents, rawMarketCents, gradedMarketCents int, ebayFeePct float64) bool {
	if ebayFeePct < 0 || ebayFeePct >= 1 {
		ebayFeePct = constants.DefaultMarketplaceFeePct
	}
	costBasis := buyCostCents + sourcingFeeCents
	breakevenRaw := int(float64(costBasis) / (1 - ebayFeePct))
	crackNet := rawMarketCents - int(float64(rawMarketCents)*ebayFeePct) - costBasis
	gradedNet := gradedMarketCents - int(float64(gradedMarketCents)*ebayFeePct) - costBasis
	return rawMarketCents > breakevenRaw && crackNet > gradedNet
}

// computeCrackOpportunitiesLive computes crack candidate purchase IDs by making live API calls.
// This is called ONLY by the background cache worker — never during page loads.
func (s *service) computeCrackOpportunitiesLive(ctx context.Context) ([]string, error) {
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	var allCandidates []string
	for _, campaign := range allCampaigns {
		candidates, err := s.crackCandidatesForCampaign(ctx, &campaign)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "crack candidates failed for campaign",
					observability.String("campaignId", campaign.ID),
					observability.Err(err))
			}
			continue
		}
		allCandidates = append(allCandidates, candidates...)
	}
	sort.Strings(allCandidates)
	return allCandidates, nil
}
