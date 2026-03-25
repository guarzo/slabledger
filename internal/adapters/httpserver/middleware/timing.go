package middleware

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"
)

// ringBufferSize is the number of recent samples kept per endpoint for p95 calculation.
const ringBufferSize = 1000

// EndpointStats holds timing stats for a single endpoint.
type EndpointStats struct {
	Count   int64   `json:"count"`
	TotalMs float64 `json:"totalMs"`
	AvgMs   float64 `json:"avgMs"`
	MaxMs   float64 `json:"maxMs"`
	P95Ms   float64 `json:"p95Ms"`
}

// TimingStore records per-endpoint response times.
type TimingStore struct {
	mu       sync.RWMutex
	stats    map[string]*endpointData
	upSince  time.Time
	patterns map[string]bool
}

type endpointData struct {
	count   int64
	totalMs float64
	maxMs   float64
	samples []float64 // ring buffer for p95
	head    int       // next write position in ring buffer
	size    int       // number of valid entries in ring buffer
}

// NewTimingStore creates a TimingStore that tracks the given endpoint patterns.
func NewTimingStore(patterns []string) *TimingStore {
	pm := make(map[string]bool, len(patterns))
	for _, p := range patterns {
		pm[p] = true
	}
	return &TimingStore{
		stats:    make(map[string]*endpointData),
		upSince:  time.Now(),
		patterns: pm,
	}
}

var campaignSubRouteRe = regexp.MustCompile(`^/api/campaigns/[^/]+/(.+)$`)

// normalizePath replaces campaign IDs with {id} so parameterized routes
// match their tracked pattern.
func normalizePath(path string) string {
	if m := campaignSubRouteRe.FindStringSubmatch(path); m != nil {
		return "/api/campaigns/{id}/" + m[1]
	}
	return path
}

// Record adds a timing sample for the given path.
func (ts *TimingStore) Record(path string, d time.Duration) {
	path = normalizePath(path)
	if !ts.patterns[path] {
		return
	}
	ms := float64(d.Milliseconds())
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ed, ok := ts.stats[path]
	if !ok {
		ed = &endpointData{samples: make([]float64, ringBufferSize)}
		ts.stats[path] = ed
	}
	ed.count++
	ed.totalMs += ms
	if ms > ed.maxMs {
		ed.maxMs = ms
	}
	// Ring buffer: O(1) insert for p95 sampling
	ed.samples[ed.head] = ms
	ed.head = (ed.head + 1) % len(ed.samples)
	if ed.size < len(ed.samples) {
		ed.size++
	}
}

// Snapshot returns a copy of the current stats.
func (ts *TimingStore) Snapshot() map[string]EndpointStats {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	out := make(map[string]EndpointStats, len(ts.stats))
	for path, ed := range ts.stats {
		s := EndpointStats{
			Count:   ed.count,
			TotalMs: ed.totalMs,
			MaxMs:   ed.maxMs,
		}
		if ed.count > 0 {
			s.AvgMs = ed.totalMs / float64(ed.count)
		}
		if ed.size > 0 {
			sorted := make([]float64, ed.size)
			copy(sorted, ed.samples[:ed.size])
			sort.Float64s(sorted)
			idx := int(float64(len(sorted)) * 0.95)
			if idx >= len(sorted) {
				idx = len(sorted) - 1
			}
			s.P95Ms = sorted[idx]
		}
		out[path] = s
	}
	return out
}

// Uptime returns the duration since the store was created.
func (ts *TimingStore) Uptime() time.Duration {
	return time.Since(ts.upSince)
}

// TimingMiddleware records endpoint response times for tracked paths.
func TimingMiddleware(store *TimingStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			normalized := normalizePath(r.URL.Path)
			if !store.patterns[normalized] {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			next.ServeHTTP(w, r)
			store.Record(normalized, time.Since(start))
		})
	}
}

// MetricsResponse is the JSON shape of the /api/admin/metrics endpoint.
type MetricsResponse struct {
	Endpoints map[string]EndpointStats `json:"endpoints"`
	UptimeS   float64                  `json:"uptimeSeconds"`
}

// HandleMetrics returns a handler that serves the timing metrics.
func HandleMetrics(store *TimingStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := MetricsResponse{
			Endpoints: store.Snapshot(),
			UptimeS:   store.Uptime().Seconds(),
		}
		data, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, "failed to encode metrics", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err = w.Write(data); err != nil {
			http.Error(w, "failed to write response", http.StatusInternalServerError)
		}
	}
}
