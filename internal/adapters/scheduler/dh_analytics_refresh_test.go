package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// fakeAnalyticsClient is a counting stub of dhAnalyticsClient used by the
// refresh-scheduler unit tests.
type fakeAnalyticsClient struct {
	available bool

	topCharactersFn       func(ctx context.Context, limit int, era string) (*dh.TopCharactersResponse, error)
	characterVelocityFn   func(ctx context.Context, opts dh.CharacterListOpts) (*dh.CharacterVelocityResponse, error)
	characterSaturationFn func(ctx context.Context, opts dh.CharacterListOpts) (*dh.CharacterSaturationResponse, error)
	characterDemandFn     func(ctx context.Context, cardIDs []int, window string, byEra bool) (*dh.CharacterDemandResponse, error)
	batchAnalyticsFn      func(ctx context.Context, cardIDs []int, fields []string) (*dh.BatchAnalyticsResponse, error)
	demandSignalsFn       func(ctx context.Context, cardIDs []int, window string) (*dh.DemandSignalsResponse, error)

	topCharactersCalls       int
	characterVelocityCalls   int
	characterSaturationCalls int
	characterDemandCalls     int
	batchAnalyticsCalls      int
	demandSignalsCalls       int
}

func (f *fakeAnalyticsClient) EnterpriseAvailable() bool { return f.available }

func (f *fakeAnalyticsClient) TopCharacters(ctx context.Context, limit int, era string) (*dh.TopCharactersResponse, error) {
	f.topCharactersCalls++
	if f.topCharactersFn != nil {
		return f.topCharactersFn(ctx, limit, era)
	}
	return &dh.TopCharactersResponse{}, nil
}

func (f *fakeAnalyticsClient) CharacterVelocity(ctx context.Context, opts dh.CharacterListOpts) (*dh.CharacterVelocityResponse, error) {
	f.characterVelocityCalls++
	if f.characterVelocityFn != nil {
		return f.characterVelocityFn(ctx, opts)
	}
	return &dh.CharacterVelocityResponse{}, nil
}

func (f *fakeAnalyticsClient) CharacterSaturation(ctx context.Context, opts dh.CharacterListOpts) (*dh.CharacterSaturationResponse, error) {
	f.characterSaturationCalls++
	if f.characterSaturationFn != nil {
		return f.characterSaturationFn(ctx, opts)
	}
	return &dh.CharacterSaturationResponse{}, nil
}

func (f *fakeAnalyticsClient) CharacterDemand(ctx context.Context, cardIDs []int, window string, byEra bool) (*dh.CharacterDemandResponse, error) {
	f.characterDemandCalls++
	if f.characterDemandFn != nil {
		return f.characterDemandFn(ctx, cardIDs, window, byEra)
	}
	return &dh.CharacterDemandResponse{}, nil
}

func (f *fakeAnalyticsClient) BatchAnalytics(ctx context.Context, cardIDs []int, fields []string) (*dh.BatchAnalyticsResponse, error) {
	f.batchAnalyticsCalls++
	if f.batchAnalyticsFn != nil {
		return f.batchAnalyticsFn(ctx, cardIDs, fields)
	}
	return &dh.BatchAnalyticsResponse{}, nil
}

func (f *fakeAnalyticsClient) DemandSignals(ctx context.Context, cardIDs []int, window string) (*dh.DemandSignalsResponse, error) {
	f.demandSignalsCalls++
	if f.demandSignalsFn != nil {
		return f.demandSignalsFn(ctx, cardIDs, window)
	}
	return &dh.DemandSignalsResponse{}, nil
}

// fakeCardLister returns a canned list of inventory card IDs.
type fakeCardLister struct {
	ids []int
	err error
}

func (f *fakeCardLister) ListUnsoldDHCardIDs(_ context.Context) ([]int, error) {
	return f.ids, f.err
}

