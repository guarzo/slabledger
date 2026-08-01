package inventory_test

import (
	"context"
	"errors"
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

// setupSaleFixture creates a campaign (10% ebay fee) and a purchase with a
// known CLValueCents, returning both for use across the provenance tests.
func setupSaleFixture(t *testing.T, repo *mocks.InMemoryCampaignStore, svc inventory.Service) (*inventory.Campaign, *inventory.Purchase) {
	t.Helper()
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78, EbayFeePct: 0.10}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup campaign: %v", err)
	}

	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "PROV01",
		GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-06-01",
		CLValueCents: 12000,
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup purchase: %v", err)
	}
	return c, p
}

func TestCreateSale_FreezesSaleProvenance(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c, p := setupSaleFixture(t, repo, svc)

	s := &inventory.Sale{
		PurchaseID:     p.ID,
		SaleChannel:    inventory.SaleChannelEbay,
		SalePriceCents: 20000,
		SaleDate:       "2026-06-20",
	}
	if err := svc.CreateSale(ctx, s, c, p); err != nil {
		t.Fatalf("CreateSale: %v", err)
	}

	if s.CLValueAtSaleCents != 12000 {
		t.Errorf("CLValueAtSaleCents = %d, want 12000", s.CLValueAtSaleCents)
	}
	if s.ChannelFeePctAtSale == nil || *s.ChannelFeePctAtSale != 0.10 {
		t.Errorf("ChannelFeePctAtSale = %v, want 0.10", s.ChannelFeePctAtSale)
	}
	if s.SaleReason != inventory.SaleReasonDiscretionary {
		t.Errorf("SaleReason = %q, want %q", s.SaleReason, inventory.SaleReasonDiscretionary)
	}
}

func TestCreateSale_PreservesExplicitReason(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c, p := setupSaleFixture(t, repo, svc)

	s := &inventory.Sale{
		PurchaseID:     p.ID,
		SaleChannel:    inventory.SaleChannelEbay,
		SalePriceCents: 20000,
		SaleDate:       "2026-06-20",
		SaleReason:     inventory.SaleReasonAgingPolicy,
	}
	if err := svc.CreateSale(ctx, s, c, p); err != nil {
		t.Fatalf("CreateSale: %v", err)
	}

	if s.SaleReason != inventory.SaleReasonAgingPolicy {
		t.Errorf("SaleReason = %q, want %q (explicit reason must be preserved)", s.SaleReason, inventory.SaleReasonAgingPolicy)
	}
}

func TestCreateSale_RejectsInvalidReason(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c, p := setupSaleFixture(t, repo, svc)

	s := &inventory.Sale{
		PurchaseID:     p.ID,
		SaleChannel:    inventory.SaleChannelEbay,
		SalePriceCents: 20000,
		SaleDate:       "2026-06-20",
		SaleReason:     "bogus",
	}
	err := svc.CreateSale(ctx, s, c, p)
	if !errors.Is(err, inventory.ErrInvalidSaleReason) {
		t.Fatalf("CreateSale error = %v, want ErrInvalidSaleReason", err)
	}
}

func TestCreateSale_IgnoresClientForgedProvenance(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c, p := setupSaleFixture(t, repo, svc)

	forgedPct := 0.99
	s := &inventory.Sale{
		PurchaseID:          p.ID,
		SaleChannel:         inventory.SaleChannelEbay,
		SalePriceCents:      20000,
		SaleDate:            "2026-06-20",
		CLValueAtSaleCents:  99999999,
		ChannelFeePctAtSale: &forgedPct,
	}
	if err := svc.CreateSale(ctx, s, c, p); err != nil {
		t.Fatalf("CreateSale: %v", err)
	}

	if s.CLValueAtSaleCents != 12000 {
		t.Errorf("CLValueAtSaleCents = %d, want 12000 (forged value must be overwritten)", s.CLValueAtSaleCents)
	}
	if s.ChannelFeePctAtSale == nil || *s.ChannelFeePctAtSale != 0.10 {
		t.Errorf("ChannelFeePctAtSale = %v, want 0.10 (forged value must be overwritten)", s.ChannelFeePctAtSale)
	}
}

