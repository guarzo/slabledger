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

func TestCreateSale_Provenance(t *testing.T) {
	tests := []struct {
		name             string
		inputReason      string
		inputPriceSource string
		forgedCLValue    int
		forgedFeePct     *float64
		wantReason       string
		wantPriceSource  string
		wantErr          error
	}{
		{
			name:            "freezes derived provenance, defaults price source to manual",
			wantReason:      inventory.SaleReasonDiscretionary,
			wantPriceSource: inventory.PriceSourceManual,
		},
		{
			name:             "preserves explicit reason and price source",
			inputReason:      inventory.SaleReasonAgingPolicy,
			inputPriceSource: inventory.PriceSourceEstimated,
			wantReason:       inventory.SaleReasonAgingPolicy,
			wantPriceSource:  inventory.PriceSourceEstimated,
		},
		{
			name:        "rejects invalid reason",
			inputReason: "bogus",
			wantErr:     inventory.ErrInvalidSaleReason,
		},
		{
			name:             "rejects invalid price source",
			inputPriceSource: "bogus",
			wantErr:          inventory.ErrInvalidPriceSource,
		},
		{
			name:            "ignores client-forged provenance",
			forgedCLValue:   99999999,
			forgedFeePct:    func() *float64 { v := 0.99; return &v }(),
			wantReason:      inventory.SaleReasonDiscretionary,
			wantPriceSource: inventory.PriceSourceManual,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewInMemoryCampaignStore()
			svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
			ctx := context.Background()

			c, p := setupSaleFixture(t, repo, svc)

			s := &inventory.Sale{
				PurchaseID:          p.ID,
				SaleChannel:         inventory.SaleChannelEbay,
				SalePriceCents:      20000,
				SaleDate:            "2026-06-20",
				SaleReason:          tt.inputReason,
				PriceSource:         tt.inputPriceSource,
				CLValueAtSaleCents:  tt.forgedCLValue,
				ChannelFeePctAtSale: tt.forgedFeePct,
			}
			err := svc.CreateSale(ctx, s, c, p)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("CreateSale error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateSale: %v", err)
			}

			if s.SaleReason != tt.wantReason {
				t.Errorf("SaleReason = %q, want %q", s.SaleReason, tt.wantReason)
			}
			if s.PriceSource != tt.wantPriceSource {
				t.Errorf("PriceSource = %q, want %q", s.PriceSource, tt.wantPriceSource)
			}
			// CLValueAtSaleCents and ChannelFeePctAtSale must always reflect the
			// purchase/campaign's real values, never client input (forged or not).
			if s.CLValueAtSaleCents != 12000 {
				t.Errorf("CLValueAtSaleCents = %d, want 12000 (must be server-derived, not the forged/input value)", s.CLValueAtSaleCents)
			}
			if s.ChannelFeePctAtSale == nil || *s.ChannelFeePctAtSale != 0.10 {
				t.Errorf("ChannelFeePctAtSale = %v, want 0.10 (must be server-derived, not the forged/input value)", s.ChannelFeePctAtSale)
			}
		})
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

	discretionarySale := findSaleByPurchaseID(t, repo, c.ID, p.ID)
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

	forcedSale := findSaleByPurchaseID(t, repo, c.ID, p2.ID)
	if forcedSale.SaleReason != inventory.SaleReasonInvoicePressure {
		t.Errorf("SaleReason = %q, want %q", forcedSale.SaleReason, inventory.SaleReasonInvoicePressure)
	}
}

func TestCreateBulkSales_CopiesPerItemFields(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c, p := setupSaleFixture(t, repo, svc)

	p2 := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Pikachu", CertNumber: "PROV03",
		GradeValue: 10, BuyCostCents: 30000, PurchaseDate: "2026-06-01",
		CLValueCents: 8000,
	}
	if err := svc.CreatePurchase(ctx, p2); err != nil {
		t.Fatalf("setup purchase 2: %v", err)
	}

	result, err := svc.CreateBulkSales(ctx, c.ID, inventory.SaleChannelEbay, "2026-06-20", []inventory.BulkSaleInput{
		{
			PurchaseID:             p.ID,
			SalePriceCents:         20000,
			OriginalListPriceCents: 1500,
			PriceReductions:        2,
			DaysListed:             9,
			SaleReason:             inventory.SaleReasonBulkLot,
		},
		{
			PurchaseID:     p2.ID,
			SalePriceCents: 15000,
			SaleReason:     "bogus",
		},
	})
	if err != nil {
		t.Fatalf("CreateBulkSales: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("created = %d, want 1 (errors: %v)", result.Created, result.Errors)
	}
	if result.Failed != 1 {
		t.Fatalf("failed = %d, want 1", result.Failed)
	}
	if len(result.Errors) != 1 || result.Errors[0].PurchaseID != p2.ID {
		t.Fatalf("errors = %+v, want single error for purchase 2", result.Errors)
	}

	sale := findSaleByPurchaseID(t, repo, c.ID, p.ID)
	salesList, _ := repo.ListSalesByCampaign(ctx, c.ID, 100, 0)
	if sale.OriginalListPriceCents != 1500 {
		t.Errorf("OriginalListPriceCents = %d, want 1500", sale.OriginalListPriceCents)
	}
	if sale.PriceReductions != 2 {
		t.Errorf("PriceReductions = %d, want 2", sale.PriceReductions)
	}
	if sale.DaysListed != 9 {
		t.Errorf("DaysListed = %d, want 9", sale.DaysListed)
	}
	if sale.SaleReason != inventory.SaleReasonBulkLot {
		t.Errorf("SaleReason = %q, want %q", sale.SaleReason, inventory.SaleReasonBulkLot)
	}

	for i := range salesList {
		if salesList[i].PurchaseID == p2.ID {
			t.Fatal("expected no sale to be created for purchase 2 (invalid reason)")
		}
	}
}

