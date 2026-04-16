package handlers

import (
	"context"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CampaignSignalsService is the narrow seam the campaign-signals handler
// depends on. Both *demand.Service and test stubs satisfy it.
type CampaignSignalsService interface {
	CampaignSignals(ctx context.Context) (demand.CampaignSignalsResponse, error)
}

// CampaignSignalsHandler serves GET /api/intelligence/campaign-signals.
type CampaignSignalsHandler struct {
	service CampaignSignalsService
	logger  observability.Logger
}

// NewCampaignSignalsHandler constructs a CampaignSignalsHandler.
func NewCampaignSignalsHandler(service CampaignSignalsService, logger observability.Logger) *CampaignSignalsHandler {
	return &CampaignSignalsHandler{service: service, logger: logger}
}

// HandleGetCampaignSignals handles GET /api/intelligence/campaign-signals.
func (h *CampaignSignalsHandler) HandleGetCampaignSignals(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.CampaignSignals(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "campaign signals failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to compute campaign signals")
		return
	}
	if resp.SkippedRows > 0 {
		h.logger.Warn(r.Context(), "campaign signals: skipped unparseable cache rows",
			observability.Int("skipped_rows", resp.SkippedRows))
	}

	writeJSON(w, http.StatusOK, toCampaignSignalsDTO(resp))
}

// --- response DTOs ---

type campaignSignalsResponseDTO struct {
	ComputedAt  *string             `json:"computed_at"`
	DataQuality string              `json:"data_quality"`
	Signals     []campaignSignalDTO `json:"signals"`
}

type campaignSignalDTO struct {
	CampaignID              int64                          `json:"campaign_id"`
	CampaignName            string                         `json:"campaign_name"`
	TrackedCharacters       int                            `json:"tracked_characters"`
	AcceleratingCount       int                            `json:"accelerating_count"`
	DeceleratingCount       int                            `json:"decelerating_count"`
	MedianVelocityChangePct float64                        `json:"median_velocity_change_pct"`
	DataQuality             string                         `json:"data_quality"`
	ComputedAt              *string                        `json:"computed_at"`
	TopAccelerating         []campaignSignalContributorDTO `json:"top_accelerating"`
	TopDecelerating         []campaignSignalContributorDTO `json:"top_decelerating"`
}

type campaignSignalContributorDTO struct {
	Character         string   `json:"character"`
	VelocityChangePct float64  `json:"velocity_change_pct"`
	MedianDaysToSell  *float64 `json:"median_days_to_sell"`
	SampleSize        int      `json:"sample_size"`
}

func toCampaignSignalsDTO(r demand.CampaignSignalsResponse) campaignSignalsResponseDTO {
	signals := make([]campaignSignalDTO, 0, len(r.Signals))
	for _, s := range r.Signals {
		signals = append(signals, toCampaignSignalDTO(s))
	}
	return campaignSignalsResponseDTO{
		ComputedAt:  formatTimePtrFromPtr(r.ComputedAt),
		DataQuality: r.DataQuality,
		Signals:     signals,
	}
}

func toCampaignSignalDTO(s demand.CampaignSignal) campaignSignalDTO {
	top := make([]campaignSignalContributorDTO, 0, len(s.TopAccelerating))
	for _, c := range s.TopAccelerating {
		top = append(top, toContributorDTO(c))
	}
	bottom := make([]campaignSignalContributorDTO, 0, len(s.TopDecelerating))
	for _, c := range s.TopDecelerating {
		bottom = append(bottom, toContributorDTO(c))
	}
	return campaignSignalDTO{
		CampaignID:              s.CampaignID,
		CampaignName:            s.CampaignName,
		TrackedCharacters:       s.TrackedCharacters,
		AcceleratingCount:       s.AcceleratingCount,
		DeceleratingCount:       s.DeceleratingCount,
		MedianVelocityChangePct: s.MedianVelocityChangePct,
		DataQuality:             s.DataQuality,
		ComputedAt:              formatTimePtrFromPtr(s.ComputedAt),
		TopAccelerating:         top,
		TopDecelerating:         bottom,
	}
}

func toContributorDTO(c demand.CampaignSignalContributor) campaignSignalContributorDTO {
	return campaignSignalContributorDTO{
		Character:         c.Character,
		VelocityChangePct: c.VelocityChangePct,
		MedianDaysToSell:  c.MedianDaysToSell,
		SampleSize:        c.SampleSize,
	}
}
