package inventory

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
)

func suggestSpendCapRebalancing(_ context.Context, insights *PortfolioInsights, campaigns []Campaign) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	if insights.DataSummary.TotalPurchases < suggMinPurchasesSpendCap || insights.DataSummary.OverallROI < 0 {
		return nil
	}

	campaignROI := make(map[string]float64)
	for _, m := range insights.CampaignMetrics {
		campaignROI[m.CampaignID] = m.ROI
	}

	type capEntry struct {
		campaign Campaign
		cap      int
		roi      float64
	}
	hasROIData := len(campaignROI) > 0
	var active []capEntry
	for _, c := range campaigns {
		if c.Phase != PhaseActive || c.DailySpendCapCents <= 0 {
			continue
		}
		roi, ok := campaignROI[c.ID]
		if hasROIData && !ok {
			continue
		}
		active = append(active, capEntry{campaign: c, cap: c.DailySpendCapCents, roi: roi})
	}

	if len(active) < 2 {
		return nil
	}

	minCap := active[0].cap
	maxCap := active[0].cap
	minEntry := active[0]
	maxEntry := active[0]
	for _, e := range active {
		if e.cap < minCap {
			minCap = e.cap
			minEntry = e
		}
		if e.cap > maxCap {
			maxCap = e.cap
			maxEntry = e
		}
	}

	if maxCap > minCap*3 && minCap > 0 {
		totalBudget := 0
		for _, e := range active {
			totalBudget += e.cap
		}

		if hasROIData {
			avgROI := insights.DataSummary.OverallROI
			var adjustments []string
			for _, e := range active {
				if e.roi > avgROI+suggROIDeviation && e.cap < totalBudget*2/len(active) {
					adjustments = append(adjustments, fmt.Sprintf("raise %s ($%d/day, %.0f%% ROI)", e.campaign.Name, e.cap/100, e.roi*100))
				} else if e.roi < avgROI-suggROIDeviation && e.cap > totalBudget/len(active)/2 {
					adjustments = append(adjustments, fmt.Sprintf("lower %s ($%d/day, %.0f%% ROI)", e.campaign.Name, e.cap/100, e.roi*100))
				}
			}

			if len(adjustments) > 0 {
				avgCap := totalBudget / len(active)
				suggestions = append(suggestions, CampaignSuggestion{
					Type:  "adjust",
					Title: "Rebalance spend caps by ROI",
					Rationale: fmt.Sprintf("Spend caps vary widely: %s at $%d/day vs %s at $%d/day. Based on per-campaign ROI: %s.",
						minEntry.campaign.Name, minCap/100, maxEntry.campaign.Name, maxCap/100, strings.Join(adjustments, "; ")),
					Confidence: "low",
					DataPoints: insights.DataSummary.TotalPurchases,
					SuggestedParams: CampaignSuggestionParams{
						DailySpendCapCents: avgCap,
					},
					ExpectedMetrics: ExpectedMetrics{
						ExpectedROI:    avgROI,
						DataConfidence: "low",
					},
				})
				return suggestions
			}
		}

		avgCap := totalBudget / len(active)
		suggestions = append(suggestions, CampaignSuggestion{
			Type:  "adjust",
			Title: "Rebalance spend caps",
			Rationale: fmt.Sprintf("Spend caps vary widely: %s at $%d/day vs %s at $%d/day. With %.0f%% overall ROI, consider evening out allocation.",
				minEntry.campaign.Name, minCap/100, maxEntry.campaign.Name, maxCap/100, insights.DataSummary.OverallROI*100),
			Confidence: "low",
			DataPoints: insights.DataSummary.TotalPurchases,
			SuggestedParams: CampaignSuggestionParams{
				DailySpendCapCents: avgCap,
			},
			ExpectedMetrics: ExpectedMetrics{
				ExpectedROI:    insights.DataSummary.OverallROI,
				DataConfidence: "low",
			},
		})
	}

	return suggestions
}

