package tuning

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// Service provides campaign tuning analytics.
type Service interface {
	GetCampaignTuning(ctx context.Context, campaignID string) (*inventory.TuningResponse, error)
}

type service struct {
	campaigns inventory.CampaignRepository
	analytics inventory.AnalyticsRepository
	logger    observability.Logger
}

// NewService creates a new tuning Service.
func NewService(
	campaigns inventory.CampaignRepository,
	analytics inventory.AnalyticsRepository,
	logger observability.Logger,
) Service {
	return &service{
		campaigns: campaigns,
		analytics: analytics,
		logger:    logger,
	}
}

func (s *service) GetCampaignTuning(ctx context.Context, campaignID string) (*inventory.TuningResponse, error) {
	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("campaign lookup: %w", err)
	}

	g, gCtx := errgroup.WithContext(ctx)

	var byGrade []inventory.GradePerformance
	var data []inventory.PurchaseWithSale
	var pnl *inventory.CampaignPNL
	var dailySpend []inventory.DailySpend
	var channelPNL []inventory.ChannelPNL

	g.Go(func() error {
		var err error
		byGrade, err = s.analytics.GetPerformanceByGrade(gCtx, campaignID)
		if err != nil {
			return fmt.Errorf("grade performance: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		data, err = s.analytics.GetPurchasesWithSales(gCtx, campaignID)
		if err != nil {
			return fmt.Errorf("purchases with sales: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		pnl, err = s.analytics.GetCampaignPNL(gCtx, campaignID)
		if err != nil {
			return fmt.Errorf("campaign PNL: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		dailySpend, err = s.analytics.GetDailySpend(gCtx, campaignID, 30)
		if err != nil {
			return fmt.Errorf("daily spend: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		channelPNL, err = s.analytics.GetPNLByChannel(gCtx, campaignID)
		if err != nil {
			return fmt.Errorf("channel PNL: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	fixedTiers, relativeTiers := inventory.ComputePriceTierPerformance(data)
	topPerformers, bottomPerformers := inventory.ComputeCardPerformance(data, 5)
	threshold := inventory.ComputeBuyThresholdAnalysis(data, campaign.BuyTermsCLPct)

	currentSnapshots := make(map[string]*inventory.MarketSnapshot)
	for _, d := range data {
		if d.Sale != nil {
			continue
		}
		key := inventory.PurchaseKey(d.Purchase.CardName, d.Purchase.CardNumber, d.Purchase.SetName, d.Purchase.GradeValue)
		if _, exists := currentSnapshots[key]; exists {
			continue
		}
		snap := inventory.SnapshotFromPurchase(&d.Purchase)
		if snap != nil && d.Purchase.CLValueCents > 0 {
			inventory.ApplyCLSignal(snap, d.Purchase.CLValueCents)
		}
		if snap != nil {
			inventory.ApplyMMSignal(snap, &d.Purchase)
		}
		if inventory.HasAnyPriceData(snap) {
			currentSnapshots[key] = snap
		}
	}
	alignment := inventory.ComputeMarketAlignment(data, currentSnapshots)
	inventory.EnrichCardPerformance(topPerformers, currentSnapshots)
	inventory.EnrichCardPerformance(bottomPerformers, currentSnapshots)

	recommendations := inventory.ComputeRecommendations(&inventory.TuningInput{
		Campaign:    campaign,
		PNL:         pnl,
		ByGrade:     byGrade,
		ByFixedTier: fixedTiers,
		Threshold:   threshold,
		Alignment:   alignment,
		DailySpend:  dailySpend,
		ChannelPNL:  channelPNL,
	})

	return &inventory.TuningResponse{
		CampaignID:       campaignID,
		CampaignName:     campaign.Name,
		ByGrade:          byGrade,
		ByFixedTier:      fixedTiers,
		ByRelativeTier:   relativeTiers,
		TopPerformers:    topPerformers,
		BottomPerformers: bottomPerformers,
		BuyThreshold:     threshold,
		MarketAlignment:  alignment,
		Recommendations:  recommendations,
	}, nil
}
