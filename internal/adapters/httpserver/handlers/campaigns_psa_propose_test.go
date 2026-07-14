package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// noDiffPortal matches the internal campaign fields exactly so
// psacampaign.TranslateToDiff produces zero changes.
func noDiffPortal() psacampaign.PortalCampaign {
	return psacampaign.PortalCampaign{
		CampaignRequestID: "portal-1",
		BuyPercentClv:     75,
		DailyBudgetCents:  400000,
		BuyBox: psacampaign.CampaignBuyBox{
			GradeMin: "9", GradeMax: "10", YearMin: 2020, YearMax: 2024,
			PriceMinCents: 10000, PriceMaxCents: 300000, ClvConfidenceMin: 3,
		},
	}
}

func diffCampaign() inventory.Campaign {
	return inventory.Campaign{
		ID:                   "c1",
		PSACampaignRequestID: "portal-1",
		BuyTermsCLPct:        0.75,
		DailySpendCapCents:   400000,
		GradeRange:           "9-10",
		YearRange:            "2020-2024",
		PriceRange:           "100-3000",
		CLConfidence:         "3-4",
	}
}

func TestHandlePSAPropose(t *testing.T) {
	tests := []struct {
		name           string
		noSnapshots    bool
		noQueue        bool
		campaign       *inventory.Campaign
		getCampaignErr error
		portalRows     []psacampaign.PortalCampaign
		wantStatus     int
		wantEnqueue    bool
		wantPushID     bool
	}{
		{
			name:       "not linked",
			campaign:   &inventory.Campaign{ID: "c1", PSACampaignRequestID: ""},
			portalRows: []psacampaign.PortalCampaign{noDiffPortal()},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "linked but no matching portal campaign",
			campaign:   &inventory.Campaign{ID: "c1", PSACampaignRequestID: "missing"},
			portalRows: []psacampaign.PortalCampaign{noDiffPortal()},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "linked with real diff",
			campaign: func() *inventory.Campaign {
				c := diffCampaign()
				c.BuyTermsCLPct = 0.80 // differs from portal's 75
				return &c
			}(),
			portalRows:  []psacampaign.PortalCampaign{noDiffPortal()},
			wantStatus:  http.StatusOK,
			wantEnqueue: true,
			wantPushID:  true,
		},
		{
			name: "linked with no diff",
			campaign: func() *inventory.Campaign {
				c := diffCampaign()
				return &c
			}(),
			portalRows:  []psacampaign.PortalCampaign{noDiffPortal()},
			wantStatus:  http.StatusOK,
			wantEnqueue: false,
			wantPushID:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enqueued := false
			var enqueuedRow psacampaign.PushRow

			svc := &mocks.MockInventoryService{
				GetCampaignFn: func(ctx context.Context, id string) (*inventory.Campaign, error) {
					if tt.getCampaignErr != nil {
						return nil, tt.getCampaignErr
					}
					return tt.campaign, nil
				},
			}
			snap := &mocks.SnapshotStoreMock{
				GetSnapshotFn: func(ctx context.Context) ([]psacampaign.PortalCampaign, time.Time, error) {
					return tt.portalRows, time.Now(), nil
				},
			}
			queue := &mocks.PushQueueStoreMock{
				EnqueueFn: func(ctx context.Context, p psacampaign.PushRow) error {
					enqueued = true
					enqueuedRow = p
					return nil
				},
			}

			var opts []CampaignsHandlerOption
			if !tt.noSnapshots {
				opts = append(opts, WithPSASnapshotStore(snap))
			}
			if !tt.noQueue {
				opts = append(opts, WithPSAPushQueue(queue))
			}
			h := NewCampaignsHandler(svc, nil, nil, nil, observability.NewNoopLogger(), context.Background(), opts...)

			req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/psa-propose", strings.NewReader(`{}`))
			req.SetPathValue("id", "c1")
			rec := httptest.NewRecorder()
			h.HandlePSAPropose(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if enqueued != tt.wantEnqueue {
				t.Fatalf("Enqueue called = %v, want %v", enqueued, tt.wantEnqueue)
			}
			if tt.wantEnqueue {
				if enqueuedRow.Status != psacampaign.PushPending {
					t.Errorf("enqueued row status = %v, want PushPending", enqueuedRow.Status)
				}
				if len(enqueuedRow.Diff.Changes) == 0 {
					t.Error("enqueued row diff is empty, want non-empty")
				}
			}
			if tt.wantStatus == http.StatusOK {
				var resp psaProposeResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if tt.wantPushID && resp.PushID == "" {
					t.Error("expected non-empty pushId")
				}
				if !tt.wantPushID && resp.PushID != "" {
					t.Errorf("expected empty pushId, got %q", resp.PushID)
				}
			}
		})
	}
}

func TestHandlePSAPropose_NoSnapshotsOrQueue(t *testing.T) {
	tests := []struct {
		name        string
		noSnapshots bool
		noQueue     bool
	}{
		{name: "no snapshots store", noSnapshots: true, noQueue: false},
		{name: "no push queue", noSnapshots: false, noQueue: true},
		{name: "neither configured", noSnapshots: true, noQueue: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []CampaignsHandlerOption
			if !tt.noSnapshots {
				opts = append(opts, WithPSASnapshotStore(&mocks.SnapshotStoreMock{}))
			}
			if !tt.noQueue {
				opts = append(opts, WithPSAPushQueue(&mocks.PushQueueStoreMock{}))
			}
			h := NewCampaignsHandler(&mocks.MockInventoryService{}, nil, nil, nil, observability.NewNoopLogger(), context.Background(), opts...)

			req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/psa-propose", strings.NewReader(`{}`))
			req.SetPathValue("id", "c1")
			rec := httptest.NewRecorder()
			h.HandlePSAPropose(rec, req)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandlePSALink(t *testing.T) {
	tests := []struct {
		name           string
		getCampaignErr error
		wantStatus     int
	}{
		{
			name:       "valid link",
			wantStatus: http.StatusOK,
		},
		{
			name:           "campaign not found",
			getCampaignErr: inventory.ErrCampaignNotFound,
			wantStatus:     http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotUpdate *inventory.Campaign
			svc := &mocks.MockInventoryService{
				GetCampaignFn: func(ctx context.Context, id string) (*inventory.Campaign, error) {
					if tt.getCampaignErr != nil {
						return nil, tt.getCampaignErr
					}
					return &inventory.Campaign{ID: id}, nil
				},
				UpdateCampaignFn: func(ctx context.Context, c *inventory.Campaign) error {
					gotUpdate = c
					return nil
				},
			}
			h := NewCampaignsHandler(svc, nil, nil, nil, observability.NewNoopLogger(), context.Background())

			body := `{"psaCampaignRequestId":"portal-99"}`
			req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/psa-link", strings.NewReader(body))
			req.SetPathValue("id", "c1")
			rec := httptest.NewRecorder()
			h.HandlePSALink(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.wantStatus == http.StatusOK {
				if gotUpdate == nil {
					t.Fatal("UpdateCampaign not called")
				}
				if gotUpdate.PSACampaignRequestID != "portal-99" {
					t.Errorf("PSACampaignRequestID = %q, want portal-99", gotUpdate.PSACampaignRequestID)
				}
				var resp inventory.Campaign
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if resp.PSACampaignRequestID != "portal-99" {
					t.Errorf("response PSACampaignRequestID = %q, want portal-99", resp.PSACampaignRequestID)
				}
			} else {
				if gotUpdate != nil {
					t.Error("UpdateCampaign should not be called on not-found path")
				}
			}
		})
	}
}
