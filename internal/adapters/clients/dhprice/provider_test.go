package dhprice

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// --- test doubles ---
// These mocks live here (not in testutil/mocks) because MarketDataClient and
// CardIDLookup are defined in the dhprice package — moving them would create a
// circular import.

type mockMarketData struct {
	RecentSalesFn func(ctx context.Context, cardID int) ([]dh.RecentSale, error)
	CardLookupFn  func(ctx context.Context, cardID int) (*dh.CardLookupResponse, error)
	sales         []dh.RecentSale
	err           error
}

func (m *mockMarketData) RecentSales(ctx context.Context, cardID int) ([]dh.RecentSale, error) {
	if m.RecentSalesFn != nil {
		return m.RecentSalesFn(ctx, cardID)
	}
	return m.sales, m.err
}

func (m *mockMarketData) CardLookup(ctx context.Context, cardID int) (*dh.CardLookupResponse, error) {
	if m.CardLookupFn != nil {
		return m.CardLookupFn(ctx, cardID)
	}
	return nil, nil
}

type mockIDLookup struct {
	GetExternalIDFn func(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
	id              string
	err             error
}

func (m *mockIDLookup) GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error) {
	if m.GetExternalIDFn != nil {
		return m.GetExternalIDFn(ctx, cardName, setName, collectorNumber, provider)
	}
	return m.id, m.err
}

// --- tests ---

func TestGetPrice_NoCardID(t *testing.T) {
	tests := []struct {
		name    string
		lookup  *mockIDLookup
		wantNil bool
		wantErr bool
	}{
		{
			name:    "unmapped card returns nil",
			lookup:  &mockIDLookup{id: "", err: nil},
			wantNil: true,
		},
		{
			name:    "lookup error propagates",
			lookup:  &mockIDLookup{id: "", err: errors.New("db down")},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := New(&mockMarketData{}, tc.lookup)
			got, err := p.GetPrice(context.Background(), pricing.Card{
				Name: "Charizard", Set: "Base Set", Number: "4",
			})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNil && got != nil {
				t.Fatalf("expected nil price, got %+v", got)
			}
		})
	}
}

func TestGetPrice_WithSales(t *testing.T) {
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "10", Price: 100.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 120.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 110.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "9", Price: 50.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "9", Price: 60.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "8", Price: 30.00, Platform: "ebay"},
		{GradingCompany: "BGS", Grade: "10", Price: 200.00, Platform: "ebay"},
		{GradingCompany: "BGS", Grade: "9.5", Price: 80.00, Platform: "ebay"},
		{GradingCompany: "CGC", Grade: "9.5", Price: 70.00, Platform: "ebay"},
		{GradingCompany: "XYZ", Grade: "99", Price: 999.00, Platform: "ebay"}, // unknown, should be skipped
	}

	p := New(
		&mockMarketData{sales: sales},
		&mockIDLookup{id: "42"},
	)

	got, err := p.GetPrice(context.Background(), pricing.Card{
		Name: "Charizard", Set: "Base Set", Number: "4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil price")
	}

	tests := []struct {
		name      string
		got       int64
		wantCents int64
	}{
		{"PSA10 median of 100,110,120", got.Grades.PSA10Cents, 11000},
		{"PSA9 median of 50,60", got.Grades.PSA9Cents, 5500},
		{"PSA8 single sale 30", got.Grades.PSA8Cents, 3000},
		{"BGS10 single sale 200", got.Grades.BGS10Cents, 20000},
		{"9.5 median of 70,80 (BGS+CGC)", got.Grades.Grade95Cents, 7500},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.wantCents {
				t.Errorf("got %d cents, want %d", tc.got, tc.wantCents)
			}
		})
	}

	// Amount should equal PSA10Cents.
	if got.Amount != got.Grades.PSA10Cents {
		t.Errorf("Amount %d != PSA10Cents %d", got.Amount, got.Grades.PSA10Cents)
	}

	// Verify GradeDetails populated.
	if got.GradeDetails == nil {
		t.Fatal("GradeDetails is nil")
	}
	psa10Detail := got.GradeDetails[pricing.GradePSA10.String()]
	if psa10Detail == nil || psa10Detail.Estimate == nil {
		t.Fatal("missing PSA10 grade detail")
	}
	if psa10Detail.Estimate.PriceCents != 11000 {
		t.Errorf("PSA10 detail price = %d, want 11000", psa10Detail.Estimate.PriceCents)
	}
	if psa10Detail.Estimate.LowCents != 10000 {
		t.Errorf("PSA10 detail low = %d, want 10000", psa10Detail.Estimate.LowCents)
	}
	if psa10Detail.Estimate.HighCents != 12000 {
		t.Errorf("PSA10 detail high = %d, want 12000", psa10Detail.Estimate.HighCents)
	}
	if psa10Detail.Estimate.Confidence != dhConfidence {
		t.Errorf("PSA10 detail confidence = %f, want %f", psa10Detail.Estimate.Confidence, dhConfidence)
	}

	// Verify source metadata.
	if got.Source != pricing.Source(pricing.SourceDH) {
		t.Errorf("Source = %q, want %q", got.Source, pricing.SourceDH)
	}
	if len(got.Sources) != 1 || got.Sources[0] != "ebay" {
		t.Errorf("Sources = %v, want [ebay]", got.Sources)
	}
	if got.Currency != "USD" {
		t.Errorf("Currency = %q, want USD", got.Currency)
	}
}

