package inventory

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
)

func internalTestIDGen() func() string {
	var counter atomic.Int64
	return func() string { return fmt.Sprintf("test-id-%d", counter.Add(1)) }
}

// --- Global Operations Tests ---

func TestService_RefreshCLValuesGlobal(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	// Create two campaigns
	c1 := &Campaign{Name: "Campaign A", BuyTermsCLPct: 0.78, PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, c1)
	c2 := &Campaign{Name: "Campaign B", BuyTermsCLPct: 0.80, PSASourcingFeeCents: 500}
	_ = svc.CreateCampaign(ctx, c2)

	// Add purchases to each campaign
	p1 := &Purchase{CampaignID: c1.ID, CardName: "Charizard", CertNumber: "CERT001", GradeValue: 9, BuyCostCents: 10000, CLValueCents: 20000, Population: 100, PurchaseDate: "2026-01-01"}
	_ = svc.CreatePurchase(ctx, p1)
	p2 := &Purchase{CampaignID: c2.ID, CardName: "Pikachu", CertNumber: "CERT002", GradeValue: 10, BuyCostCents: 5000, CLValueCents: 10000, Population: 200, PurchaseDate: "2026-01-02"}
	_ = svc.CreatePurchase(ctx, p2)

	// Global refresh with a CSV that touches both campaigns and has one unknown cert
	rows := []CLExportRow{
		{SlabSerial: "CERT001", CurrentValue: 250.00, Population: 150},
		{SlabSerial: "CERT002", CurrentValue: 120.00, Population: 0}, // 0 population should keep old
		{SlabSerial: "CERT999", CurrentValue: 100.00},                // not found
	}

	result, err := svc.RefreshCLValuesGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("RefreshCLValuesGlobal: %v", err)
	}
	if result.Updated != 2 {
		t.Errorf("Updated = %d, want 2", result.Updated)
	}
	if result.NotFound != 1 {
		t.Errorf("NotFound = %d, want 1", result.NotFound)
	}

	// Verify per-campaign breakdown
	if len(result.ByCampaign) != 2 {
		t.Errorf("ByCampaign count = %d, want 2", len(result.ByCampaign))
	}
	if summary, ok := result.ByCampaign[c1.ID]; !ok || summary.Updated != 1 {
		t.Errorf("Campaign A summary: %+v", summary)
	}

	// Verify actual values updated
	updated1, _ := repo.GetPurchaseByCertNumber(ctx, "PSA", "CERT001")
	if updated1.CLValueCents != 25000 {
		t.Errorf("CERT001 CLValueCents = %d, want 25000", updated1.CLValueCents)
	}
	if updated1.Population != 150 {
		t.Errorf("CERT001 Population = %d, want 150", updated1.Population)
	}

	updated2, _ := repo.GetPurchaseByCertNumber(ctx, "PSA", "CERT002")
	if updated2.CLValueCents != 12000 {
		t.Errorf("CERT002 CLValueCents = %d, want 12000", updated2.CLValueCents)
	}
	if updated2.Population != 200 { // should keep original since CSV had 0
		t.Errorf("CERT002 Population = %d, want 200 (kept original)", updated2.Population)
	}
}

func TestService_ImportCLExportGlobal_AutoAllocate(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	// Campaign A: high grade, high price
	cA := &Campaign{Name: "High Grade", Phase: PhaseActive, GradeRange: "9-10", PriceRange: "100-500", PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, cA)

	// Campaign B: low grade, low price
	cB := &Campaign{Name: "Low Grade", Phase: PhaseActive, GradeRange: "7-8", PriceRange: "10-100", PSASourcingFeeCents: 500}
	_ = svc.CreateCampaign(ctx, cB)

	rows := []CLExportRow{
		// Should match Campaign A (grade 9, $150)
		{SlabSerial: "NEW001", Card: "Charizard PSA 9", Investment: 150, CurrentValue: 300, DatePurchased: "2026-01-01"},
		// Should match Campaign B (grade 8, $50)
		{SlabSerial: "NEW002", Card: "Pikachu PSA 8", Investment: 50, CurrentValue: 80, DatePurchased: "2026-01-02"},
		// Grade 5 matches neither
		{SlabSerial: "NEW003", Card: "Blastoise PSA 5", Condition: "PSA 5", Investment: 30, CurrentValue: 40, DatePurchased: "2026-01-03"},
	}

	result, err := svc.ImportCLExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportCLExportGlobal: %v", err)
	}
	if result.Allocated != 2 {
		t.Errorf("Allocated = %d, want 2", result.Allocated)
	}
	if result.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", result.Unmatched)
	}

	// Verify allocations
	for _, item := range result.Results {
		switch item.CertNumber {
		case "NEW001":
			if item.Status != "allocated" || item.CampaignID != cA.ID {
				t.Errorf("NEW001: status=%s, campaignID=%s (want allocated to %s)", item.Status, item.CampaignID, cA.ID)
			}
		case "NEW002":
			if item.Status != "allocated" || item.CampaignID != cB.ID {
				t.Errorf("NEW002: status=%s, campaignID=%s (want allocated to %s)", item.Status, item.CampaignID, cB.ID)
			}
		case "NEW003":
			if item.Status != "unmatched" {
				t.Errorf("NEW003: status=%s, want unmatched", item.Status)
			}
		}
	}

	// Verify sourcing fees stamped correctly
	p1, _ := repo.GetPurchaseByCertNumber(ctx, "PSA", "NEW001")
	if p1.PSASourcingFeeCents != 300 {
		t.Errorf("NEW001 PSASourcingFeeCents = %d, want 300", p1.PSASourcingFeeCents)
	}
	p2, _ := repo.GetPurchaseByCertNumber(ctx, "PSA", "NEW002")
	if p2.PSASourcingFeeCents != 500 {
		t.Errorf("NEW002 PSASourcingFeeCents = %d, want 500", p2.PSASourcingFeeCents)
	}
}