func TestCreateBulkSales_FreezesProvenance(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c, p := setupSaleFixture(t, repo, svc)

	// Second purchase for the in-person / invoice-pressure item.
	p2 := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Pikachu", CertNumber: "PROV02",
		GradeValue: 10, BuyCostCents: 30000, PurchaseDate: "2026-06-01",
		CLValueCents: 8000,
	}
	if err := svc.CreatePurchase(ctx, p2); err != nil {
		t.Fatalf("setup purchase 2: %v", err)
	}

	// Unpaid invoice due 5 days after the in-person sale date, to trigger the
	// forced-liquidation heuristic for that item.
	if err := repo.CreateInvoice(ctx, &inventory.Invoice{
		ID: "inv-bulk", InvoiceDate: "2026-06-18",
		TotalCents: 100000, DueDate: "2026-06-25", Status: "unpaid",
	}); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}

	result, err := svc.CreateBulkSales(ctx, c.ID, inventory.SaleChannelEbay, "2026-06-20", []inventory.BulkSaleInput{
		{PurchaseID: p.ID, SalePriceCents: 20000},
	})
	if err != nil {
		t.Fatalf("CreateBulkSales: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("created = %d, want 1 (errors: %v)", result.Created, result.Errors)
	}

	var discretionarySale *inventory.Sale
	salesList, _ := repo.ListSalesByCampaign(ctx, c.ID, 100, 0)
	for i := range salesList {
		s := &salesList[i]
		if s.PurchaseID == p.ID {
			discretionarySale = s
		}
	}
	if discretionarySale == nil {
		t.Fatal("expected sale for purchase 1 to exist")
	}
	if discretionarySale.SaleReason != inventory.SaleReasonDiscretionary {
		t.Errorf("SaleReason = %q, want %q", discretionarySale.SaleReason, inventory.SaleReasonDiscretionary)
	}
	if discretionarySale.CLValueAtSaleCents != p.CLValueCents {
		t.Errorf("CLValueAtSaleCents = %d, want %d", discretionarySale.CLValueAtSaleCents, p.CLValueCents)
	}
	wantPct := inventory.EffectiveChannelFeePct(inventory.SaleChannelEbay, c)
	if discretionarySale.ChannelFeePctAtSale == nil || *discretionarySale.ChannelFeePctAtSale != wantPct {
		t.Errorf("ChannelFeePctAtSale = %v, want %v", discretionarySale.ChannelFeePctAtSale, wantPct)
	}

	// Second bulk call: in-person channel within the invoice-pressure window.
	result2, err := svc.CreateBulkSales(ctx, c.ID, inventory.SaleChannelInPerson, "2026-06-20", []inventory.BulkSaleInput{
		{PurchaseID: p2.ID, SalePriceCents: 15000},
	})
	if err != nil {
		t.Fatalf("CreateBulkSales (in-person): %v", err)
	}
	if result2.Created != 1 {
		t.Fatalf("created = %d, want 1 (errors: %v)", result2.Created, result2.Errors)
	}

	var forcedSale *inventory.Sale
	salesList2, _ := repo.ListSalesByCampaign(ctx, c.ID, 100, 0)
	for i := range salesList2 {
		s := &salesList2[i]
		if s.PurchaseID == p2.ID {
			forcedSale = s
		}
	}
	if forcedSale == nil {
		t.Fatal("expected sale for purchase 2 to exist")
	}
	if forcedSale.SaleReason != inventory.SaleReasonInvoicePressure {
		t.Errorf("SaleReason = %q, want %q", forcedSale.SaleReason, inventory.SaleReasonInvoicePressure)
	}
}

func TestConfirmOrdersSales_FreezesProvenance(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c, p := setupSaleFixture(t, repo, svc)

	result, err := svc.ConfirmOrdersSales(ctx, []inventory.OrdersConfirmItem{
		{PurchaseID: p.ID, SaleChannel: inventory.SaleChannelEbay, SaleDate: "2026-06-20", SalePriceCents: 20000},
	})
	if err != nil {
		t.Fatalf("ConfirmOrdersSales: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("created = %d, want 1 (errors: %v)", result.Created, result.Errors)
	}

	var sale *inventory.Sale
	salesList, _ := repo.ListSalesByCampaign(ctx, c.ID, 100, 0)
	for i := range salesList {
		s := &salesList[i]
		if s.PurchaseID == p.ID {
			sale = s
		}
	}
	if sale == nil {
		t.Fatal("expected sale to exist")
	}
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
