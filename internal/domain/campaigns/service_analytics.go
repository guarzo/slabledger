package campaigns

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
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
	daysHeld := 0
	if t, err := time.Parse("2006-01-02", p.PurchaseDate); err == nil {
		daysHeld = int(time.Since(t).Hours() / 24)
	}
	item := AgingItem{Purchase: *p, DaysHeld: daysHeld, CampaignName: campaignName}

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
		minMedianToBuyRatio          = 0.3
		maxMedianToBuyRatio          = 5.0
		lowFusionConfidenceThreshold = 0.5
	)
	if snap != nil && p.BuyCostCents > 0 && snap.MedianCents > 0 {
		ratio := float64(snap.MedianCents) / float64(p.BuyCostCents)
		lowConfidence := snap.SourceCount <= 1 || snap.FusionConfidence < lowFusionConfidenceThreshold || snap.IsEstimated || snap.PricingGap
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

	// Batch-load open flag status
	flaggedIDs, err := s.repo.OpenFlagPurchaseIDs(ctx)
	if err != nil {
		s.logger.Warn(ctx, "failed to load open flags", observability.Err(err))
	} else {
		for i := range items {
			if flaggedIDs[items[i].Purchase.ID] {
				items[i].HasOpenFlag = true
			}
		}
	}

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

	// Batch-load open flag status
	flaggedIDs, err := s.repo.OpenFlagPurchaseIDs(ctx)
	if err != nil {
		s.logger.Warn(ctx, "failed to load open flags", observability.Err(err))
	} else {
		for i := range items {
			if flaggedIDs[items[i].Purchase.ID] {
				items[i].HasOpenFlag = true
			}
		}
	}

	return items, nil
}

// --- Sell Sheet ---

// enrichSellSheetItem builds a SellSheetItem from a purchase using stored snapshot data.
// ebayFeePct is applied to eBay/TCGPlayer channel items to compute net revenue.
// Returns the item and whether market data was available.
func (s *service) enrichSellSheetItem(_ context.Context, purchase *Purchase, campaignName string, ebayFeePct float64) (SellSheetItem, bool) {
	costBasis := purchase.BuyCostCents + purchase.PSASourcingFeeCents
	item := SellSheetItem{
		PurchaseID:     purchase.ID,
		CampaignName:   campaignName,
		CertNumber:     purchase.CertNumber,
		CardName:       purchase.CardName,
		SetName:        purchase.SetName,
		CardNumber:     purchase.CardNumber,
		Grade:          purchase.GradeValue,
		Grader:         purchase.Grader,
		Population:     purchase.Population,
		BuyCostCents:   purchase.BuyCostCents,
		CostBasisCents: costBasis,
		CLValueCents:   purchase.CLValueCents,
	}

	hasMarket := false
	snapshot := snapshotFromPurchase(purchase)
	if snapshot == nil {
		item.PriceLookupError = fmt.Sprintf("no snapshot: card=%q set=%q grade=%g", purchase.CardName, purchase.SetName, purchase.GradeValue)
	} else if !hasAnyPriceData(snapshot) {
		item.PriceLookupError = fmt.Sprintf("zero prices: card=%q set=%q grade=%g", purchase.CardName, purchase.SetName, purchase.GradeValue)
	}
	if hasAnyPriceData(snapshot) {
		item.CurrentMarket = snapshot
		item.Recommendation = computeRecommendation(snapshot, purchase.CLValueCents)
		item.TargetSellPrice = computeTargetPrice(snapshot, item.Recommendation)
		item.MinimumAcceptPrice = snapshot.ConservativeCents
		hasMarket = true
	}

	// CL value is the price floor — never list below it
	if purchase.CLValueCents > 0 {
		if purchase.CLValueCents > item.TargetSellPrice {
			item.TargetSellPrice = purchase.CLValueCents
		}
		if purchase.CLValueCents > item.MinimumAcceptPrice {
			item.MinimumAcceptPrice = purchase.CLValueCents
		}
	}

	// Compute recommended channel server-side
	item.RecommendedChannel, item.ChannelLabel = recommendChannel(purchase.GradeValue, purchase.CLValueCents, item.CurrentMarket)

	// Deduct marketplace fees for eBay/TCGPlayer channels to project net revenue.
	// grossModeFee skips fee deduction (used by price sync to return gross prices).
	if ebayFeePct != grossModeFee && item.TargetSellPrice > 0 && (item.RecommendedChannel == SaleChannelEbay || item.RecommendedChannel == SaleChannelTCGPlayer) {
		feePct := ebayFeePct
		if feePct == 0 {
			feePct = DefaultMarketplaceFeePct
		}
		item.TargetSellPrice -= int(math.Round(float64(item.TargetSellPrice) * feePct))
	}

	// Preserve the algorithmically computed price before override
	item.ComputedPriceCents = item.TargetSellPrice

	// Override replaces the target price entirely — no CL floor or fee deduction applied
	if purchase.OverridePriceCents > 0 {
		item.TargetSellPrice = purchase.OverridePriceCents
		item.OverridePriceCents = purchase.OverridePriceCents
		item.OverrideSource = purchase.OverrideSource
		item.IsOverridden = true
	}

	// Surface AI suggestion for user review (does NOT change target price)
	if purchase.AISuggestedPriceCents > 0 {
		item.AISuggestedPriceCents = purchase.AISuggestedPriceCents
		item.AISuggestedAt = purchase.AISuggestedAt
	}

	return item, hasMarket
}