func TestService_ImportCLExportGlobal_RefreshExisting(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	c := &Campaign{Name: "Test", Phase: PhaseActive, GradeRange: "9-10", PriceRange: "50-500", PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, c)

	// Pre-existing purchase
	p := &Purchase{CampaignID: c.ID, CardName: "Charizard", CertNumber: "EXIST001", GradeValue: 9, BuyCostCents: 15000, CLValueCents: 20000, Population: 100, PurchaseDate: "2026-01-01"}
	_ = svc.CreatePurchase(ctx, p)

	rows := []CLExportRow{
		// Existing cert → should refresh
		{SlabSerial: "EXIST001", Card: "Charizard PSA 9", Investment: 150, CurrentValue: 350, Population: 200},
		// New cert → should allocate
		{SlabSerial: "NEW001", Card: "Pikachu PSA 10", Investment: 200, CurrentValue: 400, DatePurchased: "2026-02-01"},
	}

	result, err := svc.ImportCLExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportCLExportGlobal: %v", err)
	}
	if result.Refreshed != 1 {
		t.Errorf("Refreshed = %d, want 1", result.Refreshed)
	}
	if result.Allocated != 1 {
		t.Errorf("Allocated = %d, want 1", result.Allocated)
	}

	// Verify refresh updated CL value
	refreshed, _ := repo.GetPurchaseByCertNumber(ctx, "PSA", "EXIST001")
	if refreshed.CLValueCents != 35000 {
		t.Errorf("EXIST001 CLValueCents = %d, want 35000", refreshed.CLValueCents)
	}
}

func TestService_ImportCLExportGlobal_Ambiguous(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	// Two campaigns with overlapping criteria
	c1 := &Campaign{Name: "Broad A", Phase: PhaseActive, GradeRange: "9-10", PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, c1)
	c2 := &Campaign{Name: "Broad B", Phase: PhaseActive, GradeRange: "9-10", PSASourcingFeeCents: 500}
	_ = svc.CreateCampaign(ctx, c2)

	rows := []CLExportRow{
		{SlabSerial: "AMB001", Card: "Charizard PSA 9", Investment: 150, CurrentValue: 300, DatePurchased: "2026-01-01"},
	}

	result, err := svc.ImportCLExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportCLExportGlobal: %v", err)
	}
	if result.Ambiguous != 1 {
		t.Errorf("Ambiguous = %d, want 1", result.Ambiguous)
	}
	if result.Results[0].Status != "ambiguous" {
		t.Errorf("status = %s, want ambiguous", result.Results[0].Status)
	}
	if len(result.Results[0].Candidates) != 2 {
		t.Errorf("candidates = %d, want 2", len(result.Results[0].Candidates))
	}
}

func TestService_ImportCLExportGlobal_SkipsClosedCampaigns(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	// Active campaign
	cActive := &Campaign{Name: "Active", Phase: PhaseActive, GradeRange: "9-10", PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, cActive)

	// Closed campaign with same criteria — should NOT receive allocations
	cClosed := &Campaign{Name: "Closed", Phase: PhaseClosed, GradeRange: "9-10", PSASourcingFeeCents: 500}
	_ = svc.CreateCampaign(ctx, cClosed)

	rows := []CLExportRow{
		{SlabSerial: "SKIP001", Card: "Charizard PSA 9", Investment: 150, CurrentValue: 300, DatePurchased: "2026-01-01"},
	}

	result, err := svc.ImportCLExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportCLExportGlobal: %v", err)
	}
	if result.Allocated != 1 {
		t.Errorf("Allocated = %d, want 1", result.Allocated)
	}
	if result.Ambiguous != 0 {
		t.Errorf("Ambiguous = %d, want 0 (closed campaign should not cause ambiguity)", result.Ambiguous)
	}
	if result.Results[0].CampaignID != cActive.ID {
		t.Errorf("allocated to %s, want %s (active campaign)", result.Results[0].CampaignID, cActive.ID)
	}
}

