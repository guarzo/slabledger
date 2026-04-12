package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// MockArbitrageService is a test double for arbitrage.Service.
// Each method delegates to a function field, allowing per-test configuration.
//
// Example:
//
//	svc := &MockArbitrageService{
//	    GetCrackCandidatesFn: func(ctx context.Context, campaignID string) ([]arbitrage.CrackAnalysis, error) {
//	        return nil, nil
//	    },
//	}
type MockArbitrageService struct {
	GetCrackCandidatesFn     func(ctx context.Context, campaignID string) ([]arbitrage.CrackAnalysis, error)
	GetCrackOpportunitiesFn  func(ctx context.Context) ([]arbitrage.CrackAnalysis, error)
	GetAcquisitionTargetsFn  func(ctx context.Context) ([]arbitrage.AcquisitionOpportunity, error)
	GetActivationChecklistFn func(ctx context.Context, campaignID string) (*inventory.ActivationChecklist, error)
	GetExpectedValuesFn      func(ctx context.Context, campaignID string) (*arbitrage.EVPortfolio, error)
	EvaluatePurchaseFn       func(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*arbitrage.ExpectedValue, error)
	RunProjectionFn          func(ctx context.Context, campaignID string) (*arbitrage.MonteCarloComparison, error)
}

var _ arbitrage.Service = (*MockArbitrageService)(nil)

func (m *MockArbitrageService) GetCrackCandidates(ctx context.Context, campaignID string) ([]arbitrage.CrackAnalysis, error) {
	if m.GetCrackCandidatesFn != nil {
		return m.GetCrackCandidatesFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *MockArbitrageService) GetCrackOpportunities(ctx context.Context) ([]arbitrage.CrackAnalysis, error) {
	if m.GetCrackOpportunitiesFn != nil {
		return m.GetCrackOpportunitiesFn(ctx)
	}
	return nil, nil
}

func (m *MockArbitrageService) GetAcquisitionTargets(ctx context.Context) ([]arbitrage.AcquisitionOpportunity, error) {
	if m.GetAcquisitionTargetsFn != nil {
		return m.GetAcquisitionTargetsFn(ctx)
	}
	return nil, nil
}

func (m *MockArbitrageService) GetActivationChecklist(ctx context.Context, campaignID string) (*inventory.ActivationChecklist, error) {
	if m.GetActivationChecklistFn != nil {
		return m.GetActivationChecklistFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *MockArbitrageService) GetExpectedValues(ctx context.Context, campaignID string) (*arbitrage.EVPortfolio, error) {
	if m.GetExpectedValuesFn != nil {
		return m.GetExpectedValuesFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *MockArbitrageService) EvaluatePurchase(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*arbitrage.ExpectedValue, error) {
	if m.EvaluatePurchaseFn != nil {
		return m.EvaluatePurchaseFn(ctx, campaignID, cardName, grade, buyCostCents)
	}
	return nil, nil
}

func (m *MockArbitrageService) RunProjection(ctx context.Context, campaignID string) (*arbitrage.MonteCarloComparison, error) {
	if m.RunProjectionFn != nil {
		return m.RunProjectionFn(ctx, campaignID)
	}
	return nil, nil
}
