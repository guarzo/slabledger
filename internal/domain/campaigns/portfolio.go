package campaigns

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// knownCharacters is the default set of Pokemon characters to match against card names.
// This is extended at runtime by the union of all campaign inclusion lists.
var knownCharacters = []string{
	"Charizard", "Pikachu", "Blastoise", "Venusaur", "Mewtwo",
	"Mew", "Gengar", "Dragonite", "Eevee", "Umbreon",
	"Espeon", "Lugia", "Ho-Oh", "Rayquaza", "Gyarados",
	"Snorlax", "Alakazam", "Machamp", "Arcanine", "Ninetales",
	"Jolteon", "Flareon", "Vaporeon", "Sylveon", "Glaceon",
	"Leafeon", "Tyranitar", "Gardevoir", "Lucario", "Garchomp",
}

// ExtractCharacter returns the Pokemon character name from a card name using
// case-insensitive substring match against known characters. Returns "Other" if
// no match is found.
func ExtractCharacter(cardName string, campaigns []Campaign) string {
	lower := strings.ToLower(cardName)

	// Build combined character list from known + campaign inclusion lists
	chars := make([]string, len(knownCharacters))
	copy(chars, knownCharacters)
	seen := make(map[string]bool, len(knownCharacters))
	for _, c := range knownCharacters {
		seen[strings.ToLower(c)] = true
	}
	for _, camp := range campaigns {
		if camp.InclusionList == "" {
			continue
		}
		for _, name := range SplitInclusionList(camp.InclusionList) {
			name = strings.TrimSpace(name)
			if name != "" && !seen[strings.ToLower(name)] {
				seen[strings.ToLower(name)] = true
				chars = append(chars, name)
			}
		}
	}

	// Match longest first to avoid "Mew" matching before "Mewtwo"
	sort.Slice(chars, func(i, j int) bool {
		return len(chars[i]) > len(chars[j])
	})

	for _, ch := range chars {
		if strings.Contains(lower, strings.ToLower(ch)) {
			return ch
		}
	}
	return "Other"
}

// ClassifyEra derives era from the purchase date year.
func ClassifyEra(purchaseDate string) string {
	if len(purchaseDate) < 4 {
		return "unknown"
	}
	yearStr := purchaseDate[:4]
	year, err := strconv.Atoi(yearStr)
	if err != nil {
		return "unknown"
	}

	switch {
	case year <= 2003:
		return "vintage"
	case year <= 2007:
		return "ex_era"
	case year <= 2015:
		return "mid_era"
	default:
		return "modern"
	}
}

// ClassifyPriceTier assigns a purchase to a fixed price tier based on cost basis.
func ClassifyPriceTier(buyCostCents int) string {
	for _, t := range fixedTiers {
		if buyCostCents >= t.MinCents && buyCostCents < t.MaxCents {
			return t.Label
		}
	}
	return "$500+"
}

// computeSegmentPerformance aggregates PurchaseWithSale data by an arbitrary key function.
func computeSegmentPerformance(data []PurchaseWithSale, dimension string, keyFn func(PurchaseWithSale) string) []SegmentPerformance {
	type accumulator struct {
		seg            SegmentPerformance
		totalDays      float64
		totalBuyPctCL  float64
		numBuyWithCL   int
		totalMarginPct float64
		marginCount    int
		campaignIDs    map[string]bool
		channelProfit  map[SaleChannel]int
		latestSaleDate string
	}

	buckets := make(map[string]*accumulator)

	for _, d := range data {
		key := keyFn(d)
		if key == "" {
			continue
		}

		acc, ok := buckets[key]
		if !ok {
			acc = &accumulator{
				seg:           SegmentPerformance{Label: key, Dimension: dimension},
				campaignIDs:   make(map[string]bool),
				channelProfit: make(map[SaleChannel]int),
			}
			buckets[key] = acc
		}

		acc.seg.PurchaseCount++
		spend := d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
		acc.seg.TotalSpendCents += spend
		acc.campaignIDs[d.Purchase.CampaignID] = true

		if d.Purchase.CLValueCents > 0 {
			acc.totalBuyPctCL += float64(d.Purchase.BuyCostCents) / float64(d.Purchase.CLValueCents)
			acc.numBuyWithCL++
		}

		if d.Sale != nil {
			acc.seg.SoldCount++
			acc.seg.TotalRevenueCents += d.Sale.SalePriceCents
			acc.seg.TotalFeesCents += d.Sale.SaleFeeCents
			acc.seg.NetProfitCents += d.Sale.NetProfitCents
			acc.totalDays += float64(d.Sale.DaysToSell)
			acc.channelProfit[d.Sale.SaleChannel] += d.Sale.NetProfitCents

			if d.Sale.SaleDate > acc.latestSaleDate {
				acc.latestSaleDate = d.Sale.SaleDate
			}

			if spend > 0 {
				marginPct := float64(d.Sale.NetProfitCents) / float64(spend)
				acc.totalMarginPct += marginPct
				acc.marginCount++
			}
		}
	}

	result := make([]SegmentPerformance, 0, len(buckets))
	for _, acc := range buckets {
		seg := &acc.seg
		if seg.PurchaseCount > 0 {
			seg.SellThroughPct = float64(seg.SoldCount) / float64(seg.PurchaseCount)
		}
		if acc.numBuyWithCL > 0 {
			seg.AvgBuyPctOfCL = acc.totalBuyPctCL / float64(acc.numBuyWithCL)
		}
		if seg.SoldCount > 0 {
			seg.AvgDaysToSell = acc.totalDays / float64(seg.SoldCount)
		}
		if seg.TotalSpendCents > 0 {
			seg.ROI = float64(seg.NetProfitCents) / float64(seg.TotalSpendCents)
		}
		if acc.marginCount > 0 {
			seg.AvgMarginPct = acc.totalMarginPct / float64(acc.marginCount)
		}
		seg.CampaignCount = len(acc.campaignIDs)
		seg.LatestSaleDate = acc.latestSaleDate

		// Best channel by profit
		bestProfit := 0
		for ch, profit := range acc.channelProfit {
			if profit > bestProfit {
				bestProfit = profit
				seg.BestChannel = ch
			}
		}

		result = append(result, *seg)
	}

	// Sort by net profit descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].NetProfitCents > result[j].NetProfitCents
	})

	return result
}

