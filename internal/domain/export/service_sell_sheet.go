package export

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/timeutil"
)

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
	if item.TargetSellPrice > 0 && inventory.NormalizeChannel(item.RecommendedChannel) == inventory.SaleChannelEbay {
		item.TargetSellPrice -= int(math.Round(float64(item.TargetSellPrice) * ebayFeePct))
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
	sheet.Totals.SkippedItems += skipped
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
			feePct = inventory.EffectiveFeePct(c)
		} else {
			feePct = inventory.EffectiveFeePct(&inventory.Campaign{})
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
