package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/portfolio"
)

// MockPortfolioService is a test double for portfolio.Service.
// Each method delegates to a function field, allowing per-test configuration.
type MockPortfolioService struct {
	GetPortfolioHealthFn          func(ctx context.Context) (*inventory.PortfolioHealth, error)
	GetPortfolioChannelVelocityFn func(ctx context.Context) ([]inventory.ChannelVelocity, error)
	GetPortfolioInsightsFn        func(ctx context.Context) (*inventory.PortfolioInsights, error)
	GetCampaignSuggestionsFn      func(ctx context.Context) (*inventory.SuggestionsResponse, error)
	GetCapitalTimelineFn          func(ctx context.Context) (*inventory.CapitalTimeline, error)
	GetWeeklyReviewSummaryFn      func(ctx context.Context) (*inventory.WeeklyReviewSummary, error)
	GetWeeklyHistoryFn            func(ctx context.Context, weeks int) ([]inventory.WeeklyReviewSummary, error)
	GetSnapshotFn                 func(ctx context.Context) (*portfolio.PortfolioSnapshot, error)
}

var _ portfolio.Service = (*MockPortfolioService)(nil)

func (m *MockPortfolioService) GetPortfolioHealth(ctx context.Context) (*inventory.PortfolioHealth, error) {
	if m.GetPortfolioHealthFn != nil {
		return m.GetPortfolioHealthFn(ctx)
	}
	return nil, nil
}

func (m *MockPortfolioService) GetPortfolioChannelVelocity(ctx context.Context) ([]inventory.ChannelVelocity, error) {
	if m.GetPortfolioChannelVelocityFn != nil {
		return m.GetPortfolioChannelVelocityFn(ctx)
	}
	return nil, nil
}

func (m *MockPortfolioService) GetPortfolioInsights(ctx context.Context) (*inventory.PortfolioInsights, error) {
	if m.GetPortfolioInsightsFn != nil {
		return m.GetPortfolioInsightsFn(ctx)
	}
	return nil, nil
}

func (m *MockPortfolioService) GetCampaignSuggestions(ctx context.Context) (*inventory.SuggestionsResponse, error) {
	if m.GetCampaignSuggestionsFn != nil {
		return m.GetCampaignSuggestionsFn(ctx)
	}
	return nil, nil
}

func (m *MockPortfolioService) GetCapitalTimeline(ctx context.Context) (*inventory.CapitalTimeline, error) {
	if m.GetCapitalTimelineFn != nil {
		return m.GetCapitalTimelineFn(ctx)
	}
	return nil, nil
}

func (m *MockPortfolioService) GetWeeklyReviewSummary(ctx context.Context) (*inventory.WeeklyReviewSummary, error) {
	if m.GetWeeklyReviewSummaryFn != nil {
		return m.GetWeeklyReviewSummaryFn(ctx)
	}
	return nil, nil
}

func (m *MockPortfolioService) GetWeeklyHistory(ctx context.Context, weeks int) ([]inventory.WeeklyReviewSummary, error) {
	if m.GetWeeklyHistoryFn != nil {
		return m.GetWeeklyHistoryFn(ctx, weeks)
	}
	return nil, nil
}

func (m *MockPortfolioService) GetSnapshot(ctx context.Context) (*portfolio.PortfolioSnapshot, error) {
	if m.GetSnapshotFn != nil {
		return m.GetSnapshotFn(ctx)
	}
	return nil, nil
}
