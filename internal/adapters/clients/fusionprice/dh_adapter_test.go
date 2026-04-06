package fusionprice

import (
	"context"
	"fmt"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// --- mock types ---

type mockDHMarketDataClient struct {
	RecentSalesFn func(ctx context.Context, cardID int) ([]dh.RecentSale, error)
}

func (m *mockDHMarketDataClient) RecentSales(ctx context.Context, cardID int) ([]dh.RecentSale, error) {
	return m.RecentSalesFn(ctx, cardID)
}

type mockDHCardIDLookup struct {
	GetExternalIDFn func(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
}

func (m *mockDHCardIDLookup) GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error) {
	return m.GetExternalIDFn(ctx, cardName, setName, collectorNumber, provider)
}

// --- tests ---

func TestDHAdapter_FetchFusionData_WithSales(t *testing.T) {
	client := &mockDHMarketDataClient{
		RecentSalesFn: func(_ context.Context, cardID int) ([]dh.RecentSale, error) {
			if cardID != 12345 {
				t.Fatalf("unexpected cardID: %d", cardID)
			}
			return []dh.RecentSale{
				{SoldAt: "2026-03-15T10:00:00Z", GradingCompany: "PSA", Grade: "10", Price: 500.00, Platform: "eBay"},
				{SoldAt: "2026-03-14T10:00:00Z", GradingCompany: "PSA", Grade: "10", Price: 480.00, Platform: "eBay"},
				{SoldAt: "2026-03-13T10:00:00Z", GradingCompany: "PSA", Grade: "9", Price: 250.00, Platform: "TCGPlayer"},
				{SoldAt: "2026-03-12T10:00:00Z", GradingCompany: "BGS", Grade: "10", Price: 700.00, Platform: "eBay"},
				{SoldAt: "2026-03-11T10:00:00Z", GradingCompany: "PSA", Grade: "8", Price: 120.00, Platform: "eBay"},
				{SoldAt: "2026-03-10T10:00:00Z", GradingCompany: "PSA", Grade: "7", Price: 80.00, Platform: "eBay"},
				{SoldAt: "2026-03-09T10:00:00Z", GradingCompany: "PSA", Grade: "6", Price: 50.00, Platform: "eBay"},
				{SoldAt: "2026-03-08T10:00:00Z", GradingCompany: "BGS", Grade: "9.5", Price: 300.00, Platform: "eBay"},
			}, nil
		},
	}

	idLookup := &mockDHCardIDLookup{
		GetExternalIDFn: func(_ context.Context, _, _, _, provider string) (string, error) {
			if provider != pricing.SourceDH {
				t.Fatalf("unexpected provider: %s", provider)
			}
			return "12345", nil
		},
	}

	adapter := NewDHAdapter(client, idLookup, nil)

	card := pricing.Card{Name: "Charizard", Set: "Base Set", Number: "4"}
	result, meta, err := adapter.FetchFusionData(context.Background(), card)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", meta.StatusCode)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// PSA 10: two sales at 500 and 480
	psa10 := result.GradeData["psa10"]
	if len(psa10) != 2 {
		t.Fatalf("expected 2 psa10 entries, got %d", len(psa10))
	}
	if psa10[0].Value != 500.00 {
		t.Errorf("psa10[0].Value = %v, want 500.00", psa10[0].Value)
	}
	if psa10[1].Value != 480.00 {
		t.Errorf("psa10[1].Value = %v, want 480.00", psa10[1].Value)
	}
	if psa10[0].Source.Name != pricing.SourceDH {
		t.Errorf("psa10[0].Source.Name = %q, want %q", psa10[0].Source.Name, pricing.SourceDH)
	}
	if psa10[0].Source.Confidence != 0.90 {
		t.Errorf("psa10[0].Source.Confidence = %v, want 0.90", psa10[0].Source.Confidence)
	}

	// PSA 9: one sale at 250
	psa9 := result.GradeData["psa9"]
	if len(psa9) != 1 {
		t.Fatalf("expected 1 psa9 entry, got %d", len(psa9))
	}
	if psa9[0].Value != 250.00 {
		t.Errorf("psa9[0].Value = %v, want 250.00", psa9[0].Value)
	}

	// BGS 10: one sale at 700
	bgs10 := result.GradeData["bgs10"]
	if len(bgs10) != 1 {
		t.Fatalf("expected 1 bgs10 entry, got %d", len(bgs10))
	}
	if bgs10[0].Value != 700.00 {
		t.Errorf("bgs10[0].Value = %v, want 700.00", bgs10[0].Value)
	}

	// PSA 8: one sale at 120
	psa8 := result.GradeData["psa8"]
	if len(psa8) != 1 {
		t.Fatalf("expected 1 psa8 entry, got %d", len(psa8))
	}
	if psa8[0].Value != 120.00 {
		t.Errorf("psa8[0].Value = %v, want 120.00", psa8[0].Value)
	}

	// PSA 7: one sale at 80
	psa7 := result.GradeData["psa7"]
	if len(psa7) != 1 {
		t.Fatalf("expected 1 psa7 entry, got %d", len(psa7))
	}
	if psa7[0].Value != 80.00 {
		t.Errorf("psa7[0].Value = %v, want 80.00", psa7[0].Value)
	}

	// PSA 6: one sale at 50
	psa6 := result.GradeData["psa6"]
	if len(psa6) != 1 {
		t.Fatalf("expected 1 psa6 entry, got %d", len(psa6))
	}
	if psa6[0].Value != 50.00 {
		t.Errorf("psa6[0].Value = %v, want 50.00", psa6[0].Value)
	}

	// BGS 9.5 -> psa95: one sale at 300
	psa95 := result.GradeData["psa95"]
	if len(psa95) != 1 {
		t.Fatalf("expected 1 psa95 entry, got %d", len(psa95))
	}
	if psa95[0].Value != 300.00 {
		t.Errorf("psa95[0].Value = %v, want 300.00", psa95[0].Value)
	}
}

func TestDHAdapter_FetchFusionData_NoMapping(t *testing.T) {
	client := &mockDHMarketDataClient{
		RecentSalesFn: func(_ context.Context, _ int) ([]dh.RecentSale, error) {
			t.Fatal("RecentSales should not be called when there is no mapping")
			return nil, nil
		},
	}

	idLookup := &mockDHCardIDLookup{
		GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) {
			return "", nil // no mapping
		},
	}

	adapter := NewDHAdapter(client, idLookup, nil)

	card := pricing.Card{Name: "Unknown Card", Set: "Unknown Set", Number: "999"}
	result, meta, err := adapter.FetchFusionData(context.Background(), card)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.StatusCode != 0 {
		t.Errorf("expected status 0, got %d", meta.StatusCode)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestDHAdapter_FetchFusionData_NoData(t *testing.T) {
	client := &mockDHMarketDataClient{
		RecentSalesFn: func(_ context.Context, _ int) ([]dh.RecentSale, error) {
			return []dh.RecentSale{}, nil
		},
	}

	idLookup := &mockDHCardIDLookup{
		GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) {
			return "999", nil
		},
	}

	adapter := NewDHAdapter(client, idLookup, nil)

	card := pricing.Card{Name: "Some Card", Set: "Some Set", Number: "1"}
	result, meta, err := adapter.FetchFusionData(context.Background(), card)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", meta.StatusCode)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestDHAdapter_FetchFusionData_LookupError(t *testing.T) {
	client := &mockDHMarketDataClient{
		RecentSalesFn: func(_ context.Context, _ int) ([]dh.RecentSale, error) {
			t.Fatal("RecentSales should not be called when lookup errors")
			return nil, nil
		},
	}

	idLookup := &mockDHCardIDLookup{
		GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) {
			return "", fmt.Errorf("db connection failed")
		},
	}

	adapter := NewDHAdapter(client, idLookup, nil)

	card := pricing.Card{Name: "Card", Set: "Set", Number: "1"}
	result, meta, err := adapter.FetchFusionData(context.Background(), card)

	// Lookup errors are treated as skip (no mapping), not hard errors.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.StatusCode != 0 {
		t.Errorf("expected status 0, got %d", meta.StatusCode)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestDHAdapter_Available(t *testing.T) {
	tests := []struct {
		name       string
		client     DHMarketDataClient
		idResolver DHCardIDLookup
		want       bool
	}{
		{
			name:       "both set",
			client:     &mockDHMarketDataClient{},
			idResolver: &mockDHCardIDLookup{},
			want:       true,
		},
		{
			name:       "nil client",
			client:     nil,
			idResolver: &mockDHCardIDLookup{},
			want:       false,
		},
		{
			name:       "nil resolver",
			client:     &mockDHMarketDataClient{},
			idResolver: nil,
			want:       false,
		},
		{
			name:       "both nil",
			client:     nil,
			idResolver: nil,
			want:       false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewDHAdapter(tc.client, tc.idResolver, nil)
			if got := a.Available(); got != tc.want {
				t.Errorf("Available() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDHAdapter_Name(t *testing.T) {
	a := NewDHAdapter(nil, nil, nil)
	if got := a.Name(); got != pricing.SourceDH {
		t.Errorf("Name() = %q, want %q", got, pricing.SourceDH)
	}
}

func TestDHGradeToFusionKey(t *testing.T) {
	tests := []struct {
		company string
		grade   string
		want    string
	}{
		{"PSA", "10", "psa10"},
		{"PSA", "9", "psa9"},
		{"PSA", "9.5", "psa95"},
		{"PSA", "8", "psa8"},
		{"PSA", "7", "psa7"},
		{"PSA", "6", "psa6"},
		{"BGS", "10", "bgs10"},
		{"BGS", "9.5", "psa95"},
		{"CGC", "9.5", "psa95"},
		{"psa", "10", "psa10"}, // lowercase company
		{"AGS", "10", ""},      // unknown company
		{"PSA", "5", ""},       // PSA 5 not in our mapping
		{"", "10", ""},         // empty company
		{"PSA", "", ""},        // empty grade
	}
	for _, tc := range tests {
		name := fmt.Sprintf("%s_%s", tc.company, tc.grade)
		t.Run(name, func(t *testing.T) {
			got := dhGradeToFusionKey(tc.company, tc.grade)
			if got != tc.want {
				t.Errorf("dhGradeToFusionKey(%q, %q) = %q, want %q", tc.company, tc.grade, got, tc.want)
			}
		})
	}
}

func TestDHAdapter_FetchFusionData_SkipsZeroPriceSales(t *testing.T) {
	client := &mockDHMarketDataClient{
		RecentSalesFn: func(_ context.Context, _ int) ([]dh.RecentSale, error) {
			return []dh.RecentSale{
				{GradingCompany: "PSA", Grade: "10", Price: 0, Platform: "eBay"},
				{GradingCompany: "PSA", Grade: "9", Price: -10, Platform: "eBay"},
				{GradingCompany: "PSA", Grade: "8", Price: 100.00, Platform: "eBay"},
			}, nil
		},
	}

	idLookup := &mockDHCardIDLookup{
		GetExternalIDFn: func(_ context.Context, _, _, _, _ string) (string, error) {
			return "1", nil
		},
	}

	adapter := NewDHAdapter(client, idLookup, nil)
	result, _, err := adapter.FetchFusionData(context.Background(), pricing.Card{Name: "X", Set: "Y", Number: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only PSA 8 should be present (zero and negative prices skipped).
	if len(result.GradeData) != 1 {
		t.Errorf("expected 1 grade in result, got %d", len(result.GradeData))
	}
	if _, ok := result.GradeData["psa8"]; !ok {
		t.Error("expected psa8 in result")
	}
}
