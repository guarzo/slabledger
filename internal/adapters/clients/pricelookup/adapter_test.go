package pricelookup

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// mockPriceProvider implements pricing.PriceProvider for testing.
type mockPriceProvider struct {
	lookupFn func(ctx context.Context, setName string, card domainCards.Card) (*pricing.Price, error)
}

func (m *mockPriceProvider) GetPrice(context.Context, pricing.Card) (*pricing.Price, error) {
	return nil, nil
}
func (m *mockPriceProvider) Available() bool { return true }
func (m *mockPriceProvider) Name() string    { return "mock" }
func (m *mockPriceProvider) LookupCard(ctx context.Context, setName string, card domainCards.Card) (*pricing.Price, error) {
	return m.lookupFn(ctx, setName, card)
}

func TestValidGrade(t *testing.T) {
	tests := []struct {
		grade    float64
		expected bool
	}{
		{0, true},
		{6, true},
		{6.5, true},
		{7, true},
		{7.5, true},
		{8, true},
		{8.5, true},
		{9, true},
		{9.5, true},
		{10, true},
		{1, false},
		{2, false},
		{3, false},
		{4, false},
		{5, false},
		{5.5, false},
		{11, false},
		{8.3, false},
	}

	for _, tt := range tests {
		got := validGrade(tt.grade)
		if got != tt.expected {
			t.Errorf("validGrade(%g) = %v, want %v", tt.grade, got, tt.expected)
		}
	}
}

func TestToCents(t *testing.T) {
	tests := []struct {
		name     string
		dollars  float64
		expected int
	}{
		{"normal", 10.50, 1050},
		{"fractional rounding", 9.999, 1000},
		{"zero", 0.0, 0},
		{"large value", 1234.56, 123456},
		{"small fractional", 0.01, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := int(mathutil.ToCents(tt.dollars))
			if got != tt.expected {
				t.Errorf("ToCents(%v) = %d, want %d", tt.dollars, got, tt.expected)
			}
		})
	}
}

func TestGradePrice(t *testing.T) {
	grades := pricing.GradedPrices{
		RawCents:   500,
		PSA6Cents:  700,
		PSA7Cents:  850,
		PSA8Cents:  1000,
		PSA9Cents:  2000,
		PSA10Cents: 5000,
	}

	tests := []struct {
		name     string
		grade    float64
		expected int
	}{
		{"raw", 0, 500},
		{"psa6", 6, 700},
		{"psa6.5 interpolated", 6.5, 775},
		{"psa7", 7, 850},
		{"psa7.5 interpolated", 7.5, 925},
		{"psa8", 8, 1000},
		{"psa8.5 interpolated", 8.5, 1500},
		{"psa9", 9, 2000},
		{"psa9.5 interpolated", 9.5, 3500},
		{"psa10", 10, 5000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gradePrice(grades, tt.grade)
			if got != tt.expected {
				t.Errorf("gradePrice(grades, %g) = %d, want %d", tt.grade, got, tt.expected)
			}
		})
	}
}

func TestGradeInfo(t *testing.T) {
	lsbg := &pricing.LastSoldByGrade{
		PSA10: &pricing.GradeSaleInfo{LastSoldPrice: 50.0, SaleCount: 5},
		PSA9:  &pricing.GradeSaleInfo{LastSoldPrice: 30.0, SaleCount: 3},
		PSA8:  &pricing.GradeSaleInfo{LastSoldPrice: 15.0, SaleCount: 2},
		PSA7:  &pricing.GradeSaleInfo{LastSoldPrice: 10.0, SaleCount: 4},
		PSA6:  &pricing.GradeSaleInfo{LastSoldPrice: 7.0, SaleCount: 6},
		Raw:   &pricing.GradeSaleInfo{LastSoldPrice: 5.0, SaleCount: 10},
	}

	tests := []struct {
		name          string
		grade         float64
		expectedPrice float64
		expectedCount int
	}{
		{"psa10", 10, 50.0, 5},
		{"psa9", 9, 30.0, 3},
		{"psa8", 8, 15.0, 2},
		{"psa8.5 uses floor", 8.5, 15.0, 2},
		{"psa7", 7, 10.0, 4},
		{"psa7.5 uses floor", 7.5, 10.0, 4},
		{"psa6", 6, 7.0, 6},
		{"psa6.5 uses floor", 6.5, 7.0, 6},
		{"raw", 0, 5.0, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gradeInfo(lsbg, tt.grade)
			if got == nil {
				t.Fatalf("gradeInfo(lsbg, %g) returned nil", tt.grade)
				return
			}
			if got.LastSoldPrice != tt.expectedPrice {
				t.Errorf("gradeInfo(lsbg, %g).LastSoldPrice = %v, want %v", tt.grade, got.LastSoldPrice, tt.expectedPrice)
			}
			if got.SaleCount != tt.expectedCount {
				t.Errorf("gradeInfo(lsbg, %g).SaleCount = %d, want %d", tt.grade, got.SaleCount, tt.expectedCount)
			}
		})
	}

	// Test nil sub-fields
	t.Run("nil psa10", func(t *testing.T) {
		partial := &pricing.LastSoldByGrade{Raw: &pricing.GradeSaleInfo{LastSoldPrice: 1.0}}
		got := gradeInfo(partial, 10.0)
		if got != nil {
			t.Errorf("expected nil for missing PSA10, got %+v", got)
		}
	})
}

