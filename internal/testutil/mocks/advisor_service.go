package mocks

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/advisor"
)

// MockAdvisorService is a test mock for advisor.Service.
type MockAdvisorService struct {
	GenerateDigestFn     func(ctx context.Context, stream func(advisor.StreamEvent)) error
	AnalyzeCampaignFn    func(ctx context.Context, campaignID string, stream func(advisor.StreamEvent)) error
	AnalyzeLiquidationFn func(ctx context.Context, stream func(advisor.StreamEvent)) error
	AssessPurchaseFn     func(ctx context.Context, req advisor.PurchaseAssessmentRequest, stream func(advisor.StreamEvent)) error
	CollectDigestFn      func(ctx context.Context) (string, error)
	CollectLiquidationFn func(ctx context.Context) (string, error)
}

var _ advisor.Service = (*MockAdvisorService)(nil)

func (m *MockAdvisorService) GenerateDigest(ctx context.Context, stream func(advisor.StreamEvent)) error {
	if m.GenerateDigestFn != nil {
		return m.GenerateDigestFn(ctx, stream)
	}
	return nil
}

func (m *MockAdvisorService) AnalyzeCampaign(ctx context.Context, campaignID string, stream func(advisor.StreamEvent)) error {
	if m.AnalyzeCampaignFn != nil {
		return m.AnalyzeCampaignFn(ctx, campaignID, stream)
	}
	return nil
}

func (m *MockAdvisorService) AnalyzeLiquidation(ctx context.Context, stream func(advisor.StreamEvent)) error {
	if m.AnalyzeLiquidationFn != nil {
		return m.AnalyzeLiquidationFn(ctx, stream)
	}
	return nil
}

func (m *MockAdvisorService) AssessPurchase(ctx context.Context, req advisor.PurchaseAssessmentRequest, stream func(advisor.StreamEvent)) error {
	if m.AssessPurchaseFn != nil {
		return m.AssessPurchaseFn(ctx, req, stream)
	}
	return nil
}

func (m *MockAdvisorService) CollectDigest(ctx context.Context) (string, error) {
	if m.CollectDigestFn != nil {
		return m.CollectDigestFn(ctx)
	}
	return "", nil
}

func (m *MockAdvisorService) CollectLiquidation(ctx context.Context) (string, error) {
	if m.CollectLiquidationFn != nil {
		return m.CollectLiquidationFn(ctx)
	}
	return "", nil
}

// MockCacheStore is a test mock for advisor.CacheStore.
type MockCacheStore struct {
	GetFn               func(ctx context.Context, analysisType advisor.AnalysisType) (*advisor.CachedAnalysis, error)
	MarkRunningFn       func(ctx context.Context, analysisType advisor.AnalysisType) (string, error)
	AcquireRefreshFn    func(ctx context.Context, analysisType advisor.AnalysisType) (string, bool, error)
	ForceAcquireStaleFn func(ctx context.Context, analysisType advisor.AnalysisType, staleThreshold time.Duration) (string, bool, error)
	SaveResultFn        func(ctx context.Context, analysisType advisor.AnalysisType, lease, content, errMsg string) error
}

var _ advisor.CacheStore = (*MockCacheStore)(nil)

func (m *MockCacheStore) Get(ctx context.Context, analysisType advisor.AnalysisType) (*advisor.CachedAnalysis, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, analysisType)
	}
	return nil, nil
}

func (m *MockCacheStore) MarkRunning(ctx context.Context, analysisType advisor.AnalysisType) (string, error) {
	if m.MarkRunningFn != nil {
		return m.MarkRunningFn(ctx, analysisType)
	}
	return "lease-1", nil
}

func (m *MockCacheStore) AcquireRefresh(ctx context.Context, analysisType advisor.AnalysisType) (string, bool, error) {
	if m.AcquireRefreshFn != nil {
		return m.AcquireRefreshFn(ctx, analysisType)
	}
	return "lease-1", true, nil
}

func (m *MockCacheStore) ForceAcquireStale(ctx context.Context, analysisType advisor.AnalysisType, staleThreshold time.Duration) (string, bool, error) {
	if m.ForceAcquireStaleFn != nil {
		return m.ForceAcquireStaleFn(ctx, analysisType, staleThreshold)
	}
	return "", false, nil
}

func (m *MockCacheStore) SaveResult(ctx context.Context, analysisType advisor.AnalysisType, lease, content, errMsg string) error {
	if m.SaveResultFn != nil {
		return m.SaveResultFn(ctx, analysisType, lease, content, errMsg)
	}
	return nil
}
