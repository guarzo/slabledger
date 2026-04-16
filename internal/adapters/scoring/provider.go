// Package scoring implements advisor.ScoringDataProvider by wrapping inventory.Service.
package scoring

import (
	"context"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/tuning"
)

// Provider gathers raw factor data for the scoring orchestrator
// by calling service methods in parallel.
type Provider struct {
	svc       inventory.AnalyticsService
	tuningSvc tuning.Service
}

// ProviderOption configures optional dependencies on Provider.
type ProviderOption func(*Provider)

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
