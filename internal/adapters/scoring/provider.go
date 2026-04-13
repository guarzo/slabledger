// Package scoring implements advisor.ScoringDataProvider by wrapping inventory.Service.
package scoring

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/portfolio"
	"github.com/guarzo/slabledger/internal/domain/tuning"
)

// Provider gathers raw factor data for the scoring orchestrator
// by calling service methods in parallel.
type Provider struct {
	svc       inventory.AnalyticsService
	arbSvc    arbitrage.Service
	portSvc   portfolio.Service
	tuningSvc tuning.Service
}

// ProviderOption configures optional dependencies on Provider.
type ProviderOption func(*Provider)

// WithArbitrageService injects the arbitrage service.
func WithArbitrageService(svc arbitrage.Service) ProviderOption {
	return func(p *Provider) { p.arbSvc = svc }
}

// WithPortfolioService injects the portfolio service.
func WithPortfolioService(svc portfolio.Service) ProviderOption {
	return func(p *Provider) { p.portSvc = svc }
}

// WithTuningService injects the tuning service.
func WithTuningService(svc tuning.Service) ProviderOption {
	return func(p *Provider) { p.tuningSvc = svc }
}

// NewProvider creates a ScoringDataProvider backed by the given campaigns service.
func NewProvider(svc inventory.AnalyticsService, opts ...ProviderOption) *Provider {
	p := &Provider{svc: svc}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

var _ advisor.ScoringDataProvider = (*Provider)(nil)

// PurchaseData fans out three goroutines to gather EV, tuning, and portfolio data.
func (p *Provider) PurchaseData(ctx context.Context, req advisor.PurchaseAssessmentRequest) (*advisor.PurchaseFactorData, error) {
	grade := parseGrade(req.Grade)
	data := &advisor.PurchaseFactorData{}

	var mu sync.Mutex
	var wg sync.WaitGroup

	// 1. EvaluatePurchase -> ROI, SalesPerMonth, PriceChangePct, PriceConfidence
	if p.arbSvc != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev, err := p.arbSvc.EvaluatePurchase(ctx, req.CampaignID, req.CardName, grade, req.BuyCostCents)
			if err != nil || ev == nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			roiPct := ev.EVPerDollar * 100
			data.ROIPct = &roiPct
			salesPerMonth := ev.LiquidityFactor * 5
			data.SalesPerMonth = &salesPerMonth
			priceChangePct := ev.TrendAdjustment * 100
			data.PriceChangePct = &priceChangePct
			data.PriceConfidence = mapConfidence(ev.Confidence)
			data.MarketSource = "campaigns"
		}()
	}

	// 2. GetCampaignTuning -> GradeROI, CampaignAvgROI, Trend30dPct
	if p.tuningSvc != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tuningData, err := p.tuningSvc.GetCampaignTuning(ctx, req.CampaignID)
			if err != nil || tuningData == nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			gradeROI, avgROI := extractGradeROI(tuningData.ByGrade, grade)
			if gradeROI != nil {
				data.GradeROI = gradeROI
			}
			if avgROI != nil {
				data.CampaignAvgROI = avgROI
			}
			if tuningData.MarketAlignment != nil {
				trend := tuningData.MarketAlignment.AvgTrend30d * 100
				data.Trend30dPct = &trend
			}
		}()
	}

	// 3. GetPortfolioInsights -> ConcentrationRisk
	if p.portSvc != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			insights, err := p.portSvc.GetPortfolioInsights(ctx)
			if err != nil || insights == nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			data.ConcentrationRisk = computeConcentration(insights.ByCharacter, req.CardName)
		}()
	}

	wg.Wait()
	return data, nil
}

