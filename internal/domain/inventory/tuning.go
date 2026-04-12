package inventory

import (
	"fmt"
	"math"
	"strconv"
)

const (
	// trendWarningThreshold: 30-day price trend below -10% signals meaningful
	// market value decline. Used to flag at-risk inventory.
	trendWarningThreshold = -0.10

	// lowLiquidityThreshold: fewer than 2 sales/month means the card is hard
	// to sell quickly. Used in sell-sheet and aging analysis.
	lowLiquidityThreshold = 2.0

	// healthyLiquidityThreshold: 5+ sales/month indicates active demand.
	// Cards above this threshold are prioritized in sell recommendations.
	healthyLiquidityThreshold = 5.0

	// spendCapLowUtilization: spending less than 20% of daily cap suggests
	// buy parameters are too restrictive. Surfaced in tuning recommendations.
	spendCapLowUtilization = 0.20

	// marketDriftThreshold: 5% price change threshold for classifying cards
	// as appreciating or depreciating.
	marketDriftThreshold = 0.05
)

// purchaseKey builds a lookup key for a card identity and grade combination.
func PurchaseKey(cardName, cardNumber, setName string, grade float64) string {
	return cardName + "|" + cardNumber + "|" + setName + "|" + strconv.FormatFloat(grade, 'f', -1, 64)
}

// Fixed price tier boundaries (in cents).
var fixedTiers = []struct {
	Label    string
	MinCents int
	MaxCents int
}{
	{"$0-50", 0, 5000},
	{"$50-100", 5000, 10000},
	{"$100-200", 10000, 20000},
	{"$200-500", 20000, 50000},
	{"$500+", 50000, math.MaxInt},
}

// TuningInput bundles all inputs needed by computeRecommendations.
type TuningInput struct {
	Campaign    *Campaign
	PNL         *CampaignPNL
	ByGrade     []GradePerformance
	ByFixedTier []PriceTierPerformance
	Threshold   *BuyThresholdAnalysis
	Alignment   *MarketAlignment
	DailySpend  []DailySpend
	ChannelPNL  []ChannelPNL
}

// computeRecommendations generates tuning recommendations from all analysis data.
func ComputeRecommendations(input *TuningInput) []TuningRecommendation {
	var recs []TuningRecommendation

	if r := recBuyThreshold(input.Campaign, input.Threshold); r != nil {
		recs = append(recs, *r)
	}
	recs = append(recs, recUnderperformingGrades(input.Campaign, input.ByGrade)...)
	recs = append(recs, recUnderperformingTiers(input.ByFixedTier)...)
	if r := recLowSellThrough(input.Campaign, input.PNL); r != nil {
		recs = append(recs, *r)
	}
	if r := recSpendCapUtilization(input.Campaign, input.PNL, input.DailySpend); r != nil {
		recs = append(recs, *r)
	}
	recs = append(recs, recMarketAlignment(input.Campaign, input.Alignment)...)
	recs = append(recs, recChannelOptimization(input.ChannelPNL)...)

	return recs
}

// Rule 1: Buy threshold too high
func recBuyThreshold(campaign *Campaign, threshold *BuyThresholdAnalysis) *TuningRecommendation {
	if threshold == nil || threshold.SampleSize < 10 {
		return nil
	}
	// Only fire if optimal is meaningfully lower than current
	if threshold.OptimalPct >= campaign.BuyTermsCLPct-0.03 {
		return nil
	}
	// Find the optimal bucket's ROI
	var optimalROI, currentROI float64
	for _, b := range threshold.BucketedROI {
		mid := (b.RangeMinPct + b.RangeMaxPct) / 2
		if math.Abs(mid-threshold.OptimalPct) < 0.03 {
			optimalROI = b.MedianROI
		}
		if math.Abs(mid-campaign.BuyTermsCLPct) < 0.03 {
			currentROI = b.MedianROI
		}
	}

	return &TuningRecommendation{
		Parameter:    "buyTermsCLPct",
		CurrentVal:   fmt.Sprintf("%.0f%%", campaign.BuyTermsCLPct*100),
		SuggestedVal: fmt.Sprintf("%.0f%%", threshold.OptimalPct*100),
		Reasoning: fmt.Sprintf("Purchases at %.0f%% of CL value average %.1f%% ROI vs %.1f%% at your current %.0f%% threshold",
			threshold.OptimalPct*100, optimalROI*100, currentROI*100, campaign.BuyTermsCLPct*100),
		Impact:     "high",
		Confidence: threshold.Confidence,
		DataPoints: threshold.SampleSize,
	}
}

