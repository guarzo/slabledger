package liquidation

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestServicePreview(t *testing.T) {
	tests := []struct {
		name         string
		purchases    []UnsoldPurchase
		comps        map[string][]SaleComp
		req          PreviewRequest
		wantTotal    int
		wantWithComp int
		wantNoComp   int
		wantNoData   int
	}{
		{
			name: "mixed: comp, no-comp, no-data",
			purchases: []UnsoldPurchase{
				{ID: "p1", CertNumber: "111", CardName: "Card A", GradeValue: 10, CampaignName: "C1", BuyCostCents: 5000, CLValueCents: 10000, GemRateID: "gem1", ReviewedPriceCents: 9000},
				{ID: "p2", CertNumber: "222", CardName: "Card B", GradeValue: 9, CampaignName: "C1", BuyCostCents: 3000, CLValueCents: 8000, GemRateID: "", ReviewedPriceCents: 7500},
				{ID: "p3", CertNumber: "333", CardName: "Card C", GradeValue: 8, CampaignName: "C1", BuyCostCents: 1000, CLValueCents: 0, GemRateID: "", ReviewedPriceCents: 0},
			},
			comps: map[string][]SaleComp{
				"gem1:g10": {
					{SaleDate: dateStr(5), PriceCents: 9000},
					{SaleDate: dateStr(6), PriceCents: 9200},
					{SaleDate: dateStr(7), PriceCents: 9100},
				},
			},
			req:          PreviewRequest{},
			wantTotal:    3,
			wantWithComp: 1,
			wantNoComp:   1,
			wantNoData:   1,
		},
		{
			name:         "empty inventory",
			purchases:    nil,
			comps:        nil,
			req:          PreviewRequest{},
			wantTotal:    0,
			wantWithComp: 0,
			wantNoComp:   0,
			wantNoData:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lister := &stubPurchaseLister{purchases: tc.purchases}
			reader := &stubCompReader{comps: tc.comps}
			writer := &stubPriceWriter{}

			svc := NewService(lister, reader, writer)
			resp, err := svc.Preview(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("Preview error: %v", err)
			}
			if resp.Summary.TotalCards != tc.wantTotal {
				t.Errorf("TotalCards = %d, want %d", resp.Summary.TotalCards, tc.wantTotal)
			}
			if resp.Summary.WithComps != tc.wantWithComp {
				t.Errorf("WithComps = %d, want %d", resp.Summary.WithComps, tc.wantWithComp)
			}
			if resp.Summary.WithoutComps != tc.wantNoComp {
				t.Errorf("WithoutComps = %d, want %d", resp.Summary.WithoutComps, tc.wantNoComp)
			}
			if resp.Summary.NoData != tc.wantNoData {
				t.Errorf("NoData = %d, want %d", resp.Summary.NoData, tc.wantNoData)
			}
		})
	}
}

func TestServiceApply(t *testing.T) {
	tests := []struct {
		name        string
		items       []ApplyItem
		failID      string
		wantApplied int
		wantFailed  int
		wantErrors  int
	}{
		{
			name: "all succeed",
			items: []ApplyItem{
				{PurchaseID: "p1", NewPriceCents: 9000},
				{PurchaseID: "p2", NewPriceCents: 7000},
			},
			wantApplied: 2,
			wantFailed:  0,
			wantErrors:  0,
		},
		{
			name: "partial failure",
			items: []ApplyItem{
				{PurchaseID: "p1", NewPriceCents: 9000},
				{PurchaseID: "p2", NewPriceCents: 7000},
			},
			failID:      "p2",
			wantApplied: 1,
			wantFailed:  1,
			wantErrors:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			writer := &stubPriceWriter{failID: tc.failID}
			svc := NewService(&stubPurchaseLister{}, &stubCompReader{}, writer)

			result, err := svc.Apply(context.Background(), ApplyRequest{Items: tc.items})
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}
			if result.Applied != tc.wantApplied {
				t.Errorf("Applied = %d, want %d", result.Applied, tc.wantApplied)
			}
			if result.Failed != tc.wantFailed {
				t.Errorf("Failed = %d, want %d", result.Failed, tc.wantFailed)
			}
			if len(result.Errors) != tc.wantErrors {
				t.Errorf("len(Errors) = %d, want %d", len(result.Errors), tc.wantErrors)
			}

			// Verify source="liquidation" on successful writes
			for _, item := range tc.items {
				if item.PurchaseID == tc.failID {
					continue
				}
				if got := writer.applied[item.PurchaseID]; got != item.NewPriceCents {
					t.Errorf("price for %s = %d, want %d", item.PurchaseID, got, item.NewPriceCents)
				}
				if src := writer.sources[item.PurchaseID]; src != "liquidation" {
					t.Errorf("source for %s = %q, want %q", item.PurchaseID, src, "liquidation")
				}
			}
		})
	}
}

// --- test stubs (internal to this package, can't use shared mocks) ---

type stubPurchaseLister struct {
	purchases []UnsoldPurchase
}

func (s *stubPurchaseLister) ListUnsoldForLiquidation(_ context.Context) ([]UnsoldPurchase, error) {
	return s.purchases, nil
}

type stubCompReader struct {
	comps map[string][]SaleComp
}

func (s *stubCompReader) GetSaleCompsForCard(_ context.Context, gemRateID, condition string) ([]SaleComp, error) {
	if s.comps == nil {
		return nil, nil
	}
	key := gemRateID + ":" + condition
	return s.comps[key], nil
}

type stubPriceWriter struct {
	failID  string
	applied map[string]int
	sources map[string]string
}

func (s *stubPriceWriter) SetReviewedPrice(_ context.Context, purchaseID string, priceCents int, source string) error {
	if purchaseID == s.failID {
		return errors.New("db error")
	}
	if source != "liquidation" {
		return fmt.Errorf("unexpected source %q, want %q", source, "liquidation")
	}
	if s.applied == nil {
		s.applied = make(map[string]int)
		s.sources = make(map[string]string)
	}
	s.applied[purchaseID] = priceCents
	s.sources[purchaseID] = source
	return nil
}
