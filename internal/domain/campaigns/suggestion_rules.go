package campaigns

import (
	"fmt"
	"sort"
	"strings"
)

// Rule 1: Top Character Expansion
func suggestTopCharacterExpansion(insights *PortfolioInsights, campaigns []Campaign) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	for _, seg := range insights.ByCharacter {
		if seg.ROI <= suggMinROIExpansion || seg.SoldCount < suggMinSoldForConfidence || seg.Label == "Other" {
			continue
		}
		if seg.CampaignCount >= suggMaxCampaignsPerCharacter {
			continue
		}

		// Check which campaigns don't include this character
		var missingCampaigns []string
		for _, c := range campaigns {
			if c.Phase != PhaseActive {
				continue
			}
			if c.InclusionList == "" {
				if c.ExclusionMode {
					// Empty exclusion list means nothing is excluded — campaign covers this character
					continue
				}
				// Empty inclusion list with inclusion mode — skip, campaign has no explicit list
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
				// Exclusion mode: if character is in list, it's blocked
				if inList {
					missingCampaigns = append(missingCampaigns, c.Name)
				}
			} else {
				// Inclusion mode: if character is not in list, it's missing
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

// Rule 2: Grade Sweet Spot
func suggestGradeSweetSpot(insights *PortfolioInsights, campaigns []Campaign) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	// Find best-performing grade
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

// Rule 3: Coverage Gap Campaigns
func suggestCoverageGapCampaigns(insights *PortfolioInsights) []CampaignSuggestion {
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

// Rule 4: Channel-Informed Buy Terms (with GameStop payout range)
func suggestChannelInformedBuyTerms(insights *PortfolioInsights, campaigns []Campaign, now string) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	if len(insights.ByChannel) < 2 || insights.DataSummary.TotalSales < suggMinTotalSalesChannelAnalysis {
		return nil
	}

	// Find best channel by margin (netProfit / revenue)
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

	isGameStop := bestChannel.Channel == SaleChannelGameStop

	// For each active campaign, check if buy terms are aggressive relative to channel margins
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

		if isGameStop {
			// GameStop pays 70-90% of CL with 0% fees
			conservativeMaxBuy := GameStopPayoutMinPct - targetMargin
			optimisticMaxBuy := GameStopPayoutMaxPct - targetMargin

			if c.BuyTermsCLPct > conservativeMaxBuy+suggBuyTermsBuffer {
				confidence := confidenceLabelWithAge(bestChannel.SaleCount, "", now)
				suggestions = append(suggestions, CampaignSuggestion{
					Type:  "adjust",
					Title: fmt.Sprintf("Lower buy terms on %s", c.Name),
					Rationale: fmt.Sprintf("Best channel (GameStop) pays 70-90%% of CL. Conservative max buy: ~%.0f%% CL, optimistic: ~%.0f%% CL. Current: %.0f%%.",
						conservativeMaxBuy*100, optimisticMaxBuy*100, c.BuyTermsCLPct*100),
					Confidence: confidence,
					DataPoints: bestChannel.SaleCount,
					SuggestedParams: CampaignSuggestionParams{
						Name:                    c.Name,
						BuyTermsCLPct:           conservativeMaxBuy,
						BuyTermsCLPctOptimistic: optimisticMaxBuy,
					},
					ExpectedMetrics: ExpectedMetrics{
						ExpectedMarginPct: targetMargin,
						DataConfidence:    confidence,
					},
				})
			}
		} else {
			// Standard channel: maxBuy = margin - target - fees
			maxBuy := bestMargin - targetMargin - feePct
			if maxBuy <= 0 {
				continue
			}

			if c.BuyTermsCLPct > maxBuy+suggBuyTermsBuffer {
				// ChannelPNL does not carry a latest sale date, so age decay is skipped.
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
	}

	return suggestions
}

// Rule 5: Spend Cap Rebalancing (ROI-weighted)
func suggestSpendCapRebalancing(insights *PortfolioInsights, campaigns []Campaign) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	if insights.DataSummary.TotalPurchases < suggMinPurchasesSpendCap || insights.DataSummary.OverallROI < 0 {
		return nil
	}

	// Build per-campaign ROI map
	campaignROI := make(map[string]float64)
	for _, m := range insights.CampaignMetrics {
		campaignROI[m.CampaignID] = m.ROI
	}

	// Collect active campaigns with spend caps
	type capEntry struct {
		campaign Campaign
		cap      int
		roi      float64
	}
	var active []capEntry
	for _, c := range campaigns {
		if c.Phase != PhaseActive || c.DailySpendCapCents <= 0 {
			continue
		}
		roi := campaignROI[c.ID]
		active = append(active, capEntry{campaign: c, cap: c.DailySpendCapCents, roi: roi})
	}

	if len(active) < 2 {
		return nil
	}

	// Find min and max spend caps
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

	// If spend caps differ by more than 3x, suggest ROI-weighted rebalancing
	if maxCap > minCap*3 && minCap > 0 {
		// Compute total daily budget
		totalBudget := 0
		for _, e := range active {
			totalBudget += e.cap
		}

		// Check if we have per-campaign ROI data for weighting
		hasROIData := len(campaignROI) > 0
		if hasROIData {
			// ROI-weighted: suggest per-campaign adjustments
			// For high-ROI campaigns, suggest increasing cap; for low-ROI, suggest decreasing
			avgROI := insights.DataSummary.OverallROI
			var adjustments []string
			for _, e := range active {
				if e.roi > avgROI+suggROIDeviation && e.cap < totalBudget/len(active)*2 {
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
						DataConfidence: "low",
					},
				})
				return suggestions
			}
		}

		// Fallback: simple average suggestion when no ROI weighting applies
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
				DataConfidence: "low",
			},
		})
	}

	return suggestions
}

