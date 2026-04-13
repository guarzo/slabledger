package arbitrage

import (
	"context"
	"fmt"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	// HighSpendCapCents is the daily spend cap threshold (in cents) above which
	// a warning is emitted that a single fill could be significant.
	HighSpendCapCents = 500000 // $5,000/day
)

// Service provides arbitrage analysis: crack opportunities, acquisition targets,
// activation checklists, expected value calculations, and Monte Carlo projections.
type Service interface {
	GetCrackCandidates(ctx context.Context, campaignID string) ([]CrackAnalysis, error)
	GetCrackOpportunities(ctx context.Context) ([]CrackAnalysis, error)
	GetAcquisitionTargets(ctx context.Context) ([]AcquisitionOpportunity, error)
	GetActivationChecklist(ctx context.Context, campaignID string) (*inventory.ActivationChecklist, error)
	GetExpectedValues(ctx context.Context, campaignID string) (*EVPortfolio, error)
	EvaluatePurchase(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*ExpectedValue, error)
	RunProjection(ctx context.Context, campaignID string) (*MonteCarloComparison, error)
}

// ServiceOption configures the arbitrage service.
type ServiceOption func(*service)

// WithPriceLookup injects the price lookup dependency.
func WithPriceLookup(priceProv inventory.PriceLookup) ServiceOption {
	return func(s *service) {
		s.priceProv = priceProv
	}
}

// WithLogger injects the logger.
func WithLogger(logger observability.Logger) ServiceOption {
	return func(s *service) {
		s.logger = logger
	}
}

// service implements Service.
type service struct {
	campaigns inventory.CampaignRepository
	purchases inventory.PurchaseRepository
	analytics inventory.AnalyticsRepository
	finance   inventory.FinanceRepository
	priceProv inventory.PriceLookup
	logger    observability.Logger
}

