package campaigns

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// --- Cert Lookup & Quick Add ---

func (s *service) LookupCert(ctx context.Context, certNumber string) (*CertInfo, *MarketSnapshot, error) {
	if s.certLookup == nil {
		return nil, nil, ErrCertLookupNotConfigured
	}

	info, err := s.certLookup.LookupCert(ctx, certNumber)
	if err != nil {
		return nil, nil, fmt.Errorf("cert lookup: %w", err)
	}

	var snapshot *MarketSnapshot
	if s.priceProv != nil && info.CardName != "" && info.Grade > 0 {
		resolvedCategory := resolvePSACategory(info.Category)
		if isGenericSetName(resolvedCategory) {
			resolvedCategory = info.Category
		}
		snapshot, err = s.priceProv.GetMarketSnapshot(ctx, CardIdentity{CardName: info.CardName, CardNumber: info.CardNumber, SetName: resolvedCategory}, info.Grade)
		if err != nil && s.logger != nil {
			s.logger.Debug(ctx, "GetMarketSnapshot for cert lookup failed",
				observability.String("card", info.CardName),
				observability.Err(err))
		}
	}

	return info, snapshot, nil
}

func (s *service) QuickAddPurchase(ctx context.Context, campaignID string, req QuickAddRequest) (*Purchase, error) {
	if s.certLookup == nil {
		return nil, ErrCertLookupNotConfigured
	}

	info, err := s.certLookup.LookupCert(ctx, req.CertNumber)
	if err != nil {
		return nil, fmt.Errorf("cert lookup: %w", err)
	}

	purchaseDate := req.PurchaseDate
	if purchaseDate == "" {
		purchaseDate = time.Now().Format("2006-01-02")
	}

	// Normalize the PSA category to a canonical set name
	setName := resolvePSACategory(info.Category)
	if isGenericSetName(setName) {
		setName = info.Category // keep original if resolved is still generic
	}

	p := &Purchase{
		CampaignID:   campaignID,
		CardName:     info.CardName,
		CertNumber:   req.CertNumber,
		CardNumber:   info.CardNumber,
		SetName:      setName,
		Grader:       "PSA",
		GradeValue:   info.Grade,
		BuyCostCents: req.BuyCostCents,
		CLValueCents: req.CLValueCents,
		PurchaseDate: purchaseDate,
	}

	// Get sourcing fee from campaign
	campaign, err := s.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("campaign lookup: %w", err)
	}
	p.PSASourcingFeeCents = campaign.PSASourcingFeeCents

	if err := s.CreatePurchase(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// --- Crack Arbitrage ---

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

	// Sort by crack advantage descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].CrackAdvantage > results[j].CrackAdvantage
	})

	return results, nil
}

// --- Expected Value ---

