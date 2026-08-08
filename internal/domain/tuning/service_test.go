package tuning_test

import (
	"context"
	"strings"
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

func TestService_GetCampaignTuning_SpendCapUtilization(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	logger := mocks.NewMockLogger()
	svc := tuning.NewService(repo, repo, logger)
	ctx := context.Background()

	c := &inventory.Campaign{
		ID:                 "campaign-1",
		Name:               "Spend Cap Test",
		BuyTermsCLPct:      0.85,
		GradeRange:         "9-10",
		EbayFeePct:         0.1235,
		DailySpendCapCents: 1000,
	}
	repo.Campaigns[c.ID] = c
	repo.PNLData[c.ID] = &inventory.CampaignPNL{CampaignID: c.ID, TotalPurchases: 3}

	// 14 unenriched rows, exactly as the postgres store returns them. Spending
	// 100 against a 1000 cap is 10% utilization, below the 20% low-use threshold.
	repo.GetDailySpendFn = func(_ context.Context, _ string, _ int) ([]inventory.DailySpend, error) {
		rows := make([]inventory.DailySpend, 14)
		for i := range rows {
			rows[i] = inventory.DailySpend{Date: "2025-01-01", SpendCents: 100, PurchaseCount: 1}
		}
		return rows, nil
	}

	result, err := svc.GetCampaignTuning(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetCampaignTuning: %v", err)
	}

	var found *tuning.TuningRecommendation
	for i := range result.Recommendations {
		if result.Recommendations[i].Parameter == "dailySpendCap" {
			found = &result.Recommendations[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected a dailySpendCap recommendation, got none")
	}
	if !strings.Contains(found.Reasoning, "only 10% utilized") {
		t.Errorf("Reasoning = %q, want it to report 10%% utilization", found.Reasoning)
	}
}
