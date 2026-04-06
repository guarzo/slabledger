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
	sales         []dh.RecentSale
	err           error
}

func (m *mockMarketData) RecentSales(ctx context.Context, cardID int) ([]dh.RecentSale, error) {
	if m.RecentSalesFn != nil {
		return m.RecentSalesFn(ctx, cardID)
	}
	return m.sales, m.err
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
		{GradingCompany: "PSA", Grade: "10", Price: 100.00},
		{GradingCompany: "PSA", Grade: "10", Price: 120.00},
		{GradingCompany: "PSA", Grade: "10", Price: 110.00},
		{GradingCompany: "PSA", Grade: "9", Price: 50.00},
		{GradingCompany: "PSA", Grade: "9", Price: 60.00},
		{GradingCompany: "PSA", Grade: "8", Price: 30.00},
		{GradingCompany: "BGS", Grade: "10", Price: 200.00},
		{GradingCompany: "BGS", Grade: "9.5", Price: 80.00},
		{GradingCompany: "CGC", Grade: "9.5", Price: 70.00},
		{GradingCompany: "XYZ", Grade: "99", Price: 999.00}, // unknown, should be skipped
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
	if len(got.Sources) != 1 || got.Sources[0] != pricing.SourceDH {
		t.Errorf("Sources = %v, want [%q]", got.Sources, pricing.SourceDH)
	}
	if got.Currency != "USD" {
		t.Errorf("Currency = %q, want USD", got.Currency)
	}
}

func TestGetPrice_AmountFallback(t *testing.T) {
	// Only PSA 9 sales — Amount should fall back to PSA9Cents.
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "9", Price: 50.00},
		{GradingCompany: "PSA", Grade: "9", Price: 60.00},
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
	if err := p.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	if got := p.GetStats(context.Background()); got != nil {
		t.Errorf("GetStats() = %v, want nil", got)
	}
}

func TestLookupCard(t *testing.T) {
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "10", Price: 50.00},
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

// Verify Provider satisfies PriceProvider at compile time.
var _ pricing.PriceProvider = (*Provider)(nil)
