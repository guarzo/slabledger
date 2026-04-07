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

		ch := CampaignHealth{
			CampaignID:     c.ID,
			CampaignName:   c.Name,
			Phase:          c.Phase,
			ROI:            pnl.ROI,
			SellThroughPct: pnl.SellThroughPct,
			AvgDaysToSell:  pnl.AvgDaysToSell,
			TotalPurchases: pnl.TotalPurchases,
			TotalUnsold:    pnl.TotalUnsold,
			CapitalAtRisk:  capitalAtRisk,
			HealthStatus:   status,
			HealthReason:   reason,
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

	return GenerateSuggestions(ctx, insights, campaigns), nil
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
	capital, err := s.repo.GetCapitalSummary(ctx)
	if err == nil && capital != nil {
		summary.WeeksToCover = capital.WeeksToCover
	}

	return summary, nil
}
