package arbitrage

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const (
	// HighSpendCapCents is the daily spend cap threshold (in cents) above which
	// a warning is emitted that a single fill could be significant.
	HighSpendCapCents = 500000 // $5,000/day

	crackCacheRefreshInterval = 15 * time.Minute
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
	priceProv inventory.PriceLookup,
	logger observability.Logger,
) Service {
	return &service{
		campaigns: campaigns,
		purchases: purchases,
		analytics: analytics,
		finance:   finance,
		priceProv: priceProv,
		logger:    logger,
	}
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

// GetCrackOpportunities returns cross-campaign crack opportunities, computed on demand.
func (s *service) GetCrackOpportunities(ctx context.Context) ([]CrackAnalysis, error) {
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	var results []CrackAnalysis
	for _, c := range allCampaigns {
		campaign := c
		candidates, err := s.crackCandidatesForCampaign(ctx, &campaign)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "crack candidates failed for campaign",
					observability.String("campaignID", c.ID),
					observability.Err(err))
			}
			continue
		}
		results = append(results, candidates...)
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

	dailyExpOK := capital.WeeksToCover == 0 || float64(totalDailyExposure) < float64(capital.RecoveryRate30dCents)/inventory.WeeksPerMonth
	exposureMsg := fmt.Sprintf("Total daily exposure with activation: $%d/day (weekly recovery: $%d)", totalDailyExposure/100, capital.RecoveryRate30dCents/430)
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

// GetExpectedValues returns expected values for all unsold purchases in a campaign.
func (s *service) GetExpectedValues(ctx context.Context, campaignID string) (*EVPortfolio, error) {
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
