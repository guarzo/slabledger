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

func floatPtr(v float64) *float64    { return &v }
func timePtr(t time.Time) *time.Time { return &t }

func TestNichesHandler_HappyPath(t *testing.T) {
	computed := time.Date(2026, 4, 15, 2, 0, 0, 0, time.UTC)
	analytics := time.Date(2026, 4, 15, 3, 15, 0, 0, time.UTC)

	svc := &mocks.DemandServiceMock{
		LeaderboardFn: func(_ context.Context, opts demand.LeaderboardOpts) ([]demand.NicheOpportunity, error) {
			assert.Equal(t, "30d", opts.Window)
			assert.Equal(t, demand.SortOpportunityScore, opts.Sort)
			assert.Equal(t, 50, opts.Limit)
			return []demand.NicheOpportunity{
				{
					Character: "Umbreon",
					Era:       "sword_shield",
					Grade:     10,
					Demand: &demand.NicheDemand{
						Score: 0.82, Views: 843, WishlistAdds: 47,
						DataQuality: "proxy", ComputedAt: computed,
					},
					Market: &demand.NicheMarket{
						MedianDaysToSell:   floatPtr(9.8),
						VelocityChangePct:  floatPtr(15.2),
						ActiveListingCount: 42,
						SampleSize:         312,
						ComputedAt:         timePtr(analytics),
					},
					Acceleration: &demand.NicheAcceleration{
						MedianVelocityChangePct: 14.2,
						AcceleratingCount:       1,
						TotalCount:              1,
						DataQuality:             demand.QualityFull,
					},
					Coverage: demand.NicheCoverage{
						OurUnsoldCount: 2, ActiveCampaignIDs: []string{}, Covered: false,
					},
					OpportunityScore: 0.64,
				},
			}, nil
		},
	}

	h := NewNichesHandler(svc, mocks.NewMockLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/intelligence/niches", nil)
	w := httptest.NewRecorder()
	h.HandleListNiches(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp nichesResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

	require.Len(t, resp.Opportunities, 1)
	o := resp.Opportunities[0]
	assert.Equal(t, "Umbreon", o.Character)
	assert.Equal(t, "sword_shield", o.Era)
	assert.Equal(t, 10, o.Grade)
	assert.Equal(t, 0.64, o.OpportunityScore)

	require.NotNil(t, o.Demand)
	assert.Equal(t, 0.82, o.Demand.Score)
	assert.Equal(t, "proxy", o.Demand.DataQuality)
	require.NotNil(t, o.Demand.ComputedAt)
	assert.Equal(t, "2026-04-15T02:00:00Z", *o.Demand.ComputedAt)

	require.NotNil(t, o.Market)
	require.NotNil(t, o.Market.MedianDaysToSell)
	assert.Equal(t, 9.8, *o.Market.MedianDaysToSell)
	require.NotNil(t, o.Market.VelocityChangePct)
	assert.Equal(t, 15.2, *o.Market.VelocityChangePct)
	assert.Equal(t, 42, o.Market.ActiveListingCount)
	assert.Equal(t, 312, o.Market.SampleSize)
	assert.False(t, o.Market.AnalyticsNotComputed)
	require.NotNil(t, o.Market.ComputedAt)
	assert.Equal(t, "2026-04-15T03:15:00Z", *o.Market.ComputedAt)

	require.NotNil(t, o.Acceleration)
	assert.Equal(t, 14.2, o.Acceleration.MedianVelocityChangePct)
	assert.Equal(t, 1, o.Acceleration.AcceleratingCount)
	assert.Equal(t, 1, o.Acceleration.TotalCount)
	assert.Equal(t, demand.QualityFull, o.Acceleration.DataQuality)

	assert.Equal(t, 2, o.Coverage.OurUnsoldCount)
	assert.NotNil(t, o.Coverage.ActiveCampaignIDs)
	assert.Empty(t, o.Coverage.ActiveCampaignIDs)
	assert.False(t, o.Coverage.Covered)

	assert.Equal(t, "30d", resp.Meta.Window)
	assert.Equal(t, 50, resp.Meta.Limit)
	assert.Equal(t, demand.SortOpportunityScore, resp.Meta.Sort)
	assert.Equal(t, 1, resp.Meta.TotalCount)
}

func TestNichesHandler_QueryParams(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantErr    string
		check      func(t *testing.T, opts demand.LeaderboardOpts)
	}{
		{
			name:       "invalid window",
			query:      "?window=14d",
			wantStatus: http.StatusBadRequest,
			wantErr:    "invalid window",
		},
		{
			name:       "invalid sort",
			query:      "?sort=random",
			wantStatus: http.StatusBadRequest,
			wantErr:    "invalid sort",
		},
		{
			name:       "invalid min_data_quality",
			query:      "?min_data_quality=bogus",
			wantStatus: http.StatusBadRequest,
			wantErr:    "invalid min_data_quality",
		},
		{
			name:       "invalid limit (non-numeric)",
			query:      "?limit=abc",
			wantStatus: http.StatusBadRequest,
			wantErr:    "invalid limit",
		},
		{
			name:       "invalid limit (zero)",
			query:      "?limit=0",
			wantStatus: http.StatusBadRequest,
			wantErr:    "invalid limit",
		},
		{
			name:       "grade out of range low",
			query:      "?grade=5",
			wantStatus: http.StatusBadRequest,
			wantErr:    "invalid grade",
		},
		{
			name:       "grade out of range high",
			query:      "?grade=11",
			wantStatus: http.StatusBadRequest,
			wantErr:    "invalid grade",
		},
		{
			name:       "limit clamped to 200",
			query:      "?limit=500",
			wantStatus: http.StatusOK,
			check: func(t *testing.T, opts demand.LeaderboardOpts) {
				assert.Equal(t, 200, opts.Limit)
			},
		},
		{
			name:       "all valid params honoured",
			query:      "?window=7d&limit=25&sort=demand_score&min_data_quality=full&era=base&grade=9",
			wantStatus: http.StatusOK,
			check: func(t *testing.T, opts demand.LeaderboardOpts) {
				assert.Equal(t, "7d", opts.Window)
				assert.Equal(t, 25, opts.Limit)
				assert.Equal(t, demand.SortDemandScore, opts.Sort)
				assert.Equal(t, "full", opts.MinDataQuality)
				assert.Equal(t, "base", opts.Era)
				assert.Equal(t, 9, opts.Grade)
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var capturedOpts demand.LeaderboardOpts
			svc := &mocks.DemandServiceMock{
				LeaderboardFn: func(_ context.Context, opts demand.LeaderboardOpts) ([]demand.NicheOpportunity, error) {
					capturedOpts = opts
					return nil, nil
				},
			}
			h := NewNichesHandler(svc, mocks.NewMockLogger())
			req := httptest.NewRequest(http.MethodGet, "/api/intelligence/niches"+tc.query, nil)
			w := httptest.NewRecorder()
			h.HandleListNiches(w, req)

			assert.Equal(t, tc.wantStatus, w.Code)
			if tc.wantErr != "" {
				var body map[string]string
				require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
				assert.Equal(t, tc.wantErr, body["error"])
			}
			if tc.check != nil {
				tc.check(t, capturedOpts)
			}
		})
	}
}

func TestNichesHandler_EmptyResult(t *testing.T) {
	svc := &mocks.DemandServiceMock{
		LeaderboardFn: func(_ context.Context, _ demand.LeaderboardOpts) ([]demand.NicheOpportunity, error) {
			return nil, nil
		},
	}
	h := NewNichesHandler(svc, mocks.NewMockLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/intelligence/niches", nil)
	w := httptest.NewRecorder()
	h.HandleListNiches(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	// Assert raw JSON emits `[]`, not `null`, for the opportunities field.
	assert.Contains(t, w.Body.String(), `"opportunities":[]`)

	var resp nichesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Opportunities)
	assert.Empty(t, resp.Opportunities)
	assert.Equal(t, 0, resp.Meta.TotalCount)
}

func TestNichesHandler_NilDemandAndMarket(t *testing.T) {
	svc := &mocks.DemandServiceMock{
		LeaderboardFn: func(_ context.Context, _ demand.LeaderboardOpts) ([]demand.NicheOpportunity, error) {
			return []demand.NicheOpportunity{
				{
					Character: "Pikachu",
					Grade:     10,
					Demand:    nil,
					Market:    nil,
					Coverage: demand.NicheCoverage{
						OurUnsoldCount: 0, ActiveCampaignIDs: nil, Covered: false,
					},
				},
			}, nil
		},
	}
	h := NewNichesHandler(svc, mocks.NewMockLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/intelligence/niches", nil)
	w := httptest.NewRecorder()
	h.HandleListNiches(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	// demand and market fields must serialize as JSON null, not be omitted.
	body := w.Body.String()
	assert.Contains(t, body, `"demand":null`)
	assert.Contains(t, body, `"market":null`)
	// Nil ActiveCampaignIDs must become [] in JSON.
	assert.Contains(t, body, `"active_campaign_ids":[]`)
}

func TestNichesHandler_NilMarketNumericPointers(t *testing.T) {
	// Market present but velocity/days-to-sell/computed_at are nil.
	svc := &mocks.DemandServiceMock{
		LeaderboardFn: func(_ context.Context, _ demand.LeaderboardOpts) ([]demand.NicheOpportunity, error) {
			return []demand.NicheOpportunity{
				{
					Character: "Charizard",
					Grade:     10,
					Market: &demand.NicheMarket{
						MedianDaysToSell:     nil,
						VelocityChangePct:    nil,
						AnalyticsNotComputed: true,
						ComputedAt:           nil,
					},
				},
			}, nil
		},
	}
	h := NewNichesHandler(svc, mocks.NewMockLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/intelligence/niches", nil)
	w := httptest.NewRecorder()
	h.HandleListNiches(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"median_days_to_sell":null`)
	assert.Contains(t, body, `"velocity_change_pct":null`)
	assert.Contains(t, body, `"analytics_not_computed":true`)
	// Market's ComputedAt pointer is nil.
	assert.Contains(t, body, `"computed_at":null`)
}

func TestNichesHandler_ServiceError(t *testing.T) {
	svc := &mocks.DemandServiceMock{
		LeaderboardFn: func(_ context.Context, _ demand.LeaderboardOpts) ([]demand.NicheOpportunity, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewNichesHandler(svc, mocks.NewMockLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/intelligence/niches", nil)
	w := httptest.NewRecorder()
	h.HandleListNiches(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestNichesHandler_ServiceInvalidWindow(t *testing.T) {
	// If the service returns ErrInvalidWindow (shouldn't happen after handler
	// validation, but defensive) we translate to 400.
	svc := &mocks.DemandServiceMock{
		LeaderboardFn: func(_ context.Context, _ demand.LeaderboardOpts) ([]demand.NicheOpportunity, error) {
			return nil, demand.ErrInvalidWindow
		},
	}
	h := NewNichesHandler(svc, mocks.NewMockLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/intelligence/niches", nil)
	w := httptest.NewRecorder()
	h.HandleListNiches(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
