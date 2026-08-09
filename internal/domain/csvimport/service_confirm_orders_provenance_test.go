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

	result, err := imp.ConfirmOrdersSales(ctx, []csvimport.OrdersConfirmItem{
		{
			PurchaseID:     p.ID,
			SaleChannel:    inventory.SaleChannelEbay,
			SaleDate:       "2026-06-20",
			SalePriceCents: 20000,
			TheirCompCents: 19500,
		},
	})
	if err != nil {
		t.Fatalf("ConfirmOrdersSales: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("created = %d, want 1 (errors: %v)", result.Created, result.Errors)
	}

	sale := findSaleByPurchaseID(t, repo, c.ID, p.ID)
	if sale.PriceSource != inventory.PriceSourceItemized {
		t.Errorf("PriceSource = %q, want %q", sale.PriceSource, inventory.PriceSourceItemized)
	}
	if sale.TheirCompCents != 19500 {
		t.Errorf("TheirCompCents = %d, want 19500", sale.TheirCompCents)
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
