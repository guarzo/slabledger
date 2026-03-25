package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// loadFromFile reads cache entries from the JSON file on disk.
// If the file does not exist, the entries map is left empty.
// If the file is corrupt, it logs a warning and starts fresh.
func (c *FileCache) loadFromFile(ctx context.Context) error {
	// Check for context cancellation before reading file
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist; start with empty cache
			return nil
		}
		return fmt.Errorf("read cache: %w", err)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &c.entries); err != nil {
			// Log corrupt cache and start fresh
			if c.logger != nil {
				c.logger.Warn(ctx, "cache file corrupt, starting fresh",
					observability.String("cache_name", c.name),
					observability.String("path", c.path),
					observability.Err(err))
			}
			c.entries = make(map[string]*Entry)
		} else {
			// Initialize lastAccessNano from persisted LastAccess times
			for _, entry := range c.entries {
				entry.lastAccessNano.Store(entry.LastAccess.UnixNano())
			}
			// Rebuild LRU list from loaded entries
			c.rebuildLRU()
		}
	}
	return nil
}

// Flush writes entries to disk using an atomic write (temp file + rename).
// The flushMu mutex serialises concurrent flushes while allowing reads to
// proceed under the separate mu RWMutex.
func (c *FileCache) Flush(ctx context.Context) error {
	// Check for context cancellation before acquiring locks
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.flushMu.Lock()
	defer c.flushMu.Unlock()

	c.mu.Lock()
	// Sync atomic last-access times back to the serializable field
	for _, entry := range c.entries {
		entry.LastAccess = time.Unix(0, entry.lastAccessNano.Load())
	}
	// Take a snapshot for writing (compact JSON for performance)
	data, err := json.Marshal(c.entries)
	c.mu.Unlock()

	if err != nil {
		return apperrors.CacheWriteFailed("*", fmt.Errorf("marshal cache: %w", err))
	}

	// Check for context cancellation before file I/O
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Create parent directory if needed
	dir := filepath.Dir(c.path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return apperrors.CacheWriteFailed("*", fmt.Errorf("create cache dir: %w", err))
		}
	}

	// Atomic write: write to temp file then rename to avoid partial writes on crash
	tmpFile, err := os.CreateTemp(dir, ".cache-*.tmp")
	if err != nil {
		return apperrors.CacheWriteFailed("*", fmt.Errorf("create temp file: %w", err))
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()    //nolint:errcheck // best-effort cleanup
		_ = os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup
		return apperrors.CacheWriteFailed("*", fmt.Errorf("write temp cache file: %w", err))
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup
		return apperrors.CacheWriteFailed("*", fmt.Errorf("close temp cache file: %w", err))
	}
	if err := os.Rename(tmpPath, c.path); err != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup
		return apperrors.CacheWriteFailed("*", fmt.Errorf("rename temp cache file: %w", err))
	}

	return nil
}

// flushLoop runs periodic flushes in the background.
// It also triggers expired-entry sweeps on each tick so that low-write
// caches stay clean.
func (c *FileCache) flushLoop() {
	defer c.wg.Done()
	// Use background context for the flush loop since it runs in background
	ctx := context.Background()
	for {
		select {
		case <-c.flushTicker.C:
			// Sweep expired entries on each tick so low-write caches stay clean
			c.mu.Lock()
			if time.Since(c.lastSweep) >= c.sweepInterval {
				c.cleanupExpired(ctx)
			}
			c.mu.Unlock()

			if err := c.Flush(ctx); err != nil {
				if c.logger != nil {
					c.logger.Error(ctx, "periodic cache flush failed",
						observability.String("cache_name", c.name),
						observability.String("path", c.path),
						observability.Err(err))
				}
			}
		case <-c.stopChan:
			// Final flush before shutdown
			if err := c.Flush(ctx); err != nil {
				if c.logger != nil {
					c.logger.Error(ctx, "final cache flush failed",
						observability.String("cache_name", c.name),
						observability.String("path", c.path),
						observability.Err(err))
				}
			}
			return
		}
	}
}
