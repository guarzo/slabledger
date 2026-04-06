package campaigns

import (
	"context"
	"testing"
)

func makePWS(cardName string, grade float64, buyCost, clValue int, sale *Sale) PurchaseWithSale {
	return PurchaseWithSale{
		Purchase: Purchase{
			ID:                  "p-" + cardName,
			CampaignID:          "campaign-1",
			CardName:            cardName,
			CertNumber:          "cert-" + cardName,
			GradeValue:          grade,
			CLValueCents:        clValue,
			BuyCostCents:        buyCost,
			PSASourcingFeeCents: 300,
			PurchaseDate:        "2026-01-15",
			MarketSnapshotData:  MarketSnapshotData{MedianCents: clValue}, // assume median = CL at purchase time
		},
		Sale: sale,
	}
}

func makeSale(price, fee, profit, days int, channel SaleChannel) *Sale {
	return &Sale{
		ID:             "s-1",
		SaleChannel:    channel,
		SalePriceCents: price,
		SaleFeeCents:   fee,
		NetProfitCents: profit,
		DaysToSell:     days,
		SaleDate:       "2026-02-01",
	}
}

func Test_computePriceTierPerformance_FixedBuckets(t *testing.T) {
	data := []PurchaseWithSale{
		makePWS("card1", 9, 3000, 4000, makeSale(5000, 600, 1100, 10, SaleChannelEbay)),    // $30 -> $0-50
		makePWS("card2", 9, 8000, 10000, makeSale(12000, 1400, 2300, 15, SaleChannelEbay)), // $80 -> $50-100
		makePWS("card3", 10, 15000, 18000, nil),                                            // $150 -> $100-200
	}

	fixed, relative := computePriceTierPerformance(data)

	if len(fixed) != len(fixedTiers) {
		t.Fatalf("expected %d fixed tiers, got %d", len(fixedTiers), len(fixed))
	}

	// $0-50 bucket
	if fixed[0].PurchaseCount != 1 {
		t.Errorf("$0-50 PurchaseCount = %d, want 1", fixed[0].PurchaseCount)
	}
	if fixed[0].SoldCount != 1 {
		t.Errorf("$0-50 SoldCount = %d, want 1", fixed[0].SoldCount)
	}

	// $50-100 bucket
	if fixed[1].PurchaseCount != 1 {
		t.Errorf("$50-100 PurchaseCount = %d, want 1", fixed[1].PurchaseCount)
	}

	// $100-200 bucket
	if fixed[2].PurchaseCount != 1 {
		t.Errorf("$100-200 PurchaseCount = %d, want 1", fixed[2].PurchaseCount)
	}
	if fixed[2].SoldCount != 0 {
		t.Errorf("$100-200 SoldCount = %d, want 0", fixed[2].SoldCount)
	}

	// Relative quartiles should have 4 tiers
	if len(relative) != 4 {
		t.Fatalf("expected 4 relative tiers, got %d", len(relative))
	}
}

func Test_computePriceTierPerformance_Empty(t *testing.T) {
	fixed, relative := computePriceTierPerformance(nil)
	if len(fixed) != len(fixedTiers) {
		t.Errorf("expected %d fixed tiers for empty data, got %d", len(fixedTiers), len(fixed))
	}
	// Relative tiers are nil when quartileBuckets returns nil for empty input
	for _, r := range relative {
		if r.PurchaseCount != 0 {
			t.Errorf("expected 0 purchases in relative tier for empty data, got %d", r.PurchaseCount)
		}
	}
}

func Test_computeCardPerformance(t *testing.T) {
	data := []PurchaseWithSale{
		makePWS("winner", 10, 5000, 8000, makeSale(10000, 1200, 3500, 5, SaleChannelEbay)),
		makePWS("loser", 8, 20000, 22000, makeSale(15000, 1800, -7100, 30, SaleChannelEbay)),
		makePWS("mid", 9, 10000, 12000, makeSale(13000, 1500, 1200, 12, SaleChannelInPerson)),
	}

	top, bottom := computeCardPerformance(data, 2)

	if len(top) != 2 {
		t.Fatalf("expected 2 top performers, got %d", len(top))
	}
	if top[0].Purchase.CardName != "winner" {
		t.Errorf("top[0] = %s, want winner", top[0].Purchase.CardName)
	}
	if top[0].RealizedPnL != 3500 {
		t.Errorf("top[0] RealizedPnL = %d, want 3500", top[0].RealizedPnL)
	}

	if len(bottom) != 1 {
		t.Fatalf("expected 1 bottom performer, got %d", len(bottom))
	}
	if bottom[0].Purchase.CardName != "loser" {
		t.Errorf("bottom[0] = %s, want loser", bottom[0].Purchase.CardName)
	}
}

