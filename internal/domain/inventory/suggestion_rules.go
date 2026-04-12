package inventory

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
			Rationale: fmt.Sprintf("%s has %.0f%% ROI across %d sales in %d inventory. Adding to: %s",
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

// suggestBuyTermsFromLiquidation inspects per-campaign liquidation damage and
// recommends a CL buy-terms reduction sized to absorb the observed inperson
// and cardshow losses.
//
// Triggers only when real damage is observed on an active campaign (not a
// theoretical margin gap), with a sample-size guard to avoid reacting to one
// bad sale. Each 1% reduction in buy terms frees ~$5 of margin on a $500 card,
// which both improves eBay profitability and widens the liquidation buffer.
//
// Distinct from suggestChannelInformedBuyTerms: that rule reacts to portfolio-
// wide realized margin vs. target; this rule reacts to actual strictly-negative
// inperson/cardshow sales on a specific campaign.
func suggestBuyTermsFromLiquidation(_ context.Context, campaigns []Campaign, healthByCampaign map[string]CampaignHealth) []CampaignSuggestion {
	if len(healthByCampaign) == 0 {
		return nil
	}

	var suggestions []CampaignSuggestion
	for _, c := range campaigns {
		if c.Phase != PhaseActive {
			continue
		}
		h, ok := healthByCampaign[c.ID]
		if !ok {
			continue
		}
		// LiquidationLossCents is stored as a non-positive number (sum of
		// strictly-negative net profit on liquidation channels). Compare its
		// magnitude to the threshold.
		if -h.LiquidationLossCents < suggLiquidationLossThresholdCents {
			continue
		}
		if h.LiquidationSaleCount < suggLiquidationMinSampleSize {
			continue
		}
		// Only suggest "bake in a margin buffer" when the marketplace channel
		// is actually profitable. If eBay/TCGPlayer margin is zero (no data)
		// or negative (the whole channel is broken), lowering buy terms won't
		// fix the campaign — recommending it would be misleading. The zero
		// case also excludes campaigns with no marketplace sales at all,
		// where we don't have enough data to confidently recommend a target.
		if h.EbayChannelMarginPct <= 0 {
			continue
		}

		reduction := computeBuyTermsReduction(h)
		newTerms := c.BuyTermsCLPct - reduction
		if newTerms < suggLiquidationFloorPct {
			newTerms = suggLiquidationFloorPct
		}
		// Never recommend terms at or above current. Also skips campaigns
		// whose current terms are already at or below the floor.
		if newTerms >= c.BuyTermsCLPct {
			continue
		}

		confidence := "medium"
		if h.LiquidationSaleCount >= 10 {
			confidence = "high"
		}

		appliedReductionPct := (c.BuyTermsCLPct - newTerms) * 100

		suggestions = append(suggestions, CampaignSuggestion{
			Type:  "adjust",
			Title: fmt.Sprintf("Lower buy terms on %s (liquidation buffer)", c.Name),
			Rationale: fmt.Sprintf(
				"Campaign has $%.2f in liquidation losses across %d sales. Lowering CL%% from %.0f%% to %.0f%% creates a %.0f-point margin buffer on every fill, improving both eBay margin and liquidation tolerance.",
				float64(-h.LiquidationLossCents)/100,
				h.LiquidationSaleCount,
				c.BuyTermsCLPct*100,
				newTerms*100,
				appliedReductionPct,
			),
			Confidence: confidence,
			DataPoints: h.LiquidationSaleCount,
			SuggestedParams: CampaignSuggestionParams{
				Name:          c.Name,
				BuyTermsCLPct: newTerms,
			},
			ExpectedMetrics: ExpectedMetrics{
				DataConfidence: confidence,
			},
		})
	}

	return suggestions
}

// computeBuyTermsReduction maps observed average liquidation loss per sale
// into a deterministic buy-terms reduction bucket. The thresholds are tuned
// so that a typical $500 card with a 15% liquidation hit (~$75/sale) lands in
// the 8% bucket.
func computeBuyTermsReduction(h CampaignHealth) float64 {
	if h.LiquidationSaleCount <= 0 {
		return 0
	}
	avgLossCents := float64(-h.LiquidationLossCents) / float64(h.LiquidationSaleCount)
	switch {
	case avgLossCents > 5000: // > $50/sale
		return 0.08
	case avgLossCents > 3000: // $30–50/sale
		return 0.05
	case avgLossCents > 1500: // $15–30/sale
		return 0.03
	default:
		return 0.02
	}
}
