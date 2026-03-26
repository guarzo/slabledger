package campaigns

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// suggMinROIExpansion is the minimum ROI for a character segment to be considered
	// for expansion to additional campaigns (Rule 1) or addition to inclusion lists (Rule 6).
	suggMinROIExpansion = 0.15

	// suggMinROIGradeSweetSpot is the minimum ROI for a grade to be considered
	// the "sweet spot" worth dedicating a campaign to (Rule 2).
	suggMinROIGradeSweetSpot = 0.10

	// suggMinSalesPerChannel is the minimum sale count per channel to consider
	// it for buy-terms suggestions (Rule 4).
	suggMinSalesPerChannel = 3

	// suggMinTotalSalesChannelAnalysis is the minimum total sales required before
	// channel-informed buy terms suggestions are generated (Rule 4).
	suggMinTotalSalesChannelAnalysis = 10

	// suggMinPurchasesSpendCap is the minimum total purchases required before
	// spend cap rebalancing suggestions are generated (Rule 5).
	suggMinPurchasesSpendCap = 20

	// suggMinCharacterSegments is the minimum number of character segments required
	// before character adjustment suggestions are generated (Rule 6).
	suggMinCharacterSegments = 3

	// suggMinSoldForConfidence is the minimum sold count for a segment to be
	// considered in expansion and grade sweet spot suggestions.
	suggMinSoldForConfidence = 5

	// suggMinSoldCoverageGap is the minimum sold count for a coverage gap segment.
	suggMinSoldCoverageGap = 3

	// suggMinSoldForRemoval is the minimum sold count to suggest removing an
	// underperforming character from an inclusion list (Rule 6).
	suggMinSoldForRemoval = 3

	// suggMaxCampaignsPerCharacter is the maximum number of campaigns covering a
	// character before expansion stops being suggested (Rule 1).
	suggMaxCampaignsPerCharacter = 3

	// suggBuyTermsBuffer is the margin of tolerance before triggering a buy terms
	// adjustment suggestion. If current buy terms exceed the computed max by more
	// than this value, a suggestion is generated (Rule 4).
	suggBuyTermsBuffer = 0.05

	// suggROIDeviation is the minimum deviation from average ROI before
	// suggesting spend-cap rebalancing across campaigns (Rule 5).
	suggROIDeviation = 0.05

	// suggTargetMargin is the target profit margin used in buy terms calculations (Rule 4).
	suggTargetMargin = 0.10

	// suggUnderperformingROI is the ROI threshold below which a character is
	// considered underperforming and may be suggested for removal (Rule 6).
	suggUnderperformingROI = -0.05

	// suggArchiveMinPurchases is the minimum number of purchases before a phase
	// transition (archiving) suggestion is generated (Rule 7).
	suggArchiveMinPurchases = 20

	// suggArchiveROIThreshold is the ROI threshold below which an active campaign
	// may be suggested for closing (Rule 7).
	suggArchiveROIThreshold = -0.10

	// suggLowSellThroughPct is the sell-through percentage below which a campaign
	// may be suggested for closing. Shared with tuning.go's recLowSellThrough.
	suggLowSellThroughPct = 0.30

	// suggActivateMinROI is the minimum ROI for a character segment to trigger
	// a suggestion to activate a pending campaign (Rule 7).
	suggActivateMinROI = 0.10
)

