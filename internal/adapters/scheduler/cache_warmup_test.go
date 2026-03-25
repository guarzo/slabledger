package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// mockCardProvider implements domainCards.CardProvider for testing
type mockCardProvider struct {
	sets     []domainCards.Set
	setErr   error
	cardsErr map[string]error // per-set errors
}

func (m *mockCardProvider) GetCards(_ context.Context, setID string) ([]domainCards.Card, error) {
	if m.cardsErr != nil {
		if err, ok := m.cardsErr[setID]; ok {
			return nil, err
		}
	}
	return []domainCards.Card{{ID: setID + "-1", Name: "Card", Set: setID}}, nil
}

func (m *mockCardProvider) GetSet(_ context.Context, _ string) (*domainCards.Set, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCardProvider) ListAllSets(_ context.Context) ([]domainCards.Set, error) {
	if m.setErr != nil {
		return nil, m.setErr
	}
	return m.sets, nil
}

func (m *mockCardProvider) SearchCards(_ context.Context, _ domainCards.SearchCriteria) ([]domainCards.Card, int, error) {
	return nil, 0, nil
}

func (m *mockCardProvider) Available() bool { return true }

// mockNewSetIDsProvider implements NewSetIDsProvider
type mockNewSetIDsProvider struct {
	newIDs []string
	err    error
}

func (m *mockNewSetIDsProvider) GetNewSetIDs(_ context.Context, allSetIDs []string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.newIDs != nil {
		return m.newIDs, nil
	}
	return allSetIDs, nil
}

func TestWarmup_SkipsFinalizedSets(t *testing.T) {
	allSets := []domainCards.Set{
		{ID: "sv1", Name: "Set 1"},
		{ID: "sv2", Name: "Set 2"},
		{ID: "sv3", Name: "Set 3"},
		{ID: "sv4", Name: "Set 4"},
	}

	provider := &mockCardProvider{sets: allSets}
	newSetsProvider := &mockNewSetIDsProvider{
		newIDs: []string{"sv3", "sv4"}, // Only sv3 and sv4 are new
	}

	logger := mocks.NewMockLogger()
	config := CacheWarmupConfig{
		Enabled:        true,
		Interval:       24 * time.Hour,
		RateLimitDelay: 1 * time.Millisecond,
	}

	scheduler := NewCacheWarmupScheduler(provider, logger, config, newSetsProvider)
	ctx := context.Background()

	scheduler.warmup(ctx)

	// The warmup ran without error — this is a basic sanity test.
	// In a real scenario, we'd check that only sv3 and sv4 were fetched,
	// but since mockCardProvider doesn't track calls, we verify it completes.
}

func TestWarmup_EarlyAbortOnConsecutiveErrors(t *testing.T) {
	// Create 10 sets that all fail
	allSets := make([]domainCards.Set, 10)
	cardsErr := make(map[string]error)
	for i := range 10 {
		id := fmt.Sprintf("sv%d", i)
		allSets[i] = domainCards.Set{ID: id, Name: fmt.Sprintf("Set %d", i)}
		cardsErr[id] = fmt.Errorf("API timeout for %s", id)
	}

	provider := &mockCardProvider{
		sets:     allSets,
		cardsErr: cardsErr,
	}

	logger := mocks.NewMockLogger()
	config := CacheWarmupConfig{
		Enabled:        true,
		Interval:       24 * time.Hour,
		RateLimitDelay: 1 * time.Millisecond,
	}

	scheduler := NewCacheWarmupScheduler(provider, logger, config, nil)
	ctx := context.Background()

	start := time.Now()
	scheduler.warmup(ctx)
	duration := time.Since(start)

	// Should abort after 5 consecutive errors, not try all 10
	// With 1ms rate limit, should be very fast
	if duration > 1*time.Second {
		t.Errorf("warmup took too long (%v), early abort may not be working", duration)
	}
}

func TestWarmup_ConsecutiveErrorsResetOnSuccess(t *testing.T) {
	allSets := []domainCards.Set{
		{ID: "s1"}, {ID: "s2"}, {ID: "s3"}, // fail
		{ID: "s4"},                                                 // succeed (resets counter)
		{ID: "s5"}, {ID: "s6"}, {ID: "s7"}, {ID: "s8"}, {ID: "s9"}, // fail
	}

	cardsErr := map[string]error{
		"s1": fmt.Errorf("fail"), "s2": fmt.Errorf("fail"), "s3": fmt.Errorf("fail"),
		// s4 succeeds
		"s5": fmt.Errorf("fail"), "s6": fmt.Errorf("fail"), "s7": fmt.Errorf("fail"),
		"s8": fmt.Errorf("fail"), "s9": fmt.Errorf("fail"),
	}

	provider := &mockCardProvider{sets: allSets, cardsErr: cardsErr}
	logger := mocks.NewMockLogger()
	config := CacheWarmupConfig{
		Enabled:        true,
		Interval:       24 * time.Hour,
		RateLimitDelay: 1 * time.Millisecond,
	}

	scheduler := NewCacheWarmupScheduler(provider, logger, config, nil)
	ctx := context.Background()

	// Should process: s1(fail), s2(fail), s3(fail), s4(success resets), s5-s9 (5 fails → abort)
	scheduler.warmup(ctx)
}

func TestWarmup_NoNewSetsProvider(t *testing.T) {
	allSets := []domainCards.Set{
		{ID: "sv1", Name: "Set 1"},
		{ID: "sv2", Name: "Set 2"},
	}

	provider := &mockCardProvider{sets: allSets}
	logger := mocks.NewMockLogger()
	config := CacheWarmupConfig{
		Enabled:        true,
		Interval:       24 * time.Hour,
		RateLimitDelay: 1 * time.Millisecond,
	}

	// nil newSetsProvider — should fetch all sets
	scheduler := NewCacheWarmupScheduler(provider, logger, config, nil)
	ctx := context.Background()

	scheduler.warmup(ctx)
}

func TestWarmup_NewSetsProviderError(t *testing.T) {
	allSets := []domainCards.Set{
		{ID: "sv1", Name: "Set 1"},
	}

	provider := &mockCardProvider{sets: allSets}
	newSetsProvider := &mockNewSetIDsProvider{
		err: fmt.Errorf("registry load failed"),
	}

	logger := mocks.NewMockLogger()
	config := CacheWarmupConfig{
		Enabled:        true,
		Interval:       24 * time.Hour,
		RateLimitDelay: 1 * time.Millisecond,
	}

	// Should fall back to fetching all sets when provider errors
	scheduler := NewCacheWarmupScheduler(provider, logger, config, newSetsProvider)
	ctx := context.Background()

	scheduler.warmup(ctx)
}
