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

func suggestChannelInformedBuyTerms(_ context.Context, insights *PortfolioInsights, campaigns []Campaign, now string) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	if len(insights.ByChannel) < 2 || insights.DataSummary.TotalSales < suggMinTotalSalesChannelAnalysis {
		return nil
	}

	var bestChannel *ChannelPNL
	var bestMargin float64
	for i := range insights.ByChannel {
		ch := &insights.ByChannel[i]
		if ch.SaleCount < suggMinSalesPerChannel || ch.RevenueCents <= 0 {
			continue
		}
		margin := float64(ch.NetProfitCents) / float64(ch.RevenueCents)
		if bestChannel == nil || margin > bestMargin {
			bestChannel = ch
			bestMargin = margin
		}
	}

	if bestChannel == nil || bestMargin <= 0 {
		return nil
	}

	for _, c := range campaigns {
		if c.Phase != PhaseActive {
			continue
		}

		var feePct float64
		if isMarketplaceChannel(bestChannel.Channel) {
			feePct = c.EbayFeePct
			if feePct == 0 {
				feePct = DefaultMarketplaceFeePct
			}
		}

		targetMargin := suggTargetMargin

		maxBuy := bestMargin - targetMargin - feePct
		if maxBuy <= 0 {
			continue
		}

		if c.BuyTermsCLPct > maxBuy+suggBuyTermsBuffer {
			confidence := confidenceLabelWithAge(bestChannel.SaleCount, "", now)

			suggestions = append(suggestions, CampaignSuggestion{
				Type:  "adjust",
				Title: fmt.Sprintf("Lower buy terms on %s", c.Name),
				Rationale: fmt.Sprintf("Best channel (%s) margin is %.0f%%. With %.0f%% fees and 10%% target margin, max buy should be ~%.0f%% CL. Current: %.0f%%.",
					bestChannel.Channel, bestMargin*100, feePct*100, maxBuy*100, c.BuyTermsCLPct*100),
				Confidence: confidence,
				DataPoints: bestChannel.SaleCount,
				SuggestedParams: CampaignSuggestionParams{
					Name:          c.Name,
					BuyTermsCLPct: maxBuy,
				},
				ExpectedMetrics: ExpectedMetrics{
					ExpectedMarginPct: targetMargin,
					DataConfidence:    confidence,
				},
			})
		}
	}

	return suggestions
}