func TestBuildSourcePrices(t *testing.T) {
	price := &pricing.Price{
		GradeDetails: map[string]*pricing.GradeDetail{
			"psa10": {
				Ebay: &pricing.EbayGradeDetail{
					PriceCents:   4800,
					Confidence:   "high",
					SalesCount:   12,
					Trend:        "up",
					MinCents:     4000,
					MaxCents:     5500,
					Avg7DayCents: 4900,
					Volume7Day:   2.5,
				},
				Estimate: &pricing.EstimateGradeDetail{
					PriceCents: 4700,
					LowCents:   4200,
					HighCents:  5200,
					Confidence: 0.85,
				},
			},
		},
	}

	sources := buildSourcePrices(price, 10)

	// Expect 2 sources: eBay, Estimate
	if len(sources) != 2 {
		t.Fatalf("buildSourcePrices returned %d sources, want 2", len(sources))
	}

	// Verify eBay source
	pp := sources[0]
	if pp.Source != "eBay" {
		t.Errorf("sources[0].Source = %q, want %q", pp.Source, "eBay")
	}
	if pp.PriceCents != 4800 {
		t.Errorf("eBay PriceCents = %d, want 4800", pp.PriceCents)
	}
	if pp.SaleCount != 12 {
		t.Errorf("eBay SaleCount = %d, want 12", pp.SaleCount)
	}
	if pp.Trend != "up" {
		t.Errorf("eBay Trend = %q, want %q", pp.Trend, "up")
	}
	if pp.Confidence != "high" {
		t.Errorf("eBay Confidence = %q, want %q", pp.Confidence, "high")
	}
	if pp.MinCents != 4000 {
		t.Errorf("eBay MinCents = %d, want 4000", pp.MinCents)
	}
	if pp.MaxCents != 5500 {
		t.Errorf("eBay MaxCents = %d, want 5500", pp.MaxCents)
	}
	if pp.Avg7DayCents != 4900 {
		t.Errorf("eBay Avg7DayCents = %d, want 4900", pp.Avg7DayCents)
	}
	if pp.Volume7Day != 2.5 {
		t.Errorf("eBay Volume7Day = %v, want 2.5", pp.Volume7Day)
	}

	// Verify Estimate source
	ch := sources[1]
	if ch.Source != "Estimate" {
		t.Errorf("sources[1].Source = %q, want %q", ch.Source, "Estimate")
	}
	if ch.PriceCents != 4700 {
		t.Errorf("Estimate PriceCents = %d, want 4700", ch.PriceCents)
	}
	if ch.MinCents != 4200 {
		t.Errorf("Estimate MinCents = %d, want 4200", ch.MinCents)
	}
	if ch.MaxCents != 5200 {
		t.Errorf("Estimate MaxCents = %d, want 5200", ch.MaxCents)
	}
	if ch.Confidence != "high" {
		t.Errorf("Estimate Confidence = %q, want %q", ch.Confidence, "high")
	}
}

