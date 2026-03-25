// Package cache provides a disk-backed file cache with LRU eviction and TTL expiration.
package cache

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// Cache is the unified interface for all cache implementations
type Cache interface {
	Get(ctx context.Context, key string, dest any) (bool, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
	Close() error
}

// Config holds configuration for cache creation
type Config struct {
	Type     string        // Cache type ("file")
	TTL      time.Duration // Default TTL
	FilePath string        // Path for file cache

	// File cache specific
	FlushInterval time.Duration // How often to flush to disk
	MaxEntries    int           // Max entries to keep (0 = unlimited)
}

// New creates a new cache based on the configuration
func New(config Config) (Cache, error) {
	switch config.Type {
	case "file", "": // Default to file cache
		if config.FilePath == "" {
			return nil, fmt.Errorf("file cache requires FilePath in config")
		}
		simpleCfg := SimpleCacheConfig{
			FlushInterval: config.FlushInterval,
			MaxEntries:    config.MaxEntries,
		}
		return NewFileCacheBackend(config.FilePath, simpleCfg)

	default:
		return nil, fmt.Errorf("unknown cache type: %s (only 'file' is supported)", config.Type)
	}
}

// Entry represents a single cached value with metadata.
// The lastAccessNano field provides lock-free runtime tracking of last access
// time via atomic operations. LastAccess (time.Time) is kept for JSON
// serialization; it is synced from lastAccessNano before flushing to disk
// and lastAccessNano is initialized from LastAccess when loading from disk.
type Entry struct {
	Data       json.RawMessage `json:"data"`
	Timestamp  time.Time       `json:"timestamp"`
	TTL        time.Duration   `json:"ttl"`
	LastAccess time.Time       `json:"last_access"`

	lastAccessNano atomic.Int64 // runtime lock-free last access tracking
}

// SimpleCacheConfig holds configuration for the simple cache
type SimpleCacheConfig struct {
	FlushInterval time.Duration // How often to flush to disk
	MaxEntries    int           // Max entries to keep (0 = unlimited)
}

// FileCache is the file-based cache implementation.
//
// Mutex discipline:
//   - mu (RWMutex) protects entries, lruList, lruMap, metrics, setCount,
//     lastSweep, and closed. All in-memory state is guarded by this lock.
//   - flushMu (Mutex) serialises concurrent disk flushes. When both locks
//     are needed, flushMu is acquired first, then mu is acquired briefly to
//     snapshot entries and released before performing I/O.
type FileCache struct {
	path          string
	name          string // Cache name for logging
	entries       map[string]*Entry
	lruList       *list.List               // For LRU eviction
	lruMap        map[string]*list.Element // Key -> list element mapping
	mu            sync.RWMutex             // Protects all in-memory state (see doc above)
	flushMu       sync.Mutex               // Serialises disk flushes (see doc above)
	config        SimpleCacheConfig
	logger        observability.Logger // Optional logger for structured logging
	flushTicker   *time.Ticker
	stopChan      chan struct{}
	wg            sync.WaitGroup // For deterministic shutdown
	closed        bool
	setCount      int       // Counter for periodic expired-entry sweeps
	lastSweep     time.Time // Last time expired-entry sweep ran
	sweepInterval time.Duration
}

// Option is a functional option for configuring a FileCache.
type Option func(*FileCache)

// WithLogger configures the cache with a structured logger.
func WithLogger(logger observability.Logger) Option {
	return func(c *FileCache) {
		c.logger = logger
	}
}

// NewFileCacheBackend creates a new file-based cache with functional options.
//
// By default the cache has no logger. Use WithLogger to attach one:
//
//	cache, err := NewFileCacheBackend(path, cfg, WithLogger(logger))
func NewFileCacheBackend(path string, config SimpleCacheConfig, opts ...Option) (Cache, error) {
	// Set defaults
	if config.FlushInterval == 0 {
		config.FlushInterval = 30 * time.Second
	}

	c := &FileCache{
		path:          path,
		name:          "default",
		entries:       make(map[string]*Entry),
		lruList:       list.New(),
		lruMap:        make(map[string]*list.Element),
		config:        config,
		stopChan:      make(chan struct{}),
		lastSweep:     time.Now(),
		sweepInterval: 5 * time.Minute,
	}

	// Apply functional options
	for _, opt := range opts {
		opt(c)
	}

	ctx := context.Background()

	// Load existing cache from disk
	if err := c.loadFromFile(ctx); err != nil {
		return nil, err
	}

	// Start background flush goroutine
	c.flushTicker = time.NewTicker(config.FlushInterval)
	c.wg.Add(1)
	go c.flushLoop()

	return c, nil
}

// Get retrieves a value from the cache.
// Uses RLock for the read path; the last-access timestamp is updated
// atomically without holding any write lock. LRU reordering is deferred
// to a short write-lock section.
func (c *FileCache) Get(ctx context.Context, key string, target any) (bool, error) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	if !ok {
		c.mu.RUnlock()
		return false, nil
	}

	// Check TTL
	expired := entry.TTL > 0 && time.Since(entry.Timestamp) > entry.TTL
	if expired {
		c.mu.RUnlock()
		// Promote to write lock to clean up expired entry
		c.mu.Lock()
		// Re-check under write lock (another goroutine may have already cleaned up)
		if e, stillExists := c.entries[key]; stillExists && e.TTL > 0 && time.Since(e.Timestamp) > e.TTL {
			delete(c.entries, key)
			if elem, exists := c.lruMap[key]; exists {
				c.lruList.Remove(elem)
				delete(c.lruMap, key)
			}
		}
		c.mu.Unlock()
		return false, nil
	}

	// Make a copy of data for unmarshal outside the lock
	dataCopy := make([]byte, len(entry.Data))
	copy(dataCopy, entry.Data)
	c.mu.RUnlock()

	// Update access time atomically — no write lock needed
	entry.lastAccessNano.Store(time.Now().UnixNano())

	// Unmarshal outside lock
	if err := json.Unmarshal(dataCopy, target); err != nil {
		return false, apperrors.CacheCorrupted(c.path, fmt.Errorf("unmarshal cache entry for key %s: %w", key, err))
	}

	// Update LRU ordering under write lock (short critical section)
	c.mu.Lock()
	if elem, exists := c.lruMap[key]; exists {
		c.lruList.MoveToFront(elem)
	}
	c.mu.Unlock()

	return true, nil
}

