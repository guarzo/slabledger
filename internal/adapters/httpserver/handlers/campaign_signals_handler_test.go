package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

type campaignSignalsServiceStub struct {
	resp demand.CampaignSignalsResponse
	err  error
}

func (s campaignSignalsServiceStub) CampaignSignals(ctx context.Context) (demand.CampaignSignalsResponse, error) {
	return s.resp, s.err
}

func TestCampaignSignalsHandler_HappyPath(t *testing.T) {
	computed := time.Date(2026, 4, 15, 3, 15, 0, 0, time.UTC)
	median := 11.0
	svc := campaignSignalsServiceStub{
		resp: demand.CampaignSignalsResponse{
			ComputedAt:  &computed,
			DataQuality: demand.DataQualityFull,
			Signals: []demand.CampaignSignal{{
				CampaignID:              1,
				CampaignName:            "Vintage Core",
				TrackedCharacters:       3,
				AcceleratingCount:       1,
				DeceleratingCount:       0,
				MedianVelocityChangePct: 12.4,
				DataQuality:             demand.DataQualityFull,
				ComputedAt:              &computed,
				TopAccelerating: []demand.CampaignSignalContributor{
					{Character: "Pikachu", VelocityChangePct: 22.1, MedianDaysToSell: &median, SampleSize: 34},
				},
			}},
		},
	}

	h := NewCampaignSignalsHandler(svc, mocks.NewMockLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/intelligence/campaign-signals", nil)
	w := httptest.NewRecorder()

	h.HandleGetCampaignSignals(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp campaignSignalsResponseDTO
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.DataQuality != "full" {
		t.Errorf("want data_quality=full, got %s", resp.DataQuality)
	}
	if len(resp.Signals) != 1 {
		t.Fatalf("want 1 signal, got %d", len(resp.Signals))
	}
	if resp.Signals[0].CampaignID != 1 || resp.Signals[0].CampaignName != "Vintage Core" {
		t.Errorf("want campaign 1, got %+v", resp.Signals[0])
	}
	if len(resp.Signals[0].TopAccelerating) != 1 || resp.Signals[0].TopAccelerating[0].Character != "Pikachu" {
		t.Errorf("want Pikachu in top_accelerating, got %+v", resp.Signals[0].TopAccelerating)
	}
	if resp.Signals[0].TopDecelerating == nil {
		t.Errorf("want top_decelerating to be [] not nil (would serialize as null)")
	}
}

func TestCampaignSignalsHandler_Empty(t *testing.T) {
	svc := campaignSignalsServiceStub{
		resp: demand.CampaignSignalsResponse{
			DataQuality: demand.DataQualityEmpty,
			Signals:     nil,
		},
	}
	h := NewCampaignSignalsHandler(svc, mocks.NewMockLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/intelligence/campaign-signals", nil)
	w := httptest.NewRecorder()

	h.HandleGetCampaignSignals(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var resp campaignSignalsResponseDTO
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Signals == nil {
		t.Errorf("want signals to be [] not null")
	}
	if resp.DataQuality != "empty" {
		t.Errorf("want quality=empty, got %s", resp.DataQuality)
	}
	if resp.ComputedAt != nil {
		t.Errorf("want computed_at=null, got %v", resp.ComputedAt)
	}
}

func TestCampaignSignalsHandler_ServiceError(t *testing.T) {
	svc := campaignSignalsServiceStub{
		err: errors.New("boom"),
	}
	h := NewCampaignSignalsHandler(svc, mocks.NewMockLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/intelligence/campaign-signals", nil)
	w := httptest.NewRecorder()

	h.HandleGetCampaignSignals(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}