func TestGetPrice_AmountFallback(t *testing.T) {
	// Only PSA 9 sales — Amount should fall back to PSA9Cents.
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "9", Price: 50.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "9", Price: 60.00, Platform: "ebay"},
	}

	p := New(
		&mockMarketData{sales: sales},
		&mockIDLookup{id: "42"},
	)

	got, err := p.GetPrice(context.Background(), pricing.Card{
		Name: "Charizard", Set: "Base Set", Number: "4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil price")
	}
	if got.Grades.PSA10Cents != 0 {
		t.Errorf("PSA10Cents = %d, want 0", got.Grades.PSA10Cents)
	}
	if got.Amount != got.Grades.PSA9Cents {
		t.Errorf("Amount %d != PSA9Cents %d (expected fallback)", got.Amount, got.Grades.PSA9Cents)
	}
	if got.Amount != 5500 {
		t.Errorf("Amount = %d, want 5500", got.Amount)
	}
}

func TestGetPrice_NilResult(t *testing.T) {
	tests := []struct {
		name   string
		client MarketDataClient
		lookup CardIDLookup
		card   pricing.Card
	}{
		{
			name:   "no sales returns nil",
			client: &mockMarketData{sales: nil},
			lookup: &mockIDLookup{id: "42"},
			card:   pricing.Card{Name: "Pikachu", Set: "Jungle", Number: "60"},
		},
		{
			name:   "only unknown grades returns nil",
			client: &mockMarketData{sales: []dh.RecentSale{{GradingCompany: "XYZ", Grade: "99", Price: 999.00}}},
			lookup: &mockIDLookup{id: "42"},
			card:   pricing.Card{Name: "Oddish", Set: "Jungle", Number: "58"},
		},
		{
			name:   "invalid card ID returns nil",
			client: &mockMarketData{},
			lookup: &mockIDLookup{id: "not-a-number"},
			card:   pricing.Card{Name: "Test", Set: "Set", Number: "1"},
		},
		{
			name:   "nil dependencies returns nil",
			client: nil,
			lookup: nil,
			card:   pricing.Card{Name: "Test", Set: "Set", Number: "1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := New(tc.client, tc.lookup)
			got, err := p.GetPrice(context.Background(), tc.card)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != nil {
				t.Fatalf("expected nil price, got %+v", got)
			}
		})
	}
}

