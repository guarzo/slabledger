package inventory_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// TestService_ImportPSAExportGlobal_PurchaseSourceGuard pins the guard added to
// handleExistingPSAPurchase: a PSA export that omits the "purchase source"
// column must not clear a previously stored value. purchase_source is what the
// CL coverage endpoint uses to classify intake cohort, so a silent wipe
// reassigns rows from the campaign cohort to external.
func TestService_ImportPSAExportGlobal_PurchaseSourceGuard(t *testing.T) {
	tests := []struct {
		name           string
		existingSource string
		incomingSource string
		wantSource     string
		wantUpdated    int
	}{
		{
			name:           "empty incoming preserves existing",
			existingSource: "eBay",
			incomingSource: "",
			wantSource:     "eBay",
			wantUpdated:    0, // nothing else differs, so psaChanged stays false
		},
		{
			name:           "non-empty incoming overwrites existing",
			existingSource: "eBay",
			incomingSource: "PSA",
			wantSource:     "PSA",
			wantUpdated:    1,
		},
		{
			name:           "non-empty incoming fills an empty existing",
			existingSource: "",
			incomingSource: "PSA",
			wantSource:     "PSA",
			wantUpdated:    1,
		},
		{
			name:           "empty incoming leaves an empty existing empty",
			existingSource: "",
			incomingSource: "",
			wantSource:     "",
			wantUpdated:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewInMemoryCampaignStore()
			svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
			ctx := context.Background()

			c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
			if err := svc.CreateCampaign(ctx, c); err != nil {
				t.Fatalf("setup CreateCampaign: %v", err)
			}

			// Every PSA-updatable field is pre-seeded to match the incoming row
			// so PurchaseSource is the only possible difference. That is what
			// makes wantUpdated a clean read on the guard: 0 means the guard
			// held and psaChanged saw no delta, 1 means the overwrite landed.
			p := &inventory.Purchase{
				CampaignID: c.ID, CardName: "Charizard", CertNumber: "PSA100",
				GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-01-15",
				SetName: "Base Set", CardNumber: "4",
				PSAShipDate: "2026-02-01", InvoiceDate: "2026-02-01",
				PSAListingTitle: "Charizard", PurchaseSource: tt.existingSource,
			}
			if err := svc.CreatePurchase(ctx, p); err != nil {
				t.Fatalf("setup CreatePurchase: %v", err)
			}

			rows := []inventory.PSAExportRow{{
				CertNumber: "PSA100", ListingTitle: "Charizard", Grade: 9,
				PricePaid: 500, ShipDate: "2026-02-01", InvoiceDate: "2026-02-01",
				PurchaseSource: tt.incomingSource,
			}}

			result, err := svc.ImportPSAExportGlobal(ctx, rows)
			if err != nil {
				t.Fatalf("ImportPSAExportGlobal: %v", err)
			}
			if result.Updated != tt.wantUpdated {
				t.Errorf("Updated = %d, want %d", result.Updated, tt.wantUpdated)
			}

			got, err := repo.GetPurchaseByCertNumber(ctx, "PSA", "PSA100")
			if err != nil {
				t.Fatalf("GetPurchaseByCertNumber: %v", err)
			}
			if got.PurchaseSource != tt.wantSource {
				t.Errorf("PurchaseSource = %q, want %q", got.PurchaseSource, tt.wantSource)
			}
		})
	}
}
