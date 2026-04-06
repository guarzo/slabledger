package fusionprice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// mockLogger captures log calls for testing
type mockLogger struct {
	mu      sync.Mutex
	entries []logEntry
}

// Compile-time assertion that mockLogger implements Logger
var _ observability.Logger = (*mockLogger)(nil)

type logEntry struct {
	level  string
	msg    string
	fields map[string]any
}

func newMockLogger() *mockLogger {
	return &mockLogger{
		entries: make([]logEntry, 0),
	}
}

func (m *mockLogger) log(level, msg string, fields ...observability.Field) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := logEntry{
		level:  level,
		msg:    msg,
		fields: make(map[string]any),
	}
	for _, f := range fields {
		entry.fields[f.Key] = f.Value
	}
	m.entries = append(m.entries, entry)
}

func (m *mockLogger) Debug(ctx context.Context, msg string, fields ...observability.Field) {
	m.log("DEBUG", msg, fields...)
}
func (m *mockLogger) Info(ctx context.Context, msg string, fields ...observability.Field) {
	m.log("INFO", msg, fields...)
}
func (m *mockLogger) Warn(ctx context.Context, msg string, fields ...observability.Field) {
	m.log("WARN", msg, fields...)
}
func (m *mockLogger) Error(ctx context.Context, msg string, fields ...observability.Field) {
	m.log("ERROR", msg, fields...)
}
func (m *mockLogger) With(ctx context.Context, fields ...observability.Field) observability.Logger {
	return m
}

func (m *mockLogger) getEntries() []logEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]logEntry{}, m.entries...)
}

func (m *mockLogger) lastEntry() logEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.entries) == 0 {
		return logEntry{}
	}
	return m.entries[len(m.entries)-1]
}

func TestNewCardSyncCollector(t *testing.T) {
	logger := newMockLogger()
	collector := NewCardSyncCollector(logger, "Charizard", "4", "Base Set")

	if collector.CardName != "Charizard" {
		t.Errorf("expected CardName 'Charizard', got %q", collector.CardName)
	}
	if collector.CardNumber != "4" {
		t.Errorf("expected CardNumber '4', got %q", collector.CardNumber)
	}
	if collector.SetName != "Base Set" {
		t.Errorf("expected SetName 'Base Set', got %q", collector.SetName)
	}
	if collector.emitted {
		t.Error("expected emitted to be false initially")
	}
}

func TestCardSyncCollector_SuccessfulSync(t *testing.T) {
	logger := newMockLogger()
	collector := NewCardSyncCollector(logger, "Charizard", "4", "Base Set")

	// Record successful sources
	collector.RecordSource(SourcePriceCharting, true, nil, 100*time.Millisecond, false)
	collector.RecordSource("CH", true, nil, 150*time.Millisecond, false)

	// Record prices
	collector.RecordPrices(PriceResult{
		PSA10: 450.00,
		PSA9:  200.00,
		Raw:   50.00,
	}, 0.92, 2)

	// Complete
	collector.Complete(context.Background())

	// Verify log output
	entry := logger.lastEntry()
	if entry.level != "INFO" {
		t.Errorf("expected INFO level, got %s", entry.level)
	}
	if entry.msg != "card sync complete" {
		t.Errorf("expected 'card sync complete', got %q", entry.msg)
	}

	// Check card field
	card, ok := entry.fields["card"].(string)
	if !ok || card != "Charizard #4" {
		t.Errorf("expected card 'Charizard #4', got %v", entry.fields["card"])
	}

	// Check sources field
	sources, ok := entry.fields["sources"].(string)
	if !ok {
		t.Errorf("expected sources string, got %T", entry.fields["sources"])
	}
	if !strings.Contains(sources, "PC:✓") {
		t.Errorf("expected sources to contain 'PC:✓', got %q", sources)
	}
	if !strings.Contains(sources, "CH:✓") {
		t.Errorf("expected sources to contain 'CH:✓', got %q", sources)
	}

	// Check prices
	if psa10, ok := entry.fields["psa10"].(string); !ok || psa10 != "$450.00" {
		t.Errorf("expected psa10 '$450.00', got %v", entry.fields["psa10"])
	}
	if psa9, ok := entry.fields["psa9"].(string); !ok || psa9 != "$200.00" {
		t.Errorf("expected psa9 '$200.00', got %v", entry.fields["psa9"])
	}

	// Check confidence
	if conf, ok := entry.fields["confidence"].(float64); !ok || conf != 0.92 {
		t.Errorf("expected confidence 0.92, got %v", entry.fields["confidence"])
	}
}

