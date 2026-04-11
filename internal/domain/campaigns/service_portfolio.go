package campaigns

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// --- Portfolio Health ---

func (s *service) GetPortfolioHealth(ctx context.Context) (*PortfolioHealth, error) {
	allCampaigns, err := s.repo.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}

	health := &PortfolioHealth{}
	totalSoldCostBasis := 0
	totalSoldNetProfit := 0
	for _, c := range allCampaigns {
		pnl, err := s.repo.GetCampaignPNL(ctx, c.ID)
		if err != nil {
			if s.logger != nil {
				s.logger.Error(ctx, "skipping campaign in portfolio health",
					observability.String("campaignID", c.ID),
					observability.String("campaignName", c.Name),
					observability.Err(err))
			}
			continue
		}

		capitalAtRisk := 0
		if pnl.TotalUnsold > 0 {
			capitalAtRisk = pnl.TotalSpendCents - pnl.TotalRevenueCents + pnl.TotalFeesCents
			if capitalAtRisk < 0 {
				capitalAtRisk = 0
			}
		}

		status := "healthy"
		reason := "Positive ROI and acceptable sell-through"
		if pnl.ROI < 0 {
			status = "warning"
			reason = fmt.Sprintf("Negative ROI (%.1f%%)", pnl.ROI*100)
		}
		if pnl.ROI < -0.10 && pnl.TotalUnsold > 5 {
			status = "critical"
			reason = fmt.Sprintf("Significant negative ROI (%.1f%%) with %d unsold", pnl.ROI*100, pnl.TotalUnsold)
		}
		if pnl.AvgDaysToSell > 45 && pnl.TotalSold > 0 {
			if status == "healthy" {
				status = "warning"
			}
			reason += fmt.Sprintf("; slow sell-through (avg %.0f days)", pnl.AvgDaysToSell)
		}

		if pnl.TotalSold > 0 && pnl.TotalPurchases > 0 {
			soldCostBasis := int(math.Round(float64(pnl.TotalSpendCents) * float64(pnl.TotalSold) / float64(pnl.TotalPurchases)))
			soldProfit := pnl.TotalRevenueCents - pnl.TotalFeesCents - soldCostBasis
			totalSoldCostBasis += soldCostBasis
			totalSoldNetProfit += soldProfit
		}

		// Liquidation-vs-marketplace channel split. Iterates sold purchases to
		// separate "marketplace margin is broken" from "we liquidated cards that
		// would have been profitable". Uses raw stored channel values (not
		// normalized) so cardshow is counted as liquidation while ebay and
		// tcgplayer are both counted as marketplace sales.
		liquidationLossCents, liquidationSaleCount, ebayMarginPct :=
			s.computeChannelHealthSignals(ctx, c.ID)

		if liquidationLossCents < -50000 && ebayMarginPct > 0.10 {
			liquidationReason := fmt.Sprintf(
				"marketplace channels profitable (%.1f%%) but $%.2f lost to forced liquidation",
				ebayMarginPct*100,
				float64(-liquidationLossCents)/100,
			)
			// Don't downgrade a campaign that's already critical from the base
			// health logic (e.g., deeply negative ROI with unsold inventory).
			// Instead append the liquidation context to the existing reason.
			if status == "critical" {
				reason = reason + "; " + liquidationReason
			} else {
				status = "warning"
				reason = liquidationReason
			}
		}

		ch := CampaignHealth{
			CampaignID:           c.ID,
			CampaignName:         c.Name,
			Phase:                c.Phase,
			ROI:                  pnl.ROI,
			SellThroughPct:       pnl.SellThroughPct,
			AvgDaysToSell:        pnl.AvgDaysToSell,
			TotalPurchases:       pnl.TotalPurchases,
			TotalUnsold:          pnl.TotalUnsold,
			CapitalAtRisk:        capitalAtRisk,
			HealthStatus:         status,
			HealthReason:         reason,
			LiquidationLossCents: liquidationLossCents,
			LiquidationSaleCount: liquidationSaleCount,
			EbayChannelMarginPct: ebayMarginPct,
		}
		health.Campaigns = append(health.Campaigns, ch)
		health.TotalDeployed += pnl.TotalSpendCents
		health.TotalRecovered += pnl.TotalRevenueCents - pnl.TotalFeesCents
		health.TotalAtRisk += capitalAtRisk
	}

	if health.TotalDeployed > 0 {
		health.OverallROI = float64(health.TotalRecovered-health.TotalDeployed) / float64(health.TotalDeployed)
	}
	if totalSoldCostBasis > 0 {
		health.RealizedROI = float64(totalSoldNetProfit) / float64(totalSoldCostBasis)
	}

	return health, nil
}