func TestCreateBulkSales_PriceSourceDefaults(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c, p := setupSaleFixture(t, repo, svc)

	p2 := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Pikachu", CertNumber: "PROV04",
		GradeValue: 10, BuyCostCents: 30000, PurchaseDate: "2026-06-01",
		CLValueCents: 8000,
	}
	if err := svc.CreatePurchase(ctx, p2); err != nil {
		t.Fatalf("setup purchase 2: %v", err)
	}

	p3 := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Venusaur", CertNumber: "PROV06",
		GradeValue: 9, BuyCostCents: 25000, PurchaseDate: "2026-06-01",
		CLValueCents: 7000,
	}
	if err := svc.CreatePurchase(ctx, p3); err != nil {
		t.Fatalf("setup purchase 3: %v", err)
	}

	result, err := svc.CreateBulkSales(ctx, c.ID, inventory.SaleChannelEbay, "2026-06-20", []inventory.BulkSaleInput{
		{PurchaseID: p.ID, SalePriceCents: 20000},
		{PurchaseID: p2.ID, SalePriceCents: 15000, PriceSource: inventory.PriceSourceItemized},
		{PurchaseID: p3.ID, SalePriceCents: 10000, PriceSource: "bogus"},
	})
	if err != nil {
		t.Fatalf("CreateBulkSales: %v", err)
	}
	if result.Created != 2 {
		t.Fatalf("created = %d, want 2 (errors: %v)", result.Created, result.Errors)
	}
	if result.Failed != 1 {
		t.Fatalf("failed = %d, want 1", result.Failed)
	}

	defaulted := findSaleByPurchaseID(t, repo, c.ID, p.ID)
	if defaulted.PriceSource != inventory.PriceSourceEstimated {
		t.Errorf("PriceSource = %q, want %q", defaulted.PriceSource, inventory.PriceSourceEstimated)
	}

	explicit := findSaleByPurchaseID(t, repo, c.ID, p2.ID)
	if explicit.PriceSource != inventory.PriceSourceItemized {
		t.Errorf("PriceSource = %q, want %q", explicit.PriceSource, inventory.PriceSourceItemized)
	}

	// CreateBulkSales stringifies the per-item error into BulkSaleError.Error
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

func TestFreezeSaleProvenance_CLProvenanceLabel(t *testing.T) {
	tests := []struct {
		name             string
		clValueCents     int
		clValueUpdatedAt string
		wantObservedAt   string
		wantSource       string
	}{
		{
			name:             "cardladder answered: source is cardladder, observed_at is the CL update stamp",
			clValueCents:     12000,
			clValueUpdatedAt: "2026-06-15T10:00:00Z",
			wantObservedAt:   "2026-06-15T10:00:00Z",
			wantSource:       inventory.CLProvenanceSourceCardLadder,
		},
		{
			name:             "cardladder never answered but a positive value carried from intake: source is intake",
			clValueCents:     12000,
			clValueUpdatedAt: "",
			wantObservedAt:   "",
			wantSource:       inventory.CLProvenanceSourceIntake,
		},
		{
			name:             "no CL value at all: both fields stay empty",
			clValueCents:     0,
			clValueUpdatedAt: "",
			wantObservedAt:   "",
			wantSource:       "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			purchase := &inventory.Purchase{
				CLValueCents:     tt.clValueCents,
				CLValueUpdatedAt: tt.clValueUpdatedAt,
			}
			campaign := &inventory.Campaign{EbayFeePct: 0.10}
			sa := &inventory.Sale{SaleChannel: inventory.SaleChannelEbay}

			if err := inventory.FreezeSaleProvenance(sa, purchase, campaign, false); err != nil {
				t.Fatalf("FreezeSaleProvenance: %v", err)
			}

			if sa.CLValueAtSaleObservedAt != tt.wantObservedAt {
				t.Errorf("CLValueAtSaleObservedAt = %q, want %q", sa.CLValueAtSaleObservedAt, tt.wantObservedAt)
			}
			if sa.CLValueAtSaleSource != tt.wantSource {
				t.Errorf("CLValueAtSaleSource = %q, want %q", sa.CLValueAtSaleSource, tt.wantSource)
			}
		})
	}
}
