package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// CampaignRepositoryMock implements inventory.CampaignRepository with Fn-field pattern.
type CampaignRepositoryMock struct {
	CreateCampaignFn func(ctx context.Context, c *inventory.Campaign) error
	GetCampaignFn    func(ctx context.Context, id string) (*inventory.Campaign, error)
	ListCampaignsFn  func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error)
	UpdateCampaignFn func(ctx context.Context, c *inventory.Campaign) error
	DeleteCampaignFn func(ctx context.Context, id string) error
}

var _ inventory.CampaignRepository = (*CampaignRepositoryMock)(nil)

func (m *CampaignRepositoryMock) CreateCampaign(ctx context.Context, c *inventory.Campaign) error {
	if m.CreateCampaignFn != nil {
		return m.CreateCampaignFn(ctx, c)
	}
	return nil
}

func (m *CampaignRepositoryMock) GetCampaign(ctx context.Context, id string) (*inventory.Campaign, error) {
	if m.GetCampaignFn != nil {
		return m.GetCampaignFn(ctx, id)
	}
	return nil, inventory.ErrCampaignNotFound
}

func (m *CampaignRepositoryMock) ListCampaigns(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error) {
	if m.ListCampaignsFn != nil {
		return m.ListCampaignsFn(ctx, activeOnly)
	}
	return []inventory.Campaign{}, nil
}

func (m *CampaignRepositoryMock) UpdateCampaign(ctx context.Context, c *inventory.Campaign) error {
	if m.UpdateCampaignFn != nil {
		return m.UpdateCampaignFn(ctx, c)
	}
	return nil
}

func (m *CampaignRepositoryMock) DeleteCampaign(ctx context.Context, id string) error {
	if m.DeleteCampaignFn != nil {
		return m.DeleteCampaignFn(ctx, id)
	}
	return nil
}
