package export

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/timeutil"
)

// grossModeFee signals enrichSellSheetItem to skip fee deduction, returning gross prices.
const grossModeFee = -1.0

// --- Sell Sheet ---

// enrichSellSheetItem builds a SellSheetItem from a purchase using stored snapshot data.
// ebayFeePct is applied to eBay/TCGPlayer channel items to compute net revenue.
// Returns the item and whether market data was available.
func (s *service) enrichSellSheetItem(_ context.Context, purchase *inventory.Purchase, campaignName string, ebayFeePct float64, crackSet map[string]bool) (inventory.SellSheetItem, bool) {
	costBasis := purchase.BuyCostCents + purchase.PSASourcingFeeCents
	item := inventory.SellSheetItem{
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
		PSAShipDate:    purchase.PSAShipDate,
	}

	hasMarket := false
	snapshot := inventory.SnapshotFromPurchase(purchase)
	switch {
	case snapshot == nil:
		item.PriceLookupError = fmt.Sprintf("no snapshot: card=%q set=%q grade=%g", purchase.CardName, purchase.SetName, purchase.GradeValue)
	case !inventory.HasAnyPriceData(snapshot):
		item.PriceLookupError = fmt.Sprintf("zero prices: card=%q set=%q grade=%g", purchase.CardName, purchase.SetName, purchase.GradeValue)
	default:
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

	// Compute inventory signals
	agingItem := inventory.AgingItem{
		Purchase:      *purchase,
		CurrentMarket: item.CurrentMarket,
		DaysHeld:      timeutil.DaysSince(purchase.PurchaseDate),
	}
	isCrack := crackSet[purchase.ID]
	sig := inventory.ComputeInventorySignals(&agingItem, isCrack)
	if sig.HasAnySignal() {
		item.Signals = &sig
	}

	// Compute recommended channel server-side
	item.RecommendedChannel, item.ChannelLabel = recommendChannel(purchase.GradeValue, item.CurrentMarket, item.Signals)

	// Deduct marketplace fees for eBay/TCGPlayer channels to project net revenue.
	// grossModeFee skips fee deduction (used by price sync to return gross prices).
	if ebayFeePct != grossModeFee && item.TargetSellPrice > 0 && inventory.NormalizeChannel(item.RecommendedChannel) == inventory.SaleChannelEbay {
		feePct := ebayFeePct
		if feePct == 0 {
			feePct = inventory.DefaultMarketplaceFeePct
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
func recommendChannel(grade float64, mkt *inventory.MarketSnapshot, signals *inventory.InventorySignals) (inventory.SaleChannel, string) {
	if grade == 7 {
		return inventory.SaleChannelInPerson, "In Person"
	}
	if signals != nil {
		if signals.ProfitCaptureDeclining || signals.ProfitCaptureSpike || signals.CrackCandidate {
			return inventory.SaleChannelInPerson, "In Person"
		}
	}
	if mkt != nil && mkt.Trend30d > 0.05 {
		return inventory.SaleChannelInPerson, "In Person"
	}
	return inventory.SaleChannelEbay, "eBay"
}

func (s *service) GenerateSellSheet(ctx context.Context, campaignID string, purchaseIDs []string) (*inventory.SellSheet, error) {
	campaign, err := s.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("campaign lookup: %w", err)
	}

	// Batch fetch all purchases in one query instead of N separate calls.
	purchaseMap, err := s.repo.GetPurchasesByIDs(ctx, purchaseIDs)
	if err != nil {
		return nil, fmt.Errorf("batch purchase lookup: %w", err)
	}

	crackSet := s.buildCrackCandidateSet(ctx)

	sheet := &inventory.SellSheet{
		GeneratedAt:  time.Now().Format(time.RFC3339),
		CampaignName: campaign.Name,
	}

	for _, pid := range purchaseIDs {
		purchase, ok := purchaseMap[pid]
		if !ok || purchase.CampaignID != campaignID {
			sheet.Totals.SkippedItems++
			continue
		}
		if purchase.ReceivedAt == nil {
			sheet.Totals.SkippedItems++
			continue
		}

		item, ok := s.enrichSellSheetItem(ctx, purchase, "", campaign.EbayFeePct, crackSet)
		if !ok {
			sheet.Totals.SkippedItems++
			continue
		}
		sheet.Totals.TotalExpectedRevenue += item.TargetSellPrice
		sheet.Items = append(sheet.Items, item)
		sheet.Totals.TotalCostBasis += item.CostBasisCents
		sheet.Totals.ItemCount++
	}

	sheet.Totals.TotalProjectedProfit = sheet.Totals.TotalExpectedRevenue - sheet.Totals.TotalCostBasis
	return sheet, nil
}

func (s *service) GenerateGlobalSellSheet(ctx context.Context) (*inventory.SellSheet, error) {
	purchases, err := s.repo.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsold purchases: %w", err)
	}

	// Only include cards that are physically in hand — a pending PSA return
	// can't be sold at a card show or shipped off an eBay listing.
	ptrs := make([]*inventory.Purchase, 0, len(purchases))
	skipped := 0
	for i := range purchases {
		if purchases[i].ReceivedAt == nil {
			skipped++
			continue
		}
		ptrs = append(ptrs, &purchases[i])
	}

	sheet, err := s.buildCrossCampaignSellSheet(ctx, ptrs, "All Inventory")
	if err != nil {
		return nil, err
	}
	sheet.Totals.SkippedItems = skipped
	return sheet, nil
}

func (s *service) GenerateSelectedSellSheet(ctx context.Context, purchaseIDs []string) (*inventory.SellSheet, error) {
	purchaseMap, err := s.repo.GetPurchasesByIDs(ctx, purchaseIDs)
	if err != nil {
		return nil, fmt.Errorf("batch purchase lookup: %w", err)
	}

	var ptrs []*inventory.Purchase
	skipped := 0
	for _, pid := range purchaseIDs {
		purchase, ok := purchaseMap[pid]
		if !ok {
			skipped++
			continue
		}
		if purchase.ReceivedAt == nil {
			skipped++
			continue
		}
		ptrs = append(ptrs, purchase)
	}

	sheet, err := s.buildCrossCampaignSellSheet(ctx, ptrs, "Selected Inventory")
	if err != nil {
		return nil, err
	}
	sheet.Totals.SkippedItems = skipped
	return sheet, nil
}

// buildCrossCampaignSellSheet builds a sell sheet from purchases that may span
// multiple campaigns, looking up each campaign's name and fee percentage.
func (s *service) buildCrossCampaignSellSheet(ctx context.Context, purchases []*inventory.Purchase, sheetName string) (*inventory.SellSheet, error) {
	campaignList, err := s.repo.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	campaignMap := make(map[string]*inventory.Campaign, len(campaignList))
	for i := range campaignList {
		campaignMap[campaignList[i].ID] = &campaignList[i]
	}

	crackSet := s.buildCrackCandidateSet(ctx)

	sheet := &inventory.SellSheet{
		GeneratedAt:  time.Now().Format(time.RFC3339),
		CampaignName: sheetName,
	}

	for _, purchase := range purchases {
		campName := ""
		var feePct float64
		if c := campaignMap[purchase.CampaignID]; c != nil {
			campName = c.Name
			feePct = c.EbayFeePct
		}
		item, ok := s.enrichSellSheetItem(ctx, purchase, campName, feePct, crackSet)
		if !ok {
			sheet.Totals.SkippedItems++
			continue
		}
		sheet.Totals.TotalExpectedRevenue += item.TargetSellPrice
		sheet.Items = append(sheet.Items, item)
		sheet.Totals.TotalCostBasis += item.CostBasisCents
		sheet.Totals.ItemCount++
	}

	sheet.Totals.TotalProjectedProfit = sheet.Totals.TotalExpectedRevenue - sheet.Totals.TotalCostBasis
	return sheet, nil
}

func computeRecommendation(snapshot *inventory.MarketSnapshot, clValueCents int) string {
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

func computeTargetPrice(snapshot *inventory.MarketSnapshot, recommendation string) int {
	switch recommendation {
	case "rising":
		return snapshot.OptimisticCents
	case "falling":
		return snapshot.ConservativeCents
	default:
		return snapshot.MedianCents
	}
}

// MatchShopifyPrices matches Shopify CSV items against inventory by cert number
// and returns suggested market-based prices (gross, no fee deduction).
func (s *service) MatchShopifyPrices(ctx context.Context, items []inventory.ShopifyPriceSyncItem) (*inventory.ShopifyPriceSyncResponse, error) {
	// Collect all cert numbers for a single grader-agnostic batch lookup
	certs := make([]string, len(items))
	for i, item := range items {
		certs[i] = item.CertNumber
	}
	purchaseMap, err := s.repo.GetPurchasesByCertNumbers(ctx, certs)
	if err != nil {
		return nil, fmt.Errorf("lookup purchases by cert: %w", err)
	}

	resp := &inventory.ShopifyPriceSyncResponse{}
	for _, item := range items {
		purchase, ok := purchaseMap[item.CertNumber]
		if !ok {
			resp.Unmatched = append(resp.Unmatched, item.CertNumber)
			continue
		}

		// Enrich with sell sheet logic — gross price (no fee deduction) for price sync comparison
		sellItem, hasMarket := s.enrichSellSheetItem(ctx, purchase, "", grossModeFee, nil)

		match := inventory.ShopifyPriceSyncMatch{
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
			match.LastSoldCents = sellItem.CurrentMarket.LastSoldCents
		}

		// Compute recommended price using resolution hierarchy
		recPrice, recSource := recommendedPrice(purchase, sellItem.CurrentMarket)
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
		resp.Matched = []inventory.ShopifyPriceSyncMatch{}
	}

	// Batch-enrich with DH market intelligence (best-effort — skip on error)
	if s.intelRepo != nil && len(resp.Matched) > 0 {
		keys := make([]intelligence.CardKey, len(resp.Matched))
		for i, m := range resp.Matched {
			keys[i] = intelligence.CardKey{
				CardName:   m.CardName,
				SetName:    m.SetName,
				CardNumber: m.CardNumber,
			}
		}
		if intelMap, err := s.intelRepo.GetByCards(ctx, keys); err == nil {
			for i := range resp.Matched {
				if mi, ok := intelMap[keys[i]]; ok {
					resp.Matched[i].Intel = convertIntel(mi)
				}
			}
		} else if s.logger != nil {
			s.logger.Warn(ctx, "intel enrichment failed", observability.Err(err))
		}
	}

	return resp, nil
}

// recommendedPrice resolves the recommended price for a purchase using the hierarchy:
// 1. User-reviewed price (if set)
// 2. CL value (if > 0)
// 3. Market median (if > 0)
func recommendedPrice(p *inventory.Purchase, snapshot *inventory.MarketSnapshot) (int, string) {
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

// convertIntel converts a domain MarketIntelligence into the API-facing PriceSyncIntel.
// Returns nil if the input is nil.
func convertIntel(mi *intelligence.MarketIntelligence) *inventory.PriceSyncIntel {
	if mi == nil {
		return nil
	}
	out := &inventory.PriceSyncIntel{
		FetchedAt: mi.FetchedAt.Format(time.RFC3339),
	}
	if mi.Sentiment != nil {
		out.SentimentScore = mi.Sentiment.Score
		out.SentimentTrend = mi.Sentiment.Trend
		out.SentimentMentions = mi.Sentiment.MentionCount
	}
	if mi.Forecast != nil {
		out.ForecastCents = mi.Forecast.PredictedPriceCents
		out.ForecastConfidence = mi.Forecast.Confidence
		if !mi.Forecast.ForecastDate.IsZero() {
			out.ForecastDate = mi.Forecast.ForecastDate.Format(time.RFC3339)
		}
	}
	if mi.Insights != nil {
		out.InsightHeadline = mi.Insights.Headline
		out.InsightDetail = mi.Insights.Detail
	}

	// Recent sales — last 5, newest first (defensive copy to avoid mutating input)
	recentSales := make([]intelligence.Sale, len(mi.RecentSales))
	copy(recentSales, mi.RecentSales)
	sort.Slice(recentSales, func(i, j int) bool {
		return recentSales[i].SoldAt.After(recentSales[j].SoldAt)
	})
	out.RecentSalesCount = len(recentSales)
	limit := min(5, len(recentSales))
	for i := 0; i < limit; i++ {
		sale := recentSales[i]
		out.RecentSales = append(out.RecentSales, inventory.PriceSyncSale{
			SoldAt:     sale.SoldAt.Format(time.RFC3339),
			Grade:      sale.Grade,
			PriceCents: sale.PriceCents,
			Platform:   sale.Platform,
		})
	}

	// Population — PSA entries only
	for _, p := range mi.Population {
		if p.GradingCompany == "PSA" {
			out.Population = append(out.Population, inventory.PriceSyncPop{
				Grade: p.Grade,
				Count: p.Count,
			})
		}
	}

	// Grading ROI
	for _, r := range mi.GradingROI {
		out.GradingROI = append(out.GradingROI, inventory.PriceSyncROI{
			Grade:        r.Grade,
			AvgSaleCents: r.AvgSaleCents,
			ROI:          r.ROI,
		})
	}

	return out
}
