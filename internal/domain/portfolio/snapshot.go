package portfolio

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// PortfolioSnapshot is a single-load aggregate of all portfolio dashboard data.
type PortfolioSnapshot struct {
	Health          *inventory.PortfolioHealth      `json:"health"`
	Insights        *inventory.PortfolioInsights    `json:"insights"`
	WeeklyReview    *inventory.WeeklyReviewSummary  `json:"weeklyReview"`
	WeeklyHistory   []inventory.WeeklyReviewSummary `json:"weeklyHistory"`
	ChannelVelocity []inventory.ChannelVelocity     `json:"channelVelocity"`
	Suggestions     *inventory.SuggestionsResponse  `json:"suggestions"`
	CreditSummary   *inventory.CapitalSummary       `json:"creditSummary"`
	Invoices        []inventory.Invoice             `json:"invoices"`
}

// computePNLByCampaign groups PurchaseWithSale data by campaign and computes
// CampaignPNL for each, matching the semantics of the Postgres GetCampaignPNL query.
func computePNLByCampaign(data []inventory.PurchaseWithSale) map[string]inventory.CampaignPNL {
	type agg struct {
		totalSpend     int
		totalRevenue   int
		totalFees      int
		netProfit      int
		totalDays      int
		totalSold      int
		totalPurchases int
	}
	buckets := make(map[string]*agg)
	for _, d := range data {
		cid := d.Purchase.CampaignID
		b, ok := buckets[cid]
		if !ok {
			b = &agg{}
			buckets[cid] = b
		}
		b.totalSpend += d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
		b.totalPurchases++
		if d.Sale != nil {
			b.totalRevenue += d.Sale.SalePriceCents
			b.totalFees += d.Sale.SaleFeeCents
			b.netProfit += d.Sale.NetProfitCents
			b.totalDays += d.Sale.DaysToSell
			b.totalSold++
		}
	}
	result := make(map[string]inventory.CampaignPNL, len(buckets))
	for cid, b := range buckets {
		pnl := inventory.CampaignPNL{
			CampaignID:        cid,
			TotalSpendCents:   b.totalSpend,
			TotalRevenueCents: b.totalRevenue,
			TotalFeesCents:    b.totalFees,
			NetProfitCents:    b.netProfit,
			TotalPurchases:    b.totalPurchases,
			TotalSold:         b.totalSold,
			TotalUnsold:       b.totalPurchases - b.totalSold,
		}
		if b.totalSold > 0 {
			pnl.AvgDaysToSell = float64(b.totalDays) / float64(b.totalSold)
		}
		if b.totalSpend > 0 {
			pnl.ROI = float64(b.netProfit) / float64(b.totalSpend)
		}
		if b.totalPurchases > 0 {
			pnl.SellThroughPct = float64(b.totalSold) / float64(b.totalPurchases)
		}
		result[cid] = pnl
	}
	return result
}

