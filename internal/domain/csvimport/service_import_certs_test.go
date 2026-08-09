package csvimport_test

import (
	"context"
	"math"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/csvimport"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func seedCertSalePurchases(repo *mocks.InMemoryCampaignStore) {
	campaign := &inventory.Campaign{ID: "camp-1", Name: "Show Campaign", EbayFeePct: 0.1}
	repo.Campaigns["camp-1"] = campaign

	repo.Purchases["purch-1"] = &inventory.Purchase{
		ID: "purch-1", CampaignID: "camp-1", CertNumber: "05442200",
		CardName: "Charizard", BuyCostCents: 10000, PSASourcingFeeCents: 300,
		CLValueCents: 15000, PurchaseDate: "2026-01-01",
	}
	repo.Purchases["purch-2"] = &inventory.Purchase{
		ID: "purch-2", CampaignID: "camp-1", CertNumber: "17979513",
		CardName: "Pikachu", BuyCostCents: 5000, PSASourcingFeeCents: 300,
		CLValueCents: 8000, PurchaseDate: "2026-01-01",
	}
	// already sold
	repo.Purchases["purch-3"] = &inventory.Purchase{
		ID: "purch-3", CampaignID: "camp-1", CertNumber: "6098919001",
		CardName: "Blastoise", BuyCostCents: 8000, PSASourcingFeeCents: 300,
		PurchaseDate: "2026-01-01",
	}
	repo.Sales["sale-3"] = &inventory.Sale{ID: "sale-3", PurchaseID: "purch-3"}
	repo.PurchaseSales["purch-3"] = true
}

func TestImportCertSales_HappyPathWithRounding(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	seedCertSalePurchases(repo)
	svc := newOrdersImportService(repo, func() string { return "gen-id" })
	defer svc.Close()

	req := csvimport.CertSaleImportRequest{
		SaleDate:      "2026-03-10",
		SaleChannel:   inventory.SaleChannelEbay,
		NegotiatedPct: 0.72,
		Items: []csvimport.CertSaleItem{
			{CertNumber: "05442200", TheirCompCents: 10000}, // 10000*0.72=7200
			{CertNumber: "17979513", TheirCompCents: 5001},  // 5001*0.72=3600.72 -> round to 3601
		},
	}

	result, err := svc.ImportCertSales(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Matched) != 2 {
		t.Fatalf("matched: got %d, want 2", len(result.Matched))
	}
	byCert := map[string]csvimport.CertSaleMatch{}
	for _, m := range result.Matched {
		byCert[m.CertNumber] = m
	}
	if got := byCert["05442200"].SalePriceCents; got != 7200 {
		t.Errorf("salePriceCents for 05442200: got %d, want 7200", got)
	}
	if got := byCert["17979513"].SalePriceCents; got != 3601 {
		t.Errorf("salePriceCents for 17979513: got %d, want 3601 (rounded)", got)
	}
	wantTotal := 7200 + 3601
	if result.ComputedTotalCents != wantTotal {
		t.Errorf("computedTotalCents: got %d, want %d", result.ComputedTotalCents, wantTotal)
	}
	if result.ReconcileDeltaCents != 0 {
		t.Errorf("reconcileDeltaCents: got %d, want 0 (no TotalReceivedCents supplied)", result.ReconcileDeltaCents)
	}
}

func TestImportCertSales_PctOutOfRange(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	seedCertSalePurchases(repo)
	svc := newOrdersImportService(repo, func() string { return "gen-id" })
	defer svc.Close()

	for _, pct := range []float64{0, -0.1, 1.01, 72} {
		req := csvimport.CertSaleImportRequest{
			SaleDate: "2026-03-10", SaleChannel: inventory.SaleChannelEbay,
			NegotiatedPct: pct,
			Items:         []csvimport.CertSaleItem{{CertNumber: "05442200", TheirCompCents: 10000}},
		}
		if _, err := svc.ImportCertSales(context.Background(), req); err == nil {
			t.Errorf("pct %v: expected error, got nil", pct)
		}
	}
}

func TestImportCertSales_MissingSaleDate(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	seedCertSalePurchases(repo)
	svc := newOrdersImportService(repo, func() string { return "gen-id" })
	defer svc.Close()

	req := csvimport.CertSaleImportRequest{
		NegotiatedPct: 0.72,
		Items:         []csvimport.CertSaleItem{{CertNumber: "05442200", TheirCompCents: 10000}},
	}
	if _, err := svc.ImportCertSales(context.Background(), req); err == nil {
		t.Error("expected error for missing sale date")
	}
}

