package arbitrage

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// GetExpectedValues returns expected values for all unsold purchases in a campaign.
func (s *service) GetExpectedValues(ctx context.Context, campaignID string) (*EVPortfolio, error) {
	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	data, err := s.analytics.GetPurchasesWithSales(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	gradePerf := inventory.ComputeSegmentPerformance(data, "grade", func(d inventory.PurchaseWithSale) string {
		if d.Purchase.GradeValue <= 0 {
			return ""
		}
		return fmt.Sprintf("PSA %g", d.Purchase.GradeValue)
	})

	gradeMap := make(map[string]inventory.SegmentPerformance)
	for _, seg := range gradePerf {
		gradeMap[seg.Label] = seg
	}

	unsold, err := s.purchases.ListUnsoldPurchases(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	portfolio := &EVPortfolio{}
	minDP := 999

	for _, p := range unsold {
		gradeKey := fmt.Sprintf("PSA %g", p.GradeValue)
		seg, ok := gradeMap[gradeKey]
		if !ok {
			continue
		}

		costBasis := p.BuyCostCents + p.PSASourcingFeeCents

		liquidityFactor := 1.0
		trendAdj := 0.0
		if s.priceProv != nil {
			card := p.ToCardIdentity()
			if snapshot, snapErr := s.priceProv.GetMarketSnapshot(ctx, card, p.GradeValue); snapErr == nil && snapshot != nil {
				trendAdj = snapshot.Trend30d
				if snapshot.SalesLast30d > 0 && seg.AvgDaysToSell > 0 {
					expectedMonthly := 30.0 / seg.AvgDaysToSell
					if expectedMonthly > 0 {
						liquidityFactor = float64(snapshot.SalesLast30d) / expectedMonthly
					}
				}
			}
		}

		feePct := inventory.EffectiveFeePct(campaign)
		ev := computeExpectedValue(EVInput{
			CardName:               p.CardName,
			CertNumber:             p.CertNumber,
			Grade:                  p.GradeValue,
			CostBasis:              costBasis,
			SegmentSellThrough:     seg.SellThroughPct,
			SegmentMedianMarginPct: seg.AvgMarginPct,
			LiquidityFactor:        liquidityFactor,
			TrendAdjustment:        trendAdj,
			AvgDaysUnsold:          seg.AvgDaysToSell,
			AnnualCapitalCostRate:  0.05,
			DataPoints:             seg.SoldCount,
			FeePct:                 feePct,
		})

		portfolio.Items = append(portfolio.Items, *ev)
		portfolio.TotalEVCents += ev.EVCents
		if ev.EVCents >= 0 {
			portfolio.PositiveCount++
		} else {
			portfolio.NegativeCount++
		}
		if seg.SoldCount < minDP {
			minDP = seg.SoldCount
		}
	}

	if minDP < 999 {
		portfolio.MinDataPoints = minDP
	}

	return portfolio, nil
}

// EvaluatePurchase evaluates a prospective purchase's expected value.
func (s *service) EvaluatePurchase(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*ExpectedValue, error) {
	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	data, err := s.analytics.GetPurchasesWithSales(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	gradeKey := fmt.Sprintf("PSA %g", grade)
	gradePerf := inventory.ComputeSegmentPerformance(data, "grade", func(d inventory.PurchaseWithSale) string {
		if d.Purchase.GradeValue <= 0 {
			return ""
		}
		return fmt.Sprintf("PSA %g", d.Purchase.GradeValue)
	})

	var seg *inventory.SegmentPerformance
	for i := range gradePerf {
		if gradePerf[i].Label == gradeKey {
			seg = &gradePerf[i]
			break
		}
	}

	if seg == nil {
		feePct := inventory.EffectiveFeePct(campaign)
		return computeExpectedValue(EVInput{
			CardName:              cardName,
			Grade:                 grade,
			CostBasis:             buyCostCents + campaign.PSASourcingFeeCents,
			SegmentSellThrough:    0.5,
			LiquidityFactor:       1.0,
			AvgDaysUnsold:         30,
			AnnualCapitalCostRate: 0.05,
			FeePct:                feePct,
		}), nil
	}

	costBasis := buyCostCents + campaign.PSASourcingFeeCents
	feePct := inventory.EffectiveFeePct(campaign)

	return computeExpectedValue(EVInput{
		CardName:               cardName,
		Grade:                  grade,
		CostBasis:              costBasis,
		SegmentSellThrough:     seg.SellThroughPct,
		SegmentMedianMarginPct: seg.AvgMarginPct,
		LiquidityFactor:        1.0,
		AvgDaysUnsold:          seg.AvgDaysToSell,
		AnnualCapitalCostRate:  0.05,
		DataPoints:             seg.SoldCount,
		FeePct:                 feePct,
	}), nil
}

// RunProjection runs a Monte Carlo simulation projection for a campaign.
func (s *service) RunProjection(ctx context.Context, campaignID string) (*MonteCarloComparison, error) {
	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	data, err := s.analytics.GetPurchasesWithSales(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	if s.projCache != nil {
		soldCount := 0
		for _, d := range data {
			if d.Sale != nil {
				soldCount++
			}
		}
		key := projectionCacheKey{campaignID: campaignID, purchaseCount: len(data), soldCount: soldCount}
		if cached, ok := s.projCache.get(key); ok {
			// Return a deep copy to prevent callers from mutating cached state.
			// MonteCarloResult is scalar-only, so copying the Scenarios slice is sufficient.
			return copyMonteCarloComparison(cached), nil
		}
		result := RunMonteCarloProjection(campaign, data)
		s.projCache.set(key, result)
		return result, nil
	}

	return RunMonteCarloProjection(campaign, data), nil
}
