package csvimport_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/csvimport"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// TestConfirmOrdersSales_FreezesProvenance moved here from
// internal/domain/inventory when CSV intake was split out (SLA-35). It builds its
// fixture through the inventory service, because the provenance it asserts —
// CL value and channel fee pct at sale — is frozen from the campaign and purchase
// that inventory owns.
func TestConfirmOrdersSales_FreezesProvenance(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	inv, imp := newServices(repo, nil)
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78, EbayFeePct: 0.10}
	if err := inv.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup campaign: %v", err)
	}
	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "PROV01",
		GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-06-01",
		CLValueCents: 12000,
	}
	if err := inv.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup purchase: %v", err)
	}

	result, err := imp.ConfirmOrdersSales(ctx, []csvimport.OrdersConfirmItem{
		{PurchaseID: p.ID, SaleChannel: inventory.SaleChannelEbay, SaleDate: "2026-06-20", SalePriceCents: 20000},
	})
	if err != nil {
		t.Fatalf("ConfirmOrdersSales: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("created = %d, want 1 (errors: %v)", result.Created, result.Errors)
	}

	sale := findSaleByPurchaseID(t, repo, c.ID, p.ID)
	if sale.SaleReason != inventory.SaleReasonDiscretionary {
		t.Errorf("SaleReason = %q, want %q", sale.SaleReason, inventory.SaleReasonDiscretionary)
	}
	if sale.CLValueAtSaleCents != p.CLValueCents {
		t.Errorf("CLValueAtSaleCents = %d, want %d", sale.CLValueAtSaleCents, p.CLValueCents)
	}
	wantPct := inventory.EffectiveChannelFeePct(inventory.SaleChannelEbay, c)
	if sale.ChannelFeePctAtSale == nil || *sale.ChannelFeePctAtSale != wantPct {
		t.Errorf("ChannelFeePctAtSale = %v, want %v", sale.ChannelFeePctAtSale, wantPct)
	}
}

func TestConfirmOrdersSales_PriceSourceDefault(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	inv, imp := newServices(repo, nil)
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78, EbayFeePct: 0.10}
	if err := inv.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup campaign: %v", err)
	}
	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Blastoise", CertNumber: "PROV05",
		GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-06-01",
		CLValueCents: 12000,
	}
	if err := inv.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup purchase: %v", err)
	}

	p2 := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Gyarados", CertNumber: "PROV07",
		GradeValue: 8, BuyCostCents: 40000, PurchaseDate: "2026-06-01",
		CLValueCents: 9000,
	}
	if err := inv.CreatePurchase(ctx, p2); err != nil {
		t.Fatalf("setup purchase 2: %v", err)
	}

	p3 := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Vaporeon", CertNumber: "PROV08",
		GradeValue: 8, BuyCostCents: 35000, PurchaseDate: "2026-06-01",
		CLValueCents: 8500,
	}
	if err := inv.CreatePurchase(ctx, p3); err != nil {
		t.Fatalf("setup purchase 3: %v", err)
	}

	result, err := imp.ConfirmOrdersSales(ctx, []csvimport.OrdersConfirmItem{
		{
			PurchaseID:     p.ID,
			SaleChannel:    inventory.SaleChannelEbay,
			SaleDate:       "2026-06-20",
			SalePriceCents: 20000,
			TheirCompCents: 19500,
		},
		{
			PurchaseID:     p2.ID,
			SaleChannel:    inventory.SaleChannelEbay,
			SaleDate:       "2026-06-20",
			SalePriceCents: 18000,
			PriceSource:    inventory.PriceSourceEstimated,
		},
		{
			PurchaseID:     p3.ID,
			SaleChannel:    inventory.SaleChannelEbay,
			SaleDate:       "2026-06-20",
			SalePriceCents: 17000,
			PriceSource:    "bogus",
		},
	})
	if err != nil {
		t.Fatalf("ConfirmOrdersSales: %v", err)
	}
	if result.Created != 2 {
		t.Fatalf("created = %d, want 2 (errors: %v)", result.Created, result.Errors)
	}
	if result.Failed != 1 {
		t.Fatalf("failed = %d, want 1", result.Failed)
	}

	sale := findSaleByPurchaseID(t, repo, c.ID, p.ID)
	if sale.PriceSource != inventory.PriceSourceItemized {
		t.Errorf("PriceSource = %q, want %q", sale.PriceSource, inventory.PriceSourceItemized)
	}
	if sale.TheirCompCents != 19500 {
		t.Errorf("TheirCompCents = %d, want 19500", sale.TheirCompCents)
	}

	explicit := findSaleByPurchaseID(t, repo, c.ID, p2.ID)
	if explicit.PriceSource != inventory.PriceSourceEstimated {
		t.Errorf("PriceSource = %q, want %q (explicit priceSource must survive the itemized default)", explicit.PriceSource, inventory.PriceSourceEstimated)
	}

	// ConfirmOrdersSales stringifies the per-item error into BulkSaleError.Error
	// rather than returning it, so errors.Is cannot be applied to the result
	// directly; compare against the sentinel's own .Error() text instead, which
	// is exactly what FreezeSaleProvenance's error becomes on this path.
	if len(result.Errors) != 1 || result.Errors[0].PurchaseID != p3.ID {
		t.Fatalf("errors = %+v, want single error for purchase 3", result.Errors)
	}
	if result.Errors[0].Error != inventory.ErrInvalidPriceSource.Error() {
		t.Errorf("Errors[0].Error = %q, want %q", result.Errors[0].Error, inventory.ErrInvalidPriceSource.Error())
	}
}

// findSaleByPurchaseID returns the sale recorded against purchaseID, failing the
// test if the campaign has no such sale.
func findSaleByPurchaseID(t *testing.T, repo *mocks.InMemoryCampaignStore, campaignID, purchaseID string) *inventory.Sale {
	t.Helper()
	sales, err := repo.ListSalesByCampaign(context.Background(), campaignID, 100, 0)
	if err != nil {
		t.Fatalf("list sales: %v", err)
	}
	for i := range sales {
		if sales[i].PurchaseID == purchaseID {
			return &sales[i]
		}
	}
	t.Fatalf("no sale found for purchase %s", purchaseID)
	return nil
}