// computePortfolioInsights orchestrates all cross-campaign analytics dimensions.
func computePortfolioInsights(data []PurchaseWithSale, channelPNL []ChannelPNL, campaigns []Campaign) *PortfolioInsights {
	insights := &PortfolioInsights{
		ByChannel: channelPNL,
	}

	// By Character
	insights.ByCharacter = computeSegmentPerformance(data, "character", func(d PurchaseWithSale) string {
		return ExtractCharacter(d.Purchase.CardName, campaigns)
	})

	// By Grade
	insights.ByGrade = computeSegmentPerformance(data, "grade", func(d PurchaseWithSale) string {
		if d.Purchase.GradeValue <= 0 {
			return ""
		}
		grader := d.Purchase.Grader
		if grader == "" {
			grader = "PSA"
		}
		return fmt.Sprintf("%s %g", grader, d.Purchase.GradeValue)
	})

	// By Era
	insights.ByEra = computeSegmentPerformance(data, "era", func(d PurchaseWithSale) string {
		return ClassifyEra(d.Purchase.PurchaseDate)
	})

	// By Price Tier
	insights.ByPriceTier = computeSegmentPerformance(data, "priceTier", func(d PurchaseWithSale) string {
		return ClassifyPriceTier(d.Purchase.BuyCostCents)
	})

	// By Character x Grade
	insights.ByCharacterGrade = computeSegmentPerformance(data, "characterGrade", func(d PurchaseWithSale) string {
		char := ExtractCharacter(d.Purchase.CardName, campaigns)
		if d.Purchase.GradeValue <= 0 {
			return ""
		}
		grader := d.Purchase.Grader
		if grader == "" {
			grader = "PSA"
		}
		return fmt.Sprintf("%s %s %g", char, grader, d.Purchase.GradeValue)
	})

	// Coverage gaps
	insights.CoverageGaps = DetectCoverageGaps(insights.ByCharacter, insights.ByGrade, campaigns)

	// Per-campaign metrics
	insights.CampaignMetrics = computeCampaignMetrics(data)

	// Data summary
	totalSales := 0
	totalSpend := 0
	totalProfit := 0
	var minDate, maxDate string
	campaignSet := make(map[string]bool)
	for _, d := range data {
		campaignSet[d.Purchase.CampaignID] = true
		totalSpend += d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
		if d.Sale != nil {
			totalSales++
			totalProfit += d.Sale.NetProfitCents
		}
		if minDate == "" || d.Purchase.PurchaseDate < minDate {
			minDate = d.Purchase.PurchaseDate
		}
		if maxDate == "" || d.Purchase.PurchaseDate > maxDate {
			maxDate = d.Purchase.PurchaseDate
		}
	}

	dateRange := ""
	if minDate != "" && maxDate != "" {
		dateRange = minDate + " to " + maxDate
	}

	overallROI := 0.0
	if totalSpend > 0 {
		overallROI = float64(totalProfit) / float64(totalSpend)
	}

	insights.DataSummary = InsightsDataSummary{
		TotalPurchases:    len(data),
		TotalSales:        totalSales,
		CampaignsAnalyzed: len(campaignSet),
		DateRange:         dateRange,
		OverallROI:        overallROI,
	}

	return insights
}

