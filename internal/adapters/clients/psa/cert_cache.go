package psa

import (
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// A single cert is looked up three times on the way through intake: once when
// the operator scans it (ResolveCert), once when they press Import
// (importNewCert), and once again in background enrichment. PSA data for a
// graded slab is immutable — the grade and subject on a cert do not change —
// so two of those three calls buy nothing and cost quota we are chronically
// short of (SLA-108). The cache collapses them.
//
// The TTL only has to span scan → import → enrich, which is minutes. It is
// deliberately short rather than indefinite so a cert corrected on PSA's side
// is picked up on the next day's touch without a restart.
const (
	certCacheTTL     = 30 * time.Minute
	certCacheMaxSize = 512
)

type certCacheEntry struct {
	info     *inventory.CertInfo
	expires  time.Time
	inserted time.Time
}

// certCache is a small TTL cache of successful PSA cert lookups.
//
// Only successes are cached. Caching failures would turn one transient PSA
// outage into a TTL-long one for every cert it touched, which is the exact
// failure mode this whole change set exists to remove.
type certCache struct {
	mu      sync.Mutex
	entries map[string]certCacheEntry
	now     func() time.Time // injectable for tests
}

func newCertCache() *certCache {
	return &certCache{
		entries: make(map[string]certCacheEntry),
		now:     time.Now,
	}
}

func (c *certCache) get(certNumber string) (*inventory.CertInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[certNumber]
	if !ok {
		return nil, false
	}
	if c.now().After(e.expires) {
		delete(c.entries, certNumber)
		return nil, false
	}
	// Hand out a copy. The cached value outlives any one caller, and
	// CertInfo is a plain struct callers are free to adjust locally.
	info := *e.info
	return &info, true
}

func (c *certCache) put(certNumber string, info *inventory.CertInfo) {
	if info == nil {
		return
	}
	stored := *info
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if len(c.entries) >= certCacheMaxSize {
		c.evictLocked(now)
	}
	c.entries[certNumber] = certCacheEntry{
		info:     &stored,
		expires:  now.Add(certCacheTTL),
		inserted: now,
	}
}

// evictLocked drops expired entries, and if that frees nothing, the oldest
// entry. Caller must hold mu.
func (c *certCache) evictLocked(now time.Time) {
	freed := false
	for k, e := range c.entries {
		if now.After(e.expires) {
			delete(c.entries, k)
			freed = true
		}
	}
	if freed {
		return
	}
	var oldestKey string
	var oldest time.Time
	for k, e := range c.entries {
		if oldestKey == "" || e.inserted.Before(oldest) {
			oldestKey, oldest = k, e.inserted
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
