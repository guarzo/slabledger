package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/cardladder"
)

// CardLadderStoreMock is an Fn-field mock of the CardLadder persistence seam
// consumed by the HTTP admin handler (handlers.CardLadderStore). It is declared
// against the domain types rather than the handler interface so that it stays
// usable from any consumer that names the same methods.
//
// Unset Fn fields return zero values and a nil error, which for GetConfig means
// the "not configured" state the handler treats as valid.
type CardLadderStoreMock struct {
	GetConfigFn            func(ctx context.Context) (*cardladder.Config, error)
	SaveConfigFn           func(ctx context.Context, email, refreshToken, collectionID, firebaseAPIKey, firebaseUID string) error
	ListMappingsFn         func(ctx context.Context) ([]cardladder.CardMapping, error)
	SaveMappingFn          func(ctx context.Context, slabSerial, clCardID, gemRateID, condition string) error
	GetCLPriceStatsFn      func(ctx context.Context) (*cardladder.PriceStats, error)
	GetCLFailuresFn        func(ctx context.Context, sampleLimit int) (*cardladder.IntegrationFailuresReport, error)
	GetCLCoverageByMonthFn func(ctx context.Context) (*cardladder.CoverageReport, error)
}

func (m *CardLadderStoreMock) GetConfig(ctx context.Context) (*cardladder.Config, error) {
	if m.GetConfigFn != nil {
		return m.GetConfigFn(ctx)
	}
	return nil, nil
}

func (m *CardLadderStoreMock) SaveConfig(ctx context.Context, email, refreshToken, collectionID, firebaseAPIKey, firebaseUID string) error {
	if m.SaveConfigFn != nil {
		return m.SaveConfigFn(ctx, email, refreshToken, collectionID, firebaseAPIKey, firebaseUID)
	}
	return nil
}

func (m *CardLadderStoreMock) ListMappings(ctx context.Context) ([]cardladder.CardMapping, error) {
	if m.ListMappingsFn != nil {
		return m.ListMappingsFn(ctx)
	}
	return nil, nil
}

func (m *CardLadderStoreMock) SaveMapping(ctx context.Context, slabSerial, clCardID, gemRateID, condition string) error {
	if m.SaveMappingFn != nil {
		return m.SaveMappingFn(ctx, slabSerial, clCardID, gemRateID, condition)
	}
	return nil
}

func (m *CardLadderStoreMock) GetCLPriceStats(ctx context.Context) (*cardladder.PriceStats, error) {
	if m.GetCLPriceStatsFn != nil {
		return m.GetCLPriceStatsFn(ctx)
	}
	return nil, nil
}

func (m *CardLadderStoreMock) GetCLFailures(ctx context.Context, sampleLimit int) (*cardladder.IntegrationFailuresReport, error) {
	if m.GetCLFailuresFn != nil {
		return m.GetCLFailuresFn(ctx, sampleLimit)
	}
	return nil, nil
}

func (m *CardLadderStoreMock) GetCLCoverageByMonth(ctx context.Context) (*cardladder.CoverageReport, error) {
	if m.GetCLCoverageByMonthFn != nil {
		return m.GetCLCoverageByMonthFn(ctx)
	}
	return nil, nil
}
