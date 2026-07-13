package inventory_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// A re-import over an existing invoice with an empty due_date must populate the
// due_date (invoice_date + 7) even when the invoice total is unchanged — this is
// how legacy empty rows self-heal on the next import.
func TestService_ImportPSAExportGlobal_HealsEmptyDueDate(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10", PSASourcingFeeCents: 300}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup campaign: %v", err)
	}
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	// First import creates the invoice (now with a +7 due date after Task 1).
	rows := []inventory.PSAExportRow{
		{CertNumber: "HEAL001", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 200, Date: "2026-07-01", InvoiceDate: "2026-07-01", Category: "Pokemon"},
	}
	if _, err := svc.ImportPSAExportGlobal(ctx, rows); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Simulate a legacy row: blank out the due date, leaving the total intact.
	var invID string
	for id, inv := range repo.Invoices {
		if inv.InvoiceDate == "2026-07-01" {
			// A freshly-created invoice must already carry a non-empty +7 due date.
			if inv.DueDate != "2026-07-08" {
				t.Errorf("created invoice DueDate = %q, want 2026-07-08 (creation must populate due_date)", inv.DueDate)
			}
			inv.DueDate = ""
			invID = id
		}
	}
	if invID == "" {
		t.Fatal("expected an invoice for 2026-07-01 after first import")
	}

	// Re-import the identical row: total is unchanged, but the empty due date must heal.
	result, err := svc.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if result.InvoicesUpdated != 1 {
		t.Errorf("InvoicesUpdated = %d, want 1 (heal writes the invoice)", result.InvoicesUpdated)
	}
	if got := repo.Invoices[invID].DueDate; got != "2026-07-08" {
		t.Errorf("healed DueDate = %q, want 2026-07-08", got)
	}
}

// autoDetectInvoices must NOT heal a pre-cutoff invoice's empty due date. Those
// invoices had era-specific PSA terms (+14, then +1 business day) and are left
// for the reviewed era-aware backfill script rather than stamped with a uniform
// +7 during re-import.
func TestService_ImportPSAExportGlobal_SkipsHealForPreCutoffInvoice(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10", PSASourcingFeeCents: 300}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup campaign: %v", err)
	}
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	// Seed a pre-cutoff invoice with an empty due date, as the legacy rows exist
	// in production. Total matches the imported purchase below (20000 + 300 fee).
	if err := repo.CreateInvoice(ctx, &inventory.Invoice{
		ID: "legacy-pre", InvoiceDate: "2026-03-15", TotalCents: 20300, DueDate: "", Status: "unpaid",
	}); err != nil {
		t.Fatalf("seed legacy invoice: %v", err)
	}

	// Re-import a row for that same pre-cutoff invoice date.
	rows := []inventory.PSAExportRow{
		{CertNumber: "PRE001", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 200, Date: "2026-03-15", InvoiceDate: "2026-03-15", Category: "Pokemon"},
	}
	if _, err := svc.ImportPSAExportGlobal(ctx, rows); err != nil {
		t.Fatalf("import: %v", err)
	}

	// The pre-cutoff invoice's due date must remain empty — left for the backfill.
	if got := repo.Invoices["legacy-pre"].DueDate; got != "" {
		t.Errorf("pre-cutoff DueDate = %q, want %q (must be left for the era-aware backfill)", got, "")
	}
}

// autoDetectInvoices must NOT stamp a due date when CREATING an invoice for a
// pre-cutoff date (e.g. a full-history re-import into a rebuilt DB). Those had
// era-specific PSA terms and are left empty for the era-aware backfill; only the
// create path for cutoff-or-later dates gets the uniform +7 term.
func TestService_ImportPSAExportGlobal_SkipsDueDateForPreCutoffCreate(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10", PSASourcingFeeCents: 300}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup campaign: %v", err)
	}
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	// Import a pre-cutoff invoice date with NO pre-existing invoice → create path.
	rows := []inventory.PSAExportRow{
		{CertNumber: "PRECREATE001", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 200, Date: "2026-03-15", InvoiceDate: "2026-03-15", Category: "Pokemon"},
	}
	if _, err := svc.ImportPSAExportGlobal(ctx, rows); err != nil {
		t.Fatalf("import: %v", err)
	}

	// The newly-created pre-cutoff invoice must have an empty due date.
	var found *inventory.Invoice
	for _, inv := range repo.Invoices {
		if inv.InvoiceDate == "2026-03-15" {
			found = inv
		}
	}
	if found == nil {
		t.Fatal("expected an invoice for 2026-03-15 to be created")
	}
	if found.DueDate != "" {
		t.Errorf("created pre-cutoff DueDate = %q, want %q (leave for era-aware backfill)", found.DueDate, "")
	}
}
