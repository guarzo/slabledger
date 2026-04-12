package inventory

import (
	"context"
	"fmt"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	// HighSpendCapCents is the daily spend cap threshold (in cents) above which
	// a warning is emitted that a single fill could be significant.
	HighSpendCapCents = 500000 // $5,000/day
)

// GetCrackCandidates returns cached crack candidates for a single campaign.
// Returns an empty slice if the cache has not been populated yet (background worker still running).
func (s *service) GetCrackCandidates(ctx context.Context, campaignID string) ([]CrackAnalysis, error) {
	// Validate campaign exists
	if _, err := s.campaigns.GetCampaign(ctx, campaignID); err != nil {
		return nil, err
	}

	s.crackCacheMu.RLock()
	all := s.crackCacheAll
	s.crackCacheMu.RUnlock()

	if all == nil {
		if s.logger != nil {
			s.logger.Info(ctx, "crack cache not yet populated, returning empty list")
		}
		return []CrackAnalysis{}, nil
	}

	results := []CrackAnalysis{}
	for _, c := range all {
		if c.CampaignID == campaignID {
			results = append(results, c)
		}
	}
	return results, nil
}

// crackCandidatesForCampaign computes crack candidates using an already-loaded campaign,
// avoiding a redundant GetCampaign call when the caller already has the campaign.
func (s *service) crackCandidatesForCampaign(ctx context.Context, campaign *Campaign) ([]CrackAnalysis, error) {
	unsold, err := s.purchases.ListUnsoldPurchases(ctx, campaign.ID)
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
			continue
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
			continue
		}
		if gradedCents == 0 {
			gradedCents = p.CLValueCents
		}

		analysis := computeCrackAnalysis(
			p.ID, campaign.ID, p.CardName, p.CertNumber, p.GradeValue,
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
	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	capitalRaw, err := s.finance.GetCapitalRawData(ctx)
	if err != nil {
		return nil, err
	}
	capital := ComputeCapitalSummary(capitalRaw)

	invoices, err := s.finance.ListInvoices(ctx)
	if err != nil {
		return nil, err
	}

	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, err
	}

	checklist := &ActivationChecklist{
		CampaignID:   campaign.ID,
		CampaignName: campaign.Name,
		AllPassed:    true,
	}

	exposureCheckOK := capital.AlertLevel != AlertCritical
	checklist.Checks = append(checklist.Checks, ActivationCheck{
		Name:    "Capital Exposure",
		Passed:  exposureCheckOK,
		Message: fmt.Sprintf("Recovery velocity: alert=%s, weeks-to-cover=%.1f", capital.AlertLevel, capital.WeeksToCover),
	})
	checklist.AllPassed = checklist.AllPassed && exposureCheckOK

	hasPaidInvoice := false
	for _, inv := range invoices {
		if inv.Status == "paid" {
			hasPaidInvoice = true
			break
		}
	}
	invoiceMsg := "No completed invoice cycles yet — consider waiting before activating high-value campaigns"
	if hasPaidInvoice {
		invoiceMsg = "At least one invoice cycle has been completed and paid"
	}
	checklist.Checks = append(checklist.Checks, ActivationCheck{
		Name:    "Invoice Cycle Cleared",
		Passed:  hasPaidInvoice,
		Message: invoiceMsg,
	})
	checklist.AllPassed = checklist.AllPassed && hasPaidInvoice

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

	// Quantitative check: daily exposure must fit within weekly recovery capacity.
	// Stricter than Capital Exposure (which only blocks on critical alert level)
	// because this gates new spend commitments, not existing position.
	dailyRecovery := float64(capital.RecoveryRate30dCents) / 30.0
	dailyExpOK := capital.WeeksToCover == 0 || float64(totalDailyExposure) < dailyRecovery
	exposureMsg := fmt.Sprintf("Total daily exposure with activation: $%d/day (daily recovery: $%d)", totalDailyExposure/100, int(dailyRecovery)/100)
	if capital.RecoveryRate30dCents == 0 {
		dailyExpOK = true
		exposureMsg = fmt.Sprintf("Total daily exposure with activation: $%d/day (no recovery data yet)", totalDailyExposure/100)
	}
	checklist.Checks = append(checklist.Checks, ActivationCheck{
		Name:    "Daily Exposure",
		Passed:  dailyExpOK,
		Message: exposureMsg,
	})
	checklist.AllPassed = checklist.AllPassed && dailyExpOK

	// Warn on high-value campaigns: a single fill could be significant
	if campaign.DailySpendCapCents >= HighSpendCapCents {
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

// GetCrackOpportunities returns cached cross-campaign crack opportunities.
// Returns an empty slice if the cache has not been populated yet or there are no opportunities.
// Returns a zero-length slice and an error on failure.
func (s *service) GetCrackOpportunities(ctx context.Context) ([]CrackAnalysis, error) {
	s.crackCacheMu.RLock()
	all := s.crackCacheAll
	s.crackCacheMu.RUnlock()

	if all == nil {
		if s.logger != nil {
			s.logger.Info(ctx, "crack cache not yet populated, returning empty list")
		}
		return []CrackAnalysis{}, nil
	}

	// Return a copy to prevent callers from mutating the cache.
	results := make([]CrackAnalysis, len(all))
	copy(results, all)
	return results, nil
}

// computeCrackOpportunitiesLive computes crack opportunities by making live API calls.
// This is called ONLY by the background cache worker — never during page loads.
func (s *service) computeCrackOpportunitiesLive(ctx context.Context) ([]CrackAnalysis, error) {
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	var allResults []CrackAnalysis
	for _, campaign := range allCampaigns {
		results, err := s.crackCandidatesForCampaign(ctx, &campaign)
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
		if s.logger != nil {
			s.logger.Info(ctx, "skipping acquisition targets: price provider not configured")
		}
		return []AcquisitionOpportunity{}, nil
	}
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	opportunities := []AcquisitionOpportunity{}
	seen := make(map[string]bool)
	for _, campaign := range allCampaigns {
		ebayFee := campaign.EbayFeePct
		if ebayFee == 0 {
			ebayFee = DefaultMarketplaceFeePct
		}
		unsold, err := s.purchases.ListUnsoldPurchases(ctx, campaign.ID)
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
