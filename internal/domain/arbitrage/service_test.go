package arbitrage

import (
	"context"
	"fmt"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestGetCrackCandidates_NilPriceProvider(t *testing.T) {
	campaignID := "camp-nil"
	campaign := &inventory.Campaign{
		ID:         campaignID,
		Name:       "Test",
		EbayFeePct: 0.1235,
	}
	purchase := inventory.Purchase{
		ID:           "p-nil",
		CampaignID:   campaignID,
		CardName:     "Charizard",
		GradeValue:   8.0,
		BuyCostCents: 10000,
	}

	svc := NewService(
		&stubCampaignRepo{campaign: campaign},
		&stubPurchaseRepo{unsold: []inventory.Purchase{purchase}},
		&stubAnalyticsRepo{},
		&stubFinanceRepo{},
	)

	results, err := svc.GetCrackCandidates(context.Background(), campaignID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = results
}

func TestGetExpectedValues_RepoError(t *testing.T) {
	campaignID := "camp-err"
	campaign := &inventory.Campaign{ID: campaignID, Name: "Test", EbayFeePct: 0.1235}

	errRepo := &stubAnalyticsRepo{}
	analyticsWithError := &errAnalyticsRepo{err: fmt.Errorf("repo failure")}

	svc := NewService(
		&stubCampaignRepo{campaign: campaign},
		&stubPurchaseRepo{},
		analyticsWithError,
		&stubFinanceRepo{},
	)

	_, err := svc.GetExpectedValues(context.Background(), campaignID)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
	_ = errRepo
}

func TestGetAcquisitionTargets_Empty(t *testing.T) {
	campaign := &inventory.Campaign{ID: "camp-acq", Name: "Test", EbayFeePct: 0.1235}

	svc := NewService(
		&stubCampaignRepo{campaign: campaign},
		&stubPurchaseRepo{},
		&stubAnalyticsRepo{},
		&stubFinanceRepo{},
	)

	results, err := svc.GetAcquisitionTargets(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results == nil {
		t.Fatal("expected non-nil slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("expected empty slice, got %d results", len(results))
	}
}

func TestEvaluatePurchase_NoHistory(t *testing.T) {
	campaignID := "camp-eval"
	campaign := &inventory.Campaign{
		ID:         campaignID,
		Name:       "Test",
		EbayFeePct: 0.1235,
	}

	svc := NewService(
		&stubCampaignRepo{campaign: campaign},
		&stubPurchaseRepo{},
		&stubAnalyticsRepo{data: nil},
		&stubFinanceRepo{},
	)

	ev, err := svc.EvaluatePurchase(context.Background(), campaignID, "Charizard", 9.0, 8000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev == nil {
		t.Fatal("expected non-nil ExpectedValue for fallback path")
	}
}

func TestGetCrackCandidates_GradeFilter(t *testing.T) {
	tests := []struct {
		name       string
		gradeValue float64
		wantInList bool
	}{
		{name: "PSA 8 included", gradeValue: 8, wantInList: true},
		{name: "PSA 8.5 included", gradeValue: 8.5, wantInList: true},
		{name: "PSA 9 excluded", gradeValue: 9, wantInList: false},
		{name: "PSA 9.5 excluded", gradeValue: 9.5, wantInList: false},
		{name: "PSA 10 excluded", gradeValue: 10, wantInList: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			campaignID := "camp1"
			campaign := &inventory.Campaign{
				ID:         campaignID,
				Name:       "Test",
				EbayFeePct: 0.1235,
			}
			purchase := inventory.Purchase{
				ID:                  "p1",
				CampaignID:          campaignID,
				CardName:            "Charizard",
				GradeValue:          tc.gradeValue,
				BuyCostCents:        5000,
				PSASourcingFeeCents: 300,
				CLValueCents:        8000,
			}

			svc := NewService(
				&stubCampaignRepo{campaign: campaign},
				&stubPurchaseRepo{unsold: []inventory.Purchase{purchase}},
				&stubAnalyticsRepo{},
				&stubFinanceRepo{},
				WithPriceLookup(&stubPriceProvider{rawCents: 15000, gradedCents: 10000}),
			)

			results, err := svc.GetCrackCandidates(context.Background(), campaignID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			found := false
			for _, r := range results {
				if r.PurchaseID == purchase.ID {
					found = true
					break
				}
			}

			if found != tc.wantInList {
				t.Errorf("grade %g: wantInList=%v, found=%v", tc.gradeValue, tc.wantInList, found)
			}
		})
	}
}

func TestGetExpectedValues_UsesCampaignFee(t *testing.T) {
	campaignID := "camp1"

	makeHistory := func(n int) []inventory.PurchaseWithSale {
		history := make([]inventory.PurchaseWithSale, n)
		for i := range history {
			history[i] = inventory.PurchaseWithSale{
				Purchase: inventory.Purchase{
					ID:                  "sold" + string(rune('a'+i)),
					BuyCostCents:        5000,
					PSASourcingFeeCents: 300,
					GradeValue:          9,
				},
				Sale: &inventory.Sale{NetProfitCents: 1000, SaleFeeCents: 800},
			}
		}
		return history
	}

	unsold := []inventory.Purchase{
		{
			ID:                  "p-unsold",
			CampaignID:          campaignID,
			GradeValue:          9,
			BuyCostCents:        5000,
			PSASourcingFeeCents: 300,
		},
	}

	runEV := func(feePct float64) *EVPortfolio {
		campaign := &inventory.Campaign{
			ID:         campaignID,
			Name:       "Test",
			EbayFeePct: feePct,
		}
		svc := NewService(
			&stubCampaignRepo{campaign: campaign},
			&stubPurchaseRepo{unsold: unsold},
			&stubAnalyticsRepo{data: makeHistory(30)},
			&stubFinanceRepo{},
		)
		portfolio, err := svc.GetExpectedValues(context.Background(), campaignID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return portfolio
	}

	portfolioDefault := runEV(0)
	portfolioHighFee := runEV(0.2500)

	if len(portfolioDefault.Items) == 0 {
		t.Fatalf("expected non-empty EV items with default fee — insufficient segment data in test fixture")
	}
	if len(portfolioHighFee.Items) == 0 {
		t.Fatalf("expected non-empty EV items with high fee — insufficient segment data in test fixture")
	}

	if portfolioHighFee.TotalEVCents >= portfolioDefault.TotalEVCents {
		t.Errorf("expected higher fee to reduce total EV: default=%d highFee=%d",
			portfolioDefault.TotalEVCents, portfolioHighFee.TotalEVCents)
	}
}