func TestCardSyncCollector_PartialFailure(t *testing.T) {
	logger := newMockLogger()
	collector := NewCardSyncCollector(logger, "Moltres", "30", "Legendary Collection")

	// Record mixed results
	collector.RecordSource(SourcePriceCharting, true, nil, 100*time.Millisecond, false)
	collector.RecordSource("CH", false, errors.New("card not found"), 50*time.Millisecond, false)

	// Record prices (from successful source only)
	collector.RecordPrices(PriceResult{
		PSA10: 180.00,
	}, 0.73, 1)

	collector.Complete(context.Background())

	entry := logger.lastEntry()

	// Should still be INFO since we have some success
	if entry.level != "INFO" {
		t.Errorf("expected INFO level, got %s", entry.level)
	}

	// Check sources show mixed status
	sources := entry.fields["sources"].(string)
	if !strings.Contains(sources, "PC:✓") {
		t.Errorf("expected PC:✓ in sources, got %q", sources)
	}
	if !strings.Contains(sources, "CH:✗") {
		t.Errorf("expected CH:✗ in sources, got %q", sources)
	}

	// Check errors field
	errorsStr, ok := entry.fields["errors"].(string)
	if !ok {
		t.Errorf("expected errors string, got %T", entry.fields["errors"])
	}
	if !strings.Contains(errorsStr, "CH:not_found") {
		t.Errorf("expected 'CH:not_found' in errors, got %q", errorsStr)
	}
}

func TestCardSyncCollector_AllSourcesFailed(t *testing.T) {
	logger := newMockLogger()
	collector := NewCardSyncCollector(logger, "Unknown Card", "", "Test Set")

	// Record all failures
	collector.RecordSource(SourcePriceCharting, false, errors.New("card not found"), 100*time.Millisecond, false)
	collector.RecordSource("CH", false, errors.New("rate limit exceeded"), 50*time.Millisecond, false)

	collector.Complete(context.Background())

	entry := logger.lastEntry()

	// Should be WARN when all sources fail
	if entry.level != "WARN" {
		t.Errorf("expected WARN level when all sources fail, got %s", entry.level)
	}
	if entry.msg != "card sync failed" {
		t.Errorf("expected 'card sync failed', got %q", entry.msg)
	}
}

func TestCardSyncCollector_CacheHit(t *testing.T) {
	logger := newMockLogger()
	collector := NewCardSyncCollector(logger, "Pikachu", "58", "Base Set")

	collector.RecordCacheHit(2 * time.Hour)
	collector.Complete(context.Background())

	entry := logger.lastEntry()

	if entry.level != "DEBUG" {
		t.Errorf("expected DEBUG level for cache hit, got %s", entry.level)
	}
	if entry.msg != "card sync cache hit" {
		t.Errorf("expected 'card sync cache hit', got %q", entry.msg)
	}
}

func TestCardSyncCollector_CompleteOnlyOnce(t *testing.T) {
	logger := newMockLogger()
	collector := NewCardSyncCollector(logger, "Test", "1", "Test Set")

	collector.RecordSource(SourcePriceCharting, true, nil, 100*time.Millisecond, false)
	collector.Complete(context.Background())
	collector.Complete(context.Background()) // Second call should be ignored

	entries := logger.getEntries()
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 log entry, got %d", len(entries))
	}
}

func TestCardSyncCollector_ThreadSafety(t *testing.T) {
	logger := newMockLogger()
	collector := NewCardSyncCollector(logger, "Test", "1", "Test Set")

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sources := []string{SourcePriceCharting, "CH"}
			collector.RecordSource(sources[idx], true, nil, time.Duration(idx)*time.Millisecond, false)
		}(i)
	}
	wg.Wait()

	collector.Complete(context.Background())

	entry := logger.lastEntry()
	sources := entry.fields["sources"].(string)

	// All 2 sources should be recorded
	for _, src := range []string{"PC:✓", "CH:✓"} {
		if !strings.Contains(sources, src) {
			t.Errorf("expected %s in sources, got %q", src, sources)
		}
	}
}

func TestCardSyncCollector_NilLogger(t *testing.T) {
	// Should not panic with nil logger
	collector := NewCardSyncCollector(nil, "Test", "1", "Test Set")
	collector.RecordSource(SourcePriceCharting, true, nil, 100*time.Millisecond, false)
	collector.Complete(context.Background()) // Should not panic
}