func TestBuildSourcePrices_NoGradeDetails(t *testing.T) {
	price := &pricing.Price{}
	sources := buildSourcePrices(price, 10)
	if len(sources) != 0 {
		t.Errorf("buildSourcePrices with no GradeDetails returned %d sources, want 0", len(sources))
	}
}

func TestBuildSourcePrices_EstimateConfidenceLevels(t *testing.T) {
	tests := []struct {
		name       string
		confidence float64
		expected   string
	}{
		{"high confidence", 0.85, "high"},
		{"medium confidence", 0.6, "medium"},
		{"low confidence", 0.3, "low"},
		{"threshold high", 0.8, "high"},
		{"threshold medium", 0.5, "medium"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price := &pricing.Price{
				GradeDetails: map[string]*pricing.GradeDetail{
					"psa10": {
						Estimate: &pricing.EstimateGradeDetail{
							PriceCents: 1000,
							Confidence: tt.confidence,
						},
					},
				},
			}
			sources := buildSourcePrices(price, 10)
			// Find Estimate source
			var found *campaigns.SourcePrice
			for i := range sources {
				if sources[i].Source == "Estimate" {
					found = &sources[i]
					break
				}
			}
			if found == nil {
				t.Fatal("Estimate source not found")
				return
			}
			if found.Confidence != tt.expected {
				t.Errorf("Estimate Confidence = %q, want %q", found.Confidence, tt.expected)
			}
		})
	}
}

// --- Adapter integration tests for GetLastSoldCents and GetMarketSnapshot ---

var testCard = campaigns.CardIdentity{
	CardName:   "Charizard",
	CardNumber: "4",
	SetName:    "Base Set",
}

func TestGetLastSoldCents_ValidGrades(t *testing.T) {
	mock := &mockPriceProvider{
		lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
			return &pricing.Price{
				LastSoldByGrade: &pricing.LastSoldByGrade{
					PSA10: &pricing.GradeSaleInfo{LastSoldPrice: 100.50, SaleCount: 5},
					PSA9:  &pricing.GradeSaleInfo{LastSoldPrice: 50.25, SaleCount: 3},
					PSA8:  &pricing.GradeSaleInfo{LastSoldPrice: 25.00, SaleCount: 2},
					Raw:   &pricing.GradeSaleInfo{LastSoldPrice: 10.00, SaleCount: 10},
				},
			}, nil
		},
	}
	adapter := NewAdapter(mock)
	ctx := context.Background()

	tests := []struct {
		grade    float64
		expected int
	}{
		{10, 10050},
		{9, 5025},
		{8.5, 2500}, // half-grade uses floor (PSA8)
		{8, 2500},
		{0, 1000},
	}
	for _, tt := range tests {
		cents, err := adapter.GetLastSoldCents(ctx, testCard, tt.grade)
		if err != nil {
			t.Fatalf("GetLastSoldCents(grade=%g) error: %v", tt.grade, err)
		}
		if cents != tt.expected {
			t.Errorf("GetLastSoldCents(grade=%g) = %d, want %d", tt.grade, cents, tt.expected)
		}
	}
}

func TestGetLastSoldCents_InvalidGrade(t *testing.T) {
	mock := &mockPriceProvider{
		lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
			return &pricing.Price{}, nil
		},
	}
	adapter := NewAdapter(mock)
	_, err := adapter.GetLastSoldCents(context.Background(), testCard, 5)
	if err == nil {
		t.Fatal("expected error for invalid grade 5")
	}
}

func TestGetLastSoldCents_ProviderError(t *testing.T) {
	provErr := errors.New("provider down")
	mock := &mockPriceProvider{
		lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
			return nil, provErr
		},
	}
	adapter := NewAdapter(mock)
	_, err := adapter.GetLastSoldCents(context.Background(), testCard, 10)
	if err == nil {
		t.Fatal("expected error from provider")
	}
}

func TestGetLastSoldCents_NilLastSoldByGrade(t *testing.T) {
	mock := &mockPriceProvider{
		lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
			return &pricing.Price{}, nil
		},
	}
	adapter := NewAdapter(mock)
	cents, err := adapter.GetLastSoldCents(context.Background(), testCard, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cents != 0 {
		t.Errorf("expected 0 cents for nil LastSoldByGrade, got %d", cents)
	}
}

