package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
)

var _ intelligence.TrajectoryRepository = (*MockTrajectoryRepository)(nil)

// MockTrajectoryRepository implements intelligence.TrajectoryRepository for
// testing. The default in-memory store is keyed by dh_card_id; each Fn field
// can be set to override behaviour per-test.
type MockTrajectoryRepository struct {
	mu    sync.Mutex
	store map[string][]intelligence.WeeklyBucket

	UpsertFn          func(ctx context.Context, dhCardID string, buckets []intelligence.WeeklyBucket) error
	GetByDHCardIDFn   func(ctx context.Context, dhCardID string) ([]intelligence.WeeklyBucket, error)
	LatestWeekStartFn func(ctx context.Context, dhCardID string) (time.Time, error)
}

// NewMockTrajectoryRepository constructs a MockTrajectoryRepository with an
// initialised in-memory store.
func NewMockTrajectoryRepository() *MockTrajectoryRepository {
	return &MockTrajectoryRepository{
		store: make(map[string][]intelligence.WeeklyBucket),
	}
}

func (m *MockTrajectoryRepository) Upsert(ctx context.Context, dhCardID string, buckets []intelligence.WeeklyBucket) error {
	if m.UpsertFn != nil {
		return m.UpsertFn(ctx, dhCardID, buckets)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Merge-overwrite by week_start.
	existing := append([]intelligence.WeeklyBucket(nil), m.store[dhCardID]...)
	for _, nb := range buckets {
		replaced := false
		for i, eb := range existing {
			if eb.WeekStart.Equal(nb.WeekStart) {
				existing[i] = nb
				replaced = true
				break
			}
		}
		if !replaced {
			existing = append(existing, nb)
		}
	}
	m.store[dhCardID] = existing
	return nil
}

func (m *MockTrajectoryRepository) GetByDHCardID(ctx context.Context, dhCardID string) ([]intelligence.WeeklyBucket, error) {
	if m.GetByDHCardIDFn != nil {
		return m.GetByDHCardIDFn(ctx, dhCardID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]intelligence.WeeklyBucket(nil), m.store[dhCardID]...), nil
}

func (m *MockTrajectoryRepository) LatestWeekStart(ctx context.Context, dhCardID string) (time.Time, error) {
	if m.LatestWeekStartFn != nil {
		return m.LatestWeekStartFn(ctx, dhCardID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var latest time.Time
	for _, b := range m.store[dhCardID] {
		if b.WeekStart.After(latest) {
			latest = b.WeekStart
		}
	}
	return latest, nil
}
