package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// stubEventPruner records the cutoff it was handed. The scheduler's only real
// logic is turning retentionDays into that cutoff, so that is what is asserted.
type stubEventPruner struct {
	calls     int
	cutoff    time.Time
	returnN   int64
	returnErr error
}

func (s *stubEventPruner) DeleteOlderThan(_ context.Context, cutoff time.Time) (int64, error) {
	s.calls++
	s.cutoff = cutoff
	return s.returnN, s.returnErr
}

var _ dhevents.Pruner = (*stubEventPruner)(nil)

func TestDefaultDHEventCleanupConfig(t *testing.T) {
	config := DefaultDHEventCleanupConfig()

	if !config.Enabled {
		t.Error("expected Enabled to be true by default")
	}
	if config.Interval != 24*time.Hour {
		t.Errorf("expected Interval 24h, got %v", config.Interval)
	}
	if config.RetentionDays != 90 {
		t.Errorf("expected RetentionDays 90, got %d", config.RetentionDays)
	}
}

func TestNewDHEventCleanupScheduler(t *testing.T) {
	tests := []struct {
		name              string
		config            DHEventCleanupConfig
		wantInterval      time.Duration
		wantRetentionDays int
	}{
		{
			name:              "explicit values are preserved",
			config:            DHEventCleanupConfig{Enabled: true, Interval: time.Hour, RetentionDays: 30},
			wantInterval:      time.Hour,
			wantRetentionDays: 30,
		},
		{
			name:              "zero values fall back to defaults",
			config:            DHEventCleanupConfig{},
			wantInterval:      24 * time.Hour,
			wantRetentionDays: 90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewDHEventCleanupScheduler(&stubEventPruner{}, mocks.NewMockLogger(), tt.config)

			if s.interval != tt.wantInterval {
				t.Errorf("interval = %v, want %v", s.interval, tt.wantInterval)
			}
			if s.retentionDays != tt.wantRetentionDays {
				t.Errorf("retentionDays = %d, want %d", s.retentionDays, tt.wantRetentionDays)
			}
		})
	}
}

func TestDHEventCleanupScheduler_RunOnce(t *testing.T) {
	pruneErr := errors.New("prune failed")

	tests := []struct {
		name          string
		retentionDays int
		pruner        *stubEventPruner
		wantCount     int64
		wantErr       error
	}{
		{
			name:          "returns rows deleted",
			retentionDays: 90,
			pruner:        &stubEventPruner{returnN: 17},
			wantCount:     17,
		},
		{
			name:          "propagates pruner error",
			retentionDays: 90,
			pruner:        &stubEventPruner{returnErr: pruneErr},
			wantCount:     0,
			wantErr:       pruneErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewDHEventCleanupScheduler(tt.pruner, mocks.NewMockLogger(), DHEventCleanupConfig{
				Enabled:       true,
				Interval:      24 * time.Hour,
				RetentionDays: tt.retentionDays,
			})

			before := time.Now().UTC()
			count, err := s.RunOnce(context.Background())

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}
			if tt.pruner.calls != 1 {
				t.Fatalf("pruner calls = %d, want 1", tt.pruner.calls)
			}

			// The cutoff must land retentionDays back from now, within the
			// slack of the test's own execution time.
			wantCutoff := before.AddDate(0, 0, -tt.retentionDays)
			if delta := tt.pruner.cutoff.Sub(wantCutoff); delta < 0 || delta > time.Minute {
				t.Errorf("cutoff = %v, want within a minute of %v", tt.pruner.cutoff, wantCutoff)
			}
		})
	}
}

func TestDHEventCleanupScheduler_StartDisabled(t *testing.T) {
	pruner := &stubEventPruner{}
	s := NewDHEventCleanupScheduler(pruner, mocks.NewMockLogger(), DHEventCleanupConfig{
		Enabled:       false,
		Interval:      time.Millisecond,
		RetentionDays: 90,
	})

	// Start must return immediately without pruning when disabled.
	s.Start(context.Background())

	if pruner.calls != 0 {
		t.Errorf("pruner calls = %d, want 0 when disabled", pruner.calls)
	}
}
