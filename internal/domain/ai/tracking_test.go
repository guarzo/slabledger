package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

type spyTracker struct {
	recorded *AICallRecord
	err      error
}

func (s *spyTracker) RecordAICall(_ context.Context, call *AICallRecord) error {
	s.recorded = call
	return s.err
}

func (s *spyTracker) GetAIUsage(_ context.Context) (*AIUsageStats, error) {
	return nil, nil
}

type spyLogger struct {
	warned  bool
	errored bool
}

func (l *spyLogger) Debug(_ context.Context, _ string, _ ...observability.Field) {}
func (l *spyLogger) Info(_ context.Context, _ string, _ ...observability.Field)  {}
func (l *spyLogger) Warn(_ context.Context, _ string, _ ...observability.Field)  { l.warned = true }
func (l *spyLogger) Error(_ context.Context, _ string, _ ...observability.Field) { l.errored = true }
func (l *spyLogger) With(_ context.Context, _ ...observability.Field) observability.Logger {
	return l
}

func TestRecordCall_Success(t *testing.T) {
	tracker := &spyTracker{}
	logger := &spyLogger{}
	start := time.Now().Add(-100 * time.Millisecond)

	RecordCall(context.Background(), tracker, logger, OpDigest, nil, start, 3, &TokenUsage{
		InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
	})

	if tracker.recorded == nil {
		t.Fatal("expected record to be persisted")
	}
	if tracker.recorded.Operation != OpDigest {
		t.Errorf("operation = %q, want %q", tracker.recorded.Operation, OpDigest)
	}
	if tracker.recorded.ToolRounds != 3 {
		t.Errorf("rounds = %d, want 3", tracker.recorded.ToolRounds)
	}
	if tracker.recorded.Status != AIStatusSuccess {
		t.Errorf("status = %q, want %q", tracker.recorded.Status, AIStatusSuccess)
	}
	if tracker.recorded.InputTokens != 100 {
		t.Errorf("inputTokens = %d, want 100", tracker.recorded.InputTokens)
	}
	if tracker.recorded.OutputTokens != 50 {
		t.Errorf("outputTokens = %d, want 50", tracker.recorded.OutputTokens)
	}
	if tracker.recorded.TotalTokens != 150 {
		t.Errorf("totalTokens = %d, want 150", tracker.recorded.TotalTokens)
	}
	if tracker.recorded.LatencyMS <= 0 {
		t.Errorf("latencyMS = %d, want > 0", tracker.recorded.LatencyMS)
	}
}

func TestRecordCall_NilTracker(t *testing.T) {
	// Should not panic when tracker is nil.
	RecordCall(context.Background(), nil, nil, OpDigest, nil, time.Now(), 0, nil)
}

func TestRecordCall_TrackerError(t *testing.T) {
	tracker := &spyTracker{err: errors.New("db down")}
	logger := &spyLogger{}

	RecordCall(context.Background(), tracker, logger, OpDigest, nil, time.Now(), 0, nil)

	if !logger.errored {
		t.Error("expected error log when tracker returns error")
	}
}

func TestRecordCall_TrackerErrorNilLogger(t *testing.T) {
	tracker := &spyTracker{err: errors.New("db down")}

	// Should not panic when logger is nil.
	RecordCall(context.Background(), tracker, nil, OpDigest, nil, time.Now(), 0, nil)
}