// ComputeHealthFromData mirrors GetPortfolioHealth but uses pre-loaded data
// instead of N+1 GetCampaignPNL DB calls.
func ComputeHealthFromData(campaigns []inventory.Campaign, allData []inventory.PurchaseWithSale) *inventory.PortfolioHealth {
	pnlByCampaign := computePNLByCampaign(allData)
	healthByCampaign := computeChannelHealthByCampaign(allData)
	inHandStats := computeInHandStatsByCampaign(allData)

	health := &inventory.PortfolioHealth{}
	totalSoldCostBasis := 0
	totalSoldNetProfit := 0

	for _, c := range campaigns {
		pnl, ok := pnlByCampaign[c.ID]
		if !ok {
			continue
		}

		capitalAtRisk := 0
		if pnl.TotalUnsold > 0 {
			capitalAtRisk = pnl.TotalSpendCents - pnl.TotalRevenueCents + pnl.TotalFeesCents
			capitalAtRisk = max(capitalAtRisk, 0)
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

		channelHealth := healthByCampaign[c.ID]
		liquidationLossCents := channelHealth.LiquidationLossCents
		liquidationSaleCount := channelHealth.LiquidationSaleCount
		ebayMarginPct := channelHealth.EbayChannelMarginPct

		if liquidationLossCents < -50000 && ebayMarginPct > 0.10 {
			liquidationReason := fmt.Sprintf(
				"marketplace channels profitable (%.1f%%) but $%.2f lost to forced liquidation",
				ebayMarginPct*100,
				float64(-liquidationLossCents)/100,
			)
			if status == "critical" {
				reason = reason + "; " + liquidationReason
			} else {
				status = "warning"
				reason = liquidationReason
			}
		}

		stats := inHandStats[c.ID]
		ch := inventory.CampaignHealth{
			CampaignID:            c.ID,
			CampaignName:          c.Name,
			Phase:                 c.Phase,
			ROI:                   pnl.ROI,
			SellThroughPct:        pnl.SellThroughPct,
			AvgDaysToSell:         pnl.AvgDaysToSell,
			TotalPurchases:        pnl.TotalPurchases,
			TotalUnsold:           pnl.TotalUnsold,
			CapitalAtRisk:         capitalAtRisk,
			HealthStatus:          status,
			HealthReason:          reason,
			LiquidationLossCents:  liquidationLossCents,
			LiquidationSaleCount:  liquidationSaleCount,
			EbayChannelMarginPct:  ebayMarginPct,
			InHandUnsoldCount:     stats[0],
			InHandCapitalCents:    stats[1],
			InTransitUnsoldCount:  stats[2],
			InTransitCapitalCents: stats[3],
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

	return health
}

// GetSnapshot loads all shared data once and fans out to computation functions.
func (s *service) GetSnapshot(ctx context.Context) (*PortfolioSnapshot, error) {
	// Load shared data
	activeCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list all campaigns: %w", err)
	}
	allData, err := s.analytics.GetAllPurchasesWithSales(ctx, inventory.WithExcludeArchived())
	if err != nil {
		return nil, fmt.Errorf("all purchases with sales: %w", err)
	}
	channelPNL, err := s.analytics.GetGlobalPNLByChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("global channel PNL: %w", err)
	}
	channelVelocity, err := s.analytics.GetPortfolioChannelVelocity(ctx)
	if err != nil {
		return nil, fmt.Errorf("channel velocity: %w", err)
	}
	capitalRaw, err := s.finance.GetCapitalRawData(ctx)
	if err != nil {
		return nil, fmt.Errorf("capital raw data: %w", err)
	}
	invoices, err := s.finance.ListInvoices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}

	// Health
	health := ComputeHealthFromData(activeCampaigns, allData)

	// Insights + Suggestions
	insights := inventory.ComputePortfolioInsights(allData, channelPNL, allCampaigns)
	healthByCampaign := computeChannelHealthByCampaign(allData)
	suggestions := inventory.GenerateSuggestions(ctx, insights, allCampaigns, healthByCampaign)

	// Weekly review (current week)
	now := time.Now()
	weekday := now.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	weekStart := now.AddDate(0, 0, -int(weekday-time.Monday))
	weekEnd := weekStart.AddDate(0, 0, 6)
	lastWeekStart := weekStart.AddDate(0, 0, -7)
	lastWeekEnd := weekStart.AddDate(0, 0, -1)

	weeklyReview := computeWeekSummary(allData,
		weekStart.Format("2006-01-02"),
		weekEnd.Format("2006-01-02"),
		lastWeekStart.Format("2006-01-02"),
		lastWeekEnd.Format("2006-01-02"),
	)
	weeklyReview.DaysIntoWeek = int(now.Weekday())
	if capitalRaw != nil {
		capital := inventory.ComputeCapitalSummary(capitalRaw)
		weeklyReview.WeeksToCover = capital.WeeksToCover
	} else {
		weeklyReview.WeeksToCover = inventory.WeeksToCoverNoData
	}

	// Weekly history (8 weeks)
	const historyWeeks = 8
	history := make([]inventory.WeeklyReviewSummary, 0, historyWeeks)
	for i := range historyWeeks {
		wStart := weekStart.AddDate(0, 0, -i*7)
		wEnd := wStart.AddDate(0, 0, 6)
		lwStart := wStart.AddDate(0, 0, -7)
		lwEnd := wStart.AddDate(0, 0, -1)
		sum := computeWeekSummary(allData,
			wStart.Format("2006-01-02"),
			wEnd.Format("2006-01-02"),
			lwStart.Format("2006-01-02"),
			lwEnd.Format("2006-01-02"),
		)
		history = append(history, sum)
	}

	// Credit summary
	creditSummary := inventory.ComputeCapitalSummary(capitalRaw)

	return &PortfolioSnapshot{
		Health:          health,
		Insights:        insights,
		WeeklyReview:    &weeklyReview,
		WeeklyHistory:   history,
		ChannelVelocity: channelVelocity,
		Suggestions:     suggestions,
		CreditSummary:   creditSummary,
		Invoices:        invoices,
	}, nil
}
