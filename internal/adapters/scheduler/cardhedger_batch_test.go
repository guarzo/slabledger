package scheduler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBatchClient implements CardHedgerBatchClient for testing.
type mockBatchClient struct {
	available  bool
	dailyCalls int
	resp       *cardhedger.AllPricesByCardResponse
	statusCode int
	err        error
	callCount  atomic.Int32

	// CardMatch mock fields
	matchResp *cardhedger.CardMatchResponse
	matchErr  error
}

func (m *mockBatchClient) Available() bool     { return m.available }
func (m *mockBatchClient) DailyCallsUsed() int { return m.dailyCalls }
func (m *mockBatchClient) GetAllPrices(_ context.Context, _ string) (*cardhedger.AllPricesByCardResponse, int, http.Header, error) {
	m.callCount.Add(1)
	sc := m.statusCode
	if sc == 0 && m.err == nil {
		sc = 200
	}
	return m.resp, sc, nil, m.err
}
func (m *mockBatchClient) CardMatch(_ context.Context, _, _ string, _ int) (*cardhedger.CardMatchResponse, int, http.Header, error) {
	m.callCount.Add(1)
	if m.matchResp != nil || m.matchErr != nil {
		return m.matchResp, 200, nil, m.matchErr
	}
	return &cardhedger.CardMatchResponse{}, 200, nil, nil
}

// mockMappingLister implements CardIDMappingLister for testing.
type mockMappingLister struct {
	mappings []CardIDMapping
	err      error
}

func (m *mockMappingLister) ListByProvider(_ context.Context, _ string) ([]CardIDMapping, error) {
	return m.mappings, m.err
}

// mockFavoritesLister implements FavoritesLister for testing.
type mockFavoritesLister struct {
	cards []FavoriteCard
	err   error
}

func (m *mockFavoritesLister) ListAllDistinctCards(_ context.Context) ([]FavoriteCard, error) {
	return m.cards, m.err
}

// trackingMatchClient wraps mockBatchClient to track CardMatch calls
// and return query-specific results via matchResults map.
type trackingMatchClient struct {
	*mockBatchClient
	onMatch      func(query, category string)
	matchResults map[string]*cardhedger.CardMatchResponse // keyed by query substring
}

func (t *trackingMatchClient) CardMatch(_ context.Context, query, category string, _ int) (*cardhedger.CardMatchResponse, int, http.Header, error) {
	t.callCount.Add(1)
	if t.onMatch != nil {
		t.onMatch(query, category)
	}
	for key, resp := range t.matchResults {
		if strings.Contains(query, key) {
			return resp, 200, nil, nil
		}
	}
	return &cardhedger.CardMatchResponse{}, 200, nil, nil
}

func TestCardHedgerBatchScheduler_Disabled(t *testing.T) {
	s := NewCardHedgerBatchScheduler(
		&mockBatchClient{available: true},
		nil, &mockMappingLister{}, nil, nil,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: false},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() { s.Start(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler should return immediately when disabled")
	}
}

func TestCardHedgerBatchScheduler_ClientUnavailable(t *testing.T) {
	s := NewCardHedgerBatchScheduler(
		&mockBatchClient{available: false},
		nil, &mockMappingLister{}, nil, nil,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() { s.Start(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("scheduler should return immediately when client unavailable")
	}
}

func TestCardHedgerBatchScheduler_Stop(t *testing.T) {
	s := NewCardHedgerBatchScheduler(
		&mockBatchClient{available: true},
		nil, &mockMappingLister{}, nil, nil,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true, RunInterval: 1 * time.Hour},
	)

	done := make(chan struct{})
	go func() { s.Start(context.Background()); close(done) }()

	s.Stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not stop within timeout")
	}
}

func TestCardHedgerBatchScheduler_RunBatchStoresPrices(t *testing.T) {
	client := &mockBatchClient{
		available: true,
		resp: &cardhedger.AllPricesByCardResponse{
			Prices: []cardhedger.GradePrice{
				{Grade: "PSA 10", Price: "500.00"},
				{Grade: "PSA 9", Price: "250.00"},
				{Grade: "Raw", Price: "50.00"},
			},
		},
	}
	priceRepo := &mockPriceRepo{}

	s := NewCardHedgerBatchScheduler(
		client, priceRepo,
		&mockMappingLister{
			mappings: []CardIDMapping{
				{CardName: "Pikachu", SetName: "Base Set", ExternalID: "ch-42"},
			},
		},
		nil, nil,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true},
	)

	s.runBatch(context.Background())

	assert.Equal(t, int32(1), client.callCount.Load(), "should fetch prices for 1 card")
	require.Equal(t, 3, int(priceRepo.storeCount.Load()), "should store 3 grade prices")
}

