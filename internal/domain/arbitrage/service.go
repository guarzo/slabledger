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

// service implements Service.
type service struct {
	campaigns   inventory.CampaignRepository
	purchases   inventory.PurchaseRepository
	analytics   inventory.AnalyticsRepository
	finance     inventory.FinanceRepository
	priceProv   inventory.PriceLookup
	batchPricer BatchPricer
	projCache   *projectionCache // optional; if nil, projection runs on every call
	logger      observability.Logger
}

// WithPriceLookup injects the price lookup dependency.
func WithPriceLookup(priceProv inventory.PriceLookup) ServiceOption {
	return func(s *service) {
		s.priceProv = priceProv
	}
}

// WithBatchPricer injects the batch price distribution dependency.
// When set, GetCrackOpportunities and GetAcquisitionTargets use batch
// DH API calls instead of per-card lookups.
func WithBatchPricer(bp BatchPricer) ServiceOption {
	return func(s *service) {
		s.batchPricer = bp
	}
}

// WithLogger injects the logger.
func WithLogger(logger observability.Logger) ServiceOption {
	return func(s *service) {
		s.logger = logger
	}
}

// WithProjectionCache enables TTL-based caching of RunProjection results.
// Use newProjectionCache(5 * time.Minute) for production.
func WithProjectionCache(ttl time.Duration) ServiceOption {
	return func(s *service) {
		s.projCache = newProjectionCache(ttl)
	}
}

