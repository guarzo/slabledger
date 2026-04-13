package inventory

import (
	"context"
	"fmt"
	"testing"
)

func TestImportOrdersSales(t *testing.T) {
	repo := newMockRepo()

	// Set up a campaign and purchases
	campaign := &Campaign{ID: "camp-1", Name: "Test Campaign", EbayFeePct: 0.1235}
	repo.campaigns["camp-1"] = campaign

	repo.purchases["purch-1"] = &Purchase{
		ID:                  "purch-1",
		CampaignID:          "camp-1",
		CertNumber:          "111111",
		CardName:            "Charizard",
		BuyCostCents:        10000,
		PSASourcingFeeCents: 300,
		GradeValue:          9,
		PurchaseDate:        "2026-01-01",
	}
	repo.purchases["purch-2"] = &Purchase{
		ID:                  "purch-2",
		CampaignID:          "camp-1",
		CertNumber:          "222222",
		CardName:            "Pikachu",
		BuyCostCents:        5000,
		PSASourcingFeeCents: 300,
		GradeValue:          10,
		PurchaseDate:        "2026-01-01",
	}
	// purch-3 already sold
	repo.purchases["purch-3"] = &Purchase{
		ID:                  "purch-3",
		CampaignID:          "camp-1",
		CertNumber:          "333333",
		CardName:            "Blastoise",
		BuyCostCents:        8000,
		PSASourcingFeeCents: 300,
		GradeValue:          8,
		PurchaseDate:        "2026-01-01",
	}
	repo.sales["sale-3"] = &Sale{ID: "sale-3", PurchaseID: "purch-3"}
	repo.purchaseSales["purch-3"] = true

	svc := NewService(repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(func() string { return "gen-id" }))
	defer svc.Close()

	rows := []OrdersExportRow{
		{CertNumber: "111111", Date: "2026-03-10", SalesChannel: SaleChannelEbay, ProductTitle: "Charizard PSA 9", UnitPrice: 200.00},
		{CertNumber: "333333", Date: "2026-03-11", SalesChannel: SaleChannelEbay, ProductTitle: "Blastoise PSA 8", UnitPrice: 150.00},
		{CertNumber: "999999", Date: "2026-03-12", SalesChannel: SaleChannelWebsite, ProductTitle: "Unknown PSA 10", UnitPrice: 50.00},
	}

	result, err := svc.ImportOrdersSales(context.Background(), rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1 matched (cert 111111)
	if len(result.Matched) != 1 {
		t.Fatalf("matched: got %d, want 1", len(result.Matched))
	}
	m := result.Matched[0]
	if m.CertNumber != "111111" {
		t.Errorf("matched cert: got %q, want 111111", m.CertNumber)
	}
	if m.PurchaseID != "purch-1" {
		t.Errorf("matched purchaseID: got %q, want purch-1", m.PurchaseID)
	}
	if m.SalePriceCents != 20000 {
		t.Errorf("matched salePriceCents: got %d, want 20000", m.SalePriceCents)
	}
	// eBay fee: 12.35% of 20000 = 2470
	if m.SaleFeeCents != 2470 {
		t.Errorf("matched saleFeeCents: got %d, want 2470", m.SaleFeeCents)
	}
	// Net: 20000 - 10000 - 300 - 2470 = 7230
	if m.NetProfitCents != 7230 {
		t.Errorf("matched netProfit: got %d, want 7230", m.NetProfitCents)
	}

	// 1 already sold (cert 333333)
	if len(result.AlreadySold) != 1 {
		t.Fatalf("alreadySold: got %d, want 1", len(result.AlreadySold))
	}
	if result.AlreadySold[0].CertNumber != "333333" {
		t.Errorf("alreadySold cert: got %q, want 333333", result.AlreadySold[0].CertNumber)
	}

	// 1 not found (cert 999999)
	if len(result.NotFound) != 1 {
		t.Fatalf("notFound: got %d, want 1", len(result.NotFound))
	}
	if result.NotFound[0].CertNumber != "999999" {
		t.Errorf("notFound cert: got %q, want 999999", result.NotFound[0].CertNumber)
	}
}

func TestConfirmOrdersSales(t *testing.T) {
	repo := newMockRepo()

	campaign := &Campaign{ID: "camp-1", Name: "Test Campaign", EbayFeePct: 0.1235}
	repo.campaigns["camp-1"] = campaign

	repo.purchases["purch-1"] = &Purchase{
		ID:                  "purch-1",
		CampaignID:          "camp-1",
		CertNumber:          "111111",
		CardName:            "Charizard",
		BuyCostCents:        10000,
		PSASourcingFeeCents: 300,
		GradeValue:          9,
		PurchaseDate:        "2026-01-01",
	}

	idCounter := 0
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(func() string {
		idCounter++
		return fmt.Sprintf("sale-%d", idCounter)
	}))
	defer svc.Close()

	items := []OrdersConfirmItem{
		{PurchaseID: "purch-1", SaleChannel: SaleChannelEbay, SaleDate: "2026-03-10", SalePriceCents: 20000},
		{PurchaseID: "purch-bad", SaleChannel: SaleChannelEbay, SaleDate: "2026-03-10", SalePriceCents: 5000},
	}

	result, err := svc.ConfirmOrdersSales(context.Background(), items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Created != 1 {
		t.Errorf("created: got %d, want 1", result.Created)
	}
	if result.Failed != 1 {
		t.Errorf("failed: got %d, want 1", result.Failed)
	}

	// Verify the sale was actually created
	if _, exists := repo.sales["sale-1"]; !exists {
		t.Error("expected sale-1 to exist in repo")
	}
}