// Set stores a value in the cache.
func (c *FileCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return apperrors.CacheWriteFailed(key, fmt.Errorf("marshal value: %w", err))
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if cache is closed
	if c.closed {
		return nil
	}

	now := time.Now()
	entry := &Entry{
		Data:       data,
		Timestamp:  now,
		TTL:        ttl,
		LastAccess: now,
	}
	entry.lastAccessNano.Store(now.UnixNano())
	// Write directly to entries
	c.entries[key] = entry

	// Update LRU tracking
	if elem, exists := c.lruMap[key]; exists {
		c.lruList.MoveToFront(elem)
	} else {
		elem := c.lruList.PushFront(key)
		c.lruMap[key] = elem
	}

	// Check if we need to evict
	c.evictIfNecessary(ctx)

	return nil
}

// Delete removes a specific cache entry.
func (c *FileCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	delete(c.entries, key)
	if elem, exists := c.lruMap[key]; exists {
		c.lruList.Remove(elem)
		delete(c.lruMap, key)
	}
	c.mu.Unlock()
	// Don't flush immediately for single removals
	return nil
}

// Clear removes all cache entries and flushes to disk.
func (c *FileCache) Clear(ctx context.Context) error {
	// Check for context cancellation before acquiring lock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mu.Lock()
	c.entries = make(map[string]*Entry)
	c.lruList = list.New()
	c.lruMap = make(map[string]*list.Element)
	c.mu.Unlock()
	return c.Flush(ctx)
}

// Close stops the background flush loop and performs a final flush.
func (c *FileCache) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	// Stop the flush loop
	if c.flushTicker != nil {
		c.flushTicker.Stop()
	}

	close(c.stopChan)

	// Wait for goroutines to complete
	c.wg.Wait()

	return nil
}

// cacheVersion is the global cache schema version.
// Increment this when cache format changes to auto-invalidate old entries.
// This is embedded in all cache keys to ensure stale data is automatically
// evicted when the schema changes.
const cacheVersion = "v3"

// buildKey creates semantic cache keys with version prefix
func buildKey(parts ...string) string {
	// Estimate capacity: len(cacheVersion) + len(parts) separators + sum of part lengths
	capacity := len(cacheVersion) + len(parts)
	for _, part := range parts {
		capacity += len(part)
	}

	var builder strings.Builder
	builder.Grow(capacity)
	builder.WriteString(cacheVersion)
	for _, part := range parts {
		builder.WriteString("|")
		builder.WriteString(part)
	}
	return builder.String()
}

// Common cache keys
func CardsKey(setID string) string {
	return buildKey("cards", "set", setID)
}

func PriceChartingKey(setName, cardName, number string) string {
	return buildKey("pc", setName, cardName, number)
}