func Test_computeCardPerformance_Empty(t *testing.T) {
	top, bottom := computeCardPerformance(nil, 5)
	if top != nil || bottom != nil {
		t.Error("expected nil for empty data")
	}
}

func Test_computeBuyThresholdAnalysis(t *testing.T) {
	// Create purchases at various CL% levels with different outcomes
	var data []PurchaseWithSale
	// Good purchases at 70-75% CL (high ROI)
	for i := 0; i < 5; i++ {
		data = append(data, makePWS("good"+string(rune('0'+i)), 9, 7000, 10000,
			makeSale(12000, 1400, 3300, 10, SaleChannelEbay)))
	}
	// Bad purchases at 90-95% CL (low/negative ROI)
	for i := 0; i < 5; i++ {
		data = append(data, makePWS("bad"+string(rune('0'+i)), 9, 9200, 10000,
			makeSale(10000, 1200, -700, 25, SaleChannelEbay)))
	}

	result := computeBuyThresholdAnalysis(data, 0.90)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.SampleSize != 10 {
		t.Errorf("SampleSize = %d, want 10", result.SampleSize)
	}
	if result.CurrentPct != 0.90 {
		t.Errorf("CurrentPct = %f, want 0.90", result.CurrentPct)
	}
	// Optimal should be around 70-75% since those have better ROI
	if result.OptimalPct > 0.80 {
		t.Errorf("OptimalPct = %f, expected <= 0.80 (should favor lower CL%%)", result.OptimalPct)
	}
	if len(result.BucketedROI) == 0 {
		t.Error("expected non-empty BucketedROI")
	}
}

func Test_computeBuyThresholdAnalysis_NoCLValues(t *testing.T) {
	data := []PurchaseWithSale{
		makePWS("card1", 9, 5000, 0, nil), // CLValueCents = 0
	}
	result := computeBuyThresholdAnalysis(data, 0.85)
	if result != nil {
		t.Error("expected nil when no CL values")
	}
}

func Test_computeMarketAlignment(t *testing.T) {
	data := []PurchaseWithSale{
		{
			Purchase: Purchase{
				CardName: "rising", GradeValue: 10,
				MarketSnapshotData: MarketSnapshotData{MedianCents: 10000},
			},
		},
		{
			Purchase: Purchase{
				CardName: "falling", GradeValue: 9,
				MarketSnapshotData: MarketSnapshotData{MedianCents: 10000},
			},
		},
		{
			Purchase: Purchase{
				CardName: "stable", GradeValue: 9,
				MarketSnapshotData: MarketSnapshotData{MedianCents: 10000},
			},
			Sale: &Sale{}, // sold — should be excluded
		},
	}

	snapshots := map[string]*MarketSnapshot{
		"rising|||10": {MedianCents: 12000, Trend30d: 0.15, Trend90d: 0.10, SalesLast30d: 8, Volatility: 0.05},
		"falling|||9": {MedianCents: 8000, Trend30d: -0.12, Trend90d: -0.08, SalesLast30d: 3, Volatility: 0.20},
	}

	result := computeMarketAlignment(data, snapshots)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.SampleSize != 2 {
		t.Errorf("SampleSize = %d, want 2 (sold cards excluded)", result.SampleSize)
	}
	if result.AppreciatingCount != 1 {
		t.Errorf("AppreciatingCount = %d, want 1", result.AppreciatingCount)
	}
	if result.DepreciatingCount != 1 {
		t.Errorf("DepreciatingCount = %d, want 1", result.DepreciatingCount)
	}
	if result.Signal == "" {
		t.Error("expected signal to be set")
	}
}

