package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

			body := `{"pushId":"push-123","approvedBy":"alice"}`
			req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/psa-publish", strings.NewReader(body))
			req.SetPathValue("id", "c1")
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
	body := `{"pushId":"push-123","approvedBy":"alice"}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/psa-publish", strings.NewReader(body))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandlePSAPublish(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}