func TestService_ExportCLFormatGlobal(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	// Create two campaigns with purchases
	c1 := &Campaign{Name: "Campaign A", BuyTermsCLPct: 0.78, PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, c1)
	c2 := &Campaign{Name: "Campaign B", BuyTermsCLPct: 0.80, PSASourcingFeeCents: 500}
	_ = svc.CreateCampaign(ctx, c2)

	p1 := &Purchase{CampaignID: c1.ID, CardName: "Charizard", CertNumber: "CERT001", GradeValue: 9, BuyCostCents: 15000, CLValueCents: 20000, PurchaseDate: "2026-03-09"}
	_ = svc.CreatePurchase(ctx, p1)
	p2 := &Purchase{CampaignID: c2.ID, CardName: "Pikachu", CertNumber: "CERT002", GradeValue: 10, BuyCostCents: 5000, CLValueCents: 10000, PurchaseDate: "2026-01-15"}
	_ = svc.CreatePurchase(ctx, p2)

	// Mark p2 as sold so it doesn't appear in export
	s := &Sale{PurchaseID: p2.ID, SaleChannel: SaleChannelEbay, SalePriceCents: 12000, SaleDate: "2026-02-01"}
	_ = svc.CreateSale(ctx, s, c2, p2)

	entries, err := svc.ExportCLFormatGlobal(ctx, false)
	if err != nil {
		t.Fatalf("ExportCLFormatGlobal: %v", err)
	}

	// Only unsold purchase should be returned
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.CertNumber != "CERT001" {
		t.Errorf("CertNumber = %q, want CERT001", e.CertNumber)
	}
	if e.Grader != "PSA" {
		t.Errorf("Grader = %q, want PSA", e.Grader)
	}
	if e.Investment != 150.00 {
		t.Errorf("Investment = %f, want 150.00", e.Investment)
	}
	if e.EstimatedValue != 200.00 {
		t.Errorf("EstimatedValue = %f, want 200.00", e.EstimatedValue)
	}
	// Date should be converted from YYYY-MM-DD to M/D/YYYY
	if e.DatePurchased != "3/9/2026" {
		t.Errorf("DatePurchased = %q, want 3/9/2026", e.DatePurchased)
	}
}

func TestService_ExportCLFormatGlobal_Empty(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	entries, err := svc.ExportCLFormatGlobal(ctx, false)
	if err != nil {
		t.Fatalf("ExportCLFormatGlobal: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestService_ExportCLFormatGlobal_MissingCLOnly(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	c := &Campaign{Name: "Campaign A", BuyTermsCLPct: 0.78, PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, c)

	// p1 has CL data, p2 does not
	p1 := &Purchase{CampaignID: c.ID, CardName: "Charizard", CertNumber: "CERT001", GradeValue: 9, BuyCostCents: 15000, CLValueCents: 20000, PurchaseDate: "2026-03-09"}
	_ = svc.CreatePurchase(ctx, p1)
	p2 := &Purchase{CampaignID: c.ID, CardName: "Pikachu", CertNumber: "CERT002", GradeValue: 10, BuyCostCents: 5000, CLValueCents: 0, PurchaseDate: "2026-01-15"}
	_ = svc.CreatePurchase(ctx, p2)

	// Without filter: both returned
	all, err := svc.ExportCLFormatGlobal(ctx, false)
	if err != nil {
		t.Fatalf("ExportCLFormatGlobal(false): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}

	// With filter: only p2 (missing CL data)
	missing, err := svc.ExportCLFormatGlobal(ctx, true)
	if err != nil {
		t.Fatalf("ExportCLFormatGlobal(true): %v", err)
	}
	if len(missing) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(missing))
	}
	if missing[0].CertNumber != "CERT002" {
		t.Errorf("CertNumber = %q, want CERT002", missing[0].CertNumber)
	}
}

func TestService_ReassignPurchase(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	c1 := &Campaign{Name: "Source", PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, c1)
	c2 := &Campaign{Name: "Target", PSASourcingFeeCents: 500}
	_ = svc.CreateCampaign(ctx, c2)

	p := &Purchase{CampaignID: c1.ID, CardName: "Charizard", CertNumber: "MOVE001", GradeValue: 9, BuyCostCents: 15000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-01"}
	_ = svc.CreatePurchase(ctx, p)

	if err := svc.ReassignPurchase(ctx, p.ID, c2.ID); err != nil {
		t.Fatalf("ReassignPurchase: %v", err)
	}

	// Verify purchase moved
	moved, _ := repo.GetPurchase(ctx, p.ID)
	if moved.CampaignID != c2.ID {
		t.Errorf("CampaignID = %s, want %s", moved.CampaignID, c2.ID)
	}
	if moved.PSASourcingFeeCents != 500 {
		t.Errorf("PSASourcingFeeCents = %d, want 500", moved.PSASourcingFeeCents)
	}
}

func TestService_ReassignPurchase_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	c := &Campaign{Name: "Target"}
	_ = svc.CreateCampaign(ctx, c)

	err := svc.ReassignPurchase(ctx, "nonexistent", c.ID)
	if err == nil {
		t.Error("expected error for nonexistent purchase")
	}
}
