package inventory_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestService_ImportPSAExportGlobal_MatchesRealCampaignRegardlessOfPhase(t *testing.T) {
	tests := []struct {
		name            string
		phase           inventory.Phase
		row             inventory.PSAExportRow
		inclusionList   string
		includeExternal bool
		wantCostCents   int
	}{
		{
			name:  "active campaign matches reported Umbreon purchase",
			phase: inventory.PhaseActive,
			row: inventory.PSAExportRow{
				CertNumber: "163772677", ListingTitle: "UMBREON PSA 9", Grade: 9,
				PricePaid: 789.96, Date: "2026-07-15", Category: "Pokemon",
			},
			inclusionList: "Umbreon",
			wantCostCents: 78996,
		},
		{
			name:  "pending campaign matches reported Mega Charizard purchase",
			phase: inventory.PhasePending,
			row: inventory.PSAExportRow{
				CertNumber: "160987870", ListingTitle: "MEGA CHARIZARD X EX PSA 8", Grade: 8,
				PricePaid: 485.64, Date: "2026-07-15", Category: "Pokemon",
			},
			inclusionList: "Mega Charizard",
			wantCostCents: 48564,
		},
		{
			name:  "External wildcard cannot make pending real match ambiguous",
			phase: inventory.PhasePending,
			row: inventory.PSAExportRow{
				CertNumber: "PSA-EXTERNAL-GUARD", ListingTitle: "PIKACHU PSA 9", Grade: 9,
				PricePaid: 100, Date: "2026-07-15", Category: "Pokemon",
			},
			inclusionList:   "Pikachu",
			includeExternal: true,
			wantCostCents:   10000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewInMemoryCampaignStore()
			svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
			ctx := context.Background()

			campaign := &inventory.Campaign{
				Name: "PSA campaign", Sport: "Pokemon", Phase: tt.phase,
				GradeRange: "8-10", InclusionList: tt.inclusionList,
			}
			if err := svc.CreateCampaign(ctx, campaign); err != nil {
				t.Fatalf("CreateCampaign: %v", err)
			}
			if tt.includeExternal {
				if _, err := svc.EnsureExternalCampaign(ctx); err != nil {
					t.Fatalf("EnsureExternalCampaign: %v", err)
				}
			}

			result, err := svc.ImportPSAExportGlobal(ctx, []inventory.PSAExportRow{tt.row})
			if err != nil {
				t.Fatalf("ImportPSAExportGlobal: %v", err)
			}
			if result.Allocated != 1 || result.Unmatched != 0 || result.Ambiguous != 0 {
				t.Fatalf("result counters = allocated %d, unmatched %d, ambiguous %d; want 1, 0, 0",
					result.Allocated, result.Unmatched, result.Ambiguous)
			}
			if got := result.Results[0].CampaignID; got != campaign.ID {
				t.Errorf("CampaignID = %q, want %q", got, campaign.ID)
			}

			purchase, err := repo.GetPurchaseByCertNumber(ctx, "PSA", tt.row.CertNumber)
			if err != nil {
				t.Fatalf("GetPurchaseByCertNumber: %v", err)
			}
			if purchase == nil {
				t.Fatal("allocated purchase not found")
			}
			if purchase.BuyCostCents != tt.wantCostCents {
				t.Errorf("BuyCostCents = %d, want %d", purchase.BuyCostCents, tt.wantCostCents)
			}
		})
	}
}