// computeChannelHealthSignals walks a campaign's sold purchases and returns:
//   - liquidationLossCents: sum of strictly-negative net profit on inperson+cardshow
//     sales (always ≤ 0). Profitable inperson/cardshow sales do not subtract from
//     this figure — we only surface the bleed.
//   - liquidationSaleCount: number of losing sales contributing to the loss.
//   - marketplaceMarginPct: net profit / revenue across eBay and TCGPlayer sales
//     combined (0 if no marketplace sales). eBay and TCGPlayer are grouped because
//     CLAUDE.md treats them as fee-equivalent marketplaces and the campaign-level
//     fee config shares a single ebayFeePct across both.
//
// Reads per-sale data from GetPurchasesWithSales rather than ChannelPNL because the
// latter only aggregates per-channel totals and cannot isolate strictly-negative
// contributions. Errors are logged and the zero tuple is returned so that a single
// campaign failure doesn't hide the whole portfolio health response.
func (s *service) computeChannelHealthSignals(ctx context.Context, campaignID string) (int, int, float64) {
	data, err := s.repo.GetPurchasesWithSales(ctx, campaignID)
	if err != nil {
		if s.logger != nil {
			s.logger.Error(ctx, "channel health signals: fetch purchases with sales",
				observability.String("campaignID", campaignID),
				observability.Err(err))
		}
		return 0, 0, 0
	}

	var (
		liquidationLossCents int
		liquidationSaleCount int
		marketplaceRevenue   int
		marketplaceNetProfit int
	)

	for _, d := range data {
		if d.Sale == nil {
			continue
		}
		switch d.Sale.SaleChannel {
		case SaleChannelInPerson, SaleChannelCardShow:
			if d.Sale.NetProfitCents < 0 {
				liquidationLossCents += d.Sale.NetProfitCents
				liquidationSaleCount++
			}
		case SaleChannelEbay, SaleChannelTCGPlayer:
			marketplaceRevenue += d.Sale.SalePriceCents
			marketplaceNetProfit += d.Sale.NetProfitCents
		}
	}

	marketplaceMarginPct := 0.0
	if marketplaceRevenue > 0 {
		marketplaceMarginPct = float64(marketplaceNetProfit) / float64(marketplaceRevenue)
	}
	return liquidationLossCents, liquidationSaleCount, marketplaceMarginPct
}

func (s *service) GetPortfolioChannelVelocity(ctx context.Context) ([]ChannelVelocity, error) {
	return s.repo.GetPortfolioChannelVelocity(ctx)
}

func (s *service) ReassignPurchase(ctx context.Context, purchaseID string, newCampaignID string) error {
	// Verify purchase exists
	if _, err := s.repo.GetPurchase(ctx, purchaseID); err != nil {
		return fmt.Errorf("purchase lookup: %w", err)
	}

	// Prevent reassignment if purchase has a linked sale
	sale, err := s.repo.GetSaleByPurchaseID(ctx, purchaseID)
	if err != nil && !IsSaleNotFound(err) && !IsPurchaseNotFound(err) {
		return fmt.Errorf("sale lookup for purchase %s: %w", purchaseID, err)
	}
	if err == nil && sale != nil {
		return fmt.Errorf("cannot reassign purchase %s: it has a linked sale", purchaseID)
	}

	// Verify target campaign exists and get its sourcing fee
	campaign, err := s.repo.GetCampaign(ctx, newCampaignID)
	if err != nil {
		return fmt.Errorf("campaign lookup: %w", err)
	}

	return s.repo.UpdatePurchaseCampaign(ctx, purchaseID, newCampaignID, campaign.PSASourcingFeeCents)
}

// --- Portfolio Insights ---

func (s *service) GetPortfolioInsights(ctx context.Context) (*PortfolioInsights, error) {
	data, err := s.repo.GetAllPurchasesWithSales(ctx, WithExcludeArchived())
	if err != nil {
		return nil, fmt.Errorf("all purchases with sales: %w", err)
	}

	channelPNL, err := s.repo.GetGlobalPNLByChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("global channel PNL: %w", err)
	}

	campaigns, err := s.repo.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}

	return computePortfolioInsights(data, channelPNL, campaigns), nil
}

// --- Campaign Suggestions ---

func (s *service) GetCampaignSuggestions(ctx context.Context) (*SuggestionsResponse, error) {
	insights, err := s.GetPortfolioInsights(ctx)
	if err != nil {
		return nil, fmt.Errorf("portfolio insights: %w", err)
	}

	campaigns, err := s.repo.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}

	// Portfolio health feeds the liquidation-aware buy-terms rule. Failure
	// here is non-fatal: we degrade to running suggestions without health.
	healthByCampaign := make(map[string]CampaignHealth)
	health, err := s.GetPortfolioHealth(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "campaign suggestions: portfolio health unavailable, liquidation rule will skip",
				observability.Err(err))
		}
	} else {
		for _, h := range health.Campaigns {
			healthByCampaign[h.CampaignID] = h
		}
	}

	return GenerateSuggestions(ctx, insights, campaigns, healthByCampaign), nil
}

// --- Capital Timeline ---

func (s *service) GetCapitalTimeline(ctx context.Context) (*CapitalTimeline, error) {
	points, err := s.repo.GetDailyCapitalTimeSeries(ctx)
	if err != nil {
		return nil, err
	}

	invoices, err := s.repo.ListInvoices(ctx)
	if err != nil {
		return nil, err
	}

	var invoiceDates []string
	for _, inv := range invoices {
		if inv.InvoiceDate != "" {
			invoiceDates = append(invoiceDates, inv.InvoiceDate)
		}
	}

	return &CapitalTimeline{
		DataPoints:   points,
		InvoiceDates: invoiceDates,
	}, nil
}