func suggestCharacterAdjustments(_ context.Context, insights *PortfolioInsights, campaigns []Campaign) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	if len(insights.ByCharacter) < suggMinCharacterSegments {
		return nil
	}

	sorted := make([]SegmentPerformance, len(insights.ByCharacter))
	copy(sorted, insights.ByCharacter)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ROI > sorted[j].ROI
	})

	for _, c := range campaigns {
		if c.Phase != PhaseActive || c.InclusionList == "" {
			continue
		}

		var removes []string
		var adds []string
		inclChars := SplitInclusionList(c.InclusionList)

		for _, seg := range sorted {
			if seg.Label == "Other" || seg.PurchaseCount < suggMinSoldForConfidence {
				continue
			}
			inList := false
			for _, name := range inclChars {
				if strings.EqualFold(strings.TrimSpace(name), seg.Label) {
					inList = true
					break
				}
			}

			if inList && seg.ROI < suggUnderperformingROI && seg.SoldCount >= suggMinSoldForRemoval {
				removes = append(removes, seg.Label)
			}

			if !inList && seg.ROI > suggMinROIExpansion && seg.SoldCount >= suggMinSoldForConfidence && len(adds) < suggMaxCampaignsPerCharacter {
				adds = append(adds, seg.Label)
			}
		}

		if len(removes) > 0 {
			suggestions = append(suggestions, CampaignSuggestion{
				Type:  "adjust",
				Title: fmt.Sprintf("Remove underperformers from %s", c.Name),
				Rationale: fmt.Sprintf("Characters %s have underperforming ROI in campaign %s. Consider removing from inclusion list.",
					strings.Join(removes, ", "), c.Name),
				Confidence: "medium",
				DataPoints: insights.DataSummary.TotalPurchases,
				SuggestedParams: CampaignSuggestionParams{
					Name:          c.Name,
					InclusionList: fmt.Sprintf("remove: %s", strings.Join(removes, ", ")),
				},
				ExpectedMetrics: ExpectedMetrics{
					// Removing a segment — expected improvement is directional only.
					ExpectedROI:    0,
					DataConfidence: "medium",
				},
			})
		}

		if len(adds) > 0 {
			// Find the best matching segment in `sorted` for the top add
			// (adds[0] is the highest-ROI entry because `sorted` is ordered
			// by ROI descending and adds preserves that order).
			var expectedROI, expectedMargin, avgDays float64
			for _, seg := range sorted {
				if strings.EqualFold(strings.TrimSpace(seg.Label), adds[0]) {
					expectedROI = seg.ROI
					expectedMargin = seg.AvgMarginPct
					avgDays = seg.AvgDaysToSell
					break
				}
			}

			suggestions = append(suggestions, CampaignSuggestion{
				Type:  "adjust",
				Title: fmt.Sprintf("Add top performers to %s", c.Name),
				Rationale: fmt.Sprintf("Characters %s are top performers not in campaign %s. Consider adding.",
					strings.Join(adds, ", "), c.Name),
				Confidence: "medium",
				DataPoints: insights.DataSummary.TotalPurchases,
				SuggestedParams: CampaignSuggestionParams{
					Name:          c.Name,
					InclusionList: fmt.Sprintf("add: %s", strings.Join(adds, ", ")),
				},
				ExpectedMetrics: ExpectedMetrics{
					ExpectedROI:       expectedROI,
					ExpectedMarginPct: expectedMargin,
					AvgDaysToSell:     avgDays,
					DataConfidence:    "medium",
				},
			})
		}
	}

	return suggestions
}

func suggestPhaseTransitions(_ context.Context, insights *PortfolioInsights, campaigns []Campaign) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	// metricsMap is only needed for PhaseActive close suggestions
	metricsMap := make(map[string]CampaignPNLBrief)
	for _, m := range insights.CampaignMetrics {
		metricsMap[m.CampaignID] = m
	}

	for _, c := range campaigns {
		if c.Phase == PhaseActive {
			m, ok := metricsMap[c.ID]
			if !ok {
				continue
			}
			if m.PurchaseCount >= suggArchiveMinPurchases && m.ROI < suggArchiveROIThreshold {
				sellThrough := 0.0
				if m.PurchaseCount > 0 {
					sellThrough = float64(m.SoldCount) / float64(m.PurchaseCount)
				}
				if sellThrough < suggLowSellThroughPct {
					suggestions = append(suggestions, CampaignSuggestion{
						Type:  "adjust",
						Title: fmt.Sprintf("Consider closing %s", c.Name),
						Rationale: fmt.Sprintf("%s has %.0f%% ROI with %.0f%% sell-through across %d purchases. Performance is below viable thresholds.",
							c.Name, m.ROI*100, sellThrough*100, m.PurchaseCount),
						Confidence: mathutil.ConfidenceLabel(m.SoldCount),
						DataPoints: m.PurchaseCount,
						SuggestedParams: CampaignSuggestionParams{
							Name: c.Name,
						},
						ExpectedMetrics: ExpectedMetrics{
							ExpectedROI:    m.ROI,
							DataConfidence: mathutil.ConfidenceLabel(m.SoldCount),
						},
					})
				}
			}
		}

		// PhasePending activation uses insights.ByCharacter directly — no metrics needed
		if c.Phase == PhasePending && c.InclusionList != "" {
			var profitableChars []string
			var bestROI float64
			inclNames := SplitInclusionList(c.InclusionList)
			for _, seg := range insights.ByCharacter {
				if seg.ROI > suggActivateMinROI && seg.SoldCount >= suggMinSoldForConfidence {
					for _, name := range inclNames {
						if strings.EqualFold(strings.TrimSpace(name), seg.Label) {
							profitableChars = append(profitableChars, fmt.Sprintf("%s (%.0f%% ROI)", seg.Label, seg.ROI*100))
							if seg.ROI > bestROI {
								bestROI = seg.ROI
							}
						}
					}
				}
			}
			if len(profitableChars) > 0 {
				suggestions = append(suggestions, CampaignSuggestion{
					Type:  "adjust",
					Title: fmt.Sprintf("Activate %s", c.Name),
					Rationale: fmt.Sprintf("%s targets profitable characters: %s. Consider activating.",
						c.Name, strings.Join(profitableChars, ", ")),
					Confidence: "medium",
					DataPoints: insights.DataSummary.TotalSales,
					SuggestedParams: CampaignSuggestionParams{
						Name: c.Name,
					},
					ExpectedMetrics: ExpectedMetrics{
						ExpectedROI:    bestROI,
						DataConfidence: "medium",
					},
				})
			}
		}
	}

	return suggestions
}