// requestScopedPriceProv returns a per-call cached wrapper around s.priceProv if the
// underlying provider supports it, or s.priceProv itself otherwise.
// Returns nil when no price provider is configured.
func (s *service) requestScopedPriceProv() inventory.PriceLookup {
	if s.priceProv == nil {
		return nil
	}
	if cacher, ok := s.priceProv.(interface {
		WithRequestCache() inventory.PriceLookup
	}); ok {
		return cacher.WithRequestCache()
	}
	return s.priceProv
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
	if s.priceProv == nil {
		if s.logger != nil {
			s.logger.Info(ctx, "skipping crack candidates",
				observability.String("reason", "price provider not configured"))
		}
		return []CrackAnalysis{}, nil
	}

	priceProv := s.requestScopedPriceProv()

	unsold, err := s.purchases.ListUnsoldPurchases(ctx, campaign.ID)
	if err != nil {
		return nil, err
	}

	ebayFee := inventory.EffectiveFeePct(campaign)

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
		if v, err := priceProv.GetLastSoldCents(ctx, card, 0); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "crack analysis: raw price lookup failed",
					observability.String("cardName", p.CardName),
					observability.Err(err))
			}
		} else {
			rawCents = v
		}
		if v, err := priceProv.GetLastSoldCents(ctx, card, p.GradeValue); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "crack analysis: graded price lookup failed",
					observability.String("cardName", p.CardName),
					observability.Float64("grade", p.GradeValue),
					observability.Err(err))
			}
		} else {
			gradedCents = v
		}

		if rawCents == 0 {
			continue
		}
		if gradedCents == 0 {
			gradedCents = p.CLValueCents
		}

		analysis := ComputeCrackAnalysis(
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
// Dispatches to the batch path when BatchPricer is injected, otherwise falls back to legacy.
func (s *service) GetCrackOpportunities(ctx context.Context) ([]CrackAnalysis, error) {
	if s.batchPricer != nil {
		return s.getCrackOpportunitiesBatch(ctx)
	}
	return s.getCrackOpportunitiesLegacy(ctx)
}

// getCrackOpportunitiesLegacy is the original per-card price lookup path.
// Uses a single ListAllUnsoldPurchases call to avoid N+1 DB queries.
func (s *service) getCrackOpportunitiesLegacy(ctx context.Context) ([]CrackAnalysis, error) {
	if s.priceProv == nil {
		if s.logger != nil {
			s.logger.Info(ctx, "skipping crack opportunities",
				observability.String("reason", "price provider not configured"))
		}
		return []CrackAnalysis{}, nil
	}

	priceProv := s.requestScopedPriceProv()

	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}

	// Build campaignID → ebayFee map to avoid per-campaign DB lookups.
	ebayFeeMap := make(map[string]float64, len(allCampaigns))
	for _, c := range allCampaigns {
		ebayFeeMap[c.ID] = inventory.EffectiveFeePct(&c)
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
			if s.logger != nil {
				s.logger.Debug(ctx, "crack analysis: skipping purchase — campaign not active",
					observability.String("purchaseID", p.ID),
					observability.String("campaignID", p.CampaignID))
			}
			continue
		}
		card := p.ToCardIdentity()

		rawCents := 0
		gradedCents := 0
		if v, err := priceProv.GetLastSoldCents(ctx, card, 0); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "crack analysis: raw price lookup failed",
					observability.String("cardName", p.CardName),
					observability.Err(err))
			}
		} else {
			rawCents = v
		}
		if v, err := priceProv.GetLastSoldCents(ctx, card, p.GradeValue); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "crack analysis: graded price lookup failed",
					observability.String("cardName", p.CardName),
					observability.Float64("grade", p.GradeValue),
					observability.Err(err))
			}
		} else {
			gradedCents = v
		}

		if rawCents == 0 {
			continue
		}
		if gradedCents == 0 {
			gradedCents = p.CLValueCents
		}

		analysis := ComputeCrackAnalysis(
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

// getCrackOpportunitiesBatch resolves all card IDs upfront and calls BatchPriceDistribution
// (2-3 HTTP calls) instead of per-card GetLastSoldCents (~400+ calls).
func (s *service) getCrackOpportunitiesBatch(ctx context.Context) ([]CrackAnalysis, error) {
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	ebayFeeMap := make(map[string]float64, len(allCampaigns))
	for _, c := range allCampaigns {
		ebayFeeMap[c.ID] = inventory.EffectiveFeePct(&c)
	}

	allUnsold, err := s.purchases.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all unsold purchases: %w", err)
	}

	type cardKey struct{ name, set, number string }
	cardToDHID := make(map[cardKey]int)
	var dhCardIDs []int
	for _, p := range allUnsold {
		if p.GradeValue >= 9 {
			continue
		}
		if _, ok := ebayFeeMap[p.CampaignID]; !ok {
			continue
		}
		key := cardKey{p.CardName, p.SetName, p.CardNumber}
		if _, seen := cardToDHID[key]; seen {
			continue
		}
		dhID, err := s.batchPricer.ResolveDHCardID(ctx, p.CardName, p.SetName, p.CardNumber)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "crack batch: resolve failed",
					observability.String("cardName", p.CardName), observability.Err(err))
			}
			cardToDHID[key] = 0
			continue
		}
		cardToDHID[key] = dhID
		if dhID > 0 {
			dhCardIDs = append(dhCardIDs, dhID)
		}
	}

	distributions, err := s.batchPricer.BatchPriceDistribution(ctx, dhCardIDs)
	if err != nil {
		return nil, fmt.Errorf("batch price distribution: %w", err)
	}

	var results []CrackAnalysis
	for _, p := range allUnsold {
		if p.GradeValue >= 9 {
			continue
		}
		ebayFee, ok := ebayFeeMap[p.CampaignID]
		if !ok {
			continue
		}
		key := cardKey{p.CardName, p.SetName, p.CardNumber}
		dhID := cardToDHID[key]
		if dhID == 0 {
			continue
		}
		dist, ok := distributions[dhID]
		if !ok {
			continue
		}

		rawBucket := dist.ByGrade["Raw"]
		gradedBucket := dist.ByGrade[gradeKeyForValue(p.GradeValue)]

		rawCents := rawBucket.MedianCents
		if rawCents == 0 {
			continue
		}
		gradedCents := gradedBucket.MedianCents
		if gradedCents == 0 {
			gradedCents = p.CLValueCents
		}

		analysis := ComputeCrackAnalysis(
			p.ID, p.CampaignID, p.CardName, p.CertNumber, p.GradeValue,
			p.BuyCostCents, p.PSASourcingFeeCents, rawCents, gradedCents,
			ebayFee,
		)
		if analysis.CrackAdvantage > 0 {
			results = append(results, *analysis)
		}
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
// Dispatches to the batch path when BatchPricer is injected, otherwise falls back to legacy.
func (s *service) GetAcquisitionTargets(ctx context.Context) ([]AcquisitionOpportunity, error) {
	if s.batchPricer != nil {
		return s.getAcquisitionTargetsBatch(ctx)
	}
	return s.getAcquisitionTargetsLegacy(ctx)
}

// getAcquisitionTargetsLegacy is the original per-card price lookup path.
func (s *service) getAcquisitionTargetsLegacy(ctx context.Context) ([]AcquisitionOpportunity, error) {
	if s.priceProv == nil {
		if s.logger != nil {
			s.logger.Info(ctx, "skipping acquisition targets",
				observability.String("reason", "price provider not configured"))
		}
		return []AcquisitionOpportunity{}, nil
	}

	priceProv := s.requestScopedPriceProv()

	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}

	ebayFeeMap := make(map[string]float64, len(allCampaigns))
	for _, c := range allCampaigns {
		ebayFeeMap[c.ID] = inventory.EffectiveFeePct(&c)
	}

	allUnsold, err := s.purchases.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all unsold purchases: %w", err)
	}

	opportunities := []AcquisitionOpportunity{}
	seen := make(map[string]bool)
	for _, p := range allUnsold {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		ebayFee, ok := ebayFeeMap[p.CampaignID]
		if !ok {
			if s.logger != nil {
				s.logger.Debug(ctx, "acquisition targets: skipping purchase — campaign not active",
					observability.String("purchaseID", p.ID),
					observability.String("campaignID", p.CampaignID))
			}
			continue
		}
		key := p.CardName + "|" + p.SetName + "|" + p.CardNumber
		if seen[key] {
			continue
		}
		seen[key] = true
		card := p.ToCardIdentity()
		rawNMCents := 0
		if v, err := priceProv.GetLastSoldCents(ctx, card, 0); err != nil {
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
			v, err := priceProv.GetLastSoldCents(ctx, card, grade)
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

// getAcquisitionTargetsBatch resolves all card IDs upfront and calls BatchPriceDistribution
// instead of per-card GetLastSoldCents.
func (s *service) getAcquisitionTargetsBatch(ctx context.Context) ([]AcquisitionOpportunity, error) {
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	ebayFeeMap := make(map[string]float64, len(allCampaigns))
	for _, c := range allCampaigns {
		ebayFeeMap[c.ID] = inventory.EffectiveFeePct(&c)
	}

	allUnsold, err := s.purchases.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all unsold purchases: %w", err)
	}

	type cardKey struct{ name, set, number string }
	cardToDHID := make(map[cardKey]int)
	var dhCardIDs []int
	for _, p := range allUnsold {
		if _, ok := ebayFeeMap[p.CampaignID]; !ok {
			continue
		}
		key := cardKey{p.CardName, p.SetName, p.CardNumber}
		if _, seen := cardToDHID[key]; seen {
			continue
		}
		dhID, err := s.batchPricer.ResolveDHCardID(ctx, p.CardName, p.SetName, p.CardNumber)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "acquisition batch: resolve failed",
					observability.String("cardName", p.CardName), observability.Err(err))
			}
			cardToDHID[key] = 0
			continue
		}
		cardToDHID[key] = dhID
		if dhID > 0 {
			dhCardIDs = append(dhCardIDs, dhID)
		}
	}

	distributions, err := s.batchPricer.BatchPriceDistribution(ctx, dhCardIDs)
	if err != nil {
		return nil, fmt.Errorf("batch price distribution: %w", err)
	}

	seen := make(map[cardKey]bool)
	var opportunities []AcquisitionOpportunity
	for _, p := range allUnsold {
		ebayFee, ok := ebayFeeMap[p.CampaignID]
		if !ok {
			continue
		}
		key := cardKey{p.CardName, p.SetName, p.CardNumber}
		if seen[key] {
			continue
		}
		seen[key] = true

		dhID := cardToDHID[key]
		if dhID == 0 {
			continue
		}
		dist, ok := distributions[dhID]
		if !ok {
			continue
		}

		rawBucket := dist.ByGrade["Raw"]
		if rawBucket.MedianCents == 0 {
			continue
		}

		gradedEstimates := make(map[string]int)
		for _, grade := range []float64{8, 9, 10} {
			bucket := dist.ByGrade[gradeKeyForValue(grade)]
			if bucket.MedianCents > 0 {
				gradedEstimates[fmt.Sprintf("PSA %g", grade)] = bucket.MedianCents
			}
		}

		opp := computeAcquisitionOpportunity(
			p.CardName, p.SetName, p.CardNumber, p.CertNumber,
			rawBucket.MedianCents, gradedEstimates, ebayFee, "inventory",
		)
		if opp != nil {
			opportunities = append(opportunities, *opp)
		}
	}

	sortAcquisitionByProfit(opportunities)
	return opportunities, nil
}

// GetExpectedValues, EvaluatePurchase, and RunProjection live in service_ev.go.
