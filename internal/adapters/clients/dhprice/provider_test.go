package dhprice

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// --- tests ---

func TestGetPrice_NoCardID(t *testing.T) {
	tests := []struct {
		name    string
		lookup  *mocks.MockDHCardIDLookup
		wantNil bool
		wantErr bool
	}{
		{
			name: "unmapped card returns nil",
			lookup: &mocks.MockDHCardIDLookup{
				GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) { return "", nil },
			},
			wantNil: true,
		},
		{
			name: "lookup error propagates",
			lookup: &mocks.MockDHCardIDLookup{
				GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) {
					return "", errors.New("db down")
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := New(&mocks.MockDHMarketDataClient{}, tc.lookup)
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
		&mocks.MockDHMarketDataClient{
			RecentSalesFn: func(_ context.Context, _ int) ([]dh.RecentSale, error) { return sales, nil },
		},
		&mocks.MockDHCardIDLookup{
			GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) { return "42", nil },
		},
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
		{"9.5 median of 70,80 (BGS+CGC) — dropped, not PSA", got.Grades.Grade95Cents, 0},
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
		&mocks.MockDHMarketDataClient{
			RecentSalesFn: func(_ context.Context, _ int) ([]dh.RecentSale, error) { return sales, nil },
		},
		&mocks.MockDHCardIDLookup{
			GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) { return "42", nil },
		},
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
	idLookup42 := &mocks.MockDHCardIDLookup{
		GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) { return "42", nil },
	}
	tests := []struct {
		name   string
		client MarketDataClient
		lookup CardIDLookup
		card   pricing.Card
	}{
		{
			name:   "no sales returns nil",
			client: &mocks.MockDHMarketDataClient{},
			lookup: idLookup42,
			card:   pricing.Card{Name: "Pikachu", Set: "Jungle", Number: "60"},
		},
		{
			name: "only unknown grades returns nil",
			client: &mocks.MockDHMarketDataClient{
				RecentSalesFn: func(_ context.Context, _ int) ([]dh.RecentSale, error) {
					return []dh.RecentSale{{GradingCompany: "XYZ", Grade: "99", Price: 999.00}}, nil
				},
			},
			lookup: idLookup42,
			card:   pricing.Card{Name: "Oddish", Set: "Jungle", Number: "58"},
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

func TestGetPrice_InvalidCardIDReturnsError(t *testing.T) {
	cases := []struct {
		name        string
		lookupID    string
		lookupErr   error
		card        pricing.Card
		expectedErr error
	}{
		{
			name:        "NonIntegerExternalID",
			lookupID:    "not-a-number",
			card:        pricing.Card{Name: "Test", Set: "Set", Number: "1"},
			expectedErr: strconv.ErrSyntax,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idLookup := &mocks.MockDHCardIDLookup{
				GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) {
					return tc.lookupID, tc.lookupErr
				},
			}
			p := New(&mocks.MockDHMarketDataClient{}, idLookup)
			_, err := p.GetPrice(context.Background(), tc.card)
			if err == nil {
				t.Fatal("expected error for non-integer external ID, got nil")
			}
			if tc.expectedErr != nil && !errors.Is(err, tc.expectedErr) {
				t.Errorf("expected errors.Is(err, %v), got: %v", tc.expectedErr, err)
			}
		})
	}
}