// Rule 2: Underperforming grade
func recUnderperformingGrades(campaign *Campaign, byGrade []GradePerformance) []TuningRecommendation {
	if len(byGrade) < 2 {
		return nil
	}

	// Compute average ROI of positive grades
	var positiveROISum float64
	var positiveCount int
	for _, g := range byGrade {
		if g.ROI > 0 && g.PurchaseCount >= 5 {
			positiveROISum += g.ROI
			positiveCount++
		}
	}
	if positiveCount == 0 {
		return nil
	}
	avgPositiveROI := positiveROISum / float64(positiveCount)

	var recs []TuningRecommendation
	for _, g := range byGrade {
		if g.ROI < -0.05 && g.PurchaseCount >= 5 {
			recs = append(recs, TuningRecommendation{
				Parameter:    "gradeRange",
				CurrentVal:   campaign.GradeRange,
				SuggestedVal: fmt.Sprintf("exclude PSA %g", g.Grade),
				Reasoning: fmt.Sprintf("PSA %g cards have %.1f%% ROI across %d purchases. Other grades average %.1f%% ROI",
					g.Grade, g.ROI*100, g.PurchaseCount, avgPositiveROI*100),
				Impact:     "high",
				Confidence: g.PurchaseCount,
				DataPoints: g.PurchaseCount,
			})
		}
	}
	return recs
}

// Rule 3: Underperforming price tier
func recUnderperformingTiers(byFixedTier []PriceTierPerformance) []TuningRecommendation {
	if len(byFixedTier) < 2 {
		return nil
	}

	// Find best tier
	var bestTier *PriceTierPerformance
	for i := range byFixedTier {
		t := &byFixedTier[i]
		if t.PurchaseCount >= 5 && (bestTier == nil || t.ROI > bestTier.ROI) {
			bestTier = t
		}
	}
	if bestTier == nil {
		return nil
	}

	var recs []TuningRecommendation
	for _, t := range byFixedTier {
		if t.ROI < -0.05 && t.PurchaseCount >= 5 && bestTier.ROI > 0 {
			recs = append(recs, TuningRecommendation{
				Parameter:    "priceRange",
				CurrentVal:   t.TierLabel,
				SuggestedVal: fmt.Sprintf("focus on %s", bestTier.TierLabel),
				Reasoning: fmt.Sprintf("%s cards have %.1f%% ROI across %d purchases. Best tier is %s at %.1f%% ROI",
					t.TierLabel, t.ROI*100, t.PurchaseCount, bestTier.TierLabel, bestTier.ROI*100),
				Impact:     "medium",
				Confidence: t.PurchaseCount,
				DataPoints: t.PurchaseCount,
			})
		}
	}
	return recs
}

// Rule 4: Low sell-through
func recLowSellThrough(campaign *Campaign, pnl *CampaignPNL) *TuningRecommendation {
	if pnl == nil || pnl.TotalPurchases < 20 || pnl.SellThroughPct >= suggLowSellThroughPct {
		return nil
	}

	if pnl.AvgDaysToSell > 30 {
		return &TuningRecommendation{
			Parameter:    "phase",
			CurrentVal:   string(campaign.Phase),
			SuggestedVal: string(PhasePending),
			Reasoning: fmt.Sprintf("Sell-through is %.0f%% with avg %.0f days to sell. Capital is being tied up in slow-moving inventory",
				pnl.SellThroughPct*100, pnl.AvgDaysToSell),
			Impact:     "high",
			Confidence: pnl.TotalPurchases,
			DataPoints: pnl.TotalPurchases,
		}
	}

	return &TuningRecommendation{
		Parameter:    "buyTermsCLPct",
		CurrentVal:   fmt.Sprintf("%.0f%%", campaign.BuyTermsCLPct*100),
		SuggestedVal: fmt.Sprintf("%.0f%%", (campaign.BuyTermsCLPct-0.05)*100),
		Reasoning: fmt.Sprintf("Sell-through is %.0f%% — tighter buy terms may improve quality of purchases",
			pnl.SellThroughPct*100),
		Impact:     "medium",
		Confidence: pnl.TotalPurchases,
		DataPoints: pnl.TotalPurchases,
	}
}

