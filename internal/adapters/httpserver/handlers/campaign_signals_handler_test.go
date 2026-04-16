package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestCampaignSignalsHandler(t *testing.T) {
	computed := time.Date(2026, 4, 15, 3, 15, 0, 0, time.UTC)
	median := 11.0

	happyResp := demand.CampaignSignalsResponse{
		ComputedAt:  &computed,
		DataQuality: demand.QualityFull,
		Signals: []demand.CampaignSignal{{
			CampaignID:              1,
			CampaignName:            "Vintage Core",
			TrackedCharacters:       3,
			AcceleratingCount:       1,
			DeceleratingCount:       0,
			MedianVelocityChangePct: 12.4,
			DataQuality:             demand.QualityFull,
			ComputedAt:              &computed,
			TopAccelerating: []demand.CampaignSignalContributor{
				{Character: "Pikachu", VelocityChangePct: 22.1, MedianDaysToSell: &median, SampleSize: 34},
			},
		}},
	}
	emptyResp := demand.CampaignSignalsResponse{
		DataQuality: demand.QualityEmpty,
		Signals:     nil,
	}

	tests := []struct {
		name       string
		mockResp   demand.CampaignSignalsResponse
		mockErr    error
		wantStatus int
		assertBody func(t *testing.T, body campaignSignalsResponseDTO)
	}{
		{
			name:       "happy path",
			mockResp:   happyResp,
			wantStatus: http.StatusOK,
			assertBody: func(t *testing.T, body campaignSignalsResponseDTO) {
				assert.Equal(t, "full", body.DataQuality)
				require.Len(t, body.Signals, 1)
				sig := body.Signals[0]
				assert.Equal(t, int64(1), sig.CampaignID)
				assert.Equal(t, "Vintage Core", sig.CampaignName)
				require.Len(t, sig.TopAccelerating, 1)
				assert.Equal(t, "Pikachu", sig.TopAccelerating[0].Character)
				// TopDecelerating must serialize as [] not null so JSON consumers
				// don't need a nil-check on every signal.
				require.NotNil(t, sig.TopDecelerating)
				assert.Empty(t, sig.TopDecelerating)
			},
		},
		{
			name:       "empty cache",
			mockResp:   emptyResp,
			wantStatus: http.StatusOK,
			assertBody: func(t *testing.T, body campaignSignalsResponseDTO) {
				require.NotNil(t, body.Signals, "signals must be [] not null")
				assert.Empty(t, body.Signals)
				assert.Equal(t, "empty", body.DataQuality)
				assert.Nil(t, body.ComputedAt)
			},
		},
		{
			name:       "service error",
			mockErr:    errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			assertBody: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mocks.DemandServiceMock{
				CampaignSignalsFn: func(ctx context.Context) (demand.CampaignSignalsResponse, error) {
					return tc.mockResp, tc.mockErr
				},
			}

			h := NewCampaignSignalsHandler(svc, mocks.NewMockLogger())
			req := httptest.NewRequest(http.MethodGet, "/api/intelligence/campaign-signals", nil)
			w := httptest.NewRecorder()

			h.HandleGetCampaignSignals(w, req)

			require.Equal(t, tc.wantStatus, w.Code, "body=%s", w.Body.String())
			if tc.assertBody == nil {
				return
			}
			var body campaignSignalsResponseDTO
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			tc.assertBody(t, body)
		})
	}
}