func TestCardHedgerBatchScheduler_PriorityCards(t *testing.T) {
	client := &mockBatchClient{
		available: true,
		resp:      &cardhedger.AllPricesByCardResponse{},
	}

	mappingLister := &mockMappingLister{
		mappings: []CardIDMapping{
			{CardName: "Pikachu", SetName: "Base Set", ExternalID: "ch-1"},
			{CardName: "Charizard", SetName: "Base Set", ExternalID: "ch-2"},
		},
	}

	favLister := &mockFavoritesLister{
		cards: []FavoriteCard{{CardName: "Charizard", SetName: "Base Set"}},
	}

	s := NewCardHedgerBatchScheduler(
		client, &mockPriceRepo{}, mappingLister, favLister, nil,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true, MaxCardsPerRun: 1},
	)

	s.runBatch(context.Background())

	// With MaxCardsPerRun=1, only priority card (Charizard) should be processed
	assert.Equal(t, int32(1), client.callCount.Load(), "should process 1 card (budget limited)")
}

func TestCardHedgerBatchScheduler_NoMappings(t *testing.T) {
	client := &mockBatchClient{available: true}

	s := NewCardHedgerBatchScheduler(
		client, &mockPriceRepo{},
		&mockMappingLister{mappings: nil}, nil, nil,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true},
	)

	s.runBatch(context.Background())

	assert.Equal(t, int32(0), client.callCount.Load(), "should not make API calls with no mappings")
}

func TestCardHedgerBatchScheduler_SkipsInvalidPrices(t *testing.T) {
	client := &mockBatchClient{
		available: true,
		resp: &cardhedger.AllPricesByCardResponse{
			Prices: []cardhedger.GradePrice{
				{Grade: "PSA 10", Price: "not-a-number"},
				{Grade: "PSA 9", Price: "-50.00"},
				{Grade: "PSA 8", Price: "100.00"},
				{Grade: "BGS 10", Price: "200.00"}, // now recognized as valid grade
				{Grade: "CGC 10", Price: "150.00"}, // recognized but no fusion mapping, still stored in batch
			},
		},
	}
	priceRepo := &mockPriceRepo{}

	s := NewCardHedgerBatchScheduler(
		client, priceRepo,
		&mockMappingLister{
			mappings: []CardIDMapping{{CardName: "Test", SetName: "Set", ExternalID: "ch-1"}},
		},
		nil, nil,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true},
	)

	s.runBatch(context.Background())

	// "PSA 8" (100.00), "BGS 10" (200.00), "CGC 10" (150.00) stored; invalid price + negative skipped
	assert.Equal(t, 3, int(priceRepo.storeCount.Load()), "should store valid prices including non-PSA grades")
}

func TestCardHedgerBatchScheduler_TrackerRecordsSuccess(t *testing.T) {
	client := &mockBatchClient{
		available: true,
		resp: &cardhedger.AllPricesByCardResponse{
			Prices: []cardhedger.GradePrice{
				{Grade: "PSA 10", Price: "100.00"},
			},
		},
	}
	tracker := &mocks.MockAPITracker{}

	s := NewCardHedgerBatchScheduler(
		client, &mockPriceRepo{},
		&mockMappingLister{
			mappings: []CardIDMapping{{CardName: "Charizard", SetName: "Base Set", ExternalID: "ch-1"}},
		},
		nil, nil,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true},
		WithBatchAPITracker(tracker),
	)

	s.runBatch(context.Background())

	// RecordAPICall is invoked asynchronously; wait for it to complete.
	require.Eventually(t, func() bool { return len(tracker.GetRecordedCalls()) == 1 },
		500*time.Millisecond, 5*time.Millisecond)
	calls := tracker.GetRecordedCalls()
	call := calls[0]
	assert.Equal(t, "cardhedger", call.Provider)
	assert.Equal(t, "batch/all-prices", call.Endpoint)
	assert.Equal(t, 200, call.StatusCode)
	assert.Empty(t, call.Error)
	assert.Greater(t, call.LatencyMS, int64(-1))
}

func TestCardHedgerBatchScheduler_TrackerRecordsError(t *testing.T) {
	client := &mockBatchClient{
		available:  true,
		statusCode: 502,
		err:        fmt.Errorf("bad gateway"),
	}
	tracker := &mocks.MockAPITracker{}

	s := NewCardHedgerBatchScheduler(
		client, &mockPriceRepo{},
		&mockMappingLister{
			mappings: []CardIDMapping{{CardName: "Charizard", SetName: "Base Set", ExternalID: "ch-1"}},
		},
		nil, nil,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true},
		WithBatchAPITracker(tracker),
	)

	s.runBatch(context.Background())

	// RecordAPICall is invoked asynchronously; wait for it to complete.
	require.Eventually(t, func() bool { return len(tracker.GetRecordedCalls()) == 1 },
		500*time.Millisecond, 5*time.Millisecond)
	calls := tracker.GetRecordedCalls()
	call := calls[0]
	assert.Equal(t, "cardhedger", call.Provider)
	assert.Equal(t, "batch/all-prices", call.Endpoint)
	assert.Equal(t, 502, call.StatusCode)
	assert.Contains(t, call.Error, "bad gateway")
}

