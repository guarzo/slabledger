package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// NichesLeaderboardService is the narrow seam the niches handler depends on.
// Both *demand.Service and mocks satisfy this interface.
type NichesLeaderboardService interface {
	Leaderboard(ctx context.Context, opts demand.LeaderboardOpts) ([]demand.NicheOpportunity, error)
}

// NichesHandler serves the GET /api/intelligence/niches endpoint.
type NichesHandler struct {
	service NichesLeaderboardService
	logger  observability.Logger
}

// NewNichesHandler constructs a NichesHandler.
func NewNichesHandler(service NichesLeaderboardService, logger observability.Logger) *NichesHandler {
	return &NichesHandler{service: service, logger: logger}
}

const (
	nichesDefaultLimit = 50
	nichesMaxLimit     = 200
)

// HandleListNiches handles GET /api/intelligence/niches.
func (h *NichesHandler) HandleListNiches(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// window — default 30d
	window := q.Get("window")
	if window == "" {
		window = "30d"
	}
	if window != "7d" && window != "30d" {
		writeError(w, http.StatusBadRequest, "invalid window")
		return
	}

	// sort — default opportunity_score
	sortMode := q.Get("sort")
	if sortMode == "" {
		sortMode = demand.SortOpportunityScore
	}
	switch sortMode {
	case demand.SortOpportunityScore, demand.SortDemandScore,
		demand.SortVelocityChangePct, demand.SortLowCoverage:
	default:
		writeError(w, http.StatusBadRequest, "invalid sort")
		return
	}

	// limit — default 50, clamped to 200
	limit := nichesDefaultLimit
	if v := q.Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
		if limit > nichesMaxLimit {
			limit = nichesMaxLimit
		}
	}

	// min_data_quality — optional
	minQuality := q.Get("min_data_quality")
	switch minQuality {
	case "", demand.QualityProxy, demand.QualityFull:
	default:
		writeError(w, http.StatusBadRequest, "invalid min_data_quality")
		return
	}

	// era — optional string, no validation (DH enum value)
	era := q.Get("era")

	// grade — optional int 7..10 (0 = no filter)
	grade := 0
	if v := q.Get("grade"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid grade")
			return
		}
		if parsed < 7 || parsed > 10 {
			writeError(w, http.StatusBadRequest, "invalid grade")
			return
		}
		grade = parsed
	}

	opts := demand.LeaderboardOpts{
		Window:         window,
		Limit:          limit,
		Sort:           sortMode,
		MinDataQuality: minQuality,
		Era:            era,
		Grade:          grade,
	}

	items, err := h.service.Leaderboard(r.Context(), opts)
	if err != nil {
		if errors.Is(err, demand.ErrInvalidWindow) {
			writeError(w, http.StatusBadRequest, "invalid window")
			return
		}
		h.logger.Error(r.Context(), "niches leaderboard failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to compute niche leaderboard")
		return
	}

	opportunities := make([]nicheOpportunityDTO, 0, len(items))
	for _, o := range items {
		opportunities = append(opportunities, toNicheDTO(o))
	}

	writeJSON(w, http.StatusOK, nichesResponse{
		Opportunities: opportunities,
		Meta: nichesMeta{
			Window:     window,
			Limit:      limit,
			Sort:       sortMode,
			TotalCount: len(items),
		},
	})
}

// --- response DTOs ---

type nichesResponse struct {
	Opportunities []nicheOpportunityDTO `json:"opportunities"`
	Meta          nichesMeta            `json:"meta"`
}

type nichesMeta struct {
	Window     string `json:"window"`
	Limit      int    `json:"limit"`
	Sort       string `json:"sort"`
	TotalCount int    `json:"total_count"`
}

type nicheOpportunityDTO struct {
	Character        string                `json:"character"`
	Era              string                `json:"era"`
	Grade            int                   `json:"grade"`
	Demand           *nicheDemandDTO       `json:"demand"`
	Market           *nicheMarketDTO       `json:"market"`
	Acceleration     *nicheAccelerationDTO `json:"acceleration"`
	Coverage         nicheCoverageDTO      `json:"coverage"`
	OpportunityScore float64               `json:"opportunity_score"`
}

