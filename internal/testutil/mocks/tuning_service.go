package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/tuning"
)

// MockTuningService is a test double for tuning.Service.
// Each method delegates to a function field, allowing per-test configuration.
type MockTuningService struct {
	GetCampaignTuningFn func(ctx context.Context, campaignID string) (*inventory.TuningResponse, error)
}

var _ tuning.Service = (*MockTuningService)(nil)

func (m *MockTuningService) GetCampaignTuning(ctx context.Context, campaignID string) (*inventory.TuningResponse, error) {
	if m.GetCampaignTuningFn != nil {
		return m.GetCampaignTuningFn(ctx, campaignID)
	}
	return nil, nil
}
