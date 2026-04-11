package campaigns

import (
	"context"
	"fmt"
	"strings"
)

func suggestTopCharacterExpansion(_ context.Context, insights *PortfolioInsights, campaigns []Campaign) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	for _, seg := range insights.ByCharacter {
		if seg.ROI <= suggMinROIExpansion || seg.SoldCount < suggMinSoldForConfidence || seg.Label == "Other" {
			continue
		}
		if seg.CampaignCount >= suggMaxCampaignsPerCharacter {
			continue
		}

		var missingCampaigns []string
		for _, c := range campaigns {
			if c.Phase != PhaseActive {
				continue
			}
			if c.InclusionList == "" {
				if c.ExclusionMode {
					continue
				}
				continue
			}
			inList := false
			for _, name := range SplitInclusionList(c.InclusionList) {
				if strings.EqualFold(strings.TrimSpace(name), seg.Label) {
					inList = true
					break
				}
			}
			if c.ExclusionMode {
				if inList {
					missingCampaigns = append(missingCampaigns, c.Name)
				}
			} else {
				if !inList {
					missingCampaigns = append(missingCampaigns, c.Name)
				}
			}
		}

		if len(missingCampaigns) == 0 {
			continue
		}

		suggestions = append(suggestions, CampaignSuggestion{
			Type:  "new",
			Title: fmt.Sprintf("Expand %s to more campaigns", seg.Label),
			Rationale: fmt.Sprintf("%s has %.0f%% ROI across %d sales in %d campaigns. Adding to: %s",
				seg.Label, seg.ROI*100, seg.SoldCount, seg.CampaignCount, strings.Join(missingCampaigns, ", ")),
			Confidence: confidenceLabel(seg.SoldCount),
			DataPoints: seg.PurchaseCount,
			SuggestedParams: CampaignSuggestionParams{
				Name:          fmt.Sprintf("%s Focus", seg.Label),
				InclusionList: seg.Label,
				PrimaryExit:   string(seg.BestChannel),
			},
			ExpectedMetrics: ExpectedMetrics{
				ExpectedROI:       seg.ROI,
				ExpectedMarginPct: seg.AvgMarginPct,
				AvgDaysToSell:     seg.AvgDaysToSell,
				DataConfidence:    confidenceLabel(seg.SoldCount),
			},
		})
	}

	return suggestions
}

func suggestGradeSweetSpot(_ context.Context, insights *PortfolioInsights, campaigns []Campaign) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	var bestGrade *SegmentPerformance
	for i := range insights.ByGrade {
		seg := &insights.ByGrade[i]
		if seg.SoldCount < suggMinSoldForConfidence {
			continue
		}
		if bestGrade == nil || seg.ROI > bestGrade.ROI {
			bestGrade = seg
		}
	}

	if bestGrade == nil || bestGrade.ROI <= suggMinROIGradeSweetSpot {
		return nil
	}

	activeCampaigns := 0
	for _, c := range campaigns {
		if c.Phase == PhaseActive {
			activeCampaigns++
		}
	}

	if activeCampaigns == 0 || bestGrade.CampaignCount >= activeCampaigns {
		return nil
	}

	suggestions = append(suggestions, CampaignSuggestion{
		Type:  "new",
		Title: fmt.Sprintf("%s Sweet Spot Campaign", bestGrade.Label),
		Rationale: fmt.Sprintf("%s is the best-performing grade at %.0f%% ROI with %d sales, covered by %d of %d campaigns",
			bestGrade.Label, bestGrade.ROI*100, bestGrade.SoldCount, bestGrade.CampaignCount, activeCampaigns),
		Confidence: confidenceLabel(bestGrade.SoldCount),
		DataPoints: bestGrade.PurchaseCount,
		SuggestedParams: CampaignSuggestionParams{
			Name:       fmt.Sprintf("%s Focused", bestGrade.Label),
			GradeRange: gradeRangeFromLabel(bestGrade.Label),
		},
		ExpectedMetrics: ExpectedMetrics{
			ExpectedROI:       bestGrade.ROI,
			ExpectedMarginPct: bestGrade.AvgMarginPct,
			AvgDaysToSell:     bestGrade.AvgDaysToSell,
			DataConfidence:    confidenceLabel(bestGrade.SoldCount),
		},
	})

	return suggestions
}