func TestGetPrice_EmptyGradesSalesReturnsNil(t *testing.T) {
	// Sales with empty Grade field — buildPrice should skip them (no mapping in gradeKey)
	// and since byGrade is empty, buildPrice returns nil → GetPrice returns (nil, nil)
	p := New(
		&mockMarketData{sales: []dh.RecentSale{
			{GradingCompany: "PSA", Grade: "", Price: 100.00, SoldAt: "2026-01-01", Platform: "ebay"},
			{GradingCompany: "", Grade: "10", Price: 200.00, SoldAt: "2026-01-02", Platform: "ebay"},
		}},
		&mockIDLookup{id: "42"},
	)

	got, err := p.GetPrice(context.Background(), pricing.Card{
		Name: "Test", Set: "Test Set", Number: "1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil price for unrecognized grades, got %+v", got)
	}
}

func TestAvailable(t *testing.T) {
	tests := []struct {
		name   string
		client MarketDataClient
		lookup CardIDLookup
		want   bool
	}{
		{"both present", &mockMarketData{}, &mockIDLookup{}, true},
		{"nil client", nil, &mockIDLookup{}, false},
		{"nil lookup", &mockMarketData{}, nil, false},
		{"both nil", nil, nil, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := New(tc.client, tc.lookup)
			if got := p.Available(); got != tc.want {
				t.Errorf("Available() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSimpleMethods(t *testing.T) {
	p := New(nil, nil)

	if got := p.Name(); got != "doubleholo" {
		t.Errorf("Name() = %q, want %q", got, "doubleholo")
	}
}

func TestLookupCard(t *testing.T) {
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "10", Price: 50.00, Platform: "ebay"},
	}

	p := New(
		&mockMarketData{sales: sales},
		&mockIDLookup{id: "7"},
	)

	card := domainCards.Card{
		Name:            "Mewtwo",
		Number:          "10",
		PSAListingTitle: "Mewtwo Holo",
	}

	got, err := p.LookupCard(context.Background(), "Base Set", card)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil price from LookupCard")
	}
	if got.Grades.PSA10Cents != 5000 {
		t.Errorf("PSA10 = %d, want 5000", got.Grades.PSA10Cents)
	}
	if got.ProductName != "Mewtwo" {
		t.Errorf("ProductName = %q, want %q", got.ProductName, "Mewtwo")
	}
}

func TestBuildPrice_PlatformBreakdown(t *testing.T) {
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "10", Price: 100.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 120.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 110.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "9", Price: 50.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "9", Price: 60.00, Platform: "ebay"},
	}

	got := buildPrice("Charizard", sales)
	if got == nil {
		t.Fatal("expected non-nil price")
	}

	// Sources should list distinct platforms, not "doubleholo"
	if len(got.Sources) != 1 || got.Sources[0] != "ebay" {
		t.Errorf("Sources = %v, want [ebay]", got.Sources)
	}

	// PSA10 Ebay detail should be populated
	psa10 := got.GradeDetails[pricing.GradePSA10.String()]
	if psa10 == nil {
		t.Fatal("missing PSA10 grade detail")
	}
	if psa10.Ebay == nil {
		t.Fatal("PSA10 Ebay detail is nil")
	}
	if psa10.Ebay.PriceCents != 11000 {
		t.Errorf("PSA10 Ebay.PriceCents = %d, want 11000", psa10.Ebay.PriceCents)
	}
	if psa10.Ebay.MinCents != 10000 {
		t.Errorf("PSA10 Ebay.MinCents = %d, want 10000", psa10.Ebay.MinCents)
	}
	if psa10.Ebay.MaxCents != 12000 {
		t.Errorf("PSA10 Ebay.MaxCents = %d, want 12000", psa10.Ebay.MaxCents)
	}
	if psa10.Ebay.SalesCount != 3 {
		t.Errorf("PSA10 Ebay.SalesCount = %d, want 3", psa10.Ebay.SalesCount)
	}
	if psa10.Ebay.Confidence != "medium" {
		t.Errorf("PSA10 Ebay.Confidence = %q, want medium", psa10.Ebay.Confidence)
	}

	// Estimate should still contain cross-platform aggregate (same values here since all eBay)
	if psa10.Estimate == nil {
		t.Fatal("PSA10 Estimate is nil")
	}
	if psa10.Estimate.PriceCents != 11000 {
		t.Errorf("PSA10 Estimate.PriceCents = %d, want 11000", psa10.Estimate.PriceCents)
	}
}