type nicheDemandDTO struct {
	Score        float64 `json:"score"`
	Views        int     `json:"views"`
	WishlistAdds int     `json:"wishlist_adds"`
	DataQuality  string  `json:"data_quality"`
	ComputedAt   *string `json:"computed_at"`
}

type nicheMarketDTO struct {
	MedianDaysToSell     *float64 `json:"median_days_to_sell"`
	VelocityChangePct    *float64 `json:"velocity_change_pct"`
	AvgDailySales        *float64 `json:"avg_daily_sales"`
	SellThroughRate30d   *float64 `json:"sell_through_rate_30d"`
	SalesVolume7d        *int     `json:"sales_volume_7d"`
	SalesVolume30d       *int     `json:"sales_volume_30d"`
	SupplyCount          *int     `json:"supply_count"`
	ActiveListingCount   int      `json:"active_listing_count"`
	SampleSize           int      `json:"sample_size"`
	AnalyticsNotComputed bool     `json:"analytics_not_computed"`
	ComputedAt           *string  `json:"computed_at"`
}

type nicheAccelerationDTO struct {
	MedianVelocityChangePct float64 `json:"median_velocity_change_pct"`
	AcceleratingCount       int     `json:"accelerating_count"`
	TotalCount              int     `json:"total_count"`
	DataQuality             string  `json:"data_quality"`
	ComputedAt              *string `json:"computed_at"`
}

type nicheCoverageDTO struct {
	OurUnsoldCount    int     `json:"our_unsold_count"`
	ActiveCampaignIDs []int64 `json:"active_campaign_ids"`
	Covered           bool    `json:"covered"`
}

func toNicheDTO(o demand.NicheOpportunity) nicheOpportunityDTO {
	dto := nicheOpportunityDTO{
		Character:        o.Character,
		Era:              o.Era,
		Grade:            o.Grade,
		Coverage:         toCoverageDTO(o.Coverage),
		OpportunityScore: o.OpportunityScore,
	}
	if o.Demand != nil {
		dto.Demand = &nicheDemandDTO{
			Score:        o.Demand.Score,
			Views:        o.Demand.Views,
			WishlistAdds: o.Demand.WishlistAdds,
			DataQuality:  o.Demand.DataQuality,
			ComputedAt:   formatTimePtr(o.Demand.ComputedAt),
		}
	}
	if o.Market != nil {
		dto.Market = &nicheMarketDTO{
			MedianDaysToSell:     o.Market.MedianDaysToSell,
			VelocityChangePct:    o.Market.VelocityChangePct,
			AvgDailySales:        o.Market.AvgDailySales,
			SellThroughRate30d:   o.Market.SellThroughRate30d,
			SalesVolume7d:        o.Market.SalesVolume7d,
			SalesVolume30d:       o.Market.SalesVolume30d,
			SupplyCount:          o.Market.SupplyCount,
			ActiveListingCount:   o.Market.ActiveListingCount,
			SampleSize:           o.Market.SampleSize,
			AnalyticsNotComputed: o.Market.AnalyticsNotComputed,
			ComputedAt:           formatTimePtrFromPtr(o.Market.ComputedAt),
		}
	}
	if o.Acceleration != nil {
		dto.Acceleration = &nicheAccelerationDTO{
			MedianVelocityChangePct: o.Acceleration.MedianVelocityChangePct,
			AcceleratingCount:       o.Acceleration.AcceleratingCount,
			TotalCount:              o.Acceleration.TotalCount,
			DataQuality:             o.Acceleration.DataQuality,
			ComputedAt:              formatTimePtrFromPtr(o.Acceleration.ComputedAt),
		}
	}
	return dto
}

func toCoverageDTO(c demand.NicheCoverage) nicheCoverageDTO {
	ids := c.ActiveCampaignIDs
	if ids == nil {
		ids = []int64{}
	}
	return nicheCoverageDTO{
		OurUnsoldCount:    c.OurUnsoldCount,
		ActiveCampaignIDs: ids,
		Covered:           c.Covered,
	}
}

// formatTimePtr returns a pointer to the RFC3339 string for a non-zero time,
// or nil if the time is the zero value.
func formatTimePtr(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// formatTimePtrFromPtr returns a pointer to the RFC3339 string for a non-nil,
// non-zero time pointer, or nil otherwise.
func formatTimePtrFromPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	return formatTimePtr(*t)
}
