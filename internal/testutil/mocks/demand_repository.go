package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/demand"
)

// DemandRepositoryMock is a Fn-field mock of demand.Repository.
// Unset Fn fields return zero values + nil error.
type DemandRepositoryMock struct {
	UpsertCardCacheFn            func(ctx context.Context, row demand.CardCache) error
	GetCardCacheFn               func(ctx context.Context, cardID, window string) (*demand.CardCache, error)
	ListCardCacheByDemandScoreFn func(ctx context.Context, window string, limit int) ([]demand.CardCache, error)
	CardDataQualityStatsFn       func(ctx context.Context, window string) (demand.QualityStats, error)
	UpsertCharacterCacheFn       func(ctx context.Context, row demand.CharacterCache) error
	GetCharacterCacheFn          func(ctx context.Context, character, window string) (*demand.CharacterCache, error)
	ListCharacterCacheFn         func(ctx context.Context, window string) ([]demand.CharacterCache, error)
}

var _ demand.Repository = (*DemandRepositoryMock)(nil)

func (m *DemandRepositoryMock) UpsertCardCache(ctx context.Context, row demand.CardCache) error {
	if m.UpsertCardCacheFn != nil {
		return m.UpsertCardCacheFn(ctx, row)
	}
	return nil
}

func (m *DemandRepositoryMock) GetCardCache(ctx context.Context, cardID, window string) (*demand.CardCache, error) {
	if m.GetCardCacheFn != nil {
		return m.GetCardCacheFn(ctx, cardID, window)
	}
	return nil, nil
}

func (m *DemandRepositoryMock) ListCardCacheByDemandScore(ctx context.Context, window string, limit int) ([]demand.CardCache, error) {
	if m.ListCardCacheByDemandScoreFn != nil {
		return m.ListCardCacheByDemandScoreFn(ctx, window, limit)
	}
	return nil, nil
}

func (m *DemandRepositoryMock) CardDataQualityStats(ctx context.Context, window string) (demand.QualityStats, error) {
	if m.CardDataQualityStatsFn != nil {
		return m.CardDataQualityStatsFn(ctx, window)
	}
	return demand.QualityStats{}, nil
}

func (m *DemandRepositoryMock) UpsertCharacterCache(ctx context.Context, row demand.CharacterCache) error {
	if m.UpsertCharacterCacheFn != nil {
		return m.UpsertCharacterCacheFn(ctx, row)
	}
	return nil
}

func (m *DemandRepositoryMock) GetCharacterCache(ctx context.Context, character, window string) (*demand.CharacterCache, error) {
	if m.GetCharacterCacheFn != nil {
		return m.GetCharacterCacheFn(ctx, character, window)
	}
	return nil, nil
}

func (m *DemandRepositoryMock) ListCharacterCache(ctx context.Context, window string) ([]demand.CharacterCache, error) {
	if m.ListCharacterCacheFn != nil {
		return m.ListCharacterCacheFn(ctx, window)
	}
	return nil, nil
}

// CampaignCoverageLookupMock is a Fn-field mock of demand.CampaignCoverageLookup.
type CampaignCoverageLookupMock struct {
	CampaignsCoveringFn func(ctx context.Context, character, era string, grade int) ([]string, error)
	UnsoldCountForFn    func(ctx context.Context, character, era string, grade int) (int, error)
	ActiveCampaignsFn   func(ctx context.Context) ([]demand.ActiveCampaign, error)
}

var _ demand.CampaignCoverageLookup = (*CampaignCoverageLookupMock)(nil)

func (m *CampaignCoverageLookupMock) CampaignsCovering(ctx context.Context, character, era string, grade int) ([]string, error) {
	if m.CampaignsCoveringFn != nil {
		return m.CampaignsCoveringFn(ctx, character, era, grade)
	}
	return nil, nil
}

func (m *CampaignCoverageLookupMock) UnsoldCountFor(ctx context.Context, character, era string, grade int) (int, error) {
	if m.UnsoldCountForFn != nil {
		return m.UnsoldCountForFn(ctx, character, era, grade)
	}
	return 0, nil
}

func (m *CampaignCoverageLookupMock) ActiveCampaigns(ctx context.Context) ([]demand.ActiveCampaign, error) {
	if m.ActiveCampaignsFn != nil {
		return m.ActiveCampaignsFn(ctx)
	}
	return []demand.ActiveCampaign{}, nil
}