func suggestCoverageGapCampaigns(_ context.Context, insights *PortfolioInsights) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	for _, gap := range insights.CoverageGaps {
		seg := gap.Segment
		if seg.Dimension != "character" || seg.SoldCount < suggMinSoldCoverageGap {
			continue
		}

		suggestions = append(suggestions, CampaignSuggestion{
			Type:       "gap",
			Title:      fmt.Sprintf("Coverage Gap: %s", seg.Label),
			Rationale:  gap.Reason,
			Confidence: confidenceLabel(seg.SoldCount),
			DataPoints: seg.PurchaseCount,
			SuggestedParams: CampaignSuggestionParams{
				Name:          fmt.Sprintf("%s Campaign", seg.Label),
				InclusionList: seg.Label,
				PrimaryExit:   string(seg.BestChannel),
			},
			ExpectedMetrics: ExpectedMetrics{
				ExpectedROI:       seg.ROI,
				ExpectedMarginPct: seg.AvgMarginPct,
				AvgDaysToSell:     seg.AvgDaysToSell,
				DataConfidence:    confidenceLabel(seg.SoldCount),
			},
		})
	}

	return suggestions
}

// suggestChannelInformedBuyTerms flags active campaigns whose revenue-weighted
// margin across the portfolio's actual channel mix is meaningfully below the
// target margin. When the gap exceeds suggBuyTermsReductionBuffer, it suggests
// reducing CL% by the gap (floored at suggBuyTermsFloorPct).
//
// Rationale: the previous implementation compared the *best* channel's margin
// to a flat target, which produced nonsensical recommendations (e.g. "lower
// buy terms to 28.57%") whenever any channel ran hot. Using the weighted
// average of the realized mix keeps the rule honest and only fires when the
// portfolio is actually underperforming on net.
func suggestChannelInformedBuyTerms(_ context.Context, insights *PortfolioInsights, campaigns []Campaign, now string) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	if len(insights.ByChannel) == 0 || insights.DataSummary.TotalSales < suggMinTotalSalesChannelAnalysis {
		return nil
	}

	// Revenue-weighted average margin across ALL channels.
	var totalRev, totalNet float64
	var totalSales int
	for i := range insights.ByChannel {
		ch := &insights.ByChannel[i]
		if ch.RevenueCents <= 0 {
			continue
		}
		totalRev += float64(ch.RevenueCents)
		totalNet += float64(ch.NetProfitCents)
		totalSales += ch.SaleCount
	}
	if totalRev <= 0 {
		return nil
	}
	weightedMargin := totalNet / totalRev

	// Only fire when the gap between target and realized margin is large
	// enough to matter. This preserves a safety net for quiet eBay-only
	// underperformance without generating nuisance suggestions.
	gap := suggTargetMargin - weightedMargin
	if gap < suggBuyTermsReductionBuffer {
		return nil
	}

	for _, c := range campaigns {
		if c.Phase != PhaseActive {
			continue
		}

		newTerms := c.BuyTermsCLPct - gap
		if newTerms < suggBuyTermsFloorPct {
			newTerms = suggBuyTermsFloorPct
		}
		// Never recommend terms >= current. Also skip if current is already
		// at or below the floor — nothing sensible to suggest.
		if newTerms >= c.BuyTermsCLPct {
			continue
		}

		confidence := confidenceLabelWithAge(totalSales, "", now)

		suggestions = append(suggestions, CampaignSuggestion{
			Type:  "adjust",
			Title: fmt.Sprintf("Lower buy terms on %s (margin gap)", c.Name),
			Rationale: fmt.Sprintf("Weighted-average margin across channels is %.1f%%, below the %.0f%% target. Lowering CL%% from %.0f%% to %.0f%% closes the gap.",
				weightedMargin*100, suggTargetMargin*100,
				c.BuyTermsCLPct*100, newTerms*100),
			Confidence: confidence,
			DataPoints: totalSales,
			SuggestedParams: CampaignSuggestionParams{
				Name:          c.Name,
				BuyTermsCLPct: newTerms,
			},
			ExpectedMetrics: ExpectedMetrics{
				ExpectedMarginPct: suggTargetMargin,
				DataConfidence:    confidence,
			},
		})
	}

	return suggestions
}