// Thresholds for DetectCoverageGaps profitability filters.
const (
	// MinCharacterROI is the minimum ROI for a character segment to be considered profitable.
	MinCharacterROI = 0.10
	// MinCharacterSales is the minimum sold count for a character segment to be actionable.
	MinCharacterSales = 3
	// MinGradeROI is the minimum ROI for a grade segment to be considered profitable.
	MinGradeROI = 0.15
	// MinGradeSales is the minimum sold count for a grade segment to be actionable.
	MinGradeSales = 5
)

// DetectCoverageGaps compares profitable segments against active campaign coverage.
func DetectCoverageGaps(byCharacter, byGrade []SegmentPerformance, campaigns []Campaign) []CoverageGap {
	var gaps []CoverageGap

	// Build set of characters covered by active campaigns
	coveredChars := make(map[string]bool)
	for _, c := range campaigns {
		if c.Phase != PhaseActive {
			continue
		}
		for _, name := range SplitInclusionList(c.InclusionList) {
			name = strings.TrimSpace(name)
			if name != "" {
				coveredChars[strings.ToLower(name)] = true
			}
		}
	}

	// Check profitable characters not well-covered
	for _, seg := range byCharacter {
		if seg.ROI <= MinCharacterROI || seg.SoldCount < MinCharacterSales || seg.Label == "Other" {
			continue
		}
		if !coveredChars[strings.ToLower(seg.Label)] {
			gaps = append(gaps, CoverageGap{
				Segment:     seg,
				Reason:      fmt.Sprintf("%s has %.0f%% ROI across %d sales but is not in any active campaign inclusion list", seg.Label, seg.ROI*100, seg.SoldCount),
				Opportunity: fmt.Sprintf("Add %s to an existing campaign or create a dedicated campaign", seg.Label),
			})
		}
	}

	// Check profitable grades
	activeCampaignCount := 0
	for _, c := range campaigns {
		if c.Phase == PhaseActive {
			activeCampaignCount++
		}
	}
	for _, seg := range byGrade {
		if seg.ROI <= MinGradeROI || seg.SoldCount < MinGradeSales || seg.CampaignCount >= activeCampaignCount {
			continue
		}
		gaps = append(gaps, CoverageGap{
			Segment:     seg,
			Reason:      fmt.Sprintf("%s has %.0f%% ROI but only appears in %d of %d active campaigns", seg.Label, seg.ROI*100, seg.CampaignCount, activeCampaignCount),
			Opportunity: fmt.Sprintf("Expand %s coverage to more campaigns", seg.Label),
		})
	}

	return gaps
}

// computeCampaignMetrics aggregates PNL per campaign from cross-campaign data.
func computeCampaignMetrics(data []PurchaseWithSale) []CampaignPNLBrief {
	type acc struct {
		spend  int
		profit int
		sold   int
		total  int
	}
	buckets := make(map[string]*acc)
	for _, d := range data {
		id := d.Purchase.CampaignID
		a, ok := buckets[id]
		if !ok {
			a = &acc{}
			buckets[id] = a
		}
		a.total++
		a.spend += d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
		if d.Sale != nil {
			a.sold++
			a.profit += d.Sale.NetProfitCents
		}
	}

	result := make([]CampaignPNLBrief, 0, len(buckets))
	for id, a := range buckets {
		roi := 0.0
		if a.spend > 0 {
			roi = float64(a.profit) / float64(a.spend)
		}
		result = append(result, CampaignPNLBrief{
			CampaignID:    id,
			ROI:           roi,
			SpendCents:    a.spend,
			ProfitCents:   a.profit,
			SoldCount:     a.sold,
			PurchaseCount: a.total,
		})
	}
	return result
}

// confidenceLabel returns a confidence string based on data point count.
func confidenceLabel(n int) string {
	switch {
	case n >= 20:
		return "high"
	case n >= 5:
		return "medium"
	default:
		return "low"
	}
}

// confidenceLabelWithAge returns a confidence string that decays based on data age.
// If the latest data is older than 6 months, confidence is reduced by one level.
func confidenceLabelWithAge(n int, latestSaleDate string, now string) string {
	base := confidenceLabel(n)
	if latestSaleDate == "" || now == "" {
		return base
	}

	nowMonths := mustParseYearMonth(now)
	saleMonths := mustParseYearMonth(latestSaleDate)
	if nowMonths == 0 || saleMonths == 0 {
		return base
	}
	if nowMonths-saleMonths > 6 {
		switch base {
		case "high":
			return "medium"
		case "medium":
			return "low"
		}
	}
	return base
}

// mustParseYearMonth returns year*12+month from a YYYY-MM-DD string, or 0 on failure.
func mustParseYearMonth(date string) int {
	if len(date) < 7 {
		return 0
	}
	year, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	month, err := strconv.Atoi(date[5:7])
	if err != nil {
		return 0
	}
	return year*12 + month
}
