package csvimport_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/csvimport"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// A re-import must move boundary-day purchases to the next cycle, recalculate
// both affected invoices without sourcing fees, and leave earlier cycles alone.
func TestImportPSAExportGlobal_ReconcilesInvoiceCutoverOnly(t *testing.T) {
	ctx := context.Background()
	repo := mocks.NewInMemoryCampaignStore()
	svc, imp := newServices(repo, nil)
	campaign := &inventory.Campaign{Name: "Test", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10", PSASourcingFeeCents: 300}
	if err := svc.CreateCampaign(ctx, campaign); err != nil {
		t.Fatal(err)
	}
	campaign.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, campaign); err != nil {
		t.Fatal(err)
	}

	rows := []csvimport.PSAExportRow{
		{CertNumber: "OLD", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 120, Date: "2026-09-10", InvoiceDate: "2026-09-15", Category: "Pokemon"},
		{CertNumber: "SEP", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 100, Date: "2026-09-23", InvoiceDate: "2026-10-01", Category: "Pokemon"},
		{CertNumber: "OCT1", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 200, Date: "2026-10-01", InvoiceDate: "2026-10-01", Category: "Pokemon"},
		{CertNumber: "OCT2", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 150, Date: "2026-10-02", InvoiceDate: "2026-10-15", Category: "Pokemon"},
	}
	if _, err := imp.ImportPSAExportGlobal(ctx, rows); err != nil {
		t.Fatal(err)
	}
	// Model the persisted invoices before the cutover: totals included $3/card.
	for _, inv := range repo.Invoices {
		switch inv.InvoiceDate {
		case "2026-09-15":
			inv.TotalCents = 9999 // historical/manual total must stay untouched
		case "2026-10-01":
			inv.TotalCents = 30600
		case "2026-10-15":
			inv.TotalCents = 15300
		}
	}
	rows[2].InvoiceDate = "2026-10-15"
	result, err := imp.ImportPSAExportGlobal(ctx, rows[2:])
	if err != nil {
		t.Fatal(err)
	}
	if result.InvoicesUpdated != 2 {
		t.Errorf("updated invoices = %d, want 2", result.InvoicesUpdated)
	}
	if _, err := imp.ImportPSAExportGlobal(ctx, rows[:2]); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"2026-09-15": 9999, "2026-10-01": 10000, "2026-10-15": 35000}
	for _, inv := range repo.Invoices {
		if inv.TotalCents != want[inv.InvoiceDate] {
			t.Errorf("invoice %s = %d, want %d", inv.InvoiceDate, inv.TotalCents, want[inv.InvoiceDate])
		}
	}
	for _, p := range repo.Purchases {
		if p.CertNumber == "OCT1" && p.InvoiceDate != "2026-10-15" {
			t.Errorf("October 1 purchase assigned to %s", p.InvoiceDate)
		}
	}
}