// Rule 5: Daily spend cap utilization
func recSpendCapUtilization(campaign *Campaign, pnl *CampaignPNL, dailySpend []DailySpend) *TuningRecommendation {
	if len(dailySpend) < 14 || campaign.DailySpendCapCents <= 0 {
		return nil
	}

	// Use last 14 days
	recent := dailySpend
	if len(recent) > 14 {
		recent = recent[len(recent)-14:]
	}

	var totalFill float64
	for _, ds := range recent {
		if campaign.DailySpendCapCents > 0 {
			totalFill += float64(ds.SpendCents) / float64(campaign.DailySpendCapCents)
		}
	}
	avgFill := totalFill / float64(len(recent))

	if avgFill < spendCapLowUtilization {
		return &TuningRecommendation{
			Parameter:    "dailySpendCap",
			CurrentVal:   fmt.Sprintf("$%d", campaign.DailySpendCapCents/100),
			SuggestedVal: "(informational)",
			Reasoning: fmt.Sprintf("Daily spend cap is only %.0f%% utilized — few cards match your criteria. Consider widening grade range, price range, or inclusion list",
				avgFill*100),
			Impact:     "low",
			Confidence: len(recent),
			DataPoints: len(recent),
		}
	}

	if avgFill >= 0.95 && pnl != nil && pnl.ROI > 0.10 {
		suggested := campaign.DailySpendCapCents * 3 / 2 // 50% increase
		return &TuningRecommendation{
			Parameter:    "dailySpendCap",
			CurrentVal:   fmt.Sprintf("$%d", campaign.DailySpendCapCents/100),
			SuggestedVal: fmt.Sprintf("$%d", suggested/100),
			Reasoning: fmt.Sprintf("You're hitting your daily cap (%.0f%% utilized) with %.1f%% ROI — increasing the cap could scale profits",
				avgFill*100, pnl.ROI*100),
			Impact:     "medium",
			Confidence: pnl.TotalPurchases,
			DataPoints: pnl.TotalPurchases,
		}
	}

	return nil
}

// Rule 6: Market alignment warning
func recMarketAlignment(campaign *Campaign, alignment *MarketAlignment) []TuningRecommendation {
	if alignment == nil {
		return nil
	}
	var recs []TuningRecommendation

	if alignment.Signal == "warning" {
		recs = append(recs, TuningRecommendation{
			Parameter:    "phase",
			CurrentVal:   string(campaign.Phase),
			SuggestedVal: string(PhasePending),
			Reasoning:    alignment.SignalReason,
			Impact:       "high",
			Confidence:   alignment.SampleSize,
			DataPoints:   alignment.SampleSize,
		})
	}

	if alignment.Signal == "caution" && alignment.AvgVolatility > 0.30 {
		suggested := campaign.BuyTermsCLPct - 0.05
		if suggested < 0.50 {
			suggested = 0.50
		}
		recs = append(recs, TuningRecommendation{
			Parameter:    "buyTermsCLPct",
			CurrentVal:   fmt.Sprintf("%.0f%%", campaign.BuyTermsCLPct*100),
			SuggestedVal: fmt.Sprintf("%.0f%%", suggested*100),
			Reasoning: fmt.Sprintf("High market volatility (%.0f%%). Tighter buy terms reduce risk exposure",
				alignment.AvgVolatility*100),
			Impact:     "medium",
			Confidence: alignment.SampleSize,
			DataPoints: alignment.SampleSize,
		})
	}

	return recs
}

// Rule 7: Channel optimization
func recChannelOptimization(channelPNL []ChannelPNL) []TuningRecommendation {
	if len(channelPNL) < 2 {
		return nil
	}

	// Find best and worst channels by avg net profit per sale
	var best, worst *ChannelPNL
	for i := range channelPNL {
		ch := &channelPNL[i]
		if ch.SaleCount < 5 {
			continue
		}
		avgProfit := float64(ch.NetProfitCents) / float64(ch.SaleCount)
		if best == nil || avgProfit > float64(best.NetProfitCents)/float64(best.SaleCount) {
			best = ch
		}
		if worst == nil || avgProfit < float64(worst.NetProfitCents)/float64(worst.SaleCount) {
			worst = ch
		}
	}

	if worst == nil || best == nil || worst == best {
		return nil
	}

	// Only fire if worst channel is losing money
	if worst.NetProfitCents >= 0 {
		return nil
	}

	worstAvg := float64(worst.NetProfitCents) / float64(worst.SaleCount)
	bestAvg := float64(best.NetProfitCents) / float64(best.SaleCount)

	return []TuningRecommendation{{
		Parameter:    "saleChannel",
		CurrentVal:   string(worst.Channel),
		SuggestedVal: fmt.Sprintf("prefer %s", best.Channel),
		Reasoning: fmt.Sprintf("%s averages $%.2f/sale vs %s at $%.2f/sale across %d sales",
			worst.Channel, worstAvg/100, best.Channel, bestAvg/100, worst.SaleCount),
		Impact:     "medium",
		Confidence: worst.SaleCount,
		DataPoints: worst.SaleCount,
	}}
}