// mockCampaignCardLister implements CampaignCardLister for testing.
type mockCampaignCardLister struct {
	cards []UnsoldCard
}

func (m *mockCampaignCardLister) ListUnsoldCards(_ context.Context) ([]UnsoldCard, error) {
	return m.cards, nil
}

// mockMappingSaver implements CardIDMappingSaver for testing.
type mockMappingSaver struct {
	saved []CardIDMapping
}

func (m *mockMappingSaver) SaveExternalID(_ context.Context, cardName, setName, collectorNumber, _, externalID string) error {
	m.saved = append(m.saved, CardIDMapping{CardName: cardName, SetName: setName, CollectorNumber: collectorNumber, ExternalID: externalID})
	return nil
}

func TestCardHedgerBatchScheduler_DiscoverUnmappedCards(t *testing.T) {
	client := &mockBatchClient{
		available: true,
		resp: &cardhedger.AllPricesByCardResponse{
			Prices: []cardhedger.GradePrice{
				{Grade: "PSA 10", Price: "300.00"},
			},
		},
		matchResp: &cardhedger.CardMatchResponse{
			Match: &cardhedger.CardMatchResult{
				CardID:     "ch-new-1",
				Set:        "Scarlet & Violet Promos",
				Confidence: 0.85,
			},
		},
	}
	priceRepo := &mockPriceRepo{}
	saver := &mockMappingSaver{}

	// No existing mappings
	mappingLister := &mockMappingLister{mappings: nil}

	// Campaign has one unsold card that needs discovery
	campLister := &mockCampaignCardLister{
		cards: []UnsoldCard{{CardName: "Charizard ex", SetName: "Scarlet & Violet Promos"}},
	}

	s := NewCardHedgerBatchScheduler(
		client, priceRepo, mappingLister, nil, campLister,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true},
		WithBatchMappingSaver(saver),
	)

	s.runBatch(context.Background())

	// Should have discovered and saved the mapping
	require.Len(t, saver.saved, 1, "should discover and save 1 new mapping")
	assert.Equal(t, "Charizard ex", saver.saved[0].CardName)
	assert.Equal(t, "ch-new-1", saver.saved[0].ExternalID)

	// Should have called CardMatch (1) + GetAllPrices (1) for the discovered card
	assert.Equal(t, int32(2), client.callCount.Load(), "should match + fetch prices for discovered card")
	assert.Equal(t, 1, int(priceRepo.storeCount.Load()), "should store prices for discovered card")
}

func TestCardHedgerBatchScheduler_DiscoverSkipsGenericSets(t *testing.T) {
	client := &mockBatchClient{
		available: true,
		matchResp: &cardhedger.CardMatchResponse{
			Match: &cardhedger.CardMatchResult{
				CardID:     "ch-wrong",
				Confidence: 0.9,
			},
		},
	}
	saver := &mockMappingSaver{}

	// Campaign card with generic set name — should not be searched
	campLister := &mockCampaignCardLister{
		cards: []UnsoldCard{{CardName: "Umbreon ex", SetName: "TCG Cards"}},
	}

	s := NewCardHedgerBatchScheduler(
		client, &mockPriceRepo{}, &mockMappingLister{}, nil, campLister,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true},
		WithBatchMappingSaver(saver),
	)

	s.runBatch(context.Background())

	assert.Empty(t, saver.saved, "should not discover cards with generic set names")
	assert.Equal(t, int32(0), client.callCount.Load(), "should not make API calls")
}