// Rule 6: Character Adjustments for existing campaigns
func suggestCharacterAdjustments(insights *PortfolioInsights, campaigns []Campaign) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	if len(insights.ByCharacter) < suggMinCharacterSegments {
		return nil
	}

	// Sort characters by ROI
	sorted := make([]SegmentPerformance, len(insights.ByCharacter))
	copy(sorted, insights.ByCharacter)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ROI > sorted[j].ROI
	})

	// Find underperforming characters that are in active campaign inclusion lists
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

			// If in the list but underperforming, suggest removal
			if inList && seg.ROI < suggUnderperformingROI && seg.SoldCount >= suggMinSoldForRemoval {
				removes = append(removes, seg.Label)
			}

			// If not in the list but top performer, suggest addition
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
					DataConfidence: "medium",
				},
			})
		}

		if len(adds) > 0 {
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
					DataConfidence: "medium",
				},
			})
		}
	}

	return suggestions
}

// Rule 7: Phase Transition Suggestions
func suggestPhaseTransitions(insights *PortfolioInsights, campaigns []Campaign) []CampaignSuggestion {
	var suggestions []CampaignSuggestion

	if len(insights.CampaignMetrics) == 0 {
		return nil
	}

	// Build per-campaign metrics map
	metricsMap := make(map[string]CampaignPNLBrief)
	for _, m := range insights.CampaignMetrics {
		metricsMap[m.CampaignID] = m
	}

	for _, c := range campaigns {
		m, ok := metricsMap[c.ID]
		if !ok {
			continue
		}

		// Suggest archiving: active campaign with poor performance
		if c.Phase == PhaseActive && m.PurchaseCount >= suggArchiveMinPurchases && m.ROI < suggArchiveROIThreshold {
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
					Confidence: confidenceLabel(m.SoldCount),
					DataPoints: m.PurchaseCount,
					SuggestedParams: CampaignSuggestionParams{
						Name: c.Name,
					},
					ExpectedMetrics: ExpectedMetrics{
						ExpectedROI:    m.ROI,
						DataConfidence: confidenceLabel(m.SoldCount),
					},
				})
			}
		}

		// Suggest activating: pending campaign with profitable segment data
		if c.Phase == PhasePending && c.InclusionList != "" {
			// Check if any characters in the pending campaign's inclusion list are profitable
			var profitableChars []string
			for _, seg := range insights.ByCharacter {
				if seg.ROI > suggActivateMinROI && seg.SoldCount >= suggMinSoldForConfidence {
					for _, name := range SplitInclusionList(c.InclusionList) {
						if strings.EqualFold(strings.TrimSpace(name), seg.Label) {
							profitableChars = append(profitableChars, fmt.Sprintf("%s (%.0f%% ROI)", seg.Label, seg.ROI*100))
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
						DataConfidence: "medium",
					},
				})
			}
		}
	}

	return suggestions
}
