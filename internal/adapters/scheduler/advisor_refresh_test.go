package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/platform/config"
)

type mockCollector struct {
	collectFn func(ctx context.Context) (string, error)
}

func (m *mockCollector) CollectDigest(ctx context.Context) (string, error) {
	if m.collectFn != nil {
		return m.collectFn(ctx)
	}
	return "", nil
}

func (m *mockCollector) CollectLiquidation(ctx context.Context) (string, error) {
	if m.collectFn != nil {
		return m.collectFn(ctx)
	}
	return "", nil
}

type mockCache struct{}

func (m *mockCache) MarkRunning(_ context.Context, _ advisor.AnalysisType) (string, error) {
	return "lease-1", nil
}

func (m *mockCache) SaveResult(_ context.Context, _ advisor.AnalysisType, _ string, _ string, _ string) error {
	return nil
}

func (m *mockCache) Get(_ context.Context, _ advisor.AnalysisType) (*advisor.CachedAnalysis, error) {
	return nil, nil
}

func (m *mockCache) AcquireRefresh(_ context.Context, _ advisor.AnalysisType) (string, bool, error) {
	return "lease-1", true, nil
}

func (m *mockCache) ForceAcquireStale(_ context.Context, _ advisor.AnalysisType, _ time.Duration) (string, bool, error) {
	return "lease-1", true, nil
}

func TestRunAnalysis_UsesExpectedTimeout(t *testing.T) {
	const expectedTimeout = 20 * time.Minute

	var capturedDeadline time.Time
	var hasDeadline bool

	collector := &mockCollector{
		collectFn: func(ctx context.Context) (string, error) {
			capturedDeadline, hasDeadline = ctx.Deadline()
			return "result", nil
		},
	}
	cache := &mockCache{}
	s := NewAdvisorRefreshScheduler(collector, cache, nil, nopLogger{}, config.AdvisorRefreshConfig{Enabled: true, RefreshHour: -1})

	start := time.Now()
	err := s.runAnalysis(context.Background(), "digest", collector.collectFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasDeadline {
		t.Fatal("context should have a deadline")
	}
	// The deadline should be ~expectedTimeout from when runAnalysis was called.
	gap := capturedDeadline.Sub(start)
	if gap < expectedTimeout-5*time.Second || gap > expectedTimeout+5*time.Second {
		t.Errorf("deadline gap = %v, want ~%v", gap, expectedTimeout)
	}
}

func TestTimeUntilHour(t *testing.T) {
	tests := []struct {
		name    string
		now     time.Time
		hour    int
		wantMin time.Duration
		wantMax time.Duration
	}{
		{
			name:    "target hour is later today",
			now:     time.Date(2026, 3, 26, 2, 0, 0, 0, time.UTC),
			hour:    4,
			wantMin: 1*time.Hour + 59*time.Minute,
			wantMax: 2*time.Hour + 1*time.Minute,
		},
		{
			name:    "target hour already passed today",
			now:     time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC),
			hour:    4,
			wantMin: 17*time.Hour + 59*time.Minute,
			wantMax: 18*time.Hour + 1*time.Minute,
		},
		{
			name:    "target hour is current hour",
			now:     time.Date(2026, 3, 26, 4, 30, 0, 0, time.UTC),
			hour:    4,
			wantMin: 23*time.Hour + 29*time.Minute,
			wantMax: 23*time.Hour + 31*time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timeUntilHour(tt.now, tt.hour)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("timeUntilHour(%v, %d) = %v, want between %v and %v",
					tt.now, tt.hour, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestIsTransientAIError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"connection reset", fmt.Errorf("connection reset by peer"), true},
		{"broken pipe", fmt.Errorf("broken pipe"), true},
		{"EOF", fmt.Errorf("unexpected EOF"), true},
		{"i/o timeout", fmt.Errorf("i/o timeout"), true},
		{"SSE stream incomplete", fmt.Errorf("SSE stream ended without response.completed (possible network interruption)"), true},
		{"context deadline in LLM round", fmt.Errorf("llm completion (round 3): context deadline exceeded"), false},
		{"context deadline bare", fmt.Errorf("context deadline exceeded"), false},
		{"permanent 400 error", fmt.Errorf("azure ai returned 400: bad request"), false},
		{"unknown error", fmt.Errorf("something unexpected"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransientAIError(tt.err)
			if got != tt.want {
				t.Errorf("isTransientAIError(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
