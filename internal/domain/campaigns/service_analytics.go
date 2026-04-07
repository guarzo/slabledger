package campaigns

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/timeutil"
	"golang.org/x/sync/errgroup"
)

// --- Analytics ---

func (s *service) GetCampaignPNL(ctx context.Context, campaignID string) (*CampaignPNL, error) {
	return s.repo.GetCampaignPNL(ctx, campaignID)
}

func (s *service) GetPNLByChannel(ctx context.Context, campaignID string) ([]ChannelPNL, error) {
	return s.repo.GetPNLByChannel(ctx, campaignID)
}

func (s *service) GetDailySpend(ctx context.Context, campaignID string, days int) ([]DailySpend, error) {
	return s.repo.GetDailySpend(ctx, campaignID, days)
}

func (s *service) GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]DaysToSellBucket, error) {
	return s.repo.GetDaysToSellDistribution(ctx, campaignID)
}

// hasAnyPriceData returns true if any price field or SourcePrice entry is non-zero.
func hasAnyPriceData(snap *MarketSnapshot) bool {
	if snap == nil {
		return false
	}
	if snap.LastSoldCents > 0 || snap.MedianCents > 0 || snap.GradePriceCents > 0 ||
		snap.LowestListCents > 0 || snap.ConservativeCents > 0 || snap.OptimisticCents > 0 {
		return true
	}
	for _, sp := range snap.SourcePrices {
		if sp.PriceCents > 0 {
			return true
		}
	}
	return false
}

// snapshotFromPurchase builds a MarketSnapshot from the purchase's stored MarketSnapshotData.
// Returns nil if the purchase has no snapshot data.
// When SnapshotJSON is present (populated by the inventory refresh scheduler), the full
// snapshot is deserialized to include SourcePrices, velocity, percentiles, etc.
// Falls back to individual columns + fallback percentiles when JSON is absent.
func snapshotFromPurchase(p *Purchase) *MarketSnapshot {
	if p.SnapshotDate == "" {
		return nil
	}

	// Prefer full JSON snapshot when available (contains all fields)
	if p.SnapshotJSON != "" {
		var snap MarketSnapshot
		if err := json.Unmarshal([]byte(p.SnapshotJSON), &snap); err == nil {
			return &snap
		}
	}

	// Fallback to individual columns
	snap := &MarketSnapshot{
		LastSoldCents:     p.LastSoldCents,
		LowestListCents:   p.LowestListCents,
		ConservativeCents: p.ConservativeCents,
		MedianCents:       p.MedianCents,
		ActiveListings:    p.ActiveListings,
		SalesLast30d:      p.SalesLast30d,
		Trend30d:          p.Trend30d,
	}
	// Derive a base price for conservative/optimistic estimates.
	// Prefer MedianCents, fall back to LastSoldCents, then LowestListCents.
	basePrice := snap.MedianCents
	if basePrice == 0 {
		basePrice = snap.LastSoldCents
	}
	if basePrice == 0 {
		basePrice = snap.LowestListCents
	}
	if basePrice > 0 {
		if snap.ConservativeCents == 0 {
			snap.ConservativeCents = int(math.Round(float64(basePrice) * 0.85))
		}
		if snap.OptimisticCents == 0 {
			snap.OptimisticCents = int(math.Round(float64(basePrice) * 1.15))
		}
	}
	return snap
}

