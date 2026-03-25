package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimingStore_Record(t *testing.T) {
	store := NewTimingStore([]string{"/api/test"})
	store.Record("/api/test", 100*time.Millisecond)
	store.Record("/api/test", 200*time.Millisecond)
	store.Record("/api/other", 50*time.Millisecond) // not tracked

	snap := store.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 tracked endpoint, got %d", len(snap))
	}
	stats := snap["/api/test"]
	if stats.Count != 2 {
		t.Errorf("expected count=2, got %d", stats.Count)
	}
	if stats.MaxMs != 200 {
		t.Errorf("expected maxMs=200, got %f", stats.MaxMs)
	}
	if stats.AvgMs != 150 {
		t.Errorf("expected avgMs=150, got %f", stats.AvgMs)
	}
}

func TestTimingStore_P95(t *testing.T) {
	store := NewTimingStore([]string{"/api/test"})
	for i := 1; i <= 100; i++ {
		store.Record("/api/test", time.Duration(i)*time.Millisecond)
	}
	snap := store.Snapshot()
	stats := snap["/api/test"]
	// P95 of 1..100 should be ~95
	if stats.P95Ms < 90 || stats.P95Ms > 100 {
		t.Errorf("expected p95 around 95, got %f", stats.P95Ms)
	}
}

func TestTimingMiddleware(t *testing.T) {
	store := NewTimingStore([]string{"/api/test"})
	handler := TimingMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	snap := store.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 tracked endpoint, got %d", len(snap))
	}
	if snap["/api/test"].Count != 1 {
		t.Errorf("expected count=1, got %d", snap["/api/test"].Count)
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/api/campaigns/abc123/tuning", "/api/campaigns/{id}/tuning"},
		{"/api/campaigns/xyz/sell-sheet", "/api/campaigns/{id}/sell-sheet"},
		{"/api/campaigns/abc123/pnl", "/api/campaigns/{id}/pnl"},
		{"/api/portfolio/insights", "/api/portfolio/insights"},
		{"/api/campaigns", "/api/campaigns"},
		{"/api/campaigns/abc123", "/api/campaigns/abc123"},
	}
	for _, tt := range tests {
		got := normalizePath(tt.path)
		if got != tt.expected {
			t.Errorf("normalizePath(%q) = %q, want %q", tt.path, got, tt.expected)
		}
	}
}

func TestTimingMiddleware_NormalizedPath(t *testing.T) {
	store := NewTimingStore([]string{"/api/campaigns/{id}/tuning"})
	handler := TimingMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/abc123/tuning", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	snap := store.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 tracked endpoint, got %d", len(snap))
	}
	stats, ok := snap["/api/campaigns/{id}/tuning"]
	if !ok {
		t.Fatal("expected stats for /api/campaigns/{id}/tuning")
	}
	if stats.Count != 1 {
		t.Errorf("expected count=1, got %d", stats.Count)
	}
}

func TestTimingStore_Uptime(t *testing.T) {
	store := NewTimingStore(nil)
	time.Sleep(10 * time.Millisecond)
	if store.Uptime() < 10*time.Millisecond {
		t.Error("expected uptime >= 10ms")
	}
}
