package handlers

import (
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// Known daily limits per provider.
var providerDailyLimits = map[string]*int{
	pricing.SourceDH: nil, // No hard daily limit
}

// knownProviders is the ordered list of providers shown in the status response.
var knownProviders = []string{pricing.SourceDH}

// APIStatusHandler serves API usage status information.
type APIStatusHandler struct {
	apiTracker pricing.APITracker
	logger     observability.Logger
}

// NewAPIStatusHandler creates a new handler for the API status endpoint.
func NewAPIStatusHandler(apiTracker pricing.APITracker, logger observability.Logger) *APIStatusHandler {
	return &APIStatusHandler{apiTracker: apiTracker, logger: logger}
}

// apiUsageResponse is the JSON response for GET /api/status/api-usage.
type apiUsageResponse struct {
	Providers []providerStatus `json:"providers"`
	Timestamp time.Time        `json:"timestamp"`
}

type providerStatus struct {
	Name         string      `json:"name"`
	Today        providerDay `json:"today"`
	Blocked      bool        `json:"blocked"`
	BlockedUntil *time.Time  `json:"blockedUntil,omitempty"`
	LastCallAt   *time.Time  `json:"lastCallAt,omitempty"`
}

type providerDay struct {
	Calls         int64      `json:"calls"`
	Limit         *int       `json:"limit"` // nil means no hard limit
	Remaining     *int       `json:"remaining,omitempty"`
	SuccessRate   float64    `json:"successRate"`
	AvgLatencyMs  float64    `json:"avgLatencyMs"`
	RateLimitHits int64      `json:"rateLimitHits"`
	MinuteCalls   int64      `json:"minuteCalls"`
	Last429At     *time.Time `json:"last429At,omitempty"`
}

// HandleAPIUsage returns current API usage stats for all providers.
// Returns an empty provider list when API tracking is disabled.
func (h *APIStatusHandler) HandleAPIUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if h.apiTracker == nil {
		writeJSON(w, http.StatusOK, apiUsageResponse{
			Providers: []providerStatus{},
			Timestamp: time.Now().UTC(),
		})
		return
	}

	ctx := r.Context()
	resp := apiUsageResponse{
		Providers: make([]providerStatus, 0, len(knownProviders)),
		Timestamp: time.Now().UTC(),
	}

	for _, name := range knownProviders {
		stats, err := h.apiTracker.GetAPIUsage(ctx, name)
		if err != nil {
			h.logger.Warn(ctx, "failed to get API usage",
				observability.String("provider", name),
				observability.Err(err))
			stats = &pricing.APIUsageStats{Provider: name}
		}

		var successRate float64
		if stats.TotalCalls > 0 {
			successRate = 100.0 * float64(stats.TotalCalls-stats.ErrorCalls) / float64(stats.TotalCalls)
		}

		ps := providerStatus{
			Name: name,
			Today: providerDay{
				Calls:         stats.TotalCalls,
				Limit:         providerDailyLimits[name],
				SuccessRate:   successRate,
				AvgLatencyMs:  stats.AvgLatencyMS,
				RateLimitHits: stats.RateLimitHits,
			},
			Blocked: stats.BlockedUntil != nil,
		}

		if stats.BlockedUntil != nil {
			ps.BlockedUntil = stats.BlockedUntil
		}

		if !stats.LastCallAt.IsZero() {
			t := stats.LastCallAt.UTC()
			ps.LastCallAt = &t
		}

		// Compute remaining if we have a daily limit
		if limit := providerDailyLimits[name]; limit != nil {
			remaining := *limit - int(stats.TotalCalls)
			if remaining < 0 {
				remaining = 0
			}
			ps.Today.Remaining = &remaining
		}

		resp.Providers = append(resp.Providers, ps)
	}

	writeJSON(w, http.StatusOK, resp)
}