func TestBuildPrice_MixedPlatforms(t *testing.T) {
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "10", Price: 100.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 120.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 90.00, Platform: "tcgplayer"},
		{GradingCompany: "PSA", Grade: "10", Price: 95.00, Platform: "tcgplayer"},
	}

	got := buildPrice("Pikachu", sales)
	if got == nil {
		t.Fatal("expected non-nil price")
	}

	// Sources should contain both platforms (sorted)
	if len(got.Sources) != 2 {
		t.Fatalf("Sources = %v, want 2 platforms", got.Sources)
	}
	wantSources := map[string]bool{"ebay": true, "tcgplayer": true}
	for _, s := range got.Sources {
		if !wantSources[s] {
			t.Errorf("unexpected source %q", s)
		}
	}

	// Estimate is cross-platform aggregate: median of [90, 95, 100, 120] = 97.50
	psa10 := got.GradeDetails[pricing.GradePSA10.String()]
	if psa10 == nil || psa10.Estimate == nil {
		t.Fatal("missing PSA10 Estimate detail")
	}
	if psa10.Estimate.PriceCents != 9750 {
		t.Errorf("Estimate.PriceCents = %d, want 9750 (cross-platform median)", psa10.Estimate.PriceCents)
	}

	// Ebay detail: median of [100, 120] = 110
	if psa10.Ebay == nil {
		t.Fatal("PSA10 Ebay detail is nil")
	}
	if psa10.Ebay.PriceCents != 11000 {
		t.Errorf("Ebay.PriceCents = %d, want 11000", psa10.Ebay.PriceCents)
	}
	if psa10.Ebay.SalesCount != 2 {
		t.Errorf("Ebay.SalesCount = %d, want 2", psa10.Ebay.SalesCount)
	}
}

func TestBuildPrice_NoPlatform(t *testing.T) {
	// Sales with empty platform string should still work (treated as unknown platform)
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "10", Price: 100.00, Platform: ""},
	}

	got := buildPrice("Mewtwo", sales)
	if got == nil {
		t.Fatal("expected non-nil price")
	}

	// Estimate should still be populated
	psa10 := got.GradeDetails[pricing.GradePSA10.String()]
	if psa10 == nil || psa10.Estimate == nil {
		t.Fatal("missing PSA10 Estimate")
	}

	// Ebay should NOT be populated (platform is not "ebay")
	if psa10.Ebay != nil {
		t.Error("Ebay detail should be nil for empty platform")
	}
}

