package cache

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// newTestCache creates a FileCache in a temp directory with the given config.
func newTestCache(t *testing.T, cfg SimpleCacheConfig) Cache {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test_cache.json")
	if cfg.FlushInterval == 0 {
		cfg.FlushInterval = 1 * time.Hour // disable auto-flush in tests
	}
	c, err := NewFileCacheBackend(path, cfg)
	if err != nil {
		t.Fatalf("NewFileCacheBackend: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("cache Close: %v", err)
		}
	})
	return c
}

func TestGet_Hit(t *testing.T) {
	c := newTestCache(t, SimpleCacheConfig{})
	ctx := context.Background()

	if err := c.Set(ctx, "key1", "hello", 1*time.Hour); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got string
	found, err := c.Get(ctx, "key1", &got)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected cache hit")
	}
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

func TestGet_Miss(t *testing.T) {
	c := newTestCache(t, SimpleCacheConfig{})
	ctx := context.Background()

	var got string
	found, err := c.Get(ctx, "nonexistent", &got)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatal("expected cache miss")
	}
}

func TestGet_TTLExpiry(t *testing.T) {
	c := newTestCache(t, SimpleCacheConfig{})
	ctx := context.Background()

	// Set with a very short TTL
	if err := c.Set(ctx, "expires", "value", 1*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Wait for expiry
	time.Sleep(5 * time.Millisecond)

	var got string
	found, err := c.Get(ctx, "expires", &got)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatal("expected expired entry to be a cache miss")
	}
}

func TestGet_ZeroTTLNeverExpires(t *testing.T) {
	c := newTestCache(t, SimpleCacheConfig{})
	ctx := context.Background()

	// TTL=0 means no expiration
	if err := c.Set(ctx, "forever", 42, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got int
	found, err := c.Get(ctx, "forever", &got)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found || got != 42 {
		t.Fatalf("expected hit with value 42, got found=%v value=%v", found, got)
	}
}

func TestLRUEviction(t *testing.T) {
	c := newTestCache(t, SimpleCacheConfig{MaxEntries: 3})
	ctx := context.Background()

	// Fill cache to capacity
	for i := 0; i < 3; i++ {
		if err := c.Set(ctx, keyN(i), i, 1*time.Hour); err != nil {
			t.Fatalf("Set(%d): %v", i, err)
		}
	}

	// Access key0 to make it recently used
	var v int
	if _, err := c.Get(ctx, keyN(0), &v); err != nil {
		t.Fatalf("Get(key0): %v", err)
	}

	// Add a 4th entry — should evict the LRU entry (key1, since key0 was just accessed)
	if err := c.Set(ctx, keyN(3), 3, 1*time.Hour); err != nil {
		t.Fatalf("Set(3): %v", err)
	}

	// key1 should be evicted (it was the least recently used)
	found, err := c.Get(ctx, keyN(1), &v)
	if err != nil {
		t.Fatalf("Get(key1): %v", err)
	}
	if found {
		t.Fatal("expected key1 to be evicted")
	}

	// key0 should still be present (accessed recently)
	found, err = c.Get(ctx, keyN(0), &v)
	if err != nil {
		t.Fatalf("Get(key0): %v", err)
	}
	if !found {
		t.Fatal("expected key0 to survive eviction")
	}

	// key3 should be present (just added)
	found, err = c.Get(ctx, keyN(3), &v)
	if err != nil {
		t.Fatalf("Get(key3): %v", err)
	}
	if !found {
		t.Fatal("expected key3 to be present")
	}
}

func TestDelete(t *testing.T) {
	c := newTestCache(t, SimpleCacheConfig{})
	ctx := context.Background()

	if err := c.Set(ctx, "del", "value", 1*time.Hour); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := c.Delete(ctx, "del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var got string
	found, err := c.Get(ctx, "del", &got)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if found {
		t.Fatal("expected deleted key to be a miss")
	}
}

func TestClear(t *testing.T) {
	c := newTestCache(t, SimpleCacheConfig{})
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := c.Set(ctx, keyN(i), i, 1*time.Hour); err != nil {
			t.Fatalf("Set failed for key %v: %v", keyN(i), err)
		}
	}

	if err := c.Clear(ctx); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	for i := 0; i < 5; i++ {
		var v int
		found, err := c.Get(ctx, keyN(i), &v)
		if err != nil {
			t.Fatalf("Get(key%d): %v", i, err)
		}
		if found {
			t.Fatalf("expected key%d to be cleared", i)
		}
	}
}

func TestSet_OverwriteExisting(t *testing.T) {
	c := newTestCache(t, SimpleCacheConfig{})
	ctx := context.Background()

	if err := c.Set(ctx, "key", "v1", 1*time.Hour); err != nil {
		t.Fatalf("Set v1: %v", err)
	}
	if err := c.Set(ctx, "key", "v2", 1*time.Hour); err != nil {
		t.Fatalf("Set v2: %v", err)
	}

	var got string
	found, err := c.Get(ctx, "key", &got)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found || got != "v2" {
		t.Fatalf("expected v2, got found=%v value=%q", found, got)
	}
}

func TestCleanupExpired(t *testing.T) {
	c := newTestCache(t, SimpleCacheConfig{MaxEntries: 1000})
	ctx := context.Background()

	// Add entries with short TTL
	for i := 0; i < 5; i++ {
		if err := c.Set(ctx, keyN(i), i, 1*time.Millisecond); err != nil {
			t.Fatalf("Set(key%d): %v", i, err)
		}
	}
	// Add a non-expiring entry
	if err := c.Set(ctx, "keep", "alive", 1*time.Hour); err != nil {
		t.Fatalf("Set(keep): %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	// Verify expired entries are not accessible via public API
	for i := 0; i < 5; i++ {
		var v int
		found, err := c.Get(ctx, keyN(i), &v)
		if err != nil {
			t.Fatalf("Get(key%d): %v", i, err)
		}
		if found {
			t.Fatalf("expected key%d to be expired", i)
		}
	}

	var got string
	found, err := c.Get(ctx, "keep", &got)
	if err != nil {
		t.Fatalf("Get(keep): %v", err)
	}
	if !found || got != "alive" {
		t.Fatal("expected 'keep' entry to survive cleanup")
	}
}

func TestStructValue(t *testing.T) {
	type data struct {
		Name  string
		Count int
	}
	c := newTestCache(t, SimpleCacheConfig{})
	ctx := context.Background()

	if err := c.Set(ctx, "struct", data{Name: "test", Count: 42}, 1*time.Hour); err != nil {
		t.Fatalf("Set struct: %v", err)
	}

	var got data
	found, err := c.Get(ctx, "struct", &got)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found || got.Name != "test" || got.Count != 42 {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func keyN(n int) string {
	return "key" + strconv.Itoa(n)
}
