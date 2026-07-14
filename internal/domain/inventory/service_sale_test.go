package inventory_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// End-to-end: a forced-channel (inperson) sale dated within 6 days before an
// unpaid invoice's due date must persist ForcedLiquidation = true via CreateSale.
func TestService_CreateSale_FlagsForcedLiquidation(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	// Fixed reference dates so the scenario is reproducible (no clock dependency).
	// saleDate is 5 days before dueDate — inside the 0..6 forced-liquidation window.
	const (
		saleDate     = "2026-07-10"
		dueDate      = "2026-07-15" // saleDate + 5
		invoiceDate  = "2026-07-08" // saleDate - 2
		purchaseDate = "2026-06-10" // well before saleDate so CreateSale's date checks pass
	)

	// Unpaid invoice due 5 days after the sale.
	if err := repo.CreateInvoice(ctx, &inventory.Invoice{
		ID: "inv-forced", InvoiceDate: invoiceDate,
		TotalCents: 100000, DueDate: dueDate, Status: "unpaid",
	}); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup campaign: %v", err)
	}

	// Purchase dated well before the sale so CreateSale's date checks pass.
	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "FORCED01",
		GradeValue: 9, BuyCostCents: 50000, PurchaseDate: purchaseDate,
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup purchase: %v", err)
	}

	s := &inventory.Sale{
		PurchaseID:     p.ID,
		SaleChannel:    inventory.SaleChannelInPerson,
		SalePriceCents: 60000,
		SaleDate:       saleDate,
	}
	if err := svc.CreateSale(ctx, s, c, p); err != nil {
		t.Fatalf("CreateSale: %v", err)
	}

	if !s.ForcedLiquidation {
		t.Errorf("ForcedLiquidation = false, want true (inperson sale %s, invoice due %s)", saleDate, dueDate)
	}

	// Control: an ebay sale on the same date must NOT be flagged.
	p2 := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Pikachu", CertNumber: "FORCED02",
		GradeValue: 10, BuyCostCents: 30000, PurchaseDate: purchaseDate,
	}
	if err := svc.CreatePurchase(ctx, p2); err != nil {
		t.Fatalf("setup purchase 2: %v", err)
	}
	s2 := &inventory.Sale{
		PurchaseID:     p2.ID,
		SaleChannel:    inventory.SaleChannelEbay,
		SalePriceCents: 40000,
		SaleDate:       saleDate,
	}
	if err := svc.CreateSale(ctx, s2, c, p2); err != nil {
		t.Fatalf("CreateSale (control): %v", err)
	}
	if s2.ForcedLiquidation {
		t.Errorf("control ForcedLiquidation = true, want false (ebay is not a forced channel)")
	}
}