func newTestCfg() config.DHAnalyticsRefreshConfig {
	return config.DHAnalyticsRefreshConfig{
		Enabled:     true,
		RefreshHour: 3,
		Window:      "30d",
	}
}

// --- tests ---

func TestDHAnalyticsRefresh_EnterpriseUnavailable_ShortCircuits(t *testing.T) {
	client := &fakeAnalyticsClient{available: false}
	repo := &mocks.DemandRepositoryMock{
		UpsertCardCacheFn: func(ctx context.Context, _ demand.CardCache) error {
			t.Fatalf("repo should not be touched when enterprise unavailable")
			return nil
		},
		UpsertCharacterCacheFn: func(ctx context.Context, _ demand.CharacterCache) error {
			t.Fatalf("repo should not be touched when enterprise unavailable")
			return nil
		},
	}
	lister := &fakeCardLister{ids: []int{1, 2, 3}}
	sched := NewDHAnalyticsRefreshScheduler(client, repo, lister, mocks.NewMockLogger(), newTestCfg())

	sched.refresh(context.Background())

	if client.topCharactersCalls != 0 ||
		client.batchAnalyticsCalls != 0 ||
		client.demandSignalsCalls != 0 ||
		client.characterVelocityCalls != 0 ||
		client.characterSaturationCalls != 0 {
		t.Fatalf("expected no DH calls; got top=%d batch=%d demand=%d vel=%d sat=%d",
			client.topCharactersCalls, client.batchAnalyticsCalls,
			client.demandSignalsCalls, client.characterVelocityCalls, client.characterSaturationCalls)
	}
}

func TestDHAnalyticsRefresh_HappyPath(t *testing.T) {
	client := &fakeAnalyticsClient{
		available: true,
		topCharactersFn: func(ctx context.Context, limit int, era string) (*dh.TopCharactersResponse, error) {
			if era == "" {
				return &dh.TopCharactersResponse{
					CharacterDemand: []dh.CharacterDemandEntry{
						{CharacterName: "Charizard", CardCount: 10, AvgDemandScore: 87.5},
						{CharacterName: "Pikachu", CardCount: 5, AvgDemandScore: 75.1},
					},
				}, nil
			}
			return &dh.TopCharactersResponse{
				CharacterDemand: []dh.CharacterDemandEntry{
					{CharacterName: "Mewtwo", CardCount: 3, AvgDemandScore: 65},
				},
			}, nil
		},
		characterVelocityFn: func(ctx context.Context, opts dh.CharacterListOpts) (*dh.CharacterVelocityResponse, error) {
			return &dh.CharacterVelocityResponse{
				Characters: []dh.CharacterVelocityEntry{
					{
						CharacterName: "Charizard",
						CardCount:     10,
						ComputedAt:    "2026-04-15T00:00:00Z",
						Velocity: dh.CharacterVelocityFields{
							MedianDaysToSell: 14.5,
							SampleSize:       120,
						},
					},
				},
			}, nil
		},
		characterSaturationFn: func(ctx context.Context, opts dh.CharacterListOpts) (*dh.CharacterSaturationResponse, error) {
			return &dh.CharacterSaturationResponse{
				Characters: []dh.CharacterSaturationEntry{
					{
						CharacterName: "Pikachu",
						CardCount:     5,
						ComputedAt:    "2026-04-15T00:00:00Z",
						Saturation:    dh.CharacterSaturationFields{ActiveListingCount: 42},
					},
				},
			}, nil
		},
		batchAnalyticsFn: func(ctx context.Context, cardIDs []int, fields []string) (*dh.BatchAnalyticsResponse, error) {
			return &dh.BatchAnalyticsResponse{
				Results: []dh.CardAnalytics{
					{
						CardID:     101,
						ComputedAt: "2026-04-15T00:00:00Z",
						Velocity:   &dh.VelocityMetrics{MedianDaysToSell: "12.0", SampleSize: 50},
						Trend:      &dh.TrendMetrics{Direction7d: "up"},
					},
					{CardID: 202, Error: "analytics_not_computed"},
				},
			}, nil
		},
		demandSignalsFn: func(ctx context.Context, cardIDs []int, window string) (*dh.DemandSignalsResponse, error) {
			return &dh.DemandSignalsResponse{
				DemandSignals: []dh.DemandSignal{
					{CardID: 101, DemandScore: 92.3, DataQuality: "full"},
					{CardID: 202, DemandScore: 41.0, DataQuality: "proxy"},
				},
			}, nil
		},
	}

	var (
		upsertCharacters []demand.CharacterCache
		upsertCards      []demand.CardCache
	)
	repo := &mocks.DemandRepositoryMock{
		UpsertCharacterCacheFn: func(_ context.Context, row demand.CharacterCache) error {
			upsertCharacters = append(upsertCharacters, row)
			return nil
		},
		UpsertCardCacheFn: func(_ context.Context, row demand.CardCache) error {
			upsertCards = append(upsertCards, row)
			return nil
		},
		CardDataQualityStatsFn: func(_ context.Context, window string) (demand.QualityStats, error) {
			return demand.QualityStats{FullCount: 1, ProxyCount: 1, TotalRows: 2}, nil
		},
	}
	lister := &fakeCardLister{ids: []int{101, 202}}
	sched := NewDHAnalyticsRefreshScheduler(client, repo, lister, mocks.NewMockLogger(), newTestCfg())

	sched.refresh(context.Background())

	if len(upsertCharacters) == 0 {
		t.Fatalf("expected at least one character upsert, got 0")
	}
	if len(upsertCards) == 0 {
		t.Fatalf("expected at least one card upsert, got 0")
	}
	// Happy path: 2 cards with analytics+demand, but one is analytics_not_computed.
	// So we expect: 1 analytics upsert (card 101) + 2 demand upserts (101, 202) = 3 card upserts total.
	if len(upsertCards) != 3 {
		t.Fatalf("expected 3 card upserts (1 analytics + 2 demand merges), got %d", len(upsertCards))
	}
	if client.topCharactersCalls != 1+len(defaultAnalyticsEras) {
		t.Fatalf("expected %d top_characters calls (overall + per-era), got %d",
			1+len(defaultAnalyticsEras), client.topCharactersCalls)
	}
}