func TestGetLastSoldCents_PSA7(t *testing.T) {
	mock := &mockPriceProvider{
		lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
			return &pricing.Price{
				LastSoldByGrade: &pricing.LastSoldByGrade{
					PSA7: &pricing.GradeSaleInfo{LastSoldPrice: 150.00},
				},
			}, nil
		},
	}
	adapter := NewAdapter(mock)
	cents, err := adapter.GetLastSoldCents(context.Background(), testCard, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cents != 15000 {
		t.Errorf("GetLastSoldCents(grade=7) = %d, want 15000", cents)
	}
}

func TestGetLastSoldCents_RawBehavior(t *testing.T) {
	tests := []struct {
		name     string
		price    *pricing.Price
		expected int
	}{
		{
			name: "uses LastSoldByGrade.Raw when available",
			price: &pricing.Price{
				Grades:          pricing.GradedPrices{RawCents: 500},
				LastSoldByGrade: &pricing.LastSoldByGrade{Raw: &pricing.GradeSaleInfo{LastSoldPrice: 8.00}},
			},
			expected: 800,
		},
		{
			name: "returns 0 when LastSoldByGrade has no Raw entry",
			price: &pricing.Price{
				Grades:          pricing.GradedPrices{RawCents: 500},
				LastSoldByGrade: &pricing.LastSoldByGrade{},
			},
			expected: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPriceProvider{
				lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
					return tt.price, nil
				},
			}
			adapter := NewAdapter(mock)
			cents, err := adapter.GetLastSoldCents(context.Background(), testCard, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cents != tt.expected {
				t.Errorf("GetLastSoldCents(grade=0) = %d, want %d", cents, tt.expected)
			}
		})
	}
}

func TestGetMarketSnapshot_FullSnapshot(t *testing.T) {
	mock := &mockPriceProvider{
		lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
			return &pricing.Price{
				LastSoldByGrade: &pricing.LastSoldByGrade{
					PSA10: &pricing.GradeSaleInfo{LastSoldPrice: 100.0, LastSoldDate: "2025-01-15", SaleCount: 5},
				},
				Grades: pricing.GradedPrices{PSA10Cents: 9500},
				Market: &pricing.MarketData{
					LowestListing:  8000,
					ActiveListings: 12,
					SalesLast30d:   8,
					SalesLast90d:   20,
					Volatility:     0.15,
				},
				Velocity: &pricing.SalesVelocity{
					DailyAverage:  0.3,
					WeeklyAverage: 2.1,
					MonthlyTotal:  8,
				},
				Conservative: &pricing.ConservativePrices{PSA10USD: 85.0},
				Distributions: &pricing.Distributions{
					PSA10: &pricing.SalesDistribution{
						P10: 70.0, P25: 80.0, P50: 95.0, P75: 110.0, P90: 130.0,
						SampleSize: 25, Period: 90,
					},
				},
				Confidence: 0.92,
				Sources:    []string{"ebay"},
			}, nil
		},
	}
	adapter := NewAdapter(mock)
	snap, err := adapter.GetMarketSnapshot(context.Background(), testCard, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
		return
	}
	if snap.LastSoldCents != 10000 {
		t.Errorf("LastSoldCents = %d, want 10000", snap.LastSoldCents)
	}
	if snap.LastSoldDate != "2025-01-15" {
		t.Errorf("LastSoldDate = %q, want %q", snap.LastSoldDate, "2025-01-15")
	}
	if snap.GradePriceCents != 9500 {
		t.Errorf("GradePriceCents = %d, want 9500", snap.GradePriceCents)
	}
	if snap.LowestListCents != 8000 {
		t.Errorf("LowestListCents = %d, want 8000", snap.LowestListCents)
	}
	if snap.ActiveListings != 12 {
		t.Errorf("ActiveListings = %d, want 12", snap.ActiveListings)
	}
	if snap.DailyVelocity != 0.3 {
		t.Errorf("DailyVelocity = %v, want 0.3", snap.DailyVelocity)
	}
	if snap.ConservativeCents != 8500 {
		t.Errorf("ConservativeCents = %d, want 8500", snap.ConservativeCents)
	}
	if snap.MedianCents != 9500 {
		t.Errorf("MedianCents = %d, want 9500", snap.MedianCents)
	}
	if snap.OptimisticCents != 11000 {
		t.Errorf("OptimisticCents = %d, want 11000", snap.OptimisticCents)
	}
	if snap.Confidence != 0.92 {
		t.Errorf("Confidence = %v, want 0.92", snap.Confidence)
	}
	if snap.SourceCount != 1 {
		t.Errorf("SourceCount = %d, want 1", snap.SourceCount)
	}
	if len(snap.Sources) != 1 || snap.Sources[0] != "ebay" {
		t.Errorf("Sources = %v, want [ebay]", snap.Sources)
	}
}