func TestImportCertSales_EmptyItems(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	seedCertSalePurchases(repo)
	svc := newOrdersImportService(repo, func() string { return "gen-id" })
	defer svc.Close()

	req := csvimport.CertSaleImportRequest{SaleDate: "2026-03-10", NegotiatedPct: 0.72}
	if _, err := svc.ImportCertSales(context.Background(), req); err == nil {
		t.Error("expected error for empty items")
	}
}

func TestImportCertSales_AlreadySold(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	seedCertSalePurchases(repo)
	svc := newOrdersImportService(repo, func() string { return "gen-id" })
	defer svc.Close()

	req := csvimport.CertSaleImportRequest{
		SaleDate: "2026-03-10", NegotiatedPct: 0.72,
		Items: []csvimport.CertSaleItem{{CertNumber: "6098919001", TheirCompCents: 10000}},
	}
	result, err := svc.ImportCertSales(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.AlreadySold) != 1 || result.AlreadySold[0].CertNumber != "6098919001" {
		t.Fatalf("alreadySold: got %+v", result.AlreadySold)
	}
	if len(result.Matched) != 0 {
		t.Errorf("matched: got %d, want 0", len(result.Matched))
	}
}

func TestImportCertSales_NotFound(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	seedCertSalePurchases(repo)
	svc := newOrdersImportService(repo, func() string { return "gen-id" })
	defer svc.Close()

	req := csvimport.CertSaleImportRequest{
		SaleDate: "2026-03-10", NegotiatedPct: 0.72,
		Items: []csvimport.CertSaleItem{{CertNumber: "00000000", TheirCompCents: 10000}},
	}
	result, err := svc.ImportCertSales(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.NotFound) != 1 || result.NotFound[0].CertNumber != "00000000" {
		t.Fatalf("notFound: got %+v", result.NotFound)
	}
}

func TestImportCertSales_InvalidComp(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	seedCertSalePurchases(repo)
	svc := newOrdersImportService(repo, func() string { return "gen-id" })
	defer svc.Close()

	req := csvimport.CertSaleImportRequest{
		SaleDate: "2026-03-10", NegotiatedPct: 0.72,
		Items: []csvimport.CertSaleItem{{CertNumber: "05442200", TheirCompCents: 0}},
	}
	result, err := svc.ImportCertSales(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.NotFound) != 1 || result.NotFound[0].Reason != "invalid_comp" {
		t.Fatalf("notFound: got %+v", result.NotFound)
	}
}

func TestImportCertSales_ReconcileDelta(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	seedCertSalePurchases(repo)
	svc := newOrdersImportService(repo, func() string { return "gen-id" })
	defer svc.Close()

	req := csvimport.CertSaleImportRequest{
		SaleDate: "2026-03-10", NegotiatedPct: 0.72,
		TotalReceivedCents: 7000, // computed will be 7200
		Items:              []csvimport.CertSaleItem{{CertNumber: "05442200", TheirCompCents: 10000}},
	}
	result, err := svc.ImportCertSales(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ReconcileDeltaCents != 200 {
		t.Errorf("reconcileDeltaCents: got %d, want 200", result.ReconcileDeltaCents)
	}
}

func TestImportCertSales_NearDuplicateCerts(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	seedCertSalePurchases(repo)
	svc := newOrdersImportService(repo, func() string { return "gen-id" })
	defer svc.Close()

	req := csvimport.CertSaleImportRequest{
		SaleDate: "2026-03-10", NegotiatedPct: 0.72,
		Items: []csvimport.CertSaleItem{
			{CertNumber: "05442200", TheirCompCents: 10000},
			{CertNumber: "05442201", TheirCompCents: 9000}, // equal length, 1 digit different from above
			{CertNumber: "6098919001", TheirCompCents: 4000},
			{CertNumber: "609891900", TheirCompCents: 4000}, // one shorter (deletion) vs above
		},
	}
	result, err := svc.ImportCertSales(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]bool{"05442200": true, "05442201": true, "6098919001": true, "609891900": true}
	if len(result.NearDuplicateCerts) != len(want) {
		t.Fatalf("nearDuplicateCerts: got %v, want 4 entries covering %v", result.NearDuplicateCerts, want)
	}
	for _, c := range result.NearDuplicateCerts {
		if !want[c] {
			t.Errorf("unexpected cert in nearDuplicateCerts: %q", c)
		}
	}
}

var _ = math.Round // silence unused import if edits above ever drop the direct use
