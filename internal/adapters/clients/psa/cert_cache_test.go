package psa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestCertCache_HitMissAndExpiry(t *testing.T) {
	base := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		advance time.Duration
		wantHit bool
	}{
		{"fresh entry hits", 0, true},
		{"just before TTL hits", certCacheTTL - time.Second, true},
		{"exactly at TTL hits", certCacheTTL, true},
		{"past TTL misses", certCacheTTL + time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := base
			c := newCertCache()
			c.now = func() time.Time { return now }

			c.put("123", &inventory.CertInfo{CertNumber: "123", CardName: "Umbreon VMAX"})
			now = base.Add(tt.advance)

			got, ok := c.get("123")
			if ok != tt.wantHit {
				t.Fatalf("get ok = %v, want %v", ok, tt.wantHit)
			}
			if tt.wantHit && got.CardName != "Umbreon VMAX" {
				t.Errorf("cached CardName = %q, want %q", got.CardName, "Umbreon VMAX")
			}
		})
	}
}

func TestCertCache_MissOnUnknownCert(t *testing.T) {
	c := newCertCache()
	if _, ok := c.get("nope"); ok {
		t.Error("expected miss for a cert never inserted")
	}
}

func TestCertCache_NilInfoNotStored(t *testing.T) {
	c := newCertCache()
	c.put("123", nil)
	if _, ok := c.get("123"); ok {
		t.Error("nil CertInfo should not be cached")
	}
}

// The cache hands out copies so one caller's local edit cannot corrupt what a
// later caller reads.
func TestCertCache_ReturnsCopy(t *testing.T) {
	c := newCertCache()
	original := &inventory.CertInfo{CertNumber: "123", CardName: "Umbreon VMAX"}
	c.put("123", original)

	original.CardName = "mutated after put"

	first, _ := c.get("123")
	first.CardName = "mutated after get"

	second, ok := c.get("123")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if second.CardName != "Umbreon VMAX" {
		t.Errorf("cached value was corrupted: got %q, want %q", second.CardName, "Umbreon VMAX")
	}
}

func TestCertCache_EvictsWhenFull(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	c := newCertCache()
	c.now = func() time.Time { return now }

	for i := range certCacheMaxSize + 10 {
		c.put(string(rune('a'+i%26))+string(rune('0'+i/26)), &inventory.CertInfo{CertNumber: "x"})
		now = now.Add(time.Millisecond)
	}

	c.mu.Lock()
	size := len(c.entries)
	c.mu.Unlock()
	if size > certCacheMaxSize {
		t.Errorf("cache grew to %d entries, cap is %d", size, certCacheMaxSize)
	}
}

// Expired entries are the preferred eviction victims — a full cache of stale
// entries should not start dropping fresh ones.
func TestCertCache_EvictsExpiredBeforeOldest(t *testing.T) {
	base := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	now := base
	c := newCertCache()
	c.now = func() time.Time { return now }

	c.put("stale", &inventory.CertInfo{CertNumber: "stale"})
	now = base.Add(certCacheTTL + time.Minute)
	c.put("fresh", &inventory.CertInfo{CertNumber: "fresh"})

	// Force the eviction path without waiting for 512 real inserts.
	c.mu.Lock()
	c.evictLocked(now)
	_, staleStillThere := c.entries["stale"]
	_, freshStillThere := c.entries["fresh"]
	c.mu.Unlock()

	if staleStillThere {
		t.Error("expired entry should have been evicted")
	}
	if !freshStillThere {
		t.Error("unexpired entry should have survived eviction")
	}
}

// The scan → import → enrich path looks the same cert up three times. Only the
// first should reach PSA (SLA-108).
func TestCertAdapter_RepeatLookupsHitCacheNotPSA(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"PSACert":{"CertNumber":"91234567","CardGrade":"GEM MT 10","Subject":"Umbreon VMAX","Category":"EVOLVING SKIES"}}`))
	}))
	defer server.Close()

	adapter := NewCertAdapter(newTestClient(t, server.URL))
	ctx := context.Background()

	for i := range 3 {
		info, err := adapter.LookupCert(ctx, "91234567")
		if err != nil {
			t.Fatalf("lookup %d: %v", i+1, err)
		}
		if info.CertNumber != "91234567" {
			t.Errorf("lookup %d: CertNumber = %q", i+1, info.CertNumber)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("PSA was called %d times for 3 lookups of the same cert, want 1", got)
	}
}

// A failed lookup must not be cached — otherwise one transient PSA blip
// becomes a TTL-long outage for every cert it touched.
func TestCertAdapter_FailureNotCached(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if failing.Load() {
			// 404 is classified non-retryable, so httpx's retry loop does not
			// paper over the failure the way a 500 would.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"PSACert":{"CertNumber":"91234567","CardGrade":"GEM MT 10"}}`))
	}))
	defer server.Close()

	adapter := NewCertAdapter(newTestClient(t, server.URL))
	ctx := context.Background()

	if _, err := adapter.LookupCert(ctx, "91234567"); err == nil {
		t.Fatal("expected first lookup to fail")
	}

	failing.Store(false)
	if _, err := adapter.LookupCert(ctx, "91234567"); err != nil {
		t.Fatalf("expected retry after failure to reach PSA and succeed, got %v", err)
	}
}

// Distinct certs must not share a cache slot.
func TestCertAdapter_DistinctCertsNotConflated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cert := r.URL.Path[len(r.URL.Path)-8:]
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"PSACert":{"CertNumber":"` + cert + `","CardGrade":"GEM MT 10"}}`))
	}))
	defer server.Close()

	adapter := NewCertAdapter(newTestClient(t, server.URL))
	ctx := context.Background()

	first, err := adapter.LookupCert(ctx, "11111111")
	if err != nil {
		t.Fatalf("first lookup: %v", err)
	}
	second, err := adapter.LookupCert(ctx, "22222222")
	if err != nil {
		t.Fatalf("second lookup: %v", err)
	}

	if first.CertNumber != "11111111" || second.CertNumber != "22222222" {
		t.Errorf("certs conflated: got %q and %q", first.CertNumber, second.CertNumber)
	}
}