// recommendChannel determines the best exit channel for a sell-sheet item.
func recommendChannel(grade float64, clValueCents int, mkt *MarketSnapshot) (SaleChannel, string) {
	if grade >= 8 && clValueCents > 0 && clValueCents <= 150000 {
		return SaleChannelGameStop, "GameStop"
	}
	if grade == 7 {
		return SaleChannelCardShow, "Card Show"
	}
	if mkt != nil && mkt.Trend30d > 0.05 {
		return SaleChannelCardShow, "Card Show"
	}
	return SaleChannelEbay, "eBay"
}

func (s *service) GenerateSellSheet(ctx context.Context, campaignID string, purchaseIDs []string) (*SellSheet, error) {
	campaign, err := s.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("campaign lookup: %w", err)
	}

	sheet := &SellSheet{
		GeneratedAt:  time.Now().Format(time.RFC3339),
		CampaignName: campaign.Name,
	}

	for _, pid := range purchaseIDs {
		purchase, err := s.repo.GetPurchase(ctx, pid)
		if err != nil {
			sheet.Totals.SkippedItems++
			continue
		}
		if purchase.CampaignID != campaignID {
			sheet.Totals.SkippedItems++
			continue
		}

		item, _ := s.enrichSellSheetItem(ctx, purchase, "", campaign.EbayFeePct)
		sheet.Totals.TotalExpectedRevenue += item.TargetSellPrice
		sheet.Items = append(sheet.Items, item)
		sheet.Totals.TotalCostBasis += item.CostBasisCents
		sheet.Totals.ItemCount++
	}

	sheet.Totals.TotalProjectedProfit = sheet.Totals.TotalExpectedRevenue - sheet.Totals.TotalCostBasis
	return sheet, nil
}

func (s *service) GenerateGlobalSellSheet(ctx context.Context) (*SellSheet, error) {
	purchases, err := s.repo.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsold purchases: %w", err)
	}

	// Build campaign lookup for name and fee
	campaignList, err := s.repo.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	campaignMap := make(map[string]*Campaign, len(campaignList))
	for i := range campaignList {
		campaignMap[campaignList[i].ID] = &campaignList[i]
	}

	sheet := &SellSheet{
		GeneratedAt:  time.Now().Format(time.RFC3339),
		CampaignName: "All Inventory",
	}

	for i := range purchases {
		purchase := &purchases[i]
		campName := ""
		var feePct float64
		if c := campaignMap[purchase.CampaignID]; c != nil {
			campName = c.Name
			feePct = c.EbayFeePct
		}
		item, _ := s.enrichSellSheetItem(ctx, purchase, campName, feePct)
		sheet.Totals.TotalExpectedRevenue += item.TargetSellPrice
		sheet.Items = append(sheet.Items, item)
		sheet.Totals.TotalCostBasis += item.CostBasisCents
		sheet.Totals.ItemCount++
	}

	sheet.Totals.TotalProjectedProfit = sheet.Totals.TotalExpectedRevenue - sheet.Totals.TotalCostBasis
	return sheet, nil
}

func computeRecommendation(snapshot *MarketSnapshot, clValueCents int) string {
	if clValueCents <= 0 || snapshot.LastSoldCents <= 0 {
		return "stable"
	}
	deltaPct := float64(snapshot.LastSoldCents-clValueCents) / float64(clValueCents)
	if deltaPct >= 0.05 {
		return "rising"
	} else if deltaPct <= -0.05 {
		return "falling"
	}
	return "stable"
}

