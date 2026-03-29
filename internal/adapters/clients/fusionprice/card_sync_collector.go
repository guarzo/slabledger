package fusionprice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ErrorCode represents a standardized error classification for logging
type ErrorCode string

const (
	ErrCodeNone        ErrorCode = ""
	ErrCodeNotFound    ErrorCode = "not_found"
	ErrCodeRateLimited ErrorCode = "rate_limited"
	ErrCodeTimeout     ErrorCode = "timeout"
	ErrCodeCanceled    ErrorCode = "canceled"
	ErrCodeUnavailable ErrorCode = "unavailable"
	ErrCodeParseError  ErrorCode = "parse_error"
	ErrCodeBlocked     ErrorCode = "blocked"
	ErrCodeAuthFailed  ErrorCode = "auth_failed"
	ErrCodeUnknown     ErrorCode = "unknown"
)

// Source abbreviations for compact logging
const (
	SourcePriceCharting = "PC"
	SourceCardHedger    = "CH"
)

// SourceResult represents the outcome of a price source lookup
type SourceResult struct {
	Source   string // Short code: "PC", "CH"
	Success  bool
	ErrCode  ErrorCode
	Latency  time.Duration
	CacheHit bool
}

// PriceResult represents prices found for a card
type PriceResult struct {
	PSA10 float64
	PSA9  float64
	PSA8  float64
	CGC95 float64
	BGS10 float64
	Raw   float64
}

// CardSyncCollector accumulates events during card processing and emits a single summary log
type CardSyncCollector struct {
	CardName   string
	CardNumber string
	SetName    string
	StartTime  time.Time

	mu              sync.Mutex
	sources         []SourceResult
	prices          PriceResult
	finalConfidence float64
	fusionSources   int
	cacheHit        bool
	cacheAge        time.Duration
	singleflightHit bool
	generalErrors   []error

	logger  observability.Logger
	emitted bool
}

// NewCardSyncCollector creates a new collector for tracking a card's sync progress
func NewCardSyncCollector(logger observability.Logger, cardName, cardNumber, setName string) *CardSyncCollector {
	return &CardSyncCollector{
		CardName:   cardName,
		CardNumber: cardNumber,
		SetName:    setName,
		StartTime:  time.Now(),
		sources:    make([]SourceResult, 0, 4),
		logger:     logger,
	}
}

// RecordSource records a source lookup result (thread-safe)
func (c *CardSyncCollector) RecordSource(source string, success bool, err error, latency time.Duration, cacheHit bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := SourceResult{
		Source:   source,
		Success:  success,
		ErrCode:  ClassifyError(err),
		Latency:  latency,
		CacheHit: cacheHit,
	}
	c.sources = append(c.sources, result)
}

// RecordPrices records the final fused prices
func (c *CardSyncCollector) RecordPrices(prices PriceResult, confidence float64, sourceCount int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.prices = prices
	c.finalConfidence = confidence
	c.fusionSources = sourceCount
}

// RecordCacheHit records that the result was served from cache
func (c *CardSyncCollector) RecordCacheHit(age time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cacheHit = true
	c.cacheAge = age
}

// RecordSingleFlightHit records that the result was shared from a concurrent in-flight request
func (c *CardSyncCollector) RecordSingleFlightHit() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.singleflightHit = true
}

// RecordError records a non-source-specific error
func (c *CardSyncCollector) RecordError(err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.generalErrors = append(c.generalErrors, err)
}

// Duration returns the elapsed time since collection started
func (c *CardSyncCollector) Duration() time.Duration {
	return time.Since(c.StartTime)
}