func TestGetPrice_EmptyGradesSalesReturnsNil(t *testing.T) {
	// Sales with empty Grade field — buildPrice should skip them (no mapping in gradeKey)
	// and since byGrade is empty, buildPrice returns nil → GetPrice returns (nil, nil)
	p := New(
		&mocks.MockDHMarketDataClient{
			RecentSalesFn: func(_ context.Context, _ int) ([]dh.RecentSale, error) {
				return []dh.RecentSale{
					{GradingCompany: "PSA", Grade: "", Price: 100.00, SoldAt: "2026-01-01", Platform: "ebay"},
					{GradingCompany: "", Grade: "10", Price: 200.00, SoldAt: "2026-01-02", Platform: "ebay"},
				}, nil
			},
		},
		&mocks.MockDHCardIDLookup{
			GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) { return "42", nil },
		},
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
		{"both present", &mocks.MockDHMarketDataClient{}, &mocks.MockDHCardIDLookup{}, true},
		{"nil client", nil, &mocks.MockDHCardIDLookup{}, false},
		{"nil lookup", &mocks.MockDHMarketDataClient{}, nil, false},
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
		&mocks.MockDHMarketDataClient{
			RecentSalesFn: func(_ context.Context, _ int) ([]dh.RecentSale, error) { return sales, nil },
		},
		&mocks.MockDHCardIDLookup{
			GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) { return "7", nil },
		},
	)

	card := pricing.CardLookup{
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
				if got.LastSoldByGrade.PSA10.LastSoldPrice != 12000 {
					t.Errorf("PSA10.LastSoldPrice = %d, want 12000", got.LastSoldByGrade.PSA10.LastSoldPrice)
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
				if got.LastSoldByGrade.PSA10.LastSoldPrice != 12000 {
					t.Errorf("PSA10.LastSoldPrice = %d, want 12000", got.LastSoldByGrade.PSA10.LastSoldPrice)
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
				if got.LastSoldByGrade.PSA9.LastSoldPrice != 6000 {
					t.Errorf("PSA9.LastSoldPrice = %d, want 6000", got.LastSoldByGrade.PSA9.LastSoldPrice)
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
				if got.LastSoldByGrade.PSA8.LastSoldPrice != 3000 {
					t.Errorf("PSA8.LastSoldPrice = %d, want 3000", got.LastSoldByGrade.PSA8.LastSoldPrice)
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
		{
			name: "most recent sale wins across out-of-order input",
			sales: []dh.RecentSale{
				{GradingCompany: "PSA", Grade: "10", Price: 100.00, SoldAt: "2026-01-01", Platform: "ebay"},
				{GradingCompany: "PSA", Grade: "10", Price: 105.00, SoldAt: "2026-01-05", Platform: "ebay"},
				{GradingCompany: "PSA", Grade: "10", Price: 103.00, SoldAt: "2026-01-03", Platform: "ebay"},
				{GradingCompany: "PSA", Grade: "10", Price: 102.00, SoldAt: "2026-01-02", Platform: "ebay"},
				{GradingCompany: "PSA", Grade: "10", Price: 104.00, SoldAt: "2026-01-04", Platform: "ebay"},
			},
			verify: func(t *testing.T, got *pricing.Price) {
				if got.LastSoldByGrade == nil {
					t.Fatal("LastSoldByGrade is nil")
				}
				if got.LastSoldByGrade.PSA10 == nil {
					t.Fatal("PSA10 is nil")
				}
				if got.LastSoldByGrade.PSA10.LastSoldDate != "2026-01-05" {
					t.Errorf("LastSoldDate = %q, want 2026-01-05", got.LastSoldByGrade.PSA10.LastSoldDate)
				}
				if got.LastSoldByGrade.PSA10.LastSoldPrice != 10500 {
					t.Errorf("LastSoldPrice = %d, want 10500", got.LastSoldByGrade.PSA10.LastSoldPrice)
				}
				if got.LastSoldByGrade.PSA10.SaleCount != 5 {
					t.Errorf("SaleCount = %d, want 5", got.LastSoldByGrade.PSA10.SaleCount)
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

func TestGetPrice_CardLookup(t *testing.T) {
	baseSalesFn := func(_ context.Context, _ int) ([]dh.RecentSale, error) {
		return []dh.RecentSale{
			{GradingCompany: "PSA", Grade: "10", Price: 50.00, Platform: "ebay"},
		}, nil
	}

	tests := []struct {
		name         string
		cardLookupFn func(context.Context, int) (*dh.CardLookupResponse, error)
		verify       func(*testing.T, *pricing.Price)
	}{
		{
			name: "enrichment applied on success",
			cardLookupFn: func(_ context.Context, _ int) (*dh.CardLookupResponse, error) {
				return &dh.CardLookupResponse{
					MarketData: dh.CardLookupMarketData{
						BestAsk:    ptrF64(30.0),
						ActiveAsks: 8,
						Volume24h:  1,
					},
				}, nil
			},
			verify: func(t *testing.T, got *pricing.Price) {
				if got.Grades.PSA10Cents != 5000 {
					t.Errorf("PSA10Cents = %d, want 5000", got.Grades.PSA10Cents)
				}
				if got.Market == nil {
					t.Fatal("expected non-nil market data")
				}
				if got.Market.LowestListing != 3000 {
					t.Errorf("LowestListing = %d, want 3000", got.Market.LowestListing)
				}
				if got.Market.ActiveListings != 8 {
					t.Errorf("ActiveListings = %d, want 8", got.Market.ActiveListings)
				}
			},
		},
		{
			name: "CardLookup error is non-fatal",
			cardLookupFn: func(_ context.Context, _ int) (*dh.CardLookupResponse, error) {
				return nil, errors.New("lookup failed")
			},
			verify: func(t *testing.T, got *pricing.Price) {
				if got.Grades.PSA10Cents != 5000 {
					t.Errorf("PSA10Cents = %d, want 5000", got.Grades.PSA10Cents)
				}
				if got.Market != nil {
					t.Errorf("expected nil market data (CardLookup failed), got %+v", got.Market)
				}
			},
		},
		{
			name: "all-zero market data does not set Market",
			cardLookupFn: func(_ context.Context, _ int) (*dh.CardLookupResponse, error) {
				return &dh.CardLookupResponse{
					MarketData: dh.CardLookupMarketData{
						BestAsk:    nil,
						ActiveAsks: 0,
						Volume24h:  0,
					},
				}, nil
			},
			verify: func(t *testing.T, got *pricing.Price) {
				if got.Market != nil {
					t.Errorf("expected nil market data for all-zero lookup, got %+v", got.Market)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := New(
				&mocks.MockDHMarketDataClient{
					RecentSalesFn: baseSalesFn,
					CardLookupFn:  tc.cardLookupFn,
				},
				&mocks.MockDHCardIDLookup{
					GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) { return "42", nil },
				},
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
			tc.verify(t, got)
		})
	}
}

// TestGetPrice_TransientClientError verifies that errors from the underlying MarketDataClient
// are propagated cleanly by the provider. Retry/circuit-breaker logic lives in the httpx layer;
// this test verifies the provider correctly surfaces the final error after all retries are exhausted.
func TestGetPrice_TransientClientError(t *testing.T) {
	cases := []struct {
		name             string
		recentSalesErr   error
		expectErr        bool
		expectNilPrice   bool
		expectedAttempts int
	}{
		{
			name:             "server error propagates cleanly",
			recentSalesErr:   errors.New("server error: 500"),
			expectErr:        true,
			expectNilPrice:   true,
			expectedAttempts: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			attempts := 0
			client := &mocks.MockDHMarketDataClient{
				RecentSalesFn: func(_ context.Context, _ int) ([]dh.RecentSale, error) {
					attempts++
					return nil, tc.recentSalesErr
				},
			}
			lookup := &mocks.MockDHCardIDLookup{
				GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) {
					return "12345", nil
				},
			}

			p := New(client, lookup)
			got, err := p.GetPrice(context.Background(), pricing.Card{
				Name: "Charizard", Set: "Base Set", Number: "4",
			})

			if tc.expectErr && err == nil {
				t.Fatalf("expected error, got result: %+v", got)
			}
			if !tc.expectErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.expectNilPrice && got != nil {
				t.Errorf("expected nil price, got %+v", got)
			}
			if attempts != tc.expectedAttempts {
				t.Errorf("expected %d attempt(s) at the provider level, got %d", tc.expectedAttempts, attempts)
			}
		})
	}
}

// TestGetPrice_UnavailableProvider verifies that GetPrice returns (nil, nil) safely
// when the provider is not configured (nil client or resolver), rather than panicking.
// This also validates the Available() contract: an unavailable provider returns nil gracefully.
func TestGetPrice_UnavailableProvider(t *testing.T) {
	tests := []struct {
		name   string
		client MarketDataClient
		lookup CardIDLookup
	}{
		{"nil client", nil, &mocks.MockDHCardIDLookup{}},
		{"nil lookup", &mocks.MockDHMarketDataClient{}, nil},
		{"both nil", nil, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := New(tc.client, tc.lookup)
			got, err := p.GetPrice(context.Background(), pricing.Card{Name: "Charizard", Set: "Base Set"})
			if err != nil {
				t.Fatalf("expected no error for unavailable provider, got: %v", err)
			}
			if got != nil {
				t.Errorf("expected nil price for unavailable provider, got: %+v", got)
			}
		})
	}
}

// Verify Provider satisfies PriceProvider at compile time.
var _ pricing.PriceProvider = (*Provider)(nil)