func computeTargetPrice(snapshot *MarketSnapshot, recommendation string) int {
	switch recommendation {
	case "rising":
		return snapshot.OptimisticCents
	case "falling":
		return snapshot.ConservativeCents
	default:
		return snapshot.MedianCents
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

// MatchShopifyPrices matches Shopify CSV items against inventory by cert number
// and returns suggested market-based prices (gross, no fee deduction).
func (s *service) MatchShopifyPrices(ctx context.Context, items []ShopifyPriceSyncItem) (*ShopifyPriceSyncResponse, error) {
	// Collect all cert numbers for a single grader-agnostic batch lookup
	certs := make([]string, len(items))
	for i, item := range items {
		certs[i] = item.CertNumber
	}
	purchaseMap, err := s.repo.GetPurchasesByCertNumbers(ctx, certs)
	if err != nil {
		return nil, fmt.Errorf("lookup purchases by cert: %w", err)
	}

	resp := &ShopifyPriceSyncResponse{}
	for _, item := range items {
		purchase, ok := purchaseMap[item.CertNumber]
		if !ok {
			resp.Unmatched = append(resp.Unmatched, item.CertNumber)
			continue
		}

		// Enrich with sell sheet logic — gross price (no fee deduction) for price sync comparison
		sellItem, hasMarket := s.enrichSellSheetItem(ctx, purchase, "", grossModeFee)

		match := ShopifyPriceSyncMatch{
			CertNumber:            item.CertNumber,
			CardName:              sellItem.CardName,
			SetName:               sellItem.SetName,
			CardNumber:            sellItem.CardNumber,
			Grade:                 sellItem.Grade,
			Grader:                sellItem.Grader,
			CurrentPriceCents:     item.CurrentPriceCents,
			SuggestedPriceCents:   sellItem.TargetSellPrice,
			MinimumPriceCents:     sellItem.MinimumAcceptPrice,
			CostBasisCents:        sellItem.CostBasisCents,
			CLValueCents:          sellItem.CLValueCents,
			Recommendation:        sellItem.Recommendation,
			HasMarketData:         hasMarket,
			OverridePriceCents:    purchase.OverridePriceCents,
			OverrideSource:        purchase.OverrideSource,
			AISuggestedPriceCents: purchase.AISuggestedPriceCents,
		}
		if sellItem.CurrentMarket != nil {
			match.MarketPriceCents = sellItem.CurrentMarket.MedianCents
		}

		// Compute recommended price using resolution hierarchy
		var snap *MarketSnapshot
		if sellItem.CurrentMarket != nil {
			snap = sellItem.CurrentMarket
		}
		recPrice, recSource := recommendedPrice(purchase, snap)
		match.RecommendedPriceCents = recPrice
		match.RecommendedSource = recSource
		match.ReviewedAt = purchase.ReviewedAt

		if item.CurrentPriceCents > 0 && match.SuggestedPriceCents > 0 {
			match.PriceDeltaPct = float64(match.SuggestedPriceCents-item.CurrentPriceCents) / float64(item.CurrentPriceCents)
		}
		resp.Matched = append(resp.Matched, match)
	}

	if resp.Unmatched == nil {
		resp.Unmatched = []string{}
	}
	if resp.Matched == nil {
		resp.Matched = []ShopifyPriceSyncMatch{}
	}
	return resp, nil
}

// --- Price Review ---

func (s *service) SetReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error {
	if priceCents < 0 {
		return errors.NewAppError(ErrCodeCampaignValidation, "price must be non-negative")
	}
	validSources := map[string]bool{"manual": true, "cl": true, "market": true, "cost_markup": true}
	if priceCents > 0 && !validSources[source] {
		return errors.NewAppError(ErrCodeCampaignValidation, "invalid review source: "+source)
	}
	return s.repo.UpdateReviewedPrice(ctx, purchaseID, priceCents, source)
}

func (s *service) GetReviewStats(ctx context.Context, campaignID string) (ReviewStats, error) {
	return s.repo.GetReviewStats(ctx, campaignID)
}

func (s *service) GetGlobalReviewStats(ctx context.Context) (ReviewStats, error) {
	return s.repo.GetGlobalReviewStats(ctx)
}

// --- Price Flags ---

func (s *service) CreatePriceFlag(ctx context.Context, purchaseID string, userID int64, reason string) (int64, error) {
	if !ValidPriceFlagReasons[PriceFlagReason(reason)] {
		return 0, errors.NewAppError(ErrCodeCampaignValidation, "invalid flag reason: "+reason)
	}
	// Verify purchase exists
	if _, err := s.repo.GetPurchase(ctx, purchaseID); err != nil {
		return 0, err
	}
	flag := &PriceFlag{
		PurchaseID: purchaseID,
		FlaggedBy:  userID,
		FlaggedAt:  time.Now(),
		Reason:     PriceFlagReason(reason),
	}
	return s.repo.CreatePriceFlag(ctx, flag)
}

func (s *service) ListPriceFlags(ctx context.Context, status string) ([]PriceFlagWithContext, error) {
	return s.repo.ListPriceFlags(ctx, status)
}

func (s *service) ResolvePriceFlag(ctx context.Context, flagID int64, resolvedBy int64) error {
	return s.repo.ResolvePriceFlag(ctx, flagID, resolvedBy)
}

// recommendedPrice resolves the recommended price for a purchase using the hierarchy:
// 1. User-reviewed price (if set)
// 2. CL value (if > 0)
// 3. Market median (if > 0)
func recommendedPrice(p *Purchase, snapshot *MarketSnapshot) (int, string) {
	if p.ReviewedPriceCents > 0 {
		return p.ReviewedPriceCents, "user_reviewed"
	}
	if p.CLValueCents > 0 {
		return p.CLValueCents, "card_ladder"
	}
	if snapshot != nil && snapshot.MedianCents > 0 {
		return snapshot.MedianCents, "market"
	}
	return 0, ""
}