func TestBuildPrice_LastSoldByGrade(t *testing.T) {
	tests := []struct {
		name   string
		sales  []dh.RecentSale
		verify func(*testing.T, *pricing.Price)
	}{
		{
			name: "single grade PSA10",
			sales: []dh.RecentSale{
				{GradingCompany: "PSA", Grade: "10", Price: 100.00, SoldAt: "2026-01-01", Platform: "ebay"},
				{GradingCompany: "PSA", Grade: "10", Price: 120.00, SoldAt: "2026-01-03", Platform: "ebay"},
				{GradingCompany: "PSA", Grade: "10", Price: 110.00, SoldAt: "2026-01-02", Platform: "ebay"},
			},
			verify: func(t *testing.T, got *pricing.Price) {
				if got.LastSoldByGrade == nil {
					t.Fatal("LastSoldByGrade is nil")
				}
				if got.LastSoldByGrade.PSA10 == nil {
					t.Fatal("PSA10 is nil")
				}
				if got.LastSoldByGrade.PSA10.LastSoldPrice != 120.00 {
					t.Errorf("PSA10.LastSoldPrice = %f, want 120.00", got.LastSoldByGrade.PSA10.LastSoldPrice)
				}
				if got.LastSoldByGrade.PSA10.LastSoldDate != "2026-01-03" {
					t.Errorf("PSA10.LastSoldDate = %q, want 2026-01-03", got.LastSoldByGrade.PSA10.LastSoldDate)
				}
				if got.LastSoldByGrade.PSA10.SaleCount != 3 {
					t.Errorf("PSA10.SaleCount = %d, want 3", got.LastSoldByGrade.PSA10.SaleCount)
				}
				// All other grade slots should be nil
				if got.LastSoldByGrade.PSA9 != nil {
					t.Error("PSA9 should be nil")
				}
				if got.LastSoldByGrade.PSA8 != nil {
					t.Error("PSA8 should be nil")
				}
				if got.LastSoldByGrade.PSA7 != nil {
					t.Error("PSA7 should be nil")
				}
				if got.LastSoldByGrade.PSA6 != nil {
					t.Error("PSA6 should be nil")
				}
				if got.LastSoldByGrade.Raw != nil {
					t.Error("Raw should be nil")
				}
			},
		},
		{
			name: "multiple grades",
			sales: []dh.RecentSale{
				{GradingCompany: "PSA", Grade: "10", Price: 100.00, SoldAt: "2026-01-01", Platform: "ebay"},
				{GradingCompany: "PSA", Grade: "10", Price: 120.00, SoldAt: "2026-01-05", Platform: "ebay"},
				{GradingCompany: "PSA", Grade: "9", Price: 50.00, SoldAt: "2026-01-02", Platform: "ebay"},
				{GradingCompany: "PSA", Grade: "9", Price: 60.00, SoldAt: "2026-01-04", Platform: "ebay"},
				{GradingCompany: "PSA", Grade: "8", Price: 30.00, SoldAt: "2026-01-03", Platform: "ebay"},
			},
			verify: func(t *testing.T, got *pricing.Price) {
				if got.LastSoldByGrade == nil {
					t.Fatal("LastSoldByGrade is nil")
				}
				// PSA10: 2 sales, most recent is "2026-01-05" with price 120.00
				if got.LastSoldByGrade.PSA10 == nil {
					t.Fatal("PSA10 is nil")
				}
				if got.LastSoldByGrade.PSA10.LastSoldPrice != 120.00 {
					t.Errorf("PSA10.LastSoldPrice = %f, want 120.00", got.LastSoldByGrade.PSA10.LastSoldPrice)
				}
				if got.LastSoldByGrade.PSA10.LastSoldDate != "2026-01-05" {
					t.Errorf("PSA10.LastSoldDate = %q, want 2026-01-05", got.LastSoldByGrade.PSA10.LastSoldDate)
				}
				if got.LastSoldByGrade.PSA10.SaleCount != 2 {
					t.Errorf("PSA10.SaleCount = %d, want 2", got.LastSoldByGrade.PSA10.SaleCount)
				}
				// PSA9: 2 sales, most recent is "2026-01-04" with price 60.00
				if got.LastSoldByGrade.PSA9 == nil {
					t.Fatal("PSA9 is nil")
				}
				if got.LastSoldByGrade.PSA9.LastSoldPrice != 60.00 {
					t.Errorf("PSA9.LastSoldPrice = %f, want 60.00", got.LastSoldByGrade.PSA9.LastSoldPrice)
				}
				if got.LastSoldByGrade.PSA9.LastSoldDate != "2026-01-04" {
					t.Errorf("PSA9.LastSoldDate = %q, want 2026-01-04", got.LastSoldByGrade.PSA9.LastSoldDate)
				}
				if got.LastSoldByGrade.PSA9.SaleCount != 2 {
					t.Errorf("PSA9.SaleCount = %d, want 2", got.LastSoldByGrade.PSA9.SaleCount)
				}
				// PSA8: 1 sale at "2026-01-03" with price 30.00
				if got.LastSoldByGrade.PSA8 == nil {
					t.Fatal("PSA8 is nil")
				}
				if got.LastSoldByGrade.PSA8.LastSoldPrice != 30.00 {
					t.Errorf("PSA8.LastSoldPrice = %f, want 30.00", got.LastSoldByGrade.PSA8.LastSoldPrice)
				}
				if got.LastSoldByGrade.PSA8.LastSoldDate != "2026-01-03" {
					t.Errorf("PSA8.LastSoldDate = %q, want 2026-01-03", got.LastSoldByGrade.PSA8.LastSoldDate)
				}
				if got.LastSoldByGrade.PSA8.SaleCount != 1 {
					t.Errorf("PSA8.SaleCount = %d, want 1", got.LastSoldByGrade.PSA8.SaleCount)
				}
				// PSA7 and PSA6 should be nil
				if got.LastSoldByGrade.PSA7 != nil {
					t.Error("PSA7 should be nil")
				}
				if got.LastSoldByGrade.PSA6 != nil {
					t.Error("PSA6 should be nil")
				}
			},
		},
		{
			name: "BGS10 and CGC 9.5 skipped",
			sales: []dh.RecentSale{
				{GradingCompany: "BGS", Grade: "10", Price: 200.00, SoldAt: "2026-01-01", Platform: "ebay"},
				{GradingCompany: "BGS", Grade: "10", Price: 210.00, SoldAt: "2026-01-02", Platform: "ebay"},
				{GradingCompany: "CGC", Grade: "9.5", Price: 70.00, SoldAt: "2026-01-01", Platform: "ebay"},
				{GradingCompany: "CGC", Grade: "9.5", Price: 75.00, SoldAt: "2026-01-03", Platform: "ebay"},
			},
			verify: func(t *testing.T, got *pricing.Price) {
				if got.LastSoldByGrade == nil {
					t.Fatal("LastSoldByGrade is nil")
				}
				// BGS10 and CGC 9.5 map to GradePSA95 and GradeBGS10, which are not tracked in LastSoldByGrade
				// So all PSA slots and Raw should be nil
				if got.LastSoldByGrade.PSA10 != nil {
					t.Error("PSA10 should be nil")
				}
				if got.LastSoldByGrade.PSA9 != nil {
					t.Error("PSA9 should be nil")
				}
				if got.LastSoldByGrade.PSA8 != nil {
					t.Error("PSA8 should be nil")
				}
				if got.LastSoldByGrade.PSA7 != nil {
					t.Error("PSA7 should be nil")
				}
				if got.LastSoldByGrade.PSA6 != nil {
					t.Error("PSA6 should be nil")
				}
				if got.LastSoldByGrade.Raw != nil {
					t.Error("Raw should be nil")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPrice("TestCard", tc.sales)
			if got == nil {
				t.Fatal("expected non-nil price")
			}
			tc.verify(t, got)
		})
	}
}

func TestBuildPrice_LastSoldByGrade_MostRecentWins(t *testing.T) {
	// 5 PSA 10 sales with ascending dates. The most recent should win.
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "10", Price: 100.00, SoldAt: "2026-01-01", Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 105.00, SoldAt: "2026-01-05", Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 103.00, SoldAt: "2026-01-03", Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 102.00, SoldAt: "2026-01-02", Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 104.00, SoldAt: "2026-01-04", Platform: "ebay"},
	}

	got := buildPrice("Charizard", sales)
	if got == nil {
		t.Fatal("expected non-nil price")
	}

	if got.LastSoldByGrade == nil {
		t.Fatal("LastSoldByGrade is nil")
	}
	if got.LastSoldByGrade.PSA10 == nil {
		t.Fatal("PSA10 is nil")
	}

	// The sale from "2026-01-05" should be selected (most recent).
	if got.LastSoldByGrade.PSA10.LastSoldDate != "2026-01-05" {
		t.Errorf("LastSoldDate = %q, want 2026-01-05", got.LastSoldByGrade.PSA10.LastSoldDate)
	}
	if got.LastSoldByGrade.PSA10.LastSoldPrice != 105.00 {
		t.Errorf("LastSoldPrice = %f, want 105.00", got.LastSoldByGrade.PSA10.LastSoldPrice)
	}
	if got.LastSoldByGrade.PSA10.SaleCount != 5 {
		t.Errorf("SaleCount = %d, want 5", got.LastSoldByGrade.PSA10.SaleCount)
	}
}