// GenerateSuggestions produces data-driven campaign recommendations from portfolio insights.
func GenerateSuggestions(ctx context.Context, insights *PortfolioInsights, campaigns []Campaign) *SuggestionsResponse {
	resp := &SuggestionsResponse{
		DataSummary: insights.DataSummary,
	}

	now := time.Now().Format("2006-01-02")

	resp.NewCampaigns = append(resp.NewCampaigns, suggestTopCharacterExpansion(ctx, insights, campaigns)...)
	resp.NewCampaigns = append(resp.NewCampaigns, suggestGradeSweetSpot(ctx, insights, campaigns)...)
	resp.NewCampaigns = append(resp.NewCampaigns, suggestCoverageGapCampaigns(ctx, insights)...)
	resp.Adjustments = append(resp.Adjustments, suggestChannelInformedBuyTerms(ctx, insights, campaigns, now)...)
	resp.Adjustments = append(resp.Adjustments, suggestSpendCapRebalancing(ctx, insights, campaigns)...)
	resp.Adjustments = append(resp.Adjustments, suggestCharacterAdjustments(ctx, insights, campaigns)...)
	resp.Adjustments = append(resp.Adjustments, suggestPhaseTransitions(ctx, insights, campaigns)...)

	// De-duplicate conflicting suggestions
	resp.NewCampaigns = deduplicateSuggestions(resp.NewCampaigns)
	resp.Adjustments = deduplicateSuggestions(resp.Adjustments)

	return resp
}

// deduplicateSuggestions removes conflicting suggestions for the same campaign.
// When multiple suggestions target the same campaign and modify the same parameter,
// only the one with higher confidence (or more data points) is kept.
func deduplicateSuggestions(suggestions []CampaignSuggestion) []CampaignSuggestion {
	if len(suggestions) < 2 {
		return suggestions
	}

	// Group by campaign name
	byCampaign := make(map[string][]int)
	for i, s := range suggestions {
		if s.SuggestedParams.Name != "" {
			byCampaign[s.SuggestedParams.Name] = append(byCampaign[s.SuggestedParams.Name], i)
		}
	}

	remove := make(map[int]bool)
	for _, indices := range byCampaign {
		if len(indices) < 2 {
			continue
		}

		// Check for buy terms conflicts
		var buyTerms []int
		for _, idx := range indices {
			if suggestions[idx].SuggestedParams.BuyTermsCLPct > 0 {
				buyTerms = append(buyTerms, idx)
			}
		}
		if len(buyTerms) > 1 {
			best := buyTerms[0]
			for _, idx := range buyTerms[1:] {
				if betterSuggestion(suggestions[idx], suggestions[best]) {
					remove[best] = true
					best = idx
				} else {
					remove[idx] = true
				}
			}
		}

		// Check for spend cap conflicts
		var capConflicts []int
		for _, idx := range indices {
			if suggestions[idx].SuggestedParams.DailySpendCapCents > 0 {
				capConflicts = append(capConflicts, idx)
			}
		}
		if len(capConflicts) > 1 {
			best := capConflicts[0]
			for _, idx := range capConflicts[1:] {
				if betterSuggestion(suggestions[idx], suggestions[best]) {
					remove[best] = true
					best = idx
				} else {
					remove[idx] = true
				}
			}
		}
	}

	if len(remove) == 0 {
		return suggestions
	}

	result := make([]CampaignSuggestion, 0, len(suggestions)-len(remove))
	for i, s := range suggestions {
		if !remove[i] {
			result = append(result, s)
		}
	}
	return result
}

// betterSuggestion returns true if a is a higher-quality suggestion than b.
func betterSuggestion(a, b CampaignSuggestion) bool {
	confidenceRank := map[string]int{"high": 3, "medium": 2, "low": 1}
	if confidenceRank[a.Confidence] != confidenceRank[b.Confidence] {
		return confidenceRank[a.Confidence] > confidenceRank[b.Confidence]
	}
	return a.DataPoints > b.DataPoints
}

// isMarketplaceChannel returns true for channels that charge marketplace fees.
func isMarketplaceChannel(ch SaleChannel) bool {
	return ch == SaleChannelEbay || ch == SaleChannelTCGPlayer
}

// gradeRangeFromLabel converts a grade label like "PSA 9" to a range string like "9-9".
func gradeRangeFromLabel(label string) string {
	parts := strings.Fields(label)
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err == nil && n >= 1 && n <= 10 {
			return fmt.Sprintf("%d-%d", n, n)
		}
	}
	return label
}
