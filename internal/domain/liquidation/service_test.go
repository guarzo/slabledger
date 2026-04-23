package liquidation

import (
	"context"
	"errors"
	"testing"
)

type mockPurchaseLister struct {
	purchases []UnsoldPurchase
}

func (m *mockPurchaseLister) ListUnsoldForLiquidation(_ context.Context) ([]UnsoldPurchase, error) {
	return m.purchases, nil
}

type mockCompReader struct {
	comps map[string][]SaleComp // key: "gemRateID:condition"
}

func (m *mockCompReader) GetSaleCompsForCard(_ context.Context, gemRateID, condition string) ([]SaleComp, error) {
	key := gemRateID + ":" + condition
	return m.comps[key], nil
}

type mockPriceWriter struct {
	applied map[string]int
}

func (m *mockPriceWriter) SetReviewedPrice(_ context.Context, purchaseID string, priceCents int, _ string) error {
	if m.applied == nil {
		m.applied = make(map[string]int)
	}
	m.applied[purchaseID] = priceCents
	return nil
}

type failingPriceWriter struct {
	failID string
	applied map[string]int
}

func (f *failingPriceWriter) SetReviewedPrice(_ context.Context, purchaseID string, priceCents int, _ string) error {
	if purchaseID == f.failID {
		return errors.New("db error")
	}
	if f.applied == nil {
		f.applied = make(map[string]int)
	}
	f.applied[purchaseID] = priceCents
	return nil
}

func TestServicePreview(t *testing.T) {
	// 3 purchases:
	// 1. has gemRateID "gem1" with 3 comps → comp-based suggestion
	// 2. no gemRateID, but has CL value → noCompDiscount suggestion
	// 3. neither gemRateID nor CL value → no suggestion
	purchases := []UnsoldPurchase{
		{ID: "p1", CertNumber: "111", CardName: "Card A", GradeValue: 10, CampaignName: "C1", BuyCostCents: 5000, CLValueCents: 10000, GemRateID: "gem1", ReviewedPriceCents: 9000},
		{ID: "p2", CertNumber: "222", CardName: "Card B", GradeValue: 9, CampaignName: "C1", BuyCostCents: 3000, CLValueCents: 8000, GemRateID: "", ReviewedPriceCents: 7500},
		{ID: "p3", CertNumber: "333", CardName: "Card C", GradeValue: 8, CampaignName: "C1", BuyCostCents: 1000, CLValueCents: 0, GemRateID: "", ReviewedPriceCents: 0},
	}

	comps := map[string][]SaleComp{
		"gem1:g10": {
			{SaleDate: dateStr(5), PriceCents: 9000},
			{SaleDate: dateStr(6), PriceCents: 9200},
			{SaleDate: dateStr(7), PriceCents: 9100},
		},
	}

	svc := NewService(&mockPurchaseLister{purchases: purchases}, &mockCompReader{comps: comps}, &mockPriceWriter{})

	resp, err := svc.Preview(context.Background(), PreviewRequest{BaseDiscountPct: 10, NoCompDiscountPct: 20})
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}

	if resp.Summary.TotalCards != 3 {
		t.Errorf("TotalCards = %d, want 3", resp.Summary.TotalCards)
	}
	if resp.Summary.WithComps != 1 {
		t.Errorf("WithComps = %d, want 1", resp.Summary.WithComps)
	}
	if resp.Summary.WithoutComps != 1 {
		t.Errorf("WithoutComps = %d, want 1", resp.Summary.WithoutComps)
	}
	if resp.Summary.NoData != 1 {
		t.Errorf("NoData = %d, want 1", resp.Summary.NoData)
	}
}

func TestServiceApply(t *testing.T) {
	writer := &mockPriceWriter{}
	svc := NewService(&mockPurchaseLister{}, &mockCompReader{}, writer)

	req := ApplyRequest{Items: []ApplyItem{
		{PurchaseID: "p1", NewPriceCents: 9000},
		{PurchaseID: "p2", NewPriceCents: 7000},
	}}

	result, err := svc.Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if result.Applied != 2 {
		t.Errorf("Applied = %d, want 2", result.Applied)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
	if writer.applied["p1"] != 9000 {
		t.Errorf("p1 price = %d, want 9000", writer.applied["p1"])
	}
	if writer.applied["p2"] != 7000 {
		t.Errorf("p2 price = %d, want 7000", writer.applied["p2"])
	}
}

func TestServiceApplyPartialFailure(t *testing.T) {
	writer := &failingPriceWriter{failID: "p2"}
	svc := NewService(&mockPurchaseLister{}, &mockCompReader{}, writer)

	req := ApplyRequest{Items: []ApplyItem{
		{PurchaseID: "p1", NewPriceCents: 9000},
		{PurchaseID: "p2", NewPriceCents: 7000},
	}}

	result, err := svc.Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if result.Applied != 1 {
		t.Errorf("Applied = %d, want 1", result.Applied)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if len(result.Errors) != 1 {
		t.Errorf("len(Errors) = %d, want 1", len(result.Errors))
	}
}
