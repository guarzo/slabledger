package scheduler

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRefreshClient implements CardHedgerRefreshClient for testing.
type mockRefreshClient struct {
	available      bool
	dailyCalls     int
	updates        *cardhedger.PriceUpdatesResponse
	statusCode     int
	err            error
	callCount      atomic.Int32
	lastSinceParam string
}

func (m *mockRefreshClient) Available() bool     { return m.available }
func (m *mockRefreshClient) DailyCallsUsed() int { return m.dailyCalls }
func (m *mockRefreshClient) GetPriceUpdates(_ context.Context, since string) (*cardhedger.PriceUpdatesResponse, int, http.Header, error) {
	m.callCount.Add(1)
	m.lastSinceParam = since
	sc := m.statusCode
	if sc == 0 && m.err == nil {
		sc = 200
	}
	return m.updates, sc, nil, m.err
}

// mockSyncStateStore implements SyncStateStore for testing.
type mockSyncStateStore struct {
	values map[string]string
	getErr error
	setErr error
}

func newMockSyncStateStore() *mockSyncStateStore {
	return &mockSyncStateStore{values: make(map[string]string)}
}

func (m *mockSyncStateStore) Get(_ context.Context, key string) (string, error) {
	return m.values[key], m.getErr
}

func (m *mockSyncStateStore) Set(_ context.Context, key, value string) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.values[key] = value
	return nil
}

// mockIDLookup implements CardIDMappingLookup for testing.
type mockIDLookup struct {
	cards map[string][2]string // externalID -> [cardName, setName]
	err   error
}

func (m *mockIDLookup) GetLocalCard(_ context.Context, _, externalID string) (string, string, error) {
	if m.err != nil {
		return "", "", m.err
	}
	if pair, ok := m.cards[externalID]; ok {
		return pair[0], pair[1], nil
	}
	return "", "", nil
}

func TestCardHedgerRefreshScheduler_Disabled(t *testing.T) {
	s := NewCardHedgerRefreshScheduler(
		&mockRefreshClient{available: true},
		nil, newMockSyncStateStore(), &mockIDLookup{},
		mocks.NewMockLogger(),
		CardHedgerRefreshConfig{Enabled: false},
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

func TestCardHedgerRefreshScheduler_ClientUnavailable(t *testing.T) {
	s := NewCardHedgerRefreshScheduler(
		&mockRefreshClient{available: false},
		nil, newMockSyncStateStore(), &mockIDLookup{},
		mocks.NewMockLogger(),
		CardHedgerRefreshConfig{Enabled: true},
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

func TestCardHedgerRefreshScheduler_Stop(t *testing.T) {
	client := &mockRefreshClient{
		available: true,
		updates:   &cardhedger.PriceUpdatesResponse{Count: 0},
	}
	s := NewCardHedgerRefreshScheduler(
		client, nil, newMockSyncStateStore(), &mockIDLookup{},
		mocks.NewMockLogger(),
		CardHedgerRefreshConfig{Enabled: true, PollInterval: 1 * time.Hour},
	)

	done := make(chan struct{})
	go func() { s.Start(context.Background()); close(done) }()

	// Stop immediately - should terminate even with a long poll interval
	s.Stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not stop within timeout")
	}
}

func TestCardHedgerRefreshScheduler_PollStoresPrice(t *testing.T) {
	client := &mockRefreshClient{
		available: true,
		updates: &cardhedger.PriceUpdatesResponse{
			Count: 1,
			Updates: []cardhedger.PriceUpdate{
				{
					CardID:          "ch-123",
					Grade:           "PSA 10",
					Price:           "150.00",
					CardNumber:      "004/102",
					UpdateTimestamp: time.Now().UTC().Format(time.RFC3339),
				},
			},
		},
	}
	idLookup := &mockIDLookup{
		cards: map[string][2]string{"ch-123": {"Charizard", "Base Set"}},
	}
	syncStore := newMockSyncStateStore()
	priceRepo := &mockPriceRepo{}

	s := NewCardHedgerRefreshScheduler(
		client, priceRepo, syncStore, idLookup,
		mocks.NewMockLogger(),
		CardHedgerRefreshConfig{Enabled: true},
	)

	s.pollUpdates(context.Background())

	assert.Equal(t, int32(1), client.callCount.Load())
	require.Equal(t, int32(1), priceRepo.storeCount.Load(), "should store 1 price")
}

func TestCardHedgerRefreshScheduler_SkipsUnmappedCards(t *testing.T) {
	client := &mockRefreshClient{
		available: true,
		updates: &cardhedger.PriceUpdatesResponse{
			Count: 1,
			Updates: []cardhedger.PriceUpdate{
				{CardID: "unknown-id", Grade: "PSA 10", Price: "100.00"},
			},
		},
	}
	idLookup := &mockIDLookup{cards: map[string][2]string{}} // no mappings
	priceRepo := &mockPriceRepo{}

	s := NewCardHedgerRefreshScheduler(
		client, priceRepo, newMockSyncStateStore(), idLookup,
		mocks.NewMockLogger(),
		CardHedgerRefreshConfig{Enabled: true},
	)

	s.pollUpdates(context.Background())

	assert.Equal(t, int32(0), priceRepo.storeCount.Load(), "should not store unmapped cards")
}

func TestIsKnownCardHedgerGrade(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"PSA 10", true},
		{"PSA 9", true},
		{"Raw", true},
		{"BGS 10", true},
		{"BGS 9.5", true},
		{"CGC 9.5", true},
		{"CGC 10", true},
		{"CGC 10 PRISTINE", true},
		{"AGS 9", true},
		{"TAG 10", true},
		{"", false},
		{"XYZ 10", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isKnownCardHedgerGrade(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCardHedgerRefreshScheduler_TrackerRecordsSuccess(t *testing.T) {
	client := &mockRefreshClient{
		available: true,
		updates:   &cardhedger.PriceUpdatesResponse{Count: 0},
	}
	tracker := &mocks.MockAPITracker{}

	s := NewCardHedgerRefreshScheduler(
		client, &mockPriceRepo{}, newMockSyncStateStore(), &mockIDLookup{},
		mocks.NewMockLogger(),
		CardHedgerRefreshConfig{Enabled: true},
		WithRefreshAPITracker(tracker),
	)

	s.pollUpdates(context.Background())

	require.Len(t, tracker.RecordedCalls, 1)
	call := tracker.RecordedCalls[0]
	assert.Equal(t, "cardhedger", call.Provider)
	assert.Equal(t, "delta/price-updates", call.Endpoint)
	assert.Equal(t, 200, call.StatusCode)
	assert.Empty(t, call.Error)
	assert.Greater(t, call.LatencyMS, int64(-1))
}

func TestCardHedgerRefreshScheduler_TrackerRecordsError(t *testing.T) {
	client := &mockRefreshClient{
		available:  true,
		statusCode: 503,
		err:        fmt.Errorf("service unavailable"),
	}
	tracker := &mocks.MockAPITracker{}

	s := NewCardHedgerRefreshScheduler(
		client, &mockPriceRepo{}, newMockSyncStateStore(), &mockIDLookup{},
		mocks.NewMockLogger(),
		CardHedgerRefreshConfig{Enabled: true},
		WithRefreshAPITracker(tracker),
	)

	s.pollUpdates(context.Background())

	require.Len(t, tracker.RecordedCalls, 1)
	call := tracker.RecordedCalls[0]
	assert.Equal(t, "cardhedger", call.Provider)
	assert.Equal(t, "delta/price-updates", call.Endpoint)
	assert.Equal(t, 503, call.StatusCode)
	assert.Contains(t, call.Error, "service unavailable")
}