// Complete emits the summary log line and marks collector as done
func (c *CardSyncCollector) Complete(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.emitted {
		return
	}
	c.emitted = true

	if c.logger == nil {
		return
	}

	// Build card identifier, avoiding duplication if CardName already ends with #CardNumber
	cardID := c.CardName
	if c.CardNumber != "" && !strings.HasSuffix(c.CardName, "#"+c.CardNumber) {
		cardID = fmt.Sprintf("%s #%s", c.CardName, c.CardNumber)
	}

	duration := time.Since(c.StartTime)

	// Cache hit case - emit DEBUG level
	if c.cacheHit {
		c.logger.Debug(ctx, "card sync cache hit",
			observability.String("card", cardID),
			observability.String("set", c.SetName),
			observability.Duration("cache_age", c.cacheAge),
			observability.Duration("duration", duration),
		)
		return
	}

	// Singleflight dedup case - emit DEBUG level
	if c.singleflightHit {
		c.logger.Debug(ctx, "card sync singleflight dedup",
			observability.String("card", cardID),
			observability.String("set", c.SetName),
			observability.Duration("duration", duration),
		)
		return
	}

	// Build sources string: "PC:✓ PP:✗ CH:✗"
	sourcesStr := c.buildSourcesString()

	// Build errors string: "PP:not_found CH:not_found"
	errorsStr := c.buildErrorsString()

	// Check if all sources failed (including cases with only general errors and no successful sources)
	allFailed := c.countSuccessfulSources() == 0 && (len(c.sources) > 0 || len(c.generalErrors) > 0)

	// Build fields
	fields := []observability.Field{
		observability.String("card", cardID),
		observability.String("set", c.SetName),
		observability.String("sources", sourcesStr),
	}

	// Add prices if we have any
	if c.prices.PSA10 > 0 {
		fields = append(fields, observability.String("psa10", formatPrice(c.prices.PSA10)))
	}
	if c.prices.PSA9 > 0 {
		fields = append(fields, observability.String("psa9", formatPrice(c.prices.PSA9)))
	}
	if c.prices.Raw > 0 {
		fields = append(fields, observability.String("raw", formatPrice(c.prices.Raw)))
	}

	// Add confidence if available
	if c.finalConfidence > 0 {
		fields = append(fields, observability.Float64("confidence", mathutil.Round2(c.finalConfidence)))
	}

	// Add errors if any
	if errorsStr != "" {
		fields = append(fields, observability.String("errors", errorsStr))
	}

	// Add duration
	fields = append(fields, observability.String("duration", formatDuration(duration)))

	// Emit at appropriate level
	if allFailed {
		c.logger.Warn(ctx, "card sync failed", fields...)
	} else {
		c.logger.Info(ctx, "card sync complete", fields...)
	}
}

// buildSourcesString creates the compact sources status string
func (c *CardSyncCollector) buildSourcesString() string {
	if len(c.sources) == 0 {
		return ""
	}

	// Define order for consistent output
	order := []string{SourcePriceCharting, SourceCardHedger}
	sourceMap := make(map[string]SourceResult)
	for _, s := range c.sources {
		sourceMap[s.Source] = s
	}

	var parts []string
	for _, src := range order {
		if result, ok := sourceMap[src]; ok {
			symbol := "✓"
			if !result.Success {
				symbol = "✗"
			}
			parts = append(parts, fmt.Sprintf("%s:%s", src, symbol))
		}
	}

	// Add any sources not in the predefined order
	for _, s := range c.sources {
		found := false
		for _, o := range order {
			if s.Source == o {
				found = true
				break
			}
		}
		if !found {
			symbol := "✓"
			if !s.Success {
				symbol = "✗"
			}
			parts = append(parts, fmt.Sprintf("%s:%s", s.Source, symbol))
		}
	}

	return strings.Join(parts, " ")
}

// buildErrorsString creates the compact errors string
func (c *CardSyncCollector) buildErrorsString() string {
	var parts []string

	// Collect source errors
	for _, s := range c.sources {
		if !s.Success && s.ErrCode != ErrCodeNone {
			parts = append(parts, fmt.Sprintf("%s:%s", s.Source, s.ErrCode))
		}
	}

	// Add general errors
	for _, err := range c.generalErrors {
		code := ClassifyError(err)
		if code != ErrCodeNone {
			parts = append(parts, string(code))
		}
	}

	return strings.Join(parts, " ")
}

// countSuccessfulSources returns the number of successful source lookups
func (c *CardSyncCollector) countSuccessfulSources() int {
	count := 0
	for _, s := range c.sources {
		if s.Success {
			count++
		}
	}
	return count
}

// ClassifyError converts an error to a standard error code
func ClassifyError(err error) ErrorCode {
	if err == nil {
		return ErrCodeNone
	}

	errStr := strings.ToLower(err.Error())

	// Check for context errors first
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrCodeTimeout
	}
	if errors.Is(err, context.Canceled) {
		return ErrCodeCanceled
	}

	// Check error message patterns
	switch {
	case strings.Contains(errStr, "not found"):
		return ErrCodeNotFound
	case strings.Contains(errStr, "no match"):
		return ErrCodeNotFound
	case strings.Contains(errStr, "rate limit"):
		return ErrCodeRateLimited
	case strings.Contains(errStr, "429"):
		return ErrCodeRateLimited
	case strings.Contains(errStr, "timeout"):
		return ErrCodeTimeout
	case strings.Contains(errStr, "unavailable"):
		return ErrCodeUnavailable
	case strings.Contains(errStr, "503"):
		return ErrCodeUnavailable
	case strings.Contains(errStr, "parse"):
		return ErrCodeParseError
	case strings.Contains(errStr, "invalid"):
		return ErrCodeParseError
	case strings.Contains(errStr, "blocked"):
		return ErrCodeBlocked
	case strings.Contains(errStr, "unauthorized"):
		return ErrCodeAuthFailed
	case strings.Contains(errStr, "401"):
		return ErrCodeAuthFailed
	case strings.Contains(errStr, "403"):
		return ErrCodeAuthFailed
	default:
		return ErrCodeUnknown
	}
}

// formatPrice formats a price as a dollar string
func formatPrice(price float64) string {
	return fmt.Sprintf("$%.2f", price)
}

// formatDuration formats a duration as a compact string
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
