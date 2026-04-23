package arbitrage

import (
	"context"
	"fmt"
	"testing"
	"time"

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

func TestRunSimulation_PerCardCostNoPanic(t *testing.T) {
	history := []inventory.PurchaseWithSale{
		{Purchase: inventory.Purchase{BuyCostCents: 500, GradeValue: 9, CLValueCents: 2000}, Sale: &inventory.Sale{NetProfitCents: 200, SaleFeeCents: 100}},
		{Purchase: inventory.Purchase{BuyCostCents: 10000, GradeValue: 9, CLValueCents: 15000}, Sale: &inventory.Sale{NetProfitCents: 3000, SaleFeeCents: 1800}},
		{Purchase: inventory.Purchase{BuyCostCents: 600, GradeValue: 9, CLValueCents: 2200}, Sale: &inventory.Sale{NetProfitCents: 250, SaleFeeCents: 110}},
		{Purchase: inventory.Purchase{BuyCostCents: 9500, GradeValue: 9, CLValueCents: 14000}, Sale: &inventory.Sale{NetProfitCents: 2800, SaleFeeCents: 1700}},
		{Purchase: inventory.Purchase{BuyCostCents: 700, GradeValue: 9, CLValueCents: 2500}, Sale: &inventory.Sale{NetProfitCents: 300, SaleFeeCents: 120}},
		{Purchase: inventory.Purchase{BuyCostCents: 11000, GradeValue: 9, CLValueCents: 16000}, Sale: &inventory.Sale{NetProfitCents: 3200, SaleFeeCents: 1900}},
		{Purchase: inventory.Purchase{BuyCostCents: 450, GradeValue: 9, CLValueCents: 1800}, Sale: &inventory.Sale{NetProfitCents: 180, SaleFeeCents: 90}},
		{Purchase: inventory.Purchase{BuyCostCents: 12000, GradeValue: 9, CLValueCents: 17000}, Sale: &inventory.Sale{NetProfitCents: 3500, SaleFeeCents: 2000}},
		{Purchase: inventory.Purchase{BuyCostCents: 800, GradeValue: 9, CLValueCents: 2800}, Sale: &inventory.Sale{NetProfitCents: 350, SaleFeeCents: 130}},
		{Purchase: inventory.Purchase{BuyCostCents: 9000, GradeValue: 9, CLValueCents: 13500}, Sale: &inventory.Sale{NetProfitCents: 2600, SaleFeeCents: 1600}},
		{Purchase: inventory.Purchase{BuyCostCents: 550, GradeValue: 9, CLValueCents: 2100}, Sale: &inventory.Sale{NetProfitCents: 220, SaleFeeCents: 105}},
		{Purchase: inventory.Purchase{BuyCostCents: 10500, GradeValue: 9, CLValueCents: 15500}, Sale: &inventory.Sale{NetProfitCents: 3100, SaleFeeCents: 1850}},
	}

	campaign := &inventory.Campaign{BuyTermsCLPct: 0.65, GradeRange: "9-9"}
	result := RunMonteCarloProjection(campaign, history)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Confidence == "insufficient" {
		t.Errorf("expected simulation to run with 12 history entries, got Confidence=%q", result.Confidence)
	}
}

func TestRunProjection_UsesCacheOnSecondCall(t *testing.T) {
	campaignID := "camp-cache-test"
	campaign := &inventory.Campaign{
		ID:                  campaignID,
		Name:                "Cache Test",
		BuyTermsCLPct:       0.65,
		GradeRange:          "9-9",
		PSASourcingFeeCents: 100,
	}

	history := []inventory.PurchaseWithSale{
		{Purchase: inventory.Purchase{BuyCostCents: 500, GradeValue: 9, CLValueCents: 2000}, Sale: &inventory.Sale{NetProfitCents: 200, SaleFeeCents: 100}},
		{Purchase: inventory.Purchase{BuyCostCents: 10000, GradeValue: 9, CLValueCents: 15000}, Sale: &inventory.Sale{NetProfitCents: 3000, SaleFeeCents: 1800}},
		{Purchase: inventory.Purchase{BuyCostCents: 600, GradeValue: 9, CLValueCents: 2200}, Sale: &inventory.Sale{NetProfitCents: 250, SaleFeeCents: 110}},
		{Purchase: inventory.Purchase{BuyCostCents: 9500, GradeValue: 9, CLValueCents: 14000}, Sale: &inventory.Sale{NetProfitCents: 2800, SaleFeeCents: 1700}},
		{Purchase: inventory.Purchase{BuyCostCents: 700, GradeValue: 9, CLValueCents: 2500}, Sale: &inventory.Sale{NetProfitCents: 300, SaleFeeCents: 120}},
		{Purchase: inventory.Purchase{BuyCostCents: 11000, GradeValue: 9, CLValueCents: 16000}, Sale: &inventory.Sale{NetProfitCents: 3200, SaleFeeCents: 1900}},
		{Purchase: inventory.Purchase{BuyCostCents: 450, GradeValue: 9, CLValueCents: 1800}, Sale: &inventory.Sale{NetProfitCents: 180, SaleFeeCents: 90}},
		{Purchase: inventory.Purchase{BuyCostCents: 12000, GradeValue: 9, CLValueCents: 17000}, Sale: &inventory.Sale{NetProfitCents: 3500, SaleFeeCents: 2000}},
		{Purchase: inventory.Purchase{BuyCostCents: 800, GradeValue: 9, CLValueCents: 2800}, Sale: &inventory.Sale{NetProfitCents: 350, SaleFeeCents: 130}},
		{Purchase: inventory.Purchase{BuyCostCents: 9000, GradeValue: 9, CLValueCents: 13500}, Sale: &inventory.Sale{NetProfitCents: 2600, SaleFeeCents: 1600}},
		{Purchase: inventory.Purchase{BuyCostCents: 550, GradeValue: 9, CLValueCents: 2100}, Sale: &inventory.Sale{NetProfitCents: 220, SaleFeeCents: 105}},
		{Purchase: inventory.Purchase{BuyCostCents: 10500, GradeValue: 9, CLValueCents: 15500}, Sale: &inventory.Sale{NetProfitCents: 3100, SaleFeeCents: 1850}},
	}

	svc := NewService(
		&stubCampaignRepo{campaign: campaign},
		&stubPurchaseRepo{},
		&stubAnalyticsRepo{data: history},
		&stubFinanceRepo{},
		WithProjectionCache(5*time.Minute),
	)

	result1, err := svc.RunProjection(context.Background(), campaignID)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if result1 == nil {
		t.Fatal("first call returned nil result")
	}

	result2, err := svc.RunProjection(context.Background(), campaignID)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if result2 == nil {
		t.Fatal("second call returned nil result")
	}

	if result1.SampleSize != result2.SampleSize || result1.Confidence != result2.Confidence {
		t.Errorf("cached result mismatch: first=%+v second=%+v", result1, result2)
	}
}

func TestGetCrackOpportunities_NilPriceProvider(t *testing.T) {
	campaign := &inventory.Campaign{
		ID:         "camp-crack-nil",
		Name:       "Test Campaign",
		EbayFeePct: 0.1235,
	}

	svc := NewService(
		&stubCampaignRepo{campaign: campaign},
		&stubPurchaseRepo{unsold: []inventory.Purchase{
			{
				ID:           "p-crack",
				CampaignID:   campaign.ID,
				CardName:     "Charizard",
				GradeValue:   8.0,
				BuyCostCents: 10000,
			},
		}},
		&stubAnalyticsRepo{},
		&stubFinanceRepo{},
	)

	results, err := svc.GetCrackOpportunities(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results == nil {
		t.Fatal("expected non-nil slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("expected empty slice with nil priceProv, got %d results", len(results))
	}
}

func TestGetCrackOpportunities_GradeFilter(t *testing.T) {
	campaignID := "camp-crack-grade"
	campaign := &inventory.Campaign{
		ID:         campaignID,
		Name:       "Test Campaign",
		EbayFeePct: 0.1235,
	}

	purchases := []inventory.Purchase{
		{
			ID:                  "p-psa9",
			CampaignID:          campaignID,
			CardName:            "Blastoise",
			CertNumber:          "cert-9",
			GradeValue:          9.0,
			BuyCostCents:        8000,
			PSASourcingFeeCents: 250,
			CLValueCents:        12000,
		},
		{
			ID:                  "p-psa8",
			CampaignID:          campaignID,
			CardName:            "Venusaur",
			CertNumber:          "cert-8",
			GradeValue:          8.0,
			BuyCostCents:        6000,
			PSASourcingFeeCents: 200,
			CLValueCents:        10000,
		},
	}

	svc := NewService(
		&stubCampaignRepo{campaign: campaign},
		&stubPurchaseRepo{unsold: purchases},
		&stubAnalyticsRepo{},
		&stubFinanceRepo{},
		WithPriceLookup(&stubPriceProvider{rawCents: 15000, gradedCents: 12000}),
	)

	results, err := svc.GetCrackOpportunities(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (PSA 8 only), got %d", len(results))
	}

	if results[0].PurchaseID != "p-psa8" {
		t.Errorf("expected purchase p-psa8, got %s", results[0].PurchaseID)
	}
	if results[0].Grade != 8.0 {
		t.Errorf("expected grade 8.0, got %g", results[0].Grade)
	}
}

func TestGetCrackOpportunities_PositiveMatch(t *testing.T) {
	campaignID := "camp-crack-pos"
	campaign := &inventory.Campaign{
		ID:         campaignID,
		Name:       "Test Campaign",
		EbayFeePct: 0.1235,
	}

	purchase := inventory.Purchase{
		ID:                  "p-positive",
		CampaignID:          campaignID,
		CardName:            "Charizard",
		CertNumber:          "cert-pos",
		GradeValue:          7.0,
		BuyCostCents:        5000,
		PSASourcingFeeCents: 200,
		CLValueCents:        8000,
	}

	svc := NewService(
		&stubCampaignRepo{campaign: campaign},
		&stubPurchaseRepo{unsold: []inventory.Purchase{purchase}},
		&stubAnalyticsRepo{},
		&stubFinanceRepo{},
		WithPriceLookup(&stubPriceProvider{rawCents: 15000, gradedCents: 9000}),
	)

	results, err := svc.GetCrackOpportunities(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].CrackAdvantage <= 0 {
		t.Errorf("expected positive CrackAdvantage, got %d", results[0].CrackAdvantage)
	}
	if results[0].PurchaseID != "p-positive" {
		t.Errorf("expected purchase p-positive, got %s", results[0].PurchaseID)
	}
}

func TestGetActivationChecklist_AllPassed(t *testing.T) {
	campaignID := "camp-actvn-pass"
	campaign := &inventory.Campaign{
		ID:                 campaignID,
		Name:               "Test Campaign",
		DailySpendCapCents: 1000,
		EbayFeePct:         0.1235,
	}

	capitalData := &inventory.CapitalRawData{
		OutstandingCents:          100000,
		RecoveryRate30dCents:      500000,
		RecoveryRate30dPriorCents: 450000,
	}

	invoices := []inventory.Invoice{
		{
			ID:     "inv-paid",
			Status: "paid",
		},
	}

	svc := NewService(
		&stubCampaignRepo{campaign: campaign},
		&stubPurchaseRepo{},
		&stubAnalyticsRepo{},
		&stubFinanceRepoWithData{
			capital:  capitalData,
			invoices: invoices,
		},
	)

	checklist, err := svc.GetActivationChecklist(context.Background(), campaignID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if checklist == nil {
		t.Fatal("expected non-nil checklist")
	}

	if !checklist.AllPassed {
		t.Error("expected AllPassed=true, got false")
	}

	if len(checklist.Checks) < 3 {
		t.Errorf("expected at least 3 checks, got %d", len(checklist.Checks))
	}

	for _, check := range checklist.Checks {
		if !check.Passed {
			t.Errorf("expected check %q to pass, but it failed", check.Name)
		}
	}
}

func TestGetActivationChecklist_FailsNoPaidInvoice(t *testing.T) {
	campaignID := "camp-actvn-fail"
	campaign := &inventory.Campaign{
		ID:                 campaignID,
		Name:               "Test Campaign",
		DailySpendCapCents: 1000,
		EbayFeePct:         0.1235,
	}

	capitalData := &inventory.CapitalRawData{
		OutstandingCents:          50000,
		RecoveryRate30dCents:      400000,
		RecoveryRate30dPriorCents: 380000,
	}

	invoices := []inventory.Invoice{
		{
			ID:     "inv-unpaid",
			Status: "unpaid",
		},
	}

	svc := NewService(
		&stubCampaignRepo{campaign: campaign},
		&stubPurchaseRepo{},
		&stubAnalyticsRepo{},
		&stubFinanceRepoWithData{
			capital:  capitalData,
			invoices: invoices,
		},
	)

	checklist, err := svc.GetActivationChecklist(context.Background(), campaignID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if checklist == nil {
		t.Fatal("expected non-nil checklist")
	}

	if checklist.AllPassed {
		t.Error("expected AllPassed=false when no paid invoices, got true")
	}

	invoiceCycleCheckFound := false
	for _, check := range checklist.Checks {
		if check.Name == "Invoice Cycle Cleared" {
			invoiceCycleCheckFound = true
			if check.Passed {
				t.Errorf("expected Invoice Cycle check to fail when no paid invoices, got Passed=true")
			}
			break
		}
	}

	if !invoiceCycleCheckFound {
		t.Error("expected to find 'Invoice Cycle Cleared' check in checklist")
	}
}
