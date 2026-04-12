package portfolio

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// Service provides portfolio-level analytics across campaigns.
type Service interface {
	GetPortfolioHealth(ctx context.Context) (*inventory.PortfolioHealth, error)
	GetPortfolioChannelVelocity(ctx context.Context) ([]inventory.ChannelVelocity, error)
	GetPortfolioInsights(ctx context.Context) (*inventory.PortfolioInsights, error)
	GetCampaignSuggestions(ctx context.Context) (*inventory.SuggestionsResponse, error)
	GetCapitalTimeline(ctx context.Context) (*inventory.CapitalTimeline, error)
	GetWeeklyReviewSummary(ctx context.Context) (*inventory.WeeklyReviewSummary, error)
}

type service struct {
	campaigns inventory.CampaignRepository
	analytics inventory.AnalyticsRepository
	finance   inventory.FinanceRepository
	logger    observability.Logger
}

// NewService creates a new portfolio Service.
func NewService(
	campaigns inventory.CampaignRepository,
	analytics inventory.AnalyticsRepository,
	finance inventory.FinanceRepository,
	logger observability.Logger,
) Service {
	return &service{
		campaigns: campaigns,
		analytics: analytics,
		finance:   finance,
		logger:    logger,
	}
}

// --- Portfolio Health ---

func (s *service) GetPortfolioHealth(ctx context.Context) (*inventory.PortfolioHealth, error) {
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}

	health := &inventory.PortfolioHealth{}
	totalSoldCostBasis := 0
	totalSoldNetProfit := 0
	for _, c := range allCampaigns {
		pnl, err := s.analytics.GetCampaignPNL(ctx, c.ID)
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

		liquidationLossCents, liquidationSaleCount, ebayMarginPct :=
			s.computeChannelHealthSignals(ctx, c.ID)

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

		ch := inventory.CampaignHealth{
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

func (s *service) computeChannelHealthSignals(ctx context.Context, campaignID string) (int, int, float64) {
	data, err := s.analytics.GetPurchasesWithSales(ctx, campaignID)
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
		case inventory.SaleChannelInPerson, inventory.SaleChannelCardShow:
			if d.Sale.NetProfitCents < 0 {
				liquidationLossCents += d.Sale.NetProfitCents
				liquidationSaleCount++
			}
		case inventory.SaleChannelEbay, inventory.SaleChannelTCGPlayer:
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

func (s *service) GetPortfolioChannelVelocity(ctx context.Context) ([]inventory.ChannelVelocity, error) {
	return s.analytics.GetPortfolioChannelVelocity(ctx)
}

// --- Portfolio Insights ---

func (s *service) GetPortfolioInsights(ctx context.Context) (*inventory.PortfolioInsights, error) {
	data, err := s.analytics.GetAllPurchasesWithSales(ctx, inventory.WithExcludeArchived())
	if err != nil {
		return nil, fmt.Errorf("all purchases with sales: %w", err)
	}

	channelPNL, err := s.analytics.GetGlobalPNLByChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("global channel PNL: %w", err)
	}

	campaigns, err := s.campaigns.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}

	return inventory.ComputePortfolioInsights(data, channelPNL, campaigns), nil
}

// --- Campaign Suggestions ---

func computeChannelHealthByCampaign(data []inventory.PurchaseWithSale) map[string]inventory.CampaignHealth {
	type agg struct {
		liquidationLossCents int
		liquidationSaleCount int
		marketplaceRevenue   int
		marketplaceNetProfit int
	}
	byCampaign := make(map[string]*agg)
	for _, d := range data {
		if d.Sale == nil {
			continue
		}
		cid := d.Purchase.CampaignID
		bucket, ok := byCampaign[cid]
		if !ok {
			bucket = &agg{}
			byCampaign[cid] = bucket
		}
		switch d.Sale.SaleChannel {
		case inventory.SaleChannelInPerson, inventory.SaleChannelCardShow:
			if d.Sale.NetProfitCents < 0 {
				bucket.liquidationLossCents += d.Sale.NetProfitCents
				bucket.liquidationSaleCount++
			}
		case inventory.SaleChannelEbay, inventory.SaleChannelTCGPlayer:
			bucket.marketplaceRevenue += d.Sale.SalePriceCents
			bucket.marketplaceNetProfit += d.Sale.NetProfitCents
		}
	}
	result := make(map[string]inventory.CampaignHealth, len(byCampaign))
	for cid, b := range byCampaign {
		margin := 0.0
		if b.marketplaceRevenue > 0 {
			margin = float64(b.marketplaceNetProfit) / float64(b.marketplaceRevenue)
		}
		result[cid] = inventory.CampaignHealth{
			CampaignID:           cid,
			LiquidationLossCents: b.liquidationLossCents,
			LiquidationSaleCount: b.liquidationSaleCount,
			EbayChannelMarginPct: margin,
		}
	}
	return result
}

func (s *service) GetCampaignSuggestions(ctx context.Context) (*inventory.SuggestionsResponse, error) {
	data, err := s.analytics.GetAllPurchasesWithSales(ctx, inventory.WithExcludeArchived())
	if err != nil {
		return nil, fmt.Errorf("all purchases with sales: %w", err)
	}

	channelPNL, err := s.analytics.GetGlobalPNLByChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("global channel PNL: %w", err)
	}

	campaigns, err := s.campaigns.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}

	insights := inventory.ComputePortfolioInsights(data, channelPNL, campaigns)
	healthByCampaign := computeChannelHealthByCampaign(data)

	return inventory.GenerateSuggestions(ctx, insights, campaigns, healthByCampaign), nil
}