// NewService creates a new arbitrage Service.
func NewService(
	campaigns inventory.CampaignRepository,
	purchases inventory.PurchaseRepository,
	analytics inventory.AnalyticsRepository,
	finance inventory.FinanceRepository,
	opts ...ServiceOption,
) Service {
	svc := &service{
		campaigns: campaigns,
		purchases: purchases,
		analytics: analytics,
		finance:   finance,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// GetCrackCandidates returns crack candidates for a single campaign, computed on demand.
func (s *service) GetCrackCandidates(ctx context.Context, campaignID string) ([]CrackAnalysis, error) {
	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return s.crackCandidatesForCampaign(ctx, campaign)
}

// crackCandidatesForCampaign computes crack candidates for an already-loaded campaign.
func (s *service) crackCandidatesForCampaign(ctx context.Context, campaign *inventory.Campaign) ([]CrackAnalysis, error) {
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
		// Skip PSA 9+ from crack analysis — only PSA 8 and below (including half-grades like 8.5)
		// are candidates for cracking and resubmission.
		if p.GradeValue >= 9 {
			continue
		}

		card := p.ToCardIdentity()

		rawCents := 0
		gradedCents := 0
		if s.priceProv != nil {
			if v, err := s.priceProv.GetLastSoldCents(ctx, card, 0); err != nil {
				if s.logger != nil {
					s.logger.Warn(ctx, "crack analysis: raw price lookup failed",
						observability.String("cardName", p.CardName),
						observability.Err(err))
				}
			} else {
				rawCents = v
			}
			if v, err := s.priceProv.GetLastSoldCents(ctx, card, p.GradeValue); err != nil {
				if s.logger != nil {
					s.logger.Warn(ctx, "crack analysis: graded price lookup failed",
						observability.String("cardName", p.CardName),
						observability.Float64("grade", p.GradeValue),
						observability.Err(err))
				}
			} else {
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

// GetCrackOpportunities returns cross-campaign crack opportunities, computed on demand.
// Uses a single ListAllUnsoldPurchases call to avoid N+1 DB queries.
func (s *service) GetCrackOpportunities(ctx context.Context) ([]CrackAnalysis, error) {
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}

	// Build campaignID → ebayFee map to avoid per-campaign DB lookups.
	ebayFeeMap := make(map[string]float64, len(allCampaigns))
	for _, c := range allCampaigns {
		fee := c.EbayFeePct
		if fee == 0 {
			fee = DefaultMarketplaceFeePct
		}
		ebayFeeMap[c.ID] = fee
	}

	allUnsold, err := s.purchases.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all unsold purchases: %w", err)
	}

	var results []CrackAnalysis
	for _, p := range allUnsold {
		// Skip PSA 9+ from crack analysis — only PSA 8 and below are candidates.
		if p.GradeValue >= 9 {
			continue
		}
		ebayFee, ok := ebayFeeMap[p.CampaignID]
		if !ok {
			continue // purchase belongs to a non-active campaign
		}
		card := p.ToCardIdentity()

		rawCents := 0
		gradedCents := 0
		if s.priceProv != nil {
			if v, err := s.priceProv.GetLastSoldCents(ctx, card, 0); err != nil {
				if s.logger != nil {
					s.logger.Warn(ctx, "crack analysis: raw price lookup failed",
						observability.String("cardName", p.CardName),
						observability.Err(err))
				}
			} else {
				rawCents = v
			}
			if v, err := s.priceProv.GetLastSoldCents(ctx, card, p.GradeValue); err != nil {
				if s.logger != nil {
					s.logger.Warn(ctx, "crack analysis: graded price lookup failed",
						observability.String("cardName", p.CardName),
						observability.Float64("grade", p.GradeValue),
						observability.Err(err))
				}
			} else {
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
			p.ID, p.CampaignID, p.CardName, p.CertNumber, p.GradeValue,
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

// GetActivationChecklist builds a pre-activation readiness checklist.
func (s *service) GetActivationChecklist(ctx context.Context, campaignID string) (*inventory.ActivationChecklist, error) {
	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	capitalRaw, err := s.finance.GetCapitalRawData(ctx)
	if err != nil {
		return nil, err
	}
	capital := inventory.ComputeCapitalSummary(capitalRaw)

	invoices, err := s.finance.ListInvoices(ctx)
	if err != nil {
		return nil, err
	}

	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, err
	}

	checklist := &inventory.ActivationChecklist{
		CampaignID:   campaign.ID,
		CampaignName: campaign.Name,
		AllPassed:    true,
	}

	exposureCheckOK := capital.AlertLevel != inventory.AlertCritical
	checklist.Checks = append(checklist.Checks, inventory.ActivationCheck{
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
	checklist.Checks = append(checklist.Checks, inventory.ActivationCheck{
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

	dailyRecovery := float64(capital.RecoveryRate30dCents) / 30.0
	dailyExpOK := capital.WeeksToCover == 0 || float64(totalDailyExposure) < dailyRecovery
	exposureMsg := fmt.Sprintf("Total daily exposure with activation: $%d/day (daily recovery: $%d)", totalDailyExposure/100, int(dailyRecovery)/100)
	if capital.RecoveryRate30dCents == 0 {
		dailyExpOK = true
		exposureMsg = fmt.Sprintf("Total daily exposure with activation: $%d/day (no recovery data yet)", totalDailyExposure/100)
	}
	checklist.Checks = append(checklist.Checks, inventory.ActivationCheck{
		Name:    "Daily Exposure",
		Passed:  dailyExpOK,
		Message: exposureMsg,
	})
	checklist.AllPassed = checklist.AllPassed && dailyExpOK

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

// GetAcquisitionTargets returns raw-to-graded arbitrage opportunities across all active campaigns.
// Uses a single ListAllUnsoldPurchases call to avoid N+1 DB queries.
func (s *service) GetAcquisitionTargets(ctx context.Context) ([]AcquisitionOpportunity, error) {
	if s.priceProv == nil {
		if s.logger != nil {
			s.logger.Info(ctx, "skipping acquisition targets",
				observability.String("reason", "price provider not configured"))
		}
		return []AcquisitionOpportunity{}, nil
	}
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}

	// Build campaignID → ebayFee map to avoid per-campaign DB lookups.
	ebayFeeMap := make(map[string]float64, len(allCampaigns))
	for _, c := range allCampaigns {
		fee := c.EbayFeePct
		if fee == 0 {
			fee = DefaultMarketplaceFeePct
		}
		ebayFeeMap[c.ID] = fee
	}

	allUnsold, err := s.purchases.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all unsold purchases: %w", err)
	}

	opportunities := []AcquisitionOpportunity{}
	seen := make(map[string]bool)
	for _, p := range allUnsold {
		ebayFee, ok := ebayFeeMap[p.CampaignID]
		if !ok {
			continue // purchase belongs to a non-active campaign
		}
		key := p.CardName + "|" + p.SetName + "|" + p.CardNumber
		if seen[key] {
			continue
		}
		seen[key] = true
		card := p.ToCardIdentity()
		rawNMCents := 0
		if v, err := s.priceProv.GetLastSoldCents(ctx, card, 0); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "acquisition targets: raw price lookup failed",
					observability.String("cardName", p.CardName),
					observability.Err(err))
			}
		} else if v > 0 {
			rawNMCents = v
		}
		if rawNMCents == 0 {
			continue
		}
		gradedEstimates := make(map[string]int)
		for _, grade := range []float64{8, 9, 10} {
			v, err := s.priceProv.GetLastSoldCents(ctx, card, grade)
			if err != nil {
				if s.logger != nil {
					s.logger.Warn(ctx, "acquisition targets: graded price lookup failed",
						observability.String("cardName", p.CardName),
						observability.Float64("grade", grade),
						observability.Err(err))
				}
				continue
			}
			if v > 0 {
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
	sortAcquisitionByProfit(opportunities)
	return opportunities, nil
}

// GetExpectedValues returns expected values for all unsold purchases in a campaign.
func (s *service) GetExpectedValues(ctx context.Context, campaignID string) (*EVPortfolio, error) {
	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	data, err := s.analytics.GetPurchasesWithSales(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	gradePerf := inventory.ComputeSegmentPerformance(data, "grade", func(d inventory.PurchaseWithSale) string {
		if d.Purchase.GradeValue <= 0 {
			return ""
		}
		return fmt.Sprintf("PSA %g", d.Purchase.GradeValue)
	})

	gradeMap := make(map[string]inventory.SegmentPerformance)
	for _, seg := range gradePerf {
		gradeMap[seg.Label] = seg
	}

	unsold, err := s.purchases.ListUnsoldPurchases(ctx, campaignID)
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

		feePct := campaign.EbayFeePct
		if feePct == 0 {
			feePct = DefaultMarketplaceFeePct
		}
		ev := computeExpectedValue(EVInput{
			CardName:               p.CardName,
			CertNumber:             p.CertNumber,
			Grade:                  p.GradeValue,
			CostBasis:              costBasis,
			SegmentSellThrough:     seg.SellThroughPct,
			SegmentMedianMarginPct: seg.AvgMarginPct,
			LiquidityFactor:        liquidityFactor,
			TrendAdjustment:        trendAdj,
			AvgDaysUnsold:          seg.AvgDaysToSell,
			AnnualCapitalCostRate:  0.05,
			DataPoints:             seg.SoldCount,
			FeePct:                 feePct,
		})

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

// EvaluatePurchase evaluates a prospective purchase's expected value.
func (s *service) EvaluatePurchase(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*ExpectedValue, error) {
	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	data, err := s.analytics.GetPurchasesWithSales(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	gradeKey := fmt.Sprintf("PSA %g", grade)
	gradePerf := inventory.ComputeSegmentPerformance(data, "grade", func(d inventory.PurchaseWithSale) string {
		if d.Purchase.GradeValue <= 0 {
			return ""
		}
		return fmt.Sprintf("PSA %g", d.Purchase.GradeValue)
	})

	var seg *inventory.SegmentPerformance
	for i := range gradePerf {
		if gradePerf[i].Label == gradeKey {
			seg = &gradePerf[i]
			break
		}
	}

	if seg == nil {
		feePct := campaign.EbayFeePct
		if feePct == 0 {
			feePct = DefaultMarketplaceFeePct
		}
		return computeExpectedValue(EVInput{
			CardName:              cardName,
			Grade:                 grade,
			CostBasis:             buyCostCents + campaign.PSASourcingFeeCents,
			SegmentSellThrough:    0.5,
			LiquidityFactor:       1.0,
			AvgDaysUnsold:         30,
			AnnualCapitalCostRate: 0.05,
			FeePct:                feePct,
		}), nil
	}

	costBasis := buyCostCents + campaign.PSASourcingFeeCents
	feePct := campaign.EbayFeePct
	if feePct == 0 {
		feePct = DefaultMarketplaceFeePct
	}

	return computeExpectedValue(EVInput{
		CardName:               cardName,
		Grade:                  grade,
		CostBasis:              costBasis,
		SegmentSellThrough:     seg.SellThroughPct,
		SegmentMedianMarginPct: seg.AvgMarginPct,
		LiquidityFactor:        1.0,
		AvgDaysUnsold:          seg.AvgDaysToSell,
		AnnualCapitalCostRate:  0.05,
		DataPoints:             seg.SoldCount,
		FeePct:                 feePct,
	}), nil
}

// RunProjection runs a Monte Carlo simulation projection for a campaign.
func (s *service) RunProjection(ctx context.Context, campaignID string) (*MonteCarloComparison, error) {
	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	data, err := s.analytics.GetPurchasesWithSales(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	return RunMonteCarloProjection(campaign, data), nil
}