func TestGetMarketSnapshot_InvalidGrade(t *testing.T) {
	mock := &mockPriceProvider{
		lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
			return &pricing.Price{}, nil
		},
	}
	adapter := NewAdapter(mock)
	_, err := adapter.GetMarketSnapshot(context.Background(), testCard, 5.0)
	if err == nil {
		t.Fatal("expected error for invalid grade 5.0")
	}
}

func TestGetMarketSnapshot_NilPrice(t *testing.T) {
	mock := &mockPriceProvider{
		lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
			return nil, nil
		},
	}
	adapter := NewAdapter(mock)
	snap, err := adapter.GetMarketSnapshot(context.Background(), testCard, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap != nil {
		t.Errorf("expected nil snapshot, got %+v", snap)
	}
}

func TestGetMarketSnapshot_FallbackLogic(t *testing.T) {
	mock := &mockPriceProvider{
		lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
			return &pricing.Price{
				Grades: pricing.GradedPrices{PSA10Cents: 5000},
			}, nil
		},
	}
	adapter := NewAdapter(mock)
	snap, err := adapter.GetMarketSnapshot(context.Background(), testCard, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
		return
	}
	// GradePriceCents from Grades
	if snap.GradePriceCents != 5000 {
		t.Errorf("GradePriceCents = %d, want 5000", snap.GradePriceCents)
	}
	// Fallback: MedianCents = GradePriceCents when no distributions
	if snap.MedianCents != 5000 {
		t.Errorf("MedianCents = %d, want 5000 (fallback from GradePriceCents)", snap.MedianCents)
	}
	// Fallback: ConservativeCents = 85% of median
	expectedConservative := 4250 // round(5000 * 0.85)
	if snap.ConservativeCents != expectedConservative {
		t.Errorf("ConservativeCents = %d, want %d", snap.ConservativeCents, expectedConservative)
	}
	// Fallback: OptimisticCents = 115% of median
	expectedOptimistic := 5750 // round(5000 * 1.15)
	if snap.OptimisticCents != expectedOptimistic {
		t.Errorf("OptimisticCents = %d, want %d", snap.OptimisticCents, expectedOptimistic)
	}
	// Fallback: P10 = 70% of median
	expectedP10 := 3500 // round(5000 * 0.70)
	if snap.P10Cents != expectedP10 {
		t.Errorf("P10Cents = %d, want %d", snap.P10Cents, expectedP10)
	}
	// Fallback: P90 = 130% of median
	expectedP90 := 6500 // round(5000 * 1.30)
	if snap.P90Cents != expectedP90 {
		t.Errorf("P90Cents = %d, want %d", snap.P90Cents, expectedP90)
	}
}

