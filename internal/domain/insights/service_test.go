package insights_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/insights"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestService_GetOverview_ReturnsEmptyShape_WhenAllDepsEmpty(t *testing.T) {
	t.Parallel()
	svc := insights.NewService(insights.Deps{
		Campaigns: mocks.NewInMemoryCampaignStore(),
		Logger:    mocks.NewMockLogger(),
	})
	got, err := svc.GetOverview(context.Background())
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil overview")
	}
	if len(got.Actions) != 0 {
		t.Errorf("expected empty Actions, got %d", len(got.Actions))
	}
	if len(got.Campaigns) != 0 {
		t.Errorf("expected empty Campaigns, got %d", len(got.Campaigns))
	}
}

func TestService_GetOverview_IncludesOnlyActiveCampaigns(t *testing.T) {
	t.Parallel()
	store := mocks.NewInMemoryCampaignStore()
	ctx := context.Background()
	active := &inventory.Campaign{ID: "c1", Name: "Active one", Phase: inventory.PhaseActive}
	if err := store.CreateCampaign(ctx, active); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateCampaign(ctx, &inventory.Campaign{ID: "c2", Name: "Closed one", Phase: inventory.PhaseClosed}); err != nil {
		t.Fatal(err)
	}

	tuningMock := &mocks.MockTuningService{
		GetCampaignTuningFn: func(ctx context.Context, id string) (*inventory.TuningResponse, error) {
			return &inventory.TuningResponse{
				CampaignID:   id,
				CampaignName: "Active one",
				Recommendations: []inventory.TuningRecommendation{
					{Parameter: "buyTermsCLPct", CurrentVal: "55", SuggestedVal: "60", Confidence: 20, Impact: "Raise 55 → 60%"},
				},
			}, nil
		},
	}

	svc := insights.NewService(insights.Deps{
		Campaigns: store,
		Tuning:    tuningMock,
		Logger:    mocks.NewMockLogger(),
	})
	got, err := svc.GetOverview(ctx)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}
	if len(got.Campaigns) != 1 {
		t.Fatalf("expected 1 campaign row, got %d", len(got.Campaigns))
	}
	row := got.Campaigns[0]
	if row.CampaignID != active.ID {
		t.Errorf("expected row for active campaign, got %q", row.CampaignID)
	}
	if row.Cells["buyPct"].Recommendation == "" {
		t.Errorf("expected buyPct cell to be populated, got empty")
	}
	if row.Status != insights.StatusAct {
		t.Errorf("expected Status Act (confidence=20), got %q", row.Status)
	}
}

func TestService_GetOverview_AIAcceptRate(t *testing.T) {
	t.Parallel()
	pricingMock := &mocks.MockPricingService{
		GetPriceOverrideStatsFn: func(ctx context.Context) (*inventory.PriceOverrideStats, error) {
			return &inventory.PriceOverrideStats{
				AIAcceptedCount:    2,
				PendingSuggestions: 3,
			}, nil
		},
	}
	svc := insights.NewService(insights.Deps{
		Campaigns: mocks.NewInMemoryCampaignStore(),
		Pricing:   pricingMock,
		Logger:    mocks.NewMockLogger(),
	})
	got, err := svc.GetOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// With 2 accepted and 0 known-dismissed, resolved == accepted == 2, pct == 100%.
	if got.Signals.AIAcceptRate.Accepted != 2 {
		t.Errorf("Accepted = %d, want 2", got.Signals.AIAcceptRate.Accepted)
	}
	if got.Signals.AIAcceptRate.Resolved != 2 {
		t.Errorf("Resolved = %d, want 2 (accepted + dismissed when dismissed unknown)", got.Signals.AIAcceptRate.Resolved)
	}
	if got.Signals.AIAcceptRate.Pct != 100.0 {
		t.Errorf("Pct = %v, want 100.0", got.Signals.AIAcceptRate.Pct)
	}
}