func TestDHAnalyticsRefresh_EmptyTopCharacters_StillRunsCards(t *testing.T) {
	client := &fakeAnalyticsClient{
		available: true,
		// all character endpoints return empty
		batchAnalyticsFn: func(ctx context.Context, cardIDs []int, fields []string) (*dh.BatchAnalyticsResponse, error) {
			return &dh.BatchAnalyticsResponse{
				Results: []dh.CardAnalytics{
					{CardID: 7, Velocity: &dh.VelocityMetrics{SampleSize: 3}},
				},
			}, nil
		},
		demandSignalsFn: func(ctx context.Context, cardIDs []int, window string) (*dh.DemandSignalsResponse, error) {
			return &dh.DemandSignalsResponse{DemandSignals: []dh.DemandSignal{{CardID: 7, DemandScore: 50, DataQuality: "proxy"}}}, nil
		},
	}
	var cardUpserts int
	repo := &mocks.DemandRepositoryMock{
		UpsertCardCacheFn: func(_ context.Context, _ demand.CardCache) error {
			cardUpserts++
			return nil
		},
	}
	lister := &fakeCardLister{ids: []int{7}}
	sched := NewDHAnalyticsRefreshScheduler(client, repo, lister, mocks.NewMockLogger(), newTestCfg())

	sched.refresh(context.Background())

	if client.batchAnalyticsCalls != 1 {
		t.Fatalf("expected batch_analytics to run once, got %d", client.batchAnalyticsCalls)
	}
	if cardUpserts < 1 {
		t.Fatalf("expected at least one card upsert, got %d", cardUpserts)
	}
}