func TestCardSyncCollector_CardWithoutNumber(t *testing.T) {
	logger := newMockLogger()
	collector := NewCardSyncCollector(logger, "Energy", "", "Base Set")

	collector.RecordSource(SourcePriceCharting, true, nil, 100*time.Millisecond, false)
	collector.Complete(context.Background())

	entry := logger.lastEntry()
	card := entry.fields["card"].(string)

	// Should just be card name without #
	if card != "Energy" {
		t.Errorf("expected card 'Energy', got %q", card)
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCode
	}{
		{"nil error", nil, ErrCodeNone},
		{"not found", errors.New("card not found"), ErrCodeNotFound},
		{"no match", errors.New("no matching price found"), ErrCodeNotFound},
		{"rate limited", errors.New("rate limit exceeded"), ErrCodeRateLimited},
		{"429 status", errors.New("HTTP 429: too many requests"), ErrCodeRateLimited},
		{"timeout", errors.New("request timeout"), ErrCodeTimeout},
		{"context deadline", context.DeadlineExceeded, ErrCodeTimeout},
		{"context canceled", context.Canceled, ErrCodeCanceled},
		{"wrapped context deadline", fmt.Errorf("operation failed: %w", context.DeadlineExceeded), ErrCodeTimeout},
		{"wrapped context canceled", fmt.Errorf("request aborted: %w", context.Canceled), ErrCodeCanceled},
		{"double wrapped deadline", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", context.DeadlineExceeded)), ErrCodeTimeout},
		{"double wrapped canceled", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", context.Canceled)), ErrCodeCanceled},
		{"unavailable", errors.New("service unavailable"), ErrCodeUnavailable},
		{"503 status", errors.New("HTTP 503"), ErrCodeUnavailable},
		{"parse error", errors.New("failed to parse response"), ErrCodeParseError},
		{"invalid response", errors.New("invalid API response"), ErrCodeParseError},
		{"blocked", errors.New("provider blocked"), ErrCodeBlocked},
		{"unauthorized", errors.New("unauthorized access"), ErrCodeAuthFailed},
		{"401 status", errors.New("HTTP 401"), ErrCodeAuthFailed},
		{"403 status", errors.New("HTTP 403 forbidden"), ErrCodeAuthFailed},
		{"unknown error", errors.New("something weird happened"), ErrCodeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err)
			if got != tt.expected {
				t.Errorf("ClassifyError(%v) = %q, want %q", tt.err, got, tt.expected)
			}
		})
	}
}

func TestFormatPrice(t *testing.T) {
	tests := []struct {
		price    float64
		expected string
	}{
		{100.00, "$100.00"},
		{99.99, "$99.99"},
		{0.50, "$0.50"},
		{1234.56, "$1234.56"},
	}

	for _, tt := range tests {
		got := formatPrice(tt.price)
		if got != tt.expected {
			t.Errorf("formatPrice(%v) = %q, want %q", tt.price, got, tt.expected)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{500 * time.Millisecond, "500ms"},
		{1 * time.Second, "1.0s"},
		{1500 * time.Millisecond, "1.5s"},
		{2345 * time.Millisecond, "2.3s"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.duration)
		if got != tt.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.expected)
		}
	}
}

func TestSourceOrderConsistency(t *testing.T) {
	logger := newMockLogger()
	collector := NewCardSyncCollector(logger, "Test", "1", "Test Set")

	// Add sources in reverse order
	collector.RecordSource("CH", true, nil, 10*time.Millisecond, false)
	collector.RecordSource(SourcePriceCharting, true, nil, 10*time.Millisecond, false)

	collector.Complete(context.Background())

	entry := logger.lastEntry()
	sources := entry.fields["sources"].(string)

	// Should be in canonical order: PC CH
	expected := "PC:✓ CH:✓"
	if sources != expected {
		t.Errorf("expected sources in order %q, got %q", expected, sources)
	}
}

func TestPricesOmittedWhenZero(t *testing.T) {
	logger := newMockLogger()
	collector := NewCardSyncCollector(logger, "Test", "1", "Test Set")

	collector.RecordSource(SourcePriceCharting, true, nil, 100*time.Millisecond, false)
	collector.RecordPrices(PriceResult{
		PSA10: 100.00,
		// PSA9 and Raw are zero
	}, 0.85, 1)

	collector.Complete(context.Background())

	entry := logger.lastEntry()

	// PSA10 should be present
	if _, ok := entry.fields["psa10"]; !ok {
		t.Error("expected psa10 field to be present")
	}

	// PSA9 and Raw should be absent
	if _, ok := entry.fields["psa9"]; ok {
		t.Error("expected psa9 field to be absent when zero")
	}
	if _, ok := entry.fields["raw"]; ok {
		t.Error("expected raw field to be absent when zero")
	}
}