// CampaignData fans out three goroutines for PNL, tuning, and aging data.
func (p *Provider) CampaignData(ctx context.Context, campaignID string) (*advisor.CampaignFactorData, error) {
	data := &advisor.CampaignFactorData{}

	var mu sync.Mutex
	var wg sync.WaitGroup

	// 1. GetCampaignPNL -> ROIPct, SellThroughPct, PriceConfidence
	wg.Add(1)
	go func() {
		defer wg.Done()
		pnl, err := p.svc.GetCampaignPNL(ctx, campaignID)
		if err != nil || pnl == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		roiPct := pnl.ROI * 100
		data.ROIPct = &roiPct
		data.CampaignROI = &roiPct
		sellThrough := pnl.SellThroughPct
		data.SellThroughPct = &sellThrough
		data.PriceConfidence = mathutil.ConfidenceScore(pnl.TotalPurchases)
		data.MarketSource = "campaigns"
	}()

	// 2. GetCampaignTuning -> Trend30dPct
	if p.tuningSvc != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tuningData, err := p.tuningSvc.GetCampaignTuning(ctx, campaignID)
			if err != nil || tuningData == nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if tuningData.MarketAlignment != nil {
				trend := tuningData.MarketAlignment.AvgTrend30d * 100
				data.Trend30dPct = &trend
			}
		}()
	}

	// 3. GetInventoryAging -> SalesPerMonth, PriceChangePct
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, err := p.svc.GetInventoryAging(ctx, campaignID)
		if err != nil || result == nil || len(result.Items) == 0 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		avgVelocity, avgDelta := agingSignals(result.Items)
		if avgVelocity != nil {
			data.SalesPerMonth = avgVelocity
		}
		if avgDelta != nil {
			data.PriceChangePct = avgDelta
		}
	}()

	wg.Wait()
	return data, nil
}

// parseGrade extracts a numeric grade from a string like "PSA 10" or "9.5".
func parseGrade(s string) float64 {
	s = strings.TrimSpace(s)
	// Try to find the last numeric token (handles "PSA 10", "BGS 9.5", etc.)
	parts := strings.Fields(s)
	for i := len(parts) - 1; i >= 0; i-- {
		if v, err := strconv.ParseFloat(parts[i], 64); err == nil {
			return v
		}
	}
	return 0
}

// mapConfidence converts a text confidence label to a 0-1 float.
func mapConfidence(label string) float64 {
	switch strings.ToLower(label) {
	case "high":
		return 1.0
	case "medium":
		return 0.6
	case "low":
		return 0.3
	default:
		return 0.0
	}
}

// extractGradeROI finds the ROI for the matching grade and computes the campaign average.
func extractGradeROI(grades []inventory.GradePerformance, target float64) (gradeROI, avgROI *float64) {
	if len(grades) == 0 {
		return nil, nil
	}
	totalROI := 0.0
	totalWeight := 0
	for _, g := range grades {
		totalROI += g.ROI * float64(g.PurchaseCount)
		totalWeight += g.PurchaseCount
		if g.Grade == target {
			roi := g.ROI * 100
			gradeROI = &roi
		}
	}
	if totalWeight > 0 {
		avg := (totalROI / float64(totalWeight)) * 100
		avgROI = &avg
	}
	return gradeROI, avgROI
}

// computeConcentration determines concentration risk by comparing character purchases to total.
// Matches segment labels against whole words in the card name to avoid false positives
// (e.g. "Char" matching "Charged Up Pikachu").
func computeConcentration(segments []inventory.SegmentPerformance, cardName string) string {
	if len(segments) == 0 {
		return "low"
	}
	cardWords := strings.Fields(strings.ToLower(cardName))
	matchCount := 0
	totalCount := 0
	for _, seg := range segments {
		totalCount += seg.PurchaseCount
		segLower := strings.ToLower(seg.Label)
		for _, word := range cardWords {
			if word == segLower {
				matchCount += seg.PurchaseCount
				break
			}
		}
	}
	if totalCount == 0 {
		return "low"
	}
	ratio := float64(matchCount) / float64(totalCount)
	switch {
	case ratio > 0.40:
		return "high"
	case ratio < 0.15:
		return "low"
	default:
		return "medium"
	}
}

// agingSignals computes average monthly velocity and average delta pct from aging items.
func agingSignals(items []inventory.AgingItem) (avgVelocity, avgDelta *float64) {
	var velocitySum float64
	var velocityCount int
	var deltaSum float64
	var deltaCount int

	for _, item := range items {
		if item.CurrentMarket != nil && item.CurrentMarket.MonthlyVelocity > 0 {
			velocitySum += float64(item.CurrentMarket.MonthlyVelocity)
			velocityCount++
		}
		if item.Signal != nil {
			deltaSum += item.Signal.DeltaPct * 100
			deltaCount++
		}
	}
	if velocityCount > 0 {
		v := velocitySum / float64(velocityCount)
		avgVelocity = &v
	}
	if deltaCount > 0 {
		d := deltaSum / float64(deltaCount)
		avgDelta = &d
	}
	return avgVelocity, avgDelta
}
