package insights_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/insights"
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