func (s *service) GetExpectedValues(ctx context.Context, campaignID string) (*EVPortfolio, error) {
	// Get historical data for segments
	data, err := s.repo.GetPurchasesWithSales(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	// Build segment performance by grade
	gradePerf := computeSegmentPerformance(data, "grade", func(d PurchaseWithSale) string {
		if d.Purchase.GradeValue <= 0 {
			return ""
		}
		return fmt.Sprintf("PSA %g", d.Purchase.GradeValue)
	})

	// Build a lookup map
	gradeMap := make(map[string]SegmentPerformance)
	for _, seg := range gradePerf {
		gradeMap[seg.Label] = seg
	}

	// Get unsold inventory
	unsold, err := s.repo.ListUnsoldPurchases(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	portfolio := &EVPortfolio{}
	minDP := 999

	for _, p := range unsold {
		gradeKey := fmt.Sprintf("PSA %g", p.GradeValue)
		seg, ok := gradeMap[gradeKey]
		if !ok {
			continue
		}

		costBasis := p.BuyCostCents + p.PSASourcingFeeCents

		// Get current market data for liquidity/trend if available
		liquidityFactor := 1.0
		trendAdj := 0.0
		if s.priceProv != nil {
			card := p.ToCardIdentity()
			if snapshot, snapErr := s.priceProv.GetMarketSnapshot(ctx, card, p.GradeValue); snapErr == nil && snapshot != nil {
				trendAdj = snapshot.Trend30d
				if snapshot.SalesLast30d > 0 && seg.AvgDaysToSell > 0 {
					expectedMonthly := 30.0 / seg.AvgDaysToSell
					if expectedMonthly > 0 {
						liquidityFactor = float64(snapshot.SalesLast30d) / expectedMonthly
					}
				}
			}
		}

		ev := computeExpectedValue(
			p.CardName, p.CertNumber, p.GradeValue,
			costBasis, seg.SellThroughPct, seg.AvgMarginPct,
			liquidityFactor, trendAdj, seg.AvgDaysToSell,
			0.05, seg.SoldCount,
		)

		portfolio.Items = append(portfolio.Items, *ev)
		portfolio.TotalEVCents += ev.EVCents
		if ev.EVCents >= 0 {
			portfolio.PositiveCount++
		} else {
			portfolio.NegativeCount++
		}
		if seg.SoldCount < minDP {
			minDP = seg.SoldCount
		}
	}

	if minDP < 999 {
		portfolio.MinDataPoints = minDP
	}

	return portfolio, nil
}

func (s *service) EvaluatePurchase(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*ExpectedValue, error) {
	campaign, err := s.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	data, err := s.repo.GetPurchasesWithSales(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	gradeKey := fmt.Sprintf("PSA %g", grade)
	gradePerf := computeSegmentPerformance(data, "grade", func(d PurchaseWithSale) string {
		if d.Purchase.GradeValue <= 0 {
			return ""
		}
		return fmt.Sprintf("PSA %g", d.Purchase.GradeValue)
	})

	var seg *SegmentPerformance
	for i := range gradePerf {
		if gradePerf[i].Label == gradeKey {
			seg = &gradePerf[i]
			break
		}
	}

	if seg == nil {
		// No historical data for this grade
		return computeExpectedValue(cardName, "", grade, buyCostCents+campaign.PSASourcingFeeCents, 0.5, 0.0, 1.0, 0.0, 30, 0.05, 0), nil
	}

	costBasis := buyCostCents + campaign.PSASourcingFeeCents

	return computeExpectedValue(
		cardName, "", grade, costBasis,
		seg.SellThroughPct, seg.AvgMarginPct,
		1.0, 0.0, seg.AvgDaysToSell,
		0.05, seg.SoldCount,
	), nil
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

	// Get all active campaigns to compute total daily exposure
	allCampaigns, err := s.repo.ListCampaigns(ctx, true)
	if err != nil {
		return nil, err
	}

	checklist := &ActivationChecklist{
		CampaignID:   campaign.ID,
		CampaignName: campaign.Name,
		AllPassed:    true,
	}

	// Check 1: Credit utilization below 70%
	utilizationOK := credit.UtilizationPct < 70
	checklist.Checks = append(checklist.Checks, ActivationCheck{
		Name:    "Credit Utilization",
		Passed:  utilizationOK,
		Message: fmt.Sprintf("Current utilization: %.0f%% (threshold: 70%%)", credit.UtilizationPct),
	})
	if !utilizationOK {
		checklist.AllPassed = false
	}

	// Check 2: At least one invoice cycle cleared (has a paid invoice)
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

	// Check 3: Total daily exposure calculation
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

	// Warnings for high-value campaigns
	if campaign.DailySpendCapCents >= 500000 { // $5,000/day
		checklist.Warnings = append(checklist.Warnings,
			fmt.Sprintf("This campaign has a $%d/day spend cap — a single fill could be significant", campaign.DailySpendCapCents/100))
	}

	// Warning if there are unpaid invoices
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

func (s *service) RunProjection(ctx context.Context, campaignID string) (*MonteCarloComparison, error) {
	campaign, err := s.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	data, err := s.repo.GetPurchasesWithSales(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	return RunMonteCarloProjection(campaign, data), nil
}
