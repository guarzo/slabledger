package arbitrage

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

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
