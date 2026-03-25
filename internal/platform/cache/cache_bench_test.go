package cache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// BenchmarkCache_Get_Hit measures cache hit performance
func BenchmarkCache_Get_Hit(b *testing.B) {
	tmpDir := b.TempDir()
	cachePath := filepath.Join(tmpDir, "bench_cache.json")

	cache, err := NewFileCacheBackend(cachePath, SimpleCacheConfig{
		FlushInterval: 1 * time.Hour, // Don't flush during benchmark
		MaxEntries:    100000,
	})
	if err != nil {
		b.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	// Pre-populate cache
	ctx := context.Background()
	err = cache.Set(ctx, "test:key", "test_value", 1*time.Hour)
	if err != nil {
		b.Fatalf("failed to set cache: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var result string
		_, _ = cache.Get(ctx, "test:key", &result)
	}
}

// BenchmarkCache_Get_Miss measures cache miss performance
func BenchmarkCache_Get_Miss(b *testing.B) {
	tmpDir := b.TempDir()
	cachePath := filepath.Join(tmpDir, "bench_cache.json")

	cache, err := NewFileCacheBackend(cachePath, SimpleCacheConfig{
		FlushInterval: 1 * time.Hour,
	})
	if err != nil {
		b.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var result string
		_, _ = cache.Get(ctx, "nonexistent:key", &result)
	}
}

// BenchmarkCache_Set_NoEviction measures set performance without eviction
func BenchmarkCache_Set_NoEviction(b *testing.B) {
	tmpDir := b.TempDir()
	cachePath := filepath.Join(tmpDir, "bench_cache.json")

	cache, err := NewFileCacheBackend(cachePath, SimpleCacheConfig{
		FlushInterval: 1 * time.Hour, // Don't flush during benchmark
		MaxEntries:    100000,        // Large enough to avoid eviction
	})
	if err != nil {
		b.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = cache.Set(ctx, fmt.Sprintf("key:%d", i), "value", 1*time.Hour)
	}
}

// BenchmarkCache_Set_WithEviction measures set performance with eviction
func BenchmarkCache_Set_WithEviction(b *testing.B) {
	tmpDir := b.TempDir()
	cachePath := filepath.Join(tmpDir, "bench_cache.json")

	cache, err := NewFileCacheBackend(cachePath, SimpleCacheConfig{
		FlushInterval: 1 * time.Hour,
		MaxEntries:    100, // Small limit to trigger eviction
	})
	if err != nil {
		b.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = cache.Set(ctx, fmt.Sprintf("key:%d", i), "value", 1*time.Hour)
	}
}

// BenchmarkCache_Flush measures flush performance
func BenchmarkCache_Flush(b *testing.B) {
	tmpDir := b.TempDir()
	cachePath := filepath.Join(tmpDir, "bench_cache.json")

	cache, err := NewFileCacheBackend(cachePath, SimpleCacheConfig{
		FlushInterval: 1 * time.Hour, // Don't auto-flush during benchmark
	})
	if err != nil {
		b.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Add 1000 entries
	for i := 0; i < 1000; i++ {
		_ = cache.Set(ctx, fmt.Sprintf("key:%d", i), "value", 1*time.Hour)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Cast to FileCache to access Flush method
		if fc, ok := cache.(*FileCache); ok {
			_ = fc.Flush(ctx)
		}

		// Add more entries for next iteration
		for j := 0; j < 10; j++ {
			_ = cache.Set(ctx, fmt.Sprintf("key:%d:%d", i, j), "value", 1*time.Hour)
		}
	}
}

// BenchmarkCache_ConcurrentAccess measures concurrent read/write performance
func BenchmarkCache_ConcurrentAccess(b *testing.B) {
	tmpDir := b.TempDir()
	cachePath := filepath.Join(tmpDir, "bench_cache.json")

	cache, err := NewFileCacheBackend(cachePath, SimpleCacheConfig{
		FlushInterval: 1 * time.Hour,
		MaxEntries:    1000,
	})
	if err != nil {
		b.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key:%d", i%100)
			if i%2 == 0 {
				_ = cache.Set(ctx, key, "value", 1*time.Hour)
			} else {
				var result string
				_, _ = cache.Get(ctx, key, &result)
			}
			i++
		}
	})
}

// Cleanup benchmark cache files
func TestMain(m *testing.M) {
	code := m.Run()
	// Cleanup any leftover temp files
	os.Exit(code)
}
