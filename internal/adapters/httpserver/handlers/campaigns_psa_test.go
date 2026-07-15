package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newTestPSAHandler(snap *mocks.SnapshotStoreMock, queue *mocks.PushQueueStoreMock) *CampaignsHandler {
	var opts []CampaignsHandlerOption
	if snap != nil {
		opts = append(opts, WithPSASnapshotStore(snap))
	}
	if queue != nil {
		opts = append(opts, WithPSAPushQueue(queue))
	}
	return NewCampaignsHandler(nil, nil, nil, nil, observability.NewNoopLogger(), context.Background(), opts...)
}

func TestHandleListPSACampaigns_NoStore(t *testing.T) {
	h := newTestPSAHandler(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/psa-campaigns", nil)
	rec := httptest.NewRecorder()
	h.HandleListPSACampaigns(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleListPSACampaigns_Success(t *testing.T) {
	snap := &mocks.SnapshotStoreMock{
		GetSnapshotFn: func(ctx context.Context) ([]psacampaign.PortalCampaign, time.Time, error) {
			return []psacampaign.PortalCampaign{{CampaignRequestID: "pc1"}}, time.Now(), nil
		},
	}
	h := newTestPSAHandler(snap, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/psa-campaigns", nil)
	rec := httptest.NewRecorder()
	h.HandleListPSACampaigns(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandlePSAPublish(t *testing.T) {
	tests := []struct {
		name       string
		approveErr error
		wantStatus int
	}{
		{
			name:       "pending row approved",
			approveErr: nil,
			wantStatus: http.StatusOK,
		},
		{
			name:       "not pending",
			approveErr: psacampaign.ErrPushNotPending,
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotID, gotApprovedBy string
			queue := &mocks.PushQueueStoreMock{
				ApproveFn: func(ctx context.Context, id, approvedBy string) error {
					gotID = id
					gotApprovedBy = approvedBy
					return tt.approveErr
				},
			}
			h := newTestPSAHandler(nil, queue)

			body := `{"pushId":"push-123"}`
			req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/psa-publish", strings.NewReader(body))
			req.SetPathValue("id", "c1")
			ctx := context.WithValue(req.Context(), middleware.UserContextKey, &auth.User{Username: "alice"})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			h.HandlePSAPublish(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.approveErr == nil {
				if gotID != "push-123" || gotApprovedBy != "alice" {
					t.Fatalf("Approve called with wrong args: id=%s approvedBy=%s", gotID, gotApprovedBy)
				}
			}
		})
	}
}

func TestHandlePSAPublish_NoQueue(t *testing.T) {
	h := newTestPSAHandler(nil, nil)
	body := `{"pushId":"push-123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/psa-publish", strings.NewReader(body))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandlePSAPublish(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandlePSAPublish_Unauthenticated(t *testing.T) {
	queue := &mocks.PushQueueStoreMock{
		ApproveFn: func(ctx context.Context, id, approvedBy string) error {
			t.Fatal("Approve should not be called without an authenticated user")
			return nil
		},
	}
	h := newTestPSAHandler(nil, queue)
	body := `{"pushId":"push-123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/psa-publish", strings.NewReader(body))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandlePSAPublish(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListPSAPushes(t *testing.T) {
	fd := psacampaign.CampaignFormData{CampaignName: "Modern 10s", BidPercentage: 72}
	tests := []struct {
		name       string
		queue      *mocks.PushQueueStoreMock
		wantStatus int
		wantBody   []string // substrings that must appear in the response body
	}{
		{
			name:       "queue not configured returns 503",
			queue:      nil,
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name: "create row maps formData, update row maps diff",
			queue: &mocks.PushQueueStoreMock{
				LatestPerCampaignFn: func(ctx context.Context) ([]psacampaign.PushRow, error) {
					return []psacampaign.PushRow{
						{
							ID: "push-c", Operation: psacampaign.OpCreate, InternalCampaignID: "camp-1",
							RequestedBy: "alice", Status: psacampaign.PushPending,
							Diff:      psacampaign.ProposedDiff{Create: &fd},
							UpdatedAt: time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
						},
						{
							ID: "push-u", Operation: psacampaign.OpUpdate, InternalCampaignID: "camp-2",
							PSACampaignID: "psa-2", RequestedBy: "bob", ApprovedBy: "carol",
							Status: psacampaign.PushFailed, Error: "portal 500",
							Diff: psacampaign.ProposedDiff{Changes: []psacampaign.FieldChange{
								{Field: "bidPercentage", Old: "70", New: "72"},
							}},
							UpdatedAt: time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC),
						},
					}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantBody: []string{
				`"campaignId":"camp-1"`, `"pushId":"push-c"`, `"operation":"create"`,
				`"formData"`, `"campaignName":"Modern 10s"`,
				`"campaignId":"camp-2"`, `"operation":"update"`, `"status":"failed"`,
				`"error":"portal 500"`, `"diff"`, `"bidPercentage"`, `"approvedBy":"carol"`,
			},
		},
		{
			name: "empty queue returns empty array not null",
			queue: &mocks.PushQueueStoreMock{
				LatestPerCampaignFn: func(ctx context.Context) ([]psacampaign.PushRow, error) {
					return nil, nil
				},
			},
			wantStatus: http.StatusOK,
			wantBody:   []string{`"pushes":[]`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestPSAHandler(nil, tt.queue)
			req := httptest.NewRequest(http.MethodGet, "/api/psa-pushes", nil)
			rec := httptest.NewRecorder()
			h.HandleListPSAPushes(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			for _, want := range tt.wantBody {
				if !strings.Contains(rec.Body.String(), want) {
					t.Fatalf("body missing %q: %s", want, rec.Body.String())
				}
			}
			if tt.name == "create row maps formData, update row maps diff" {
				body := rec.Body.String()
				if strings.Contains(body, `"formData":null`) || strings.Contains(body, `"diff":null`) {
					t.Fatalf("null formData/diff should be omitted: %s", body)
				}
			}
		})
	}
}