func TestCardHedgerBatchScheduler_DiscoverVariantsIndependently(t *testing.T) {
	matchCallQueries := []string{}
	client := &mockBatchClient{
		available: true,
		resp: &cardhedger.AllPricesByCardResponse{
			Prices: []cardhedger.GradePrice{
				{Grade: "PSA 10", Price: "100.00"},
			},
		},
	}
	// Use a custom match client that returns variant-specific results
	matchClient := &trackingMatchClient{
		mockBatchClient: client,
		onMatch: func(query, _ string) {
			matchCallQueries = append(matchCallQueries, query)
		},
		matchResults: map[string]*cardhedger.CardMatchResponse{
			"25": {
				Match: &cardhedger.CardMatchResult{
					CardID:     "ch-25",
					Set:        "Celebrations",
					Number:     "25",
					Confidence: 0.9,
				},
			},
			"SWSH039": {
				Match: &cardhedger.CardMatchResult{
					CardID:     "ch-swsh039",
					Set:        "Celebrations",
					Number:     "SWSH039",
					Confidence: 0.9,
				},
			},
		},
	}
	saver := &mockMappingSaver{}

	// Two favorites with same name+set but different card numbers
	favLister := &mockFavoritesLister{
		cards: []FavoriteCard{
			{CardName: "Pikachu", SetName: "Celebrations", CardNumber: "25"},
			{CardName: "Pikachu", SetName: "Celebrations", CardNumber: "SWSH039"},
		},
	}

	s := NewCardHedgerBatchScheduler(
		matchClient, &mockPriceRepo{}, &mockMappingLister{}, favLister, nil,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true},
		WithBatchMappingSaver(saver),
	)

	s.runBatch(context.Background())

	// Both variants should have been matched independently
	assert.Len(t, matchCallQueries, 2, "should match for each variant independently")
	require.Len(t, saver.saved, 2, "should discover and save 2 mappings (one per variant)")

	// Verify each variant got its own mapping
	savedNumbers := map[string]string{}
	for _, m := range saver.saved {
		savedNumbers[m.CollectorNumber] = m.ExternalID
	}
	assert.Equal(t, "ch-25", savedNumbers["25"], "variant 25 should map to ch-25")
	assert.Equal(t, "ch-swsh039", savedNumbers["SWSH039"], "variant SWSH039 should map to ch-swsh039")
}

func TestCardHedgerBatchScheduler_FavoritesWithoutNumberUseBaseKey(t *testing.T) {
	client := &mockBatchClient{
		available: true,
		resp: &cardhedger.AllPricesByCardResponse{
			Prices: []cardhedger.GradePrice{
				{Grade: "PSA 10", Price: "100.00"},
			},
		},
		matchResp: &cardhedger.CardMatchResponse{
			Match: &cardhedger.CardMatchResult{
				CardID:     "ch-base",
				Set:        "Base Set",
				Confidence: 0.85,
			},
		},
	}
	saver := &mockMappingSaver{}

	// Favorite without card number should still work
	favLister := &mockFavoritesLister{
		cards: []FavoriteCard{
			{CardName: "Charizard", SetName: "Base Set"},
		},
	}

	s := NewCardHedgerBatchScheduler(
		client, &mockPriceRepo{}, &mockMappingLister{}, favLister, nil,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true},
		WithBatchMappingSaver(saver),
	)

	s.runBatch(context.Background())

	require.Len(t, saver.saved, 1, "should discover card without number")
	assert.Equal(t, "Charizard", saver.saved[0].CardName)
}

func TestCardHedgerBatchScheduler_CampaignCardsWithNumber(t *testing.T) {
	client := &mockBatchClient{
		available: true,
		resp: &cardhedger.AllPricesByCardResponse{
			Prices: []cardhedger.GradePrice{
				{Grade: "PSA 10", Price: "300.00"},
			},
		},
		matchResp: &cardhedger.CardMatchResponse{
			Match: &cardhedger.CardMatchResult{
				CardID:     "ch-camp-1",
				Set:        "Evolving Skies",
				Number:     "215",
				Confidence: 0.9,
			},
		},
	}
	saver := &mockMappingSaver{}
	campLister := &mockCampaignCardLister{
		cards: []UnsoldCard{
			{CardName: "Umbreon VMAX", SetName: "Evolving Skies", CardNumber: "215"},
		},
	}

	s := NewCardHedgerBatchScheduler(
		client, &mockPriceRepo{}, &mockMappingLister{}, nil, campLister,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true},
		WithBatchMappingSaver(saver),
	)

	s.runBatch(context.Background())

	require.Len(t, saver.saved, 1, "should discover campaign card with number")
	assert.Equal(t, "215", saver.saved[0].CollectorNumber)
}

func TestCardHedgerBatchScheduler_DiscoverSkipsAlreadyMapped(t *testing.T) {
	client := &mockBatchClient{
		available: true,
		resp:      &cardhedger.AllPricesByCardResponse{},
	}
	saver := &mockMappingSaver{}

	mappingLister := &mockMappingLister{
		mappings: []CardIDMapping{
			{CardName: "Pikachu", SetName: "Base Set", ExternalID: "ch-existing"},
		},
	}

	// Same card as already mapped — should not search
	campLister := &mockCampaignCardLister{
		cards: []UnsoldCard{{CardName: "Pikachu", SetName: "Base Set"}},
	}

	s := NewCardHedgerBatchScheduler(
		client, &mockPriceRepo{}, mappingLister, nil, campLister,
		mocks.NewMockLogger(),
		CardHedgerBatchConfig{Enabled: true},
		WithBatchMappingSaver(saver),
	)

	s.runBatch(context.Background())

	assert.Empty(t, saver.saved, "should not search for already-mapped cards")
	assert.Equal(t, int32(1), client.callCount.Load(), "should fetch prices for existing mapping but not search")
}
