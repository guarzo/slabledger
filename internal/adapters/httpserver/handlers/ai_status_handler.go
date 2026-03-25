package handlers

import (
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// AIStatusHandler serves AI usage status information.
type AIStatusHandler struct {
	tracker ai.AICallTracker
	logger  observability.Logger
}

// NewAIStatusHandler creates a new handler for the AI status endpoint.
func NewAIStatusHandler(tracker ai.AICallTracker, logger observability.Logger) *AIStatusHandler {
	return &AIStatusHandler{tracker: tracker, logger: logger}
}

type aiUsageResponse struct {
	Configured bool                 `json:"configured"`
	Summary    aiSummary            `json:"summary"`
	Operations []aiOperationSummary `json:"operations"`
	Timestamp  time.Time            `json:"timestamp"`
}

type aiSummary struct {
	TotalCalls        int64      `json:"totalCalls"`
	SuccessRate       float64    `json:"successRate"`
	TotalInputTokens  int64      `json:"totalInputTokens"`
	TotalOutputTokens int64      `json:"totalOutputTokens"`
	TotalTokens       int64      `json:"totalTokens"`
	AvgLatencyMs      float64    `json:"avgLatencyMs"`
	RateLimitHits     int64      `json:"rateLimitHits"`
	CallsLast24h      int64      `json:"callsLast24h"`
	LastCallAt        *time.Time `json:"lastCallAt,omitempty"`
	TotalCostCents    int64      `json:"totalCostCents"`
}

type aiOperationSummary struct {
	Operation      string  `json:"operation"`
	Calls          int64   `json:"calls"`
	Errors         int64   `json:"errors"`
	SuccessRate    float64 `json:"successRate"`
	AvgLatencyMs   float64 `json:"avgLatencyMs"`
	TotalTokens    int64   `json:"totalTokens"`
	TotalCostCents int64   `json:"totalCostCents"`
}

// HandleAIUsage returns current AI usage stats.
func (h *AIStatusHandler) HandleAIUsage(w http.ResponseWriter, r *http.Request) {
	if h.tracker == nil {
		writeJSON(w, http.StatusOK, aiUsageResponse{
			Configured: false,
			Summary:    aiSummary{},
			Operations: []aiOperationSummary{},
			Timestamp:  time.Now().UTC(),
		})
		return
	}

	stats, err := h.tracker.GetAIUsage(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "failed to get AI usage", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Failed to get AI usage")
		return
	}

	var successRate float64
	if stats.TotalCalls > 0 {
		successRate = 100.0 * float64(stats.SuccessCalls) / float64(stats.TotalCalls)
	}

	summary := aiSummary{
		TotalCalls:        stats.TotalCalls,
		SuccessRate:       successRate,
		TotalInputTokens:  stats.TotalInputTokens,
		TotalOutputTokens: stats.TotalOutputTokens,
		TotalTokens:       stats.TotalTokens,
		AvgLatencyMs:      stats.AvgLatencyMS,
		RateLimitHits:     stats.RateLimitHits,
		CallsLast24h:      stats.CallsLast24h,
		LastCallAt:        stats.LastCallAt,
		TotalCostCents:    stats.TotalCostCents,
	}

	// Build per-operation list in stable order.
	opOrder := ai.AIOperations
	ops := make([]aiOperationSummary, 0, len(opOrder))
	for _, name := range opOrder {
		opStats, ok := stats.ByOperation[name]
		if !ok {
			continue
		}
		var opRate float64
		if opStats.Calls > 0 {
			opRate = 100.0 * float64(opStats.Calls-opStats.Errors) / float64(opStats.Calls)
		}
		ops = append(ops, aiOperationSummary{
			Operation:      string(name),
			Calls:          opStats.Calls,
			Errors:         opStats.Errors,
			SuccessRate:    opRate,
			AvgLatencyMs:   opStats.AvgLatencyMS,
			TotalTokens:    opStats.TotalTokens,
			TotalCostCents: opStats.TotalCostCents,
		})
	}

	writeJSON(w, http.StatusOK, aiUsageResponse{
		Configured: true,
		Summary:    summary,
		Operations: ops,
		Timestamp:  time.Now().UTC(),
	})
}