// Helper to create a float64 pointer.
func ptrF64(v float64) *float64 {
	return &v
}

func TestApplyMarketData(t *testing.T) {
	tests := []struct {
		name             string
		md               *dh.CardLookupMarketData
		verifyMarketData func(*testing.T, *pricing.MarketData)
	}{
		{
			name: "nil market data",
			md:   nil,
			verifyMarketData: func(t *testing.T, market *pricing.MarketData) {
				if market != nil {
					t.Errorf("expected nil market data, got %+v", market)
				}
			},
		},
		{
			name: "with BestAsk and ActiveAsks",
			md: &dh.CardLookupMarketData{
				BestAsk:    ptrF64(25.50),
				ActiveAsks: 12,
				Volume24h:  0,
			},
			verifyMarketData: func(t *testing.T, market *pricing.MarketData) {
				if market == nil {
					t.Fatal("expected non-nil market data")
				}
				if market.LowestListing != 2550 {
					t.Errorf("LowestListing = %d, want 2550", market.LowestListing)
				}
				if market.ActiveListings != 12 {
					t.Errorf("ActiveListings = %d, want 12", market.ActiveListings)
				}
				if market.SalesLast30d != 0 {
					t.Errorf("SalesLast30d = %d, want 0", market.SalesLast30d)
				}
				if market.SalesLast90d != 0 {
					t.Errorf("SalesLast90d = %d, want 0", market.SalesLast90d)
				}
			},
		},
		{
			name: "with volume extrapolation",
			md: &dh.CardLookupMarketData{
				BestAsk:    nil,
				ActiveAsks: 0,
				Volume24h:  3,
			},
			verifyMarketData: func(t *testing.T, market *pricing.MarketData) {
				if market == nil {
					t.Fatal("expected non-nil market data")
				}
				if market.LowestListing != 0 {
					t.Errorf("LowestListing = %d, want 0", market.LowestListing)
				}
				if market.SalesLast30d != 90 {
					t.Errorf("SalesLast30d = %d, want 90", market.SalesLast30d)
				}
				if market.SalesLast90d != 270 {
					t.Errorf("SalesLast90d = %d, want 270", market.SalesLast90d)
				}
			},
		},
		{
			name: "zero BestAsk ignored",
			md: &dh.CardLookupMarketData{
				BestAsk:    ptrF64(0.0),
				ActiveAsks: 5,
				Volume24h:  1,
			},
			verifyMarketData: func(t *testing.T, market *pricing.MarketData) {
				if market == nil {
					t.Fatal("expected non-nil market data")
				}
				if market.LowestListing != 0 {
					t.Errorf("LowestListing = %d, want 0", market.LowestListing)
				}
				if market.ActiveListings != 5 {
					t.Errorf("ActiveListings = %d, want 5", market.ActiveListings)
				}
				if market.SalesLast30d != 30 {
					t.Errorf("SalesLast30d = %d, want 30", market.SalesLast30d)
				}
			},
		},
		{
			name: "full market data",
			md: &dh.CardLookupMarketData{
				BestAsk:    ptrF64(100.0),
				ActiveAsks: 5,
				Volume24h:  2,
			},
			verifyMarketData: func(t *testing.T, market *pricing.MarketData) {
				if market == nil {
					t.Fatal("expected non-nil market data")
				}
				if market.LowestListing != 10000 {
					t.Errorf("LowestListing = %d, want 10000", market.LowestListing)
				}
				if market.ActiveListings != 5 {
					t.Errorf("ActiveListings = %d, want 5", market.ActiveListings)
				}
				if market.SalesLast30d != 60 {
					t.Errorf("SalesLast30d = %d, want 60", market.SalesLast30d)
				}
				if market.SalesLast90d != 180 {
					t.Errorf("SalesLast90d = %d, want 180", market.SalesLast90d)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			price := &pricing.Price{}
			applyMarketData(price, tc.md)
			tc.verifyMarketData(t, price.Market)
		})
	}
}

func TestGetPrice_CardLookupEnrichment(t *testing.T) {
	p := New(
		&mockMarketData{
			RecentSalesFn: func(ctx context.Context, cardID int) ([]dh.RecentSale, error) {
				return []dh.RecentSale{
					{GradingCompany: "PSA", Grade: "10", Price: 50.00, Platform: "ebay"},
				}, nil
			},
			CardLookupFn: func(ctx context.Context, cardID int) (*dh.CardLookupResponse, error) {
				return &dh.CardLookupResponse{
					MarketData: dh.CardLookupMarketData{
						BestAsk:    ptrF64(30.0),
						ActiveAsks: 8,
						Volume24h:  1,
					},
				}, nil
			},
		},
		&mockIDLookup{id: "42"},
	)

	got, err := p.GetPrice(context.Background(), pricing.Card{
		Name: "Charizard", Set: "Base Set", Number: "4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil price")
	}

	// Verify sales data still works.
	if got.Grades.PSA10Cents != 5000 {
		t.Errorf("PSA10Cents = %d, want 5000", got.Grades.PSA10Cents)
	}

	// Verify market data was enriched.
	if got.Market == nil {
		t.Fatal("expected non-nil market data")
	}
	if got.Market.LowestListing != 3000 {
		t.Errorf("LowestListing = %d, want 3000", got.Market.LowestListing)
	}
	if got.Market.ActiveListings != 8 {
		t.Errorf("ActiveListings = %d, want 8", got.Market.ActiveListings)
	}
}

func TestGetPrice_CardLookupError_NonFatal(t *testing.T) {
	p := New(
		&mockMarketData{
			RecentSalesFn: func(ctx context.Context, cardID int) ([]dh.RecentSale, error) {
				return []dh.RecentSale{
					{GradingCompany: "PSA", Grade: "10", Price: 50.00, Platform: "ebay"},
				}, nil
			},
			CardLookupFn: func(ctx context.Context, cardID int) (*dh.CardLookupResponse, error) {
				return nil, errors.New("lookup failed")
			},
		},
		&mockIDLookup{id: "42"},
	)

	got, err := p.GetPrice(context.Background(), pricing.Card{
		Name: "Charizard", Set: "Base Set", Number: "4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil price")
	}

	// Verify sales data still works.
	if got.Grades.PSA10Cents != 5000 {
		t.Errorf("PSA10Cents = %d, want 5000", got.Grades.PSA10Cents)
	}

	// Verify market data was NOT enriched (error was non-fatal).
	if got.Market != nil {
		t.Errorf("expected nil market data (CardLookup failed), got %+v", got.Market)
	}
}

// Verify Provider satisfies PriceProvider at compile time.
var _ pricing.PriceProvider = (*Provider)(nil)
