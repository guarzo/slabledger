package cache

import (
	"context"
	"sort"
	"time"
)

// expiredSweepInterval controls how often the O(n) expired-entry scan runs.
// Rather than scanning all entries on every Set call, we only sweep every Nth call.
const expiredSweepInterval = 100

// rebuildLRU reconstructs the LRU list sorted by LastAccess (most recent at front).
func (c *FileCache) rebuildLRU() {
	type entryPair struct {
		key   string
		entry *Entry
	}

	pairs := make([]entryPair, 0, len(c.entries))
	for k, v := range c.entries {
		pairs = append(pairs, entryPair{k, v})
	}

	// Sort by lastAccessNano ascending so the oldest entries are inserted first
	// using c.lruList.PushFront. Each successive PushFront displaces the
	// previous entry, so the most recently accessed entries end up at the
	// front of the list (c.lruMap[pair.key] tracks each element).
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].entry.lastAccessNano.Load() < pairs[j].entry.lastAccessNano.Load()
	})

	for _, pair := range pairs {
		elem := c.lruList.PushFront(pair.key)
		c.lruMap[pair.key] = elem
	}
}

// evictIfNecessary removes entries when the cache exceeds MaxEntries and
// periodically sweeps expired entries based on TTL.
// Caller must hold c.mu.
func (c *FileCache) evictIfNecessary(ctx context.Context) {
	// Check entry count limit
	if c.config.MaxEntries > 0 {
		totalEntries := len(c.entries)
		for totalEntries > c.config.MaxEntries && c.lruList.Len() > 0 {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Remove least recently used
			oldest := c.lruList.Back()
			if oldest != nil {
				key, ok := oldest.Value.(string)
				if !ok {
					break // Unexpected type, stop eviction
				}
				delete(c.entries, key)
				delete(c.lruMap, key)
				c.lruList.Remove(oldest)
				totalEntries--
			} else {
				break
			}
		}
	}

	// Clean up expired entries when either the set-count threshold or the
	// time-based sweep interval is reached.  The time-based trigger ensures
	// expired entries are cleaned up even under low-write workloads.
	c.setCount++
	if c.setCount%expiredSweepInterval != 0 && time.Since(c.lastSweep) < c.sweepInterval {
		return
	}

	c.cleanupExpired(ctx)
}

// cleanupExpired removes all TTL-expired entries. Called from evictIfNecessary
// and from the periodic flush loop.
// Caller must hold c.mu.
func (c *FileCache) cleanupExpired(ctx context.Context) {
	now := time.Now()
	for key, entry := range c.entries {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return
		default:
		}

		if entry.TTL > 0 && now.Sub(entry.Timestamp) > entry.TTL {
			delete(c.entries, key)
			if elem, exists := c.lruMap[key]; exists {
				c.lruList.Remove(elem)
				delete(c.lruMap, key)
			}
		}
	}
	c.lastSweep = now
}