// --- Weekly Review ---

func (s *service) GetWeeklyReviewSummary(ctx context.Context) (*WeeklyReviewSummary, error) {
	now := time.Now()
	// Find start of current week (Monday)
	weekday := now.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	weekStart := now.AddDate(0, 0, -int(weekday-time.Monday))
	weekEnd := weekStart.AddDate(0, 0, 6)
	lastWeekStart := weekStart.AddDate(0, 0, -7)
	lastWeekEnd := weekStart.AddDate(0, 0, -1)

	thisWeekStr := weekStart.Format("2006-01-02")
	thisWeekEndStr := weekEnd.Format("2006-01-02")
	lastWeekStr := lastWeekStart.Format("2006-01-02")
	lastWeekEndStr := lastWeekEnd.Format("2006-01-02")

	eightWeeksAgo := weekStart.AddDate(0, 0, -8*7).Format("2006-01-02")
	allData, err := s.repo.GetAllPurchasesWithSales(ctx, WithSinceDate(eightWeeksAgo), WithExcludeArchived())
	if err != nil {
		return nil, err
	}

	summary := &WeeklyReviewSummary{
		WeekStart: thisWeekStr,
		WeekEnd:   thisWeekEndStr,
	}

	channelProfits := make(map[SaleChannel]int)
	channelRevenue := make(map[SaleChannel]int)
	channelFees := make(map[SaleChannel]int)
	channelCounts := make(map[SaleChannel]int)
	channelDays := make(map[SaleChannel]float64)

	var topSales []WeeklyPerformer

	for _, d := range allData {
		pd := d.Purchase.PurchaseDate
		if pd >= thisWeekStr && pd <= thisWeekEndStr {
			summary.PurchasesThisWeek++
			summary.SpendThisWeekCents += d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
		} else if pd >= lastWeekStr && pd <= lastWeekEndStr {
			summary.PurchasesLastWeek++
			summary.SpendLastWeekCents += d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
		}

		if d.Sale != nil {
			sd := d.Sale.SaleDate
			if sd >= thisWeekStr && sd <= thisWeekEndStr {
				summary.SalesThisWeek++
				summary.RevenueThisWeekCents += d.Sale.SalePriceCents
				summary.ProfitThisWeekCents += d.Sale.NetProfitCents
				channelProfits[d.Sale.SaleChannel] += d.Sale.NetProfitCents
				channelRevenue[d.Sale.SaleChannel] += d.Sale.SalePriceCents
				channelFees[d.Sale.SaleChannel] += d.Sale.SaleFeeCents
				channelCounts[d.Sale.SaleChannel]++
				channelDays[d.Sale.SaleChannel] += float64(d.Sale.DaysToSell)
				topSales = append(topSales, WeeklyPerformer{
					CardName:    d.Purchase.CardName,
					CertNumber:  d.Purchase.CertNumber,
					Grade:       d.Purchase.GradeValue,
					ProfitCents: d.Sale.NetProfitCents,
					Channel:     string(d.Sale.SaleChannel),
					DaysToSell:  d.Sale.DaysToSell,
				})
			} else if sd >= lastWeekStr && sd <= lastWeekEndStr {
				summary.SalesLastWeek++
				summary.RevenueLastWeekCents += d.Sale.SalePriceCents
				summary.ProfitLastWeekCents += d.Sale.NetProfitCents
			}
		}
	}

	// Build channel breakdown for this week
	for ch, count := range channelCounts {
		avgDays := 0.0
		if count > 0 {
			avgDays = channelDays[ch] / float64(count)
		}
		summary.ByChannel = append(summary.ByChannel, ChannelPNL{
			Channel:        ch,
			SaleCount:      count,
			RevenueCents:   channelRevenue[ch],
			FeesCents:      channelFees[ch],
			NetProfitCents: channelProfits[ch],
			AvgDaysToSell:  avgDays,
		})
	}

	// Sort top/bottom performers
	sort.Slice(topSales, func(i, j int) bool {
		return topSales[i].ProfitCents > topSales[j].ProfitCents
	})
	if len(topSales) > 10 {
		summary.TopPerformers = topSales[:5]
		summary.BottomPerformers = topSales[len(topSales)-5:]
	} else if len(topSales) > 5 {
		summary.TopPerformers = topSales[:5]
		summary.BottomPerformers = topSales[5:]
	} else {
		summary.TopPerformers = topSales
		summary.BottomPerformers = nil
	}

	// Capital exposure (default to sentinel 99 = no data, not zero which implies fully covered)
	summary.WeeksToCover = 99.0
	capitalRaw, err := s.repo.GetCapitalRawData(ctx)
	if err == nil && capitalRaw != nil {
		capital := ComputeCapitalSummary(capitalRaw)
		summary.WeeksToCover = capital.WeeksToCover
	}

	return summary, nil
}