func Test_computeMarketAlignment_NoUnsold(t *testing.T) {
	data := []PurchaseWithSale{
		{Purchase: Purchase{CardName: "sold", GradeValue: 9}, Sale: &Sale{}},
	}
	result := computeMarketAlignment(data, nil)
	if result != nil {
		t.Error("expected nil when no unsold cards")
	}
}

func Test_computeRecommendations_BuyThreshold(t *testing.T) {
	campaign := &Campaign{BuyTermsCLPct: 0.90, GradeRange: "8-10", Phase: PhaseActive}

	threshold := &BuyThresholdAnalysis{
		OptimalPct: 0.75,
		CurrentPct: 0.90,
		SampleSize: 30,
		Confidence: 30,
		BucketedROI: []ThresholdBucket{
			{RangeMinPct: 0.70, RangeMaxPct: 0.80, MedianROI: 0.20, Count: 10},
			{RangeMinPct: 0.85, RangeMaxPct: 0.95, MedianROI: 0.03, Count: 10},
		},
	}

	recs := computeRecommendations(&TuningInput{Campaign: campaign, Threshold: threshold})
	found := false
	for _, r := range recs {
		if r.Parameter == "buyTermsCLPct" {
			found = true
			if r.Impact != "high" {
				t.Errorf("expected high impact, got %s", r.Impact)
			}
		}
	}
	if !found {
		t.Error("expected buyTermsCLPct recommendation")
	}
}

func Test_computeRecommendations_UnderperformingGrade(t *testing.T) {
	campaign := &Campaign{GradeRange: "8-10", BuyTermsCLPct: 0.85}

	byGrade := []GradePerformance{
		{Grade: 8, PurchaseCount: 10, ROI: -0.08},
		{Grade: 9, PurchaseCount: 15, ROI: 0.12},
		{Grade: 10, PurchaseCount: 12, ROI: 0.18},
	}

	recs := computeRecommendations(&TuningInput{Campaign: campaign, ByGrade: byGrade})
	found := false
	for _, r := range recs {
		if r.Parameter == "gradeRange" {
			found = true
			if r.DataPoints != 10 {
				t.Errorf("expected 10 data points, got %d", r.DataPoints)
			}
		}
	}
	if !found {
		t.Error("expected gradeRange recommendation for underperforming PSA 8")
	}
}

func Test_computeRecommendations_LowSellThrough(t *testing.T) {
	campaign := &Campaign{BuyTermsCLPct: 0.85, Phase: PhaseActive}
	pnl := &CampaignPNL{
		TotalPurchases: 30,
		TotalSold:      5,
		SellThroughPct: 0.17,
		AvgDaysToSell:  45,
	}

	recs := computeRecommendations(&TuningInput{Campaign: campaign, PNL: pnl})
	found := false
	for _, r := range recs {
		if r.Parameter == "phase" && r.SuggestedVal == string(PhasePending) {
			found = true
		}
	}
	if !found {
		t.Error("expected pause recommendation for low sell-through with high days-to-sell")
	}
}

func Test_computeRecommendations_MarketWarning(t *testing.T) {
	campaign := &Campaign{BuyTermsCLPct: 0.85, Phase: PhaseActive}
	alignment := &MarketAlignment{
		Signal:       "warning",
		SignalReason: "Market trending down -15%",
		SampleSize:   10,
	}

	recs := computeRecommendations(&TuningInput{Campaign: campaign, Alignment: alignment})
	found := false
	for _, r := range recs {
		if r.Parameter == "phase" {
			found = true
		}
	}
	if !found {
		t.Error("expected phase recommendation for market warning")
	}
}

func Test_computeRecommendations_NoRecsWhenDataInsufficient(t *testing.T) {
	campaign := &Campaign{BuyTermsCLPct: 0.85}
	// Only 2 grades, each with < 5 purchases — should not fire grade rule
	byGrade := []GradePerformance{
		{Grade: 9, PurchaseCount: 3, ROI: -0.10},
		{Grade: 10, PurchaseCount: 4, ROI: 0.15},
	}
	pnl := &CampaignPNL{TotalPurchases: 7, SellThroughPct: 0.20} // < 20, won't fire sell-through rule

	recs := computeRecommendations(&TuningInput{Campaign: campaign, PNL: pnl, ByGrade: byGrade})
	for _, r := range recs {
		if r.Parameter == "gradeRange" {
			t.Error("should not recommend grade change with < 5 purchases per grade")
		}
		if r.Parameter == "phase" {
			t.Error("should not recommend pause with < 20 total purchases")
		}
	}
}