// TestGetMarketSnapshot_LastSoldFallbackChain validates that LastSoldCents is populated
// from fallback sources when LastSoldByGrade is nil or missing data.
func TestGetMarketSnapshot_LastSoldFallbackChain(t *testing.T) {
	t.Run("fallback to eBay when LastSoldByGrade is nil", func(t *testing.T) {
		mock := &mockPriceProvider{
			lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
				return &pricing.Price{
					GradeDetails: map[string]*pricing.GradeDetail{
						"psa8": {
							Ebay: &pricing.EbayGradeDetail{PriceCents: 2500, SalesCount: 4},
						},
					},
				}, nil
			},
		}
		adapter := NewAdapter(mock)
		snap, err := adapter.GetMarketSnapshot(context.Background(), testCard, 8)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if snap.LastSoldCents != 2500 {
			t.Errorf("LastSoldCents = %d, want 2500 (from eBay)", snap.LastSoldCents)
		}
		if snap.SaleCount != 4 {
			t.Errorf("SaleCount = %d, want 4 (from eBay)", snap.SaleCount)
		}
	})

	t.Run("fallback to estimate when eBay missing", func(t *testing.T) {
		mock := &mockPriceProvider{
			lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
				return &pricing.Price{
					GradeDetails: map[string]*pricing.GradeDetail{
						"psa9": {
							Estimate: &pricing.EstimateGradeDetail{PriceCents: 7000, Confidence: 0.8},
						},
					},
				}, nil
			},
		}
		adapter := NewAdapter(mock)
		snap, err := adapter.GetMarketSnapshot(context.Background(), testCard, 9)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if snap.LastSoldCents != 0 {
			t.Errorf("LastSoldCents = %d, want 0 (estimate should not set LastSoldCents)", snap.LastSoldCents)
		}
		if snap.EstimatedValueCents != 7000 {
			t.Errorf("EstimatedValueCents = %d, want 7000 (from estimate)", snap.EstimatedValueCents)
		}
		if snap.EstimateSource != pricing.SourceDH {
			t.Errorf("EstimateSource = %q, want %q", snap.EstimateSource, pricing.SourceDH)
		}
	})

	t.Run("nil GradeDetail entry does not panic", func(t *testing.T) {
		mock := &mockPriceProvider{
			lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
				return &pricing.Price{
					// Key exists but value is nil — must not panic
					GradeDetails: map[string]*pricing.GradeDetail{
						"psa9": nil,
					},
				}, nil
			},
		}
		adapter := NewAdapter(mock)
		snap, err := adapter.GetMarketSnapshot(context.Background(), testCard, 9)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if snap.LastSoldCents != 0 {
			t.Errorf("LastSoldCents = %d, want 0 (nil GradeDetail)", snap.LastSoldCents)
		}
	})

	t.Run("actual LastSoldByGrade takes priority over eBay fallback", func(t *testing.T) {
		mock := &mockPriceProvider{
			lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
				return &pricing.Price{
					LastSoldByGrade: &pricing.LastSoldByGrade{
						PSA10: &pricing.GradeSaleInfo{LastSoldPrice: 200.0, SaleCount: 5},
					},
					GradeDetails: map[string]*pricing.GradeDetail{
						"psa10": {
							Ebay: &pricing.EbayGradeDetail{PriceCents: 19000},
						},
					},
				}, nil
			},
		}
		adapter := NewAdapter(mock)
		snap, err := adapter.GetMarketSnapshot(context.Background(), testCard, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if snap.LastSoldCents != 20000 {
			t.Errorf("LastSoldCents = %d, want 20000 (from actual LastSoldByGrade)", snap.LastSoldCents)
		}
	})
}

func TestGetMarketSnapshot_SourcesPassthrough(t *testing.T) {
	mock := &mockPriceProvider{
		lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
			return &pricing.Price{
				Grades:  pricing.GradedPrices{PSA10Cents: 10000},
				Sources: []string{"ebay", "tcgplayer"},
				GradeDetails: map[string]*pricing.GradeDetail{
					"psa10": {
						Ebay: &pricing.EbayGradeDetail{
							PriceCents: 10500,
							SalesCount: 5,
							Confidence: "medium",
						},
						Estimate: &pricing.EstimateGradeDetail{
							PriceCents: 10000,
							Confidence: 0.9,
						},
					},
				},
			}, nil
		},
	}
	adapter := NewAdapter(mock)
	snap, err := adapter.GetMarketSnapshot(context.Background(), testCard, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}

	// Sources should be passed through from Price
	if len(snap.Sources) != 2 {
		t.Fatalf("Sources = %v, want [ebay tcgplayer]", snap.Sources)
	}
	if snap.Sources[0] != "ebay" || snap.Sources[1] != "tcgplayer" {
		t.Errorf("Sources = %v, want [ebay tcgplayer]", snap.Sources)
	}
	if snap.SourceCount != 2 {
		t.Errorf("SourceCount = %d, want 2", snap.SourceCount)
	}

	// Ebay data should flow through to LastSoldCents fallback
	if snap.LastSoldCents != 10500 {
		t.Errorf("LastSoldCents = %d, want 10500 (from Ebay fallback)", snap.LastSoldCents)
	}
}