func TestDHAnalyticsRefresh_AllAnalyticsNotComputed_NoCardUpsertsFromAnalytics(t *testing.T) {
	client := &fakeAnalyticsClient{
		available: true,
		batchAnalyticsFn: func(ctx context.Context, cardIDs []int, fields []string) (*dh.BatchAnalyticsResponse, error) {
			results := make([]dh.CardAnalytics, 0, len(cardIDs))
			for _, id := range cardIDs {
				results = append(results, dh.CardAnalytics{CardID: id, Error: "analytics_not_computed"})
			}
			return &dh.BatchAnalyticsResponse{Results: results}, nil
		},
		demandSignalsFn: func(ctx context.Context, cardIDs []int, window string) (*dh.DemandSignalsResponse, error) {
			// demand signals still succeed — proxy mode
			return &dh.DemandSignalsResponse{DemandSignals: []dh.DemandSignal{
				{CardID: 1, DemandScore: 30, DataQuality: "proxy"},
			}}, nil
		},
	}
	var analyticsUpserts, demandUpserts int
	repo := &mocks.DemandRepositoryMock{
		UpsertCardCacheFn: func(_ context.Context, row demand.CardCache) error {
			// Any row with a DemandScore came from the demand-signals merge.
			// Rows without demand came from pure analytics results.
			if row.DemandScore != nil {
				demandUpserts++
			} else {
				analyticsUpserts++
			}
			return nil
		},
	}
	lister := &fakeCardLister{ids: []int{1, 2, 3}}
	sched := NewDHAnalyticsRefreshScheduler(client, repo, lister, mocks.NewMockLogger(), newTestCfg())

	sched.refresh(context.Background())

	if analyticsUpserts != 0 {
		t.Fatalf("expected 0 analytics-only upserts when all cards are analytics_not_computed, got %d", analyticsUpserts)
	}
	if demandUpserts == 0 {
		t.Fatalf("expected demand signals to still upsert, got 0")
	}
}

func TestDHAnalyticsRefresh_BatchAnalyticsHardError_DemandStillRuns(t *testing.T) {
	client := &fakeAnalyticsClient{
		available: true,
		batchAnalyticsFn: func(ctx context.Context, cardIDs []int, fields []string) (*dh.BatchAnalyticsResponse, error) {
			return nil, errors.New("boom")
		},
		demandSignalsFn: func(ctx context.Context, cardIDs []int, window string) (*dh.DemandSignalsResponse, error) {
			return &dh.DemandSignalsResponse{DemandSignals: []dh.DemandSignal{{CardID: 99, DemandScore: 10, DataQuality: "proxy"}}}, nil
		},
	}
	var demandUpserts int
	repo := &mocks.DemandRepositoryMock{
		UpsertCardCacheFn: func(_ context.Context, row demand.CardCache) error {
			if row.DemandScore != nil {
				demandUpserts++
			}
			return nil
		},
	}
	lister := &fakeCardLister{ids: []int{99}}
	sched := NewDHAnalyticsRefreshScheduler(client, repo, lister, mocks.NewMockLogger(), newTestCfg())

	sched.refresh(context.Background())

	if client.demandSignalsCalls != 1 {
		t.Fatalf("expected demand_signals to run despite batch_analytics failure, got %d", client.demandSignalsCalls)
	}
	if demandUpserts != 1 {
		t.Fatalf("expected 1 demand upsert, got %d", demandUpserts)
	}
}

func TestDHAnalyticsRefresh_DisabledByConfig_Noop(t *testing.T) {
	client := &fakeAnalyticsClient{available: true}
	repo := &mocks.DemandRepositoryMock{}
	lister := &fakeCardLister{}
	cfg := newTestCfg()
	cfg.Enabled = false
	sched := NewDHAnalyticsRefreshScheduler(client, repo, lister, mocks.NewMockLogger(), cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // ensure Start doesn't block
	sched.Start(ctx)

	if client.topCharactersCalls != 0 {
		t.Fatalf("expected no work when disabled")
	}
}