// enrichAgingItem enriches a purchase into an AgingItem with market data and signals.
// Uses stored snapshot data (no API calls) for fast page loads. The inventory refresh
// scheduler keeps snapshots fresh in the background.
func (s *service) enrichAgingItem(_ context.Context, p *Purchase, campaignName string) AgingItem {
	item := AgingItem{Purchase: *p, DaysHeld: timeutil.DaysSince(p.PurchaseDate), CampaignName: campaignName}

	snap := snapshotFromPurchase(p)

	// Incorporate Card Ladder value — works even when stored snapshot is empty
	if p.CLValueCents > 0 {
		if snap == nil {
			snap = &MarketSnapshot{}
		}
		applyCLSignal(snap, p.CLValueCents)
	}

	if hasAnyPriceData(snap) {
		item.CurrentMarket = snap

		// Compute market signal if CL value and last sold data are available
		if p.CLValueCents > 0 && snap.LastSoldCents > 0 {
			lastSold := snap.LastSoldCents
			deltaPct := float64(lastSold-p.CLValueCents) / float64(p.CLValueCents)
			direction := "stable"
			rec := "Either channel — local for speed, eBay for margin"
			if deltaPct >= marketDriftThreshold {
				direction = "rising"
				rec = "Consider eBay/TCGPlayer — market ahead of valuations"
			} else if deltaPct <= -marketDriftThreshold {
				direction = "falling"
				rec = "Consider local (GameStop at 90% CL) — lock in before drop"
			}
			item.Signal = &MarketSignal{
				CardName: p.CardName, CertNumber: p.CertNumber,
				Grade: p.GradeValue, CLValueCents: p.CLValueCents,
				LastSoldCents: lastSold, DeltaPct: deltaPct,
				Direction: direction, Recommendation: rec,
			}
		}
	}

	// Flag price anomalies: large buy/market deviations with low-confidence pricing
	const (
		minMedianToBuyRatio    = 0.3
		maxMedianToBuyRatio    = 5.0
		lowConfidenceThreshold = 0.5
	)
	if snap != nil && p.BuyCostCents > 0 && snap.MedianCents > 0 {
		ratio := float64(snap.MedianCents) / float64(p.BuyCostCents)
		lowConfidence := snap.SourceCount <= 1 || snap.Confidence < lowConfidenceThreshold || snap.IsEstimated || snap.PricingGap
		if lowConfidence && (ratio < minMedianToBuyRatio || ratio > maxMedianToBuyRatio) {
			item.PriceAnomaly = true
			item.AnomalyReason = "low confidence pricing"
		}
	}

	// Resolve recommended price from hierarchy
	recPrice, recSource := recommendedPrice(p, item.CurrentMarket)
	item.RecommendedPriceCents = recPrice
	item.RecommendedSource = recSource

	return item
}

// applyCLSignal incorporates Card Ladder auction-based data into the market snapshot.
// CL is weighted at 30% alongside market data (70%) and serves as a floor — the
// market price should not fall below CL's auction-derived valuation.
func applyCLSignal(snap *MarketSnapshot, clCents int) {
	if snap == nil || clCents <= 0 {
		return
	}

	// Add CL as a visible source price
	snap.SourcePrices = append(snap.SourcePrices, SourcePrice{
		Source:     "CardLadder",
		PriceCents: clCents,
	})

	// Blend CL into the median: 70% market data, 30% CL
	if snap.MedianCents > 0 {
		blended := int(math.Round(float64(snap.MedianCents)*0.7 + float64(clCents)*0.3))
		// CL as floor: don't let blended price drop below CL
		if blended < clCents {
			blended = clCents
		}
		snap.MedianCents = blended
	} else {
		// No market median — use CL directly
		snap.MedianCents = clCents
	}

	// Recalculate fallback-derived percentiles from updated median
	// (only when they were originally computed as fallbacks, i.e., proportional to median)
	if snap.ConservativeCents > 0 && snap.P10Cents > 0 {
		// Distribution data exists — leave percentiles as-is
		return
	}
	if snap.MedianCents > 0 {
		if snap.ConservativeCents == 0 {
			snap.ConservativeCents = int(math.Round(float64(snap.MedianCents) * 0.85))
		}
		if snap.OptimisticCents == 0 {
			snap.OptimisticCents = int(math.Round(float64(snap.MedianCents) * 1.15))
		}
		if snap.P10Cents == 0 {
			snap.P10Cents = int(math.Round(float64(snap.MedianCents) * 0.70))
		}
		if snap.P90Cents == 0 {
			snap.P90Cents = int(math.Round(float64(snap.MedianCents) * 1.30))
		}
	}
}