// --- Capital Timeline ---

func (s *service) GetCapitalTimeline(ctx context.Context) (*inventory.CapitalTimeline, error) {
	points, err := s.analytics.GetDailyCapitalTimeSeries(ctx)
	if err != nil {
		return nil, err
	}

	invoices, err := s.finance.ListInvoices(ctx)
	if err != nil {
		return nil, err
	}

	var invoiceDates []string
	for _, inv := range invoices {
		if inv.InvoiceDate != "" {
			invoiceDates = append(invoiceDates, inv.InvoiceDate)
		}
	}

	return &inventory.CapitalTimeline{
		DataPoints:   points,
		InvoiceDates: invoiceDates,
	}, nil
}

// --- Weekly Review ---

func (s *service) GetWeeklyReviewSummary(ctx context.Context) (*inventory.WeeklyReviewSummary, error) {
	now := time.Now()
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
	allData, err := s.analytics.GetAllPurchasesWithSales(ctx, inventory.WithSinceDate(eightWeeksAgo), inventory.WithExcludeArchived())
	if err != nil {
		return nil, err
	}

	summary := &inventory.WeeklyReviewSummary{
		WeekStart: thisWeekStr,
		WeekEnd:   thisWeekEndStr,
	}

	channelProfits := make(map[inventory.SaleChannel]int)
	channelRevenue := make(map[inventory.SaleChannel]int)
	channelFees := make(map[inventory.SaleChannel]int)
	channelCounts := make(map[inventory.SaleChannel]int)
	channelDays := make(map[inventory.SaleChannel]float64)

	var topSales []inventory.WeeklyPerformer

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
				topSales = append(topSales, inventory.WeeklyPerformer{
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

	for ch, count := range channelCounts {
		avgDays := 0.0
		if count > 0 {
			avgDays = channelDays[ch] / float64(count)
		}
		summary.ByChannel = append(summary.ByChannel, inventory.ChannelPNL{
			Channel:        ch,
			SaleCount:      count,
			RevenueCents:   channelRevenue[ch],
			FeesCents:      channelFees[ch],
			NetProfitCents: channelProfits[ch],
			AvgDaysToSell:  avgDays,
		})
	}

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

	summary.WeeksToCover = 99.0
	capitalRaw, err := s.finance.GetCapitalRawData(ctx)
	if err == nil && capitalRaw != nil {
		capital := inventory.ComputeCapitalSummary(capitalRaw)
		summary.WeeksToCover = capital.WeeksToCover
	}

	return summary, nil
}
