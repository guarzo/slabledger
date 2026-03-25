package scheduler

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/platform/config"
)

// --- mocks ---

type mockInventoryLister struct {
	listFn func(ctx context.Context) ([]InventoryPurchase, error)
}

func (m *mockInventoryLister) ListUnsoldInventory(ctx context.Context) ([]InventoryPurchase, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

type mockSnapshotRefresher struct {
	refreshFn func(ctx context.Context, p InventoryPurchase) bool
}

func (m *mockSnapshotRefresher) RefreshSnapshot(ctx context.Context, p InventoryPurchase) bool {
	if m.refreshFn != nil {
		return m.refreshFn(ctx, p)
	}
	return true
}

func newTestScheduler(
	lister InventoryLister,
	refresher SnapshotRefresher,
	cfg config.InventoryRefreshConfig,
) *InventoryRefreshScheduler {
	return NewInventoryRefreshScheduler(lister, refresher, nopLogger{}, cfg)
}

// --- filterStale tests ---

func TestFilterStale_EmptyCardName(t *testing.T) {
	s := newTestScheduler(nil, nil, config.InventoryRefreshConfig{
		Enabled:        true,
		StaleThreshold: 12 * time.Hour,
	})

	purchases := []InventoryPurchase{
		{ID: "1", CardName: "", GradeValue: 9.5, SnapshotDate: ""},
	}
	result := s.filterStale(context.Background(), purchases)
	if len(result) != 0 {
		t.Errorf("expected 0 results for empty CardName, got %d", len(result))
	}
}

func TestFilterStale_ZeroGradeValue(t *testing.T) {
	s := newTestScheduler(nil, nil, config.InventoryRefreshConfig{
		Enabled:        true,
		StaleThreshold: 12 * time.Hour,
	})

	purchases := []InventoryPurchase{
		{ID: "1", CardName: "Charizard", GradeValue: 0, SnapshotDate: ""},
	}
	result := s.filterStale(context.Background(), purchases)
	if len(result) != 0 {
		t.Errorf("expected 0 results for zero GradeValue, got %d", len(result))
	}
}

func TestFilterStale_NegativeGradeValue(t *testing.T) {
	s := newTestScheduler(nil, nil, config.InventoryRefreshConfig{
		Enabled:        true,
		StaleThreshold: 12 * time.Hour,
	})

	purchases := []InventoryPurchase{
		{ID: "1", CardName: "Charizard", GradeValue: -1, SnapshotDate: ""},
	}
	result := s.filterStale(context.Background(), purchases)
	if len(result) != 0 {
		t.Errorf("expected 0 results for negative GradeValue, got %d", len(result))
	}
}

func TestFilterStale_EmptySnapshotDate(t *testing.T) {
	s := newTestScheduler(nil, nil, config.InventoryRefreshConfig{
		Enabled:        true,
		StaleThreshold: 12 * time.Hour,
	})

	purchases := []InventoryPurchase{
		{ID: "1", CardName: "Charizard", GradeValue: 9.5, SnapshotDate: ""},
	}
	result := s.filterStale(context.Background(), purchases)
	if len(result) != 1 {
		t.Errorf("expected 1 result for empty SnapshotDate (never captured), got %d", len(result))
	}
}

func TestFilterStale_UnparseableDate(t *testing.T) {
	s := newTestScheduler(nil, nil, config.InventoryRefreshConfig{
		Enabled:        true,
		StaleThreshold: 12 * time.Hour,
	})

	purchases := []InventoryPurchase{
		{ID: "1", CardName: "Charizard", GradeValue: 9.5, SnapshotDate: "not-a-date"},
	}
	result := s.filterStale(context.Background(), purchases)
	if len(result) != 1 {
		t.Errorf("expected 1 result for unparseable date (treated as stale), got %d", len(result))
	}
}

func TestFilterStale_StaleDate(t *testing.T) {
	s := newTestScheduler(nil, nil, config.InventoryRefreshConfig{
		Enabled:        true,
		StaleThreshold: 12 * time.Hour,
	})

	// A date from 2 days ago should be stale
	staleDate := time.Now().Add(-48 * time.Hour).Format("2006-01-02")
	purchases := []InventoryPurchase{
		{ID: "1", CardName: "Charizard", GradeValue: 9.5, SnapshotDate: staleDate},
	}
	result := s.filterStale(context.Background(), purchases)
	if len(result) != 1 {
		t.Errorf("expected 1 result for stale date, got %d", len(result))
	}
}

func TestFilterStale_FreshDate(t *testing.T) {
	s := newTestScheduler(nil, nil, config.InventoryRefreshConfig{
		Enabled:        true,
		StaleThreshold: 48 * time.Hour, // Use a large threshold so today is definitely fresh
	})

	// Today's date will be well within a 48-hour threshold
	freshDate := time.Now().Format("2006-01-02")
	purchases := []InventoryPurchase{
		{ID: "1", CardName: "Charizard", GradeValue: 9.5, SnapshotDate: freshDate},
	}
	result := s.filterStale(context.Background(), purchases)
	if len(result) != 0 {
		t.Errorf("expected 0 results for fresh date, got %d", len(result))
	}
}

func TestFilterStale_MixedBatch(t *testing.T) {
	s := newTestScheduler(nil, nil, config.InventoryRefreshConfig{
		Enabled:        true,
		StaleThreshold: 48 * time.Hour, // Use 48h threshold so today is fresh but 5-days-ago is stale
	})

	staleDate := time.Now().Add(-120 * time.Hour).Format("2006-01-02") // 5 days ago
	freshDate := time.Now().Format("2006-01-02")

	purchases := []InventoryPurchase{
		{ID: "1", CardName: "", GradeValue: 9.5, SnapshotDate: ""},                         // excluded: empty name
		{ID: "2", CardName: "Charizard", GradeValue: 0, SnapshotDate: ""},                  // excluded: zero grade
		{ID: "3", CardName: "Charizard", GradeValue: 9.5, SnapshotDate: ""},                // included: never captured
		{ID: "4", CardName: "Charizard", GradeValue: 9.5, SnapshotDate: staleDate},         // included: stale
		{ID: "5", CardName: "Charizard", GradeValue: 9.5, SnapshotDate: freshDate},         // excluded: fresh
		{ID: "6", CardName: "Charizard", GradeValue: 9.5, SnapshotDate: "not-a-real-date"}, // included: unparseable
	}
	result := s.filterStale(context.Background(), purchases)
	if len(result) != 3 {
		t.Errorf("expected 3 stale items, got %d", len(result))
	}
}

// --- refreshBatch tests ---

func TestRefreshBatch_ListerError(t *testing.T) {
	var refreshCalled bool
	lister := &mockInventoryLister{
		listFn: func(_ context.Context) ([]InventoryPurchase, error) {
			return nil, context.DeadlineExceeded
		},
	}
	refresher := &mockSnapshotRefresher{
		refreshFn: func(_ context.Context, _ InventoryPurchase) bool {
			refreshCalled = true
			return true
		},
	}

	s := newTestScheduler(lister, refresher, config.InventoryRefreshConfig{
		Enabled:        true,
		StaleThreshold: 12 * time.Hour,
		BatchSize:      10,
		BatchDelay:     1 * time.Millisecond,
	})

	s.refreshBatch(context.Background())

	if refreshCalled {
		t.Error("refresher should not be called when lister returns error")
	}
}

func TestRefreshBatch_NoStaleItems(t *testing.T) {
	var refreshCalled bool
	freshDate := time.Now().Format("2006-01-02")
	lister := &mockInventoryLister{
		listFn: func(_ context.Context) ([]InventoryPurchase, error) {
			return []InventoryPurchase{
				{ID: "1", CardName: "Charizard", GradeValue: 9.5, SnapshotDate: freshDate},
			}, nil
		},
	}
	refresher := &mockSnapshotRefresher{
		refreshFn: func(_ context.Context, _ InventoryPurchase) bool {
			refreshCalled = true
			return true
		},
	}

	s := newTestScheduler(lister, refresher, config.InventoryRefreshConfig{
		Enabled:        true,
		StaleThreshold: 48 * time.Hour, // Large threshold so today is fresh
		BatchSize:      10,
		BatchDelay:     1 * time.Millisecond,
	})

	s.refreshBatch(context.Background())

	if refreshCalled {
		t.Error("refresher should not be called when no items are stale")
	}
}

func TestRefreshBatch_BatchSizeCap(t *testing.T) {
	var callCount atomic.Int32
	lister := &mockInventoryLister{
		listFn: func(_ context.Context) ([]InventoryPurchase, error) {
			purchases := make([]InventoryPurchase, 30)
			for i := range purchases {
				purchases[i] = InventoryPurchase{
					ID: string(rune('A' + i)), CardName: fmt.Sprintf("Card%d", i), GradeValue: 9.5, SnapshotDate: "",
					BuyCostCents: i * 100,
				}
			}
			return purchases, nil
		},
	}
	refresher := &mockSnapshotRefresher{
		refreshFn: func(_ context.Context, _ InventoryPurchase) bool {
			callCount.Add(1)
			return true
		},
	}

	s := newTestScheduler(lister, refresher, config.InventoryRefreshConfig{
		Enabled:        true,
		StaleThreshold: 12 * time.Hour,
		BatchSize:      5,
		BatchDelay:     1 * time.Millisecond,
	})

	s.refreshBatch(context.Background())

	if c := callCount.Load(); c != 5 {
		t.Errorf("expected 5 refresh calls (batch size cap), got %d", c)
	}
}

func TestRefreshBatch_ValuePriority(t *testing.T) {
	var firstID string
	lister := &mockInventoryLister{
		listFn: func(_ context.Context) ([]InventoryPurchase, error) {
			return []InventoryPurchase{
				{ID: "low", CardName: "Pikachu", GradeValue: 9.5, SnapshotDate: "", BuyCostCents: 100},
				{ID: "high", CardName: "Charizard", GradeValue: 9.5, SnapshotDate: "", BuyCostCents: 50000},
				{ID: "mid", CardName: "Umbreon", GradeValue: 9.5, SnapshotDate: "", BuyCostCents: 5000},
			}, nil
		},
	}
	refresher := &mockSnapshotRefresher{
		refreshFn: func(_ context.Context, p InventoryPurchase) bool {
			if firstID == "" {
				firstID = p.ID
			}
			return true
		},
	}

	s := newTestScheduler(lister, refresher, config.InventoryRefreshConfig{
		Enabled:        true,
		StaleThreshold: 12 * time.Hour,
		BatchSize:      10,
		BatchDelay:     1 * time.Millisecond,
	})

	s.refreshBatch(context.Background())

	if firstID != "high" {
		t.Errorf("expected highest-value item first (high), got %s", firstID)
	}
}

func TestRefreshBatch_RefreshesAllPurchasesIncludingDuplicateCards(t *testing.T) {
	var callCount atomic.Int32
	var refreshedIDs []string
	lister := &mockInventoryLister{
		listFn: func(_ context.Context) ([]InventoryPurchase, error) {
			return []InventoryPurchase{
				// Three copies of the same card (same name+set+number)
				{ID: "umb1", CardName: "Umbreon Ex", SetName: "SV Promo", CardNumber: "176", GradeValue: 10, SnapshotDate: "", BuyCostCents: 5000},
				{ID: "umb2", CardName: "Umbreon Ex", SetName: "SV Promo", CardNumber: "176", GradeValue: 9, SnapshotDate: "", BuyCostCents: 3000},
				{ID: "umb3", CardName: "Umbreon Ex", SetName: "SV Promo", CardNumber: "176", GradeValue: 10, SnapshotDate: "", BuyCostCents: 2000},
				// A different card
				{ID: "char1", CardName: "Charizard", SetName: "Base Set", CardNumber: "4", GradeValue: 9, SnapshotDate: "", BuyCostCents: 90000},
			}, nil
		},
	}
	refresher := &mockSnapshotRefresher{
		refreshFn: func(_ context.Context, p InventoryPurchase) bool {
			callCount.Add(1)
			refreshedIDs = append(refreshedIDs, p.ID)
			return true
		},
	}

	s := newTestScheduler(lister, refresher, config.InventoryRefreshConfig{
		Enabled:        true,
		StaleThreshold: 12 * time.Hour,
		BatchSize:      10,
		BatchDelay:     1 * time.Millisecond,
	})

	s.refreshBatch(context.Background())

	// Every purchase gets its own RefreshSnapshot call (per-purchase snapshot).
	// Duplicate price lookups are coalesced at the fusion provider layer.
	if c := callCount.Load(); c != 4 {
		t.Errorf("expected 4 refresh calls (all purchases), got %d", c)
	}
	// Highest value (Charizard 90000) should be first due to value-descending sort.
	if len(refreshedIDs) >= 1 && refreshedIDs[0] != "char1" {
		t.Errorf("expected char1 first (highest value), got %s", refreshedIDs[0])
	}
}

// --- Start tests ---

func TestStart_DisabledConfig(t *testing.T) {
	var listerCalled bool
	lister := &mockInventoryLister{
		listFn: func(_ context.Context) ([]InventoryPurchase, error) {
			listerCalled = true
			return nil, nil
		},
	}

	s := newTestScheduler(lister, &mockSnapshotRefresher{}, config.InventoryRefreshConfig{
		Enabled: false,
	})

	done := make(chan struct{})
	go func() {
		s.Start(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// expected — returns immediately when disabled
	case <-time.After(1 * time.Second):
		t.Fatal("Start should return immediately when disabled")
	}

	if listerCalled {
		t.Error("lister should not be called when scheduler is disabled")
	}
}

// --- Constructor defaults ---

func TestNewInventoryRefreshScheduler_Defaults(t *testing.T) {
	s := NewInventoryRefreshScheduler(nil, nil, nopLogger{}, config.InventoryRefreshConfig{
		Enabled: true,
		// All zero values
	})

	if s.config.Interval != 1*time.Hour {
		t.Errorf("expected default interval 1h, got %v", s.config.Interval)
	}
	if s.config.StaleThreshold != 12*time.Hour {
		t.Errorf("expected default stale threshold 12h, got %v", s.config.StaleThreshold)
	}
	if s.config.BatchSize != 20 {
		t.Errorf("expected default batch size 20, got %d", s.config.BatchSize)
	}
	if s.config.BatchDelay != 2*time.Second {
		t.Errorf("expected default batch delay 2s, got %v", s.config.BatchDelay)
	}
}