func (s *service) GetInventoryAging(ctx context.Context, campaignID string) ([]AgingItem, error) {
	unsold, err := s.repo.ListUnsoldPurchases(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	items := make([]AgingItem, 0, len(unsold))
	for i := range unsold {
		items = append(items, s.enrichAgingItem(ctx, &unsold[i], ""))
	}

	s.applyOpenFlags(ctx, items)
	s.enrichCompSummaries(ctx, items)
	return items, nil
}

func (s *service) GetGlobalInventoryAging(ctx context.Context) ([]AgingItem, error) {
	purchases, err := s.repo.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsold purchases: %w", err)
	}

	// Build campaign name lookup
	campaignList, err := s.repo.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	campaignNames := make(map[string]string, len(campaignList))
	for _, c := range campaignList {
		campaignNames[c.ID] = c.Name
	}

	items := make([]AgingItem, 0, len(purchases))
	for i := range purchases {
		items = append(items, s.enrichAgingItem(ctx, &purchases[i], campaignNames[purchases[i].CampaignID]))
	}

	s.applyOpenFlags(ctx, items)
	s.enrichCompSummaries(ctx, items)

	// Compute crack candidates for signal enrichment
	crackSet := s.buildCrackCandidateSet(ctx)

	// Apply inventory signals
	for i := range items {
		isCrack := crackSet[items[i].Purchase.ID]
		sig := ComputeInventorySignals(&items[i], isCrack)
		if sig.HasAnySignal() {
			items[i].Signals = &sig
		}
	}

	return items, nil
}

// GetFlaggedInventory returns only unsold cards that have at least one
// inventory signal set. Used by the liquidation analysis to receive
// pre-filtered, actionable cards instead of the full inventory.
func (s *service) GetFlaggedInventory(ctx context.Context) ([]AgingItem, error) {
	all, err := s.GetGlobalInventoryAging(ctx)
	if err != nil {
		return nil, err
	}
	var flagged []AgingItem
	for _, item := range all {
		if item.Signals.HasAnySignal() {
			flagged = append(flagged, item)
		}
	}
	return flagged, nil
}

// buildCrackCandidateSet returns the cached set of crack candidate purchase IDs.
// Returns nil on cold start (before the background worker has completed its first run);
// callers handle nil safely (Go nil-map index returns false).
//
// Safe: refreshCrackCandidates replaces the map atomically (never mutates in-place),
// so callers can iterate the returned reference without holding the lock.
func (s *service) buildCrackCandidateSet(ctx context.Context) map[string]bool {
	s.crackCacheMu.RLock()
	set := s.crackCacheSet
	s.crackCacheMu.RUnlock()
	if set == nil && s.logger != nil {
		s.logger.Info(ctx, "crack cache not yet populated, signals will be incomplete")
	}
	return set
}

// refreshCrackCandidates recomputes the crack candidate set and stores it in the cache.
func (s *service) refreshCrackCandidates(ctx context.Context) error {
	cracks, err := s.GetCrackOpportunities(ctx)
	if err != nil {
		return err
	}
	set := make(map[string]bool, len(cracks))
	for _, c := range cracks {
		if c.IsCrackCandidate {
			set[c.PurchaseID] = true
		}
	}
	s.crackCacheMu.Lock()
	s.crackCacheSet = set
	s.crackCacheMu.Unlock()
	return nil
}

const crackCacheRefreshInterval = 15 * time.Minute

