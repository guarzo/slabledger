package inventory

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) LookupCert(ctx context.Context, certNumber string) (*CertInfo, *MarketSnapshot, error) {
	if s.certLookup == nil {
		return nil, nil, ErrCertLookupNotConfigured
	}

	info, err := s.certLookup.LookupCert(ctx, certNumber)
	if err != nil {
		return nil, nil, fmt.Errorf("cert lookup: %w", err)
	}

	var snapshot *MarketSnapshot
	if s.priceProv != nil && info.CardName != "" && info.Grade > 0 {
		resolvedCategory := resolvePSACategory(info.Category)
		if IsGenericSetName(resolvedCategory) {
			resolvedCategory = info.Category
		}
		snapshot, err = s.priceProv.GetMarketSnapshot(ctx, CardIdentity{CardName: info.CardName, CardNumber: info.CardNumber, SetName: resolvedCategory}, info.Grade)
		if err != nil && s.logger != nil {
			s.logger.Debug(ctx, "GetMarketSnapshot for cert lookup failed",
				observability.String("card", info.CardName),
				observability.Err(err))
		}
	}

	return info, snapshot, nil
}

func (s *service) QuickAddPurchase(ctx context.Context, campaignID string, req QuickAddRequest) (*Purchase, error) {
	if s.certLookup == nil {
		return nil, ErrCertLookupNotConfigured
	}

	info, err := s.certLookup.LookupCert(ctx, req.CertNumber)
	if err != nil {
		return nil, fmt.Errorf("cert lookup: %w", err)
	}

	purchaseDate := req.PurchaseDate
	if purchaseDate == "" {
		purchaseDate = time.Now().Format("2006-01-02")
	}

	setName := resolvePSACategory(info.Category)
	if IsGenericSetName(setName) {
		setName = info.Category // keep original if resolved is still generic
	}

	p := &Purchase{
		CampaignID:   campaignID,
		CardName:     info.CardName,
		CertNumber:   req.CertNumber,
		CardNumber:   info.CardNumber,
		SetName:      setName,
		Grader:       "PSA",
		GradeValue:   info.Grade,
		BuyCostCents: req.BuyCostCents,
		CLValueCents: req.CLValueCents,
		PurchaseDate: purchaseDate,
	}

	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("campaign lookup: %w", err)
	}
	p.PSASourcingFeeCents = campaign.PSASourcingFeeCents

	if err := s.CreatePurchase(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *service) GetExpectedValues(ctx context.Context, campaignID string) (*EVPortfolio, error) {
	data, err := s.analytics.GetPurchasesWithSales(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	gradePerf := ComputeSegmentPerformance(data, "grade", func(d PurchaseWithSale) string {
		if d.Purchase.GradeValue <= 0 {
			return ""
		}
		return fmt.Sprintf("PSA %g", d.Purchase.GradeValue)
	})

	gradeMap := make(map[string]SegmentPerformance)
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

		ev := computeExpectedValue(
			p.CardName, p.CertNumber, p.GradeValue,
			costBasis, seg.SellThroughPct, seg.AvgMarginPct,
			liquidityFactor, trendAdj, seg.AvgDaysToSell,
			0.05, seg.SoldCount,
		)

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
	gradePerf := ComputeSegmentPerformance(data, "grade", func(d PurchaseWithSale) string {
		if d.Purchase.GradeValue <= 0 {
			return ""
		}
		return fmt.Sprintf("PSA %g", d.Purchase.GradeValue)
	})

	var seg *SegmentPerformance
	for i := range gradePerf {
		if gradePerf[i].Label == gradeKey {
			seg = &gradePerf[i]
			break
		}
	}

	if seg == nil {
		// No historical data for this grade
		return computeExpectedValue(cardName, "", grade, buyCostCents+campaign.PSASourcingFeeCents, 0.5, 0.0, 1.0, 0.0, 30, 0.05, 0), nil
	}

	costBasis := buyCostCents + campaign.PSASourcingFeeCents

	return computeExpectedValue(
		cardName, "", grade, costBasis,
		seg.SellThroughPct, seg.AvgMarginPct,
		1.0, 0.0, seg.AvgDaysToSell,
		0.05, seg.SoldCount,
	), nil
}

func (s *service) RunProjection(ctx context.Context, campaignID string) (*MonteCarloComparison, error) {
	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	data, err := s.analytics.GetPurchasesWithSales(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	return RunMonteCarloProjection(campaign, data), nil
}
