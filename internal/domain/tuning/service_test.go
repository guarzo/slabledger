package tuning_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/tuning"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestService_GetCampaignTuning(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	logger := mocks.NewMockLogger()
	svc := tuning.NewService(repo, repo, logger)
	ctx := context.Background()

	// Set up a campaign directly in the mock
	c := &inventory.Campaign{
		ID:            "campaign-1",
		Name:          "Tuning Test",
		BuyTermsCLPct: 0.85,
		GradeRange:    "9-10",
		EbayFeePct:    0.1235,
	}
	repo.Campaigns[c.ID] = c

	// Add PNL data so analytics calls return something
	repo.PNLData[c.ID] = &inventory.CampaignPNL{
		CampaignID:     c.ID,
		TotalPurchases: 3,
	}

	tuningResult, err := svc.GetCampaignTuning(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetCampaignTuning: %v", err)
	}
	if tuningResult.CampaignID != c.ID {
		t.Errorf("CampaignID = %s, want %s", tuningResult.CampaignID, c.ID)
	}
	if tuningResult.CampaignName != "Tuning Test" {
		t.Errorf("CampaignName = %s, want Tuning Test", tuningResult.CampaignName)
	}
}
