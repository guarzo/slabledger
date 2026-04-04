package campaigns

import (
	"context"
	"fmt"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) GetCrackCandidates(ctx context.Context, campaignID string) ([]CrackAnalysis, error) {
	campaign, err := s.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	unsold, err := s.repo.ListUnsoldPurchases(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	ebayFee := campaign.EbayFeePct
	if ebayFee == 0 {
		ebayFee = DefaultMarketplaceFeePct
	}

	var results []CrackAnalysis
	for _, p := range unsold {
		if p.GradeValue > 8 {
			continue // Only analyze PSA 8 and below
		}

		card := p.ToCardIdentity()

		rawCents := 0
		gradedCents := 0
		if s.priceProv != nil {
			if v, err := s.priceProv.GetLastSoldCents(ctx, card, 0); err == nil {
				rawCents = v
			}
			if v, err := s.priceProv.GetLastSoldCents(ctx, card, p.GradeValue); err == nil {
				gradedCents = v
			}
		}

		if rawCents == 0 {
			continue // Can't analyze without raw price
		}
		if gradedCents == 0 {
			gradedCents = p.CLValueCents // Fall back to CL value
		}

		analysis := computeCrackAnalysis(
			p.ID, p.CardName, p.CertNumber, p.GradeValue,
			p.BuyCostCents, p.PSASourcingFeeCents, rawCents, gradedCents,
			ebayFee,
		)
		results = append(results, *analysis)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CrackAdvantage > results[j].CrackAdvantage
	})

	return results, nil
}

func (s *service) GetActivationChecklist(ctx context.Context, campaignID string) (*ActivationChecklist, error) {
	campaign, err := s.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	credit, err := s.repo.GetCreditSummary(ctx)
	if err != nil {
		return nil, err
	}

	invoices, err := s.repo.ListInvoices(ctx)
	if err != nil {
		return nil, err
	}

	allCampaigns, err := s.repo.ListCampaigns(ctx, true)
	if err != nil {
		return nil, err
	}

	checklist := &ActivationChecklist{
		CampaignID:   campaign.ID,
		CampaignName: campaign.Name,
		AllPassed:    true,
	}

	utilizationOK := credit.UtilizationPct < 70
	checklist.Checks = append(checklist.Checks, ActivationCheck{
		Name:    "Credit Utilization",
		Passed:  utilizationOK,
		Message: fmt.Sprintf("Current utilization: %.0f%% (threshold: 70%%)", credit.UtilizationPct),
	})
	if !utilizationOK {
		checklist.AllPassed = false
	}

	hasPaidInvoice := false
	for _, inv := range invoices {
		if inv.Status == "paid" {
			hasPaidInvoice = true
			break
		}
	}
	checklist.Checks = append(checklist.Checks, ActivationCheck{
		Name:   "Invoice Cycle Cleared",
		Passed: hasPaidInvoice,
		Message: func() string {
			if hasPaidInvoice {
				return "At least one invoice cycle has been completed and paid"
			}
			return "No completed invoice cycles yet — consider waiting before activating high-value campaigns"
		}(),
	})
	if !hasPaidInvoice {
		checklist.AllPassed = false
	}

	totalDailyExposure := 0
	alreadyIncluded := false
	for _, c := range allCampaigns {
		totalDailyExposure += c.DailySpendCapCents
		if c.ID == campaign.ID {
			alreadyIncluded = true
		}
	}
	if !alreadyIncluded {
		totalDailyExposure += campaign.DailySpendCapCents
	}

	exposureOK := credit.CreditLimitCents == 0 || totalDailyExposure < credit.CreditLimitCents/10 // daily exposure < 10% of limit; skip if unconfigured
	exposureMsg := fmt.Sprintf("Total daily exposure with activation: $%d/day (credit limit: $%d)", totalDailyExposure/100, credit.CreditLimitCents/100)
	if credit.CreditLimitCents == 0 {
		exposureMsg = fmt.Sprintf("Total daily exposure with activation: $%d/day (credit limit: not configured)", totalDailyExposure/100)
	}
	checklist.Checks = append(checklist.Checks, ActivationCheck{
		Name:    "Daily Exposure",
		Passed:  exposureOK,
		Message: exposureMsg,
	})
	if !exposureOK {
		checklist.AllPassed = false
	}

	// Warn on high-value campaigns: a single fill could be significant
	if campaign.DailySpendCapCents >= 500000 { // $5,000/day
		checklist.Warnings = append(checklist.Warnings,
			fmt.Sprintf("This campaign has a $%d/day spend cap — a single fill could be significant", campaign.DailySpendCapCents/100))
	}

	unpaidCount := 0
	for _, inv := range invoices {
		if inv.Status == "unpaid" {
			unpaidCount++
		}
	}
	if unpaidCount > 0 {
		checklist.Warnings = append(checklist.Warnings,
			fmt.Sprintf("%d unpaid invoice(s) outstanding", unpaidCount))
	}

	return checklist, nil
}

func (s *service) GetCrackOpportunities(ctx context.Context) ([]CrackAnalysis, error) {
	allCampaigns, err := s.repo.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	var allResults []CrackAnalysis
	for _, campaign := range allCampaigns {
		results, err := s.GetCrackCandidates(ctx, campaign.ID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "crack candidates failed for campaign",
					observability.String("campaignId", campaign.ID),
					observability.Err(err))
			}
			continue
		}
		allResults = append(allResults, results...)
	}
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].CrackAdvantage > allResults[j].CrackAdvantage
	})
	return allResults, nil
}

func (s *service) GetAcquisitionTargets(ctx context.Context) ([]AcquisitionOpportunity, error) {
	if s.priceProv == nil {
		return nil, nil
	}
	allCampaigns, err := s.repo.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	var opportunities []AcquisitionOpportunity
	seen := make(map[string]bool)
	for _, campaign := range allCampaigns {
		ebayFee := campaign.EbayFeePct
		if ebayFee == 0 {
			ebayFee = DefaultMarketplaceFeePct
		}
		unsold, err := s.repo.ListUnsoldPurchases(ctx, campaign.ID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "list unsold purchases failed for campaign",
					observability.String("campaignId", campaign.ID),
					observability.Err(err))
			}
			continue
		}
		for _, p := range unsold {
			key := p.CardName + "|" + p.SetName + "|" + p.CardNumber
			if seen[key] {
				continue
			}
			seen[key] = true
			card := p.ToCardIdentity()
			rawNMCents := 0
			if v, err := s.priceProv.GetLastSoldCents(ctx, card, 0); err == nil && v > 0 {
				rawNMCents = v
			}
			if rawNMCents == 0 {
				continue
			}
			gradedEstimates := make(map[string]int)
			for _, grade := range []float64{8, 9, 10} {
				if v, err := s.priceProv.GetLastSoldCents(ctx, card, grade); err == nil && v > 0 {
					gradedEstimates[fmt.Sprintf("PSA %g", grade)] = v
				}
			}
			opp := computeAcquisitionOpportunity(
				p.CardName, p.SetName, p.CardNumber, p.CertNumber,
				rawNMCents, gradedEstimates, ebayFee, "inventory",
			)
			if opp != nil {
				opportunities = append(opportunities, *opp)
			}
		}
	}
	sortAcquisitionByProfit(opportunities)
	return opportunities, nil
}