func Test_computeRecommendations_ChannelOptimization(t *testing.T) {
	campaign := &Campaign{BuyTermsCLPct: 0.85}
	channelPNL := []ChannelPNL{
		{Channel: SaleChannelEbay, SaleCount: 10, NetProfitCents: -5000},
		{Channel: SaleChannelInPerson, SaleCount: 8, NetProfitCents: 16000},
	}

	recs := computeRecommendations(&TuningInput{Campaign: campaign, ChannelPNL: channelPNL})
	found := false
	for _, r := range recs {
		if r.Parameter == "saleChannel" {
			found = true
			if r.Impact != "medium" {
				t.Errorf("expected medium impact, got %s", r.Impact)
			}
		}
	}
	if !found {
		t.Error("expected saleChannel recommendation when worst channel loses money")
	}
}

func Test_computeRecommendations_ChannelOptimization_NoRecWhenProfitable(t *testing.T) {
	campaign := &Campaign{BuyTermsCLPct: 0.85}
	channelPNL := []ChannelPNL{
		{Channel: SaleChannelEbay, SaleCount: 10, NetProfitCents: 5000},
		{Channel: SaleChannelInPerson, SaleCount: 8, NetProfitCents: 16000},
	}

	recs := computeRecommendations(&TuningInput{Campaign: campaign, ChannelPNL: channelPNL})
	for _, r := range recs {
		if r.Parameter == "saleChannel" {
			t.Error("should not recommend channel change when worst channel is profitable")
		}
	}
}

func TestService_GetCampaignTuning(t *testing.T) {
	repo := newMockRepo()
	closedCtx, closedCancel := context.WithCancel(context.Background())
	closedCancel()
	svc := NewService(repo, WithIDGenerator(internalTestIDGen()), WithBaseContext(closedCtx), WithPriceLookup(newDefaultPriceLookup(nil, "")))
	ctx := context.Background()

	c := &Campaign{Name: "Tuning Test", BuyTermsCLPct: 0.85, GradeRange: "9-10", EbayFeePct: 0.1235}
	_ = svc.CreateCampaign(ctx, c)

	// Add some purchases
	for i, name := range []string{"Charizard", "Pikachu", "Blastoise"} {
		p := &Purchase{
			CampaignID:   c.ID,
			CardName:     name,
			CertNumber:   string(rune('A'+i)) + "1234",
			GradeValue:   9,
			CLValueCents: 10000,
			BuyCostCents: 8500,
			PurchaseDate: "2026-01-15",
		}
		_ = svc.CreatePurchase(ctx, p)
	}

	tuning, err := svc.GetCampaignTuning(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetCampaignTuning: %v", err)
	}
	if tuning.CampaignID != c.ID {
		t.Errorf("CampaignID = %s, want %s", tuning.CampaignID, c.ID)
	}
	if tuning.CampaignName != "Tuning Test" {
		t.Errorf("CampaignName = %s", tuning.CampaignName)
	}
	// Should have fixed tiers (even if empty)
	if len(tuning.ByFixedTier) != len(fixedTiers) {
		t.Errorf("expected %d fixed tiers, got %d", len(fixedTiers), len(tuning.ByFixedTier))
	}
}

func TestEnrichCardPerformance(t *testing.T) {
	cards := []CardPerformance{
		{
			Purchase: Purchase{CardName: "Charizard", GradeValue: 10, BuyCostCents: 5000, PSASourcingFeeCents: 300},
		},
	}
	snapshots := map[string]*MarketSnapshot{
		"Charizard|||10": {MedianCents: 8000, Trend30d: 0.10},
	}

	enrichCardPerformance(cards, snapshots)

	if cards[0].CurrentMarket == nil {
		t.Fatal("expected CurrentMarket to be set")
	}
	if cards[0].UnrealizedPnL != 2700 { // 8000 - 5300
		t.Errorf("UnrealizedPnL = %d, want 2700", cards[0].UnrealizedPnL)
	}
}