// crackCacheWorker periodically refreshes the crack candidate cache.
func (s *service) crackCacheWorker(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if err := s.refreshCrackCandidates(ctx); err != nil && s.logger != nil {
		s.logger.Error(ctx, "initial crack cache refresh failed — inventory signals unavailable", observability.Err(err))
	}
	ticker := time.NewTicker(crackCacheRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.refreshCrackCandidates(ctx); err != nil && s.logger != nil {
				s.logger.Warn(ctx, "crack cache refresh failed", observability.Err(err))
			}
		}
	}
}

// applyOpenFlags batch-loads open price flag status and sets HasOpenFlag on matching items.
func (s *service) applyOpenFlags(ctx context.Context, items []AgingItem) {
	flaggedIDs, err := s.repo.OpenFlagPurchaseIDs(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "failed to load open flags", observability.Err(err))
		}
		return
	}
	for i := range items {
		if flaggedIDs[items[i].Purchase.ID] {
			items[i].HasOpenFlag = true
		}
	}
}

// --- Tuning ---

func (s *service) GetCampaignTuning(ctx context.Context, campaignID string) (*TuningResponse, error) {
	campaign, err := s.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("campaign lookup: %w", err)
	}

	// Fan out independent repository queries in parallel.
	// Each goroutine writes to its own variable — no shared mutable state.
	g, gCtx := errgroup.WithContext(ctx)

	var byGrade []GradePerformance
	var data []PurchaseWithSale
	var pnl *CampaignPNL
	var dailySpend []DailySpend
	var channelPNL []ChannelPNL

	g.Go(func() error {
		var err error
		byGrade, err = s.repo.GetPerformanceByGrade(gCtx, campaignID)
		if err != nil {
			return fmt.Errorf("grade performance: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		data, err = s.repo.GetPurchasesWithSales(gCtx, campaignID)
		if err != nil {
			return fmt.Errorf("purchases with sales: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		pnl, err = s.repo.GetCampaignPNL(gCtx, campaignID)
		if err != nil {
			return fmt.Errorf("campaign PNL: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		dailySpend, err = s.repo.GetDailySpend(gCtx, campaignID, 30)
		if err != nil {
			return fmt.Errorf("daily spend: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		channelPNL, err = s.repo.GetPNLByChannel(gCtx, campaignID)
		if err != nil {
			return fmt.Errorf("channel PNL: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Layer 1: Segmented performance (pure functions)
	fixedTiers, relativeTiers := computePriceTierPerformance(data)
	topPerformers, bottomPerformers := computeCardPerformance(data, 5)
	threshold := computeBuyThresholdAnalysis(data, campaign.BuyTermsCLPct)

	// Layer 2: Market alignment — use stored snapshot data (no live API calls).
	// The inventory refresh scheduler keeps snapshots fresh in the background.
	currentSnapshots := make(map[string]*MarketSnapshot)
	for _, d := range data {
		if d.Sale != nil {
			continue
		}
		key := purchaseKey(d.Purchase.CardName, d.Purchase.CardNumber, d.Purchase.SetName, d.Purchase.GradeValue)
		if _, exists := currentSnapshots[key]; exists {
			continue
		}
		snap := snapshotFromPurchase(&d.Purchase)
		if snap != nil && d.Purchase.CLValueCents > 0 {
			applyCLSignal(snap, d.Purchase.CLValueCents)
		}
		if hasAnyPriceData(snap) {
			currentSnapshots[key] = snap
		}
	}
	alignment := computeMarketAlignment(data, currentSnapshots)
	enrichCardPerformance(topPerformers, currentSnapshots)
	enrichCardPerformance(bottomPerformers, currentSnapshots)

	// Layer 3: Recommendations
	recommendations := computeRecommendations(&TuningInput{
		Campaign:    campaign,
		PNL:         pnl,
		ByGrade:     byGrade,
		ByFixedTier: fixedTiers,
		Threshold:   threshold,
		Alignment:   alignment,
		DailySpend:  dailySpend,
		ChannelPNL:  channelPNL,
	})

	return &TuningResponse{
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
