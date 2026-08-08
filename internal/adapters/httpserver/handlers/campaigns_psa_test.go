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
	"github.com/guarzo/slabledger/internal/platform/crypto"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newTestPSAHandler(snap *mocks.SnapshotStoreMock, queue *mocks.PushQueueStoreMock, extra ...CampaignsHandlerOption) *CampaignsHandler {
	var opts []CampaignsHandlerOption
	if snap != nil {
		opts = append(opts, WithPSASnapshotStore(snap))
	}
	if queue != nil {
		opts = append(opts, WithPSAPushQueue(queue))
	}
	opts = append(opts, extra...)
	return NewCampaignsHandler(nil, nil, nil, nil, observability.NewNoopLogger(), context.Background(), opts...)
}

// testApprovalSigner mints the signer the publish endpoint needs. Without one
// HandlePSAPublish answers 503, so every publish test that expects to reach the
// queue must pass this.
func testApprovalSigner(t *testing.T) psacampaign.ApprovalSigner {
	t.Helper()
	s, err := crypto.NewHMACSigner(strings.Repeat("k", 32), "test-key")
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	return s
}

// pendingPushRow is the row Get returns for publish tests: a plausible pending
// update whose diff is what the approval signature ends up covering.
func pendingPushRow(id string) psacampaign.PushRow {
	return psacampaign.PushRow{
		ID:                 id,
		Operation:          psacampaign.OpUpdate,
		InternalCampaignID: "c1",
		PSACampaignID:      "pc1",
		Status:             psacampaign.PushPending,
		Diff: psacampaign.ProposedDiff{
			Changes: []psacampaign.FieldChange{{Field: "bidPercentage", Old: "70", New: "72"}},
		},
	}
}

// freshCatalog returns a CatalogStoreMock whose fetchedAt is always "now", so
// tests exercising propose/create do not trip the new staleness guard.
func freshCatalog(specLists []psacampaign.SpecListRef, subjects []psacampaign.SubjectRef) *mocks.CatalogStoreMock {
	now := time.Now()
	return &mocks.CatalogStoreMock{
		SpecListsFn: func(ctx context.Context) ([]psacampaign.SpecListRef, time.Time, error) {
			return specLists, now, nil
		},
		SubjectsFn: func(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, time.Time, error) {
			return subjects, now, nil
		},
	}
}

// staleCatalog returns a CatalogStoreMock whose fetchedAt is well past
// psacampaign.CatalogMaxAge, so buildResolver returns psacampaign.ErrCatalogStale
// and the caller must map that to a 503 naming the harvester.
func staleCatalog(specLists []psacampaign.SpecListRef, subjects []psacampaign.SubjectRef) *mocks.CatalogStoreMock {
	stale := time.Now().Add(-2 * psacampaign.CatalogMaxAge)
	return &mocks.CatalogStoreMock{
		SpecListsFn: func(ctx context.Context) ([]psacampaign.SpecListRef, time.Time, error) {
			return specLists, stale, nil
		},
		SubjectsFn: func(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, time.Time, error) {
			return subjects, stale, nil
		},
	}
}

func TestHandleGetPSASubjects_NoStore(t *testing.T) {
	h := NewCampaignsHandler(nil, nil, nil, nil, observability.NewNoopLogger(), context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/psa/subjects", nil)
	rec := httptest.NewRecorder()
	h.HandleGetPSASubjects(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetPSASubjects_Success(t *testing.T) {
	fetchedAt := time.Now().Add(-time.Hour)
	catalog := &mocks.CatalogStoreMock{
		SubjectsFn: func(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, time.Time, error) {
			if categoryID != psacampaign.PokemonCategoryID {
				t.Fatalf("categoryID = %d, want PokemonCategoryID", categoryID)
			}
			return []psacampaign.SubjectRef{{ID: 22210, Name: "Machamp"}}, fetchedAt, nil
		},
	}
	h := NewCampaignsHandler(nil, nil, nil, nil, observability.NewNoopLogger(), context.Background(),
		WithPSACatalogStore(catalog))
	req := httptest.NewRequest(http.MethodGet, "/api/psa/subjects", nil)
	rec := httptest.NewRecorder()
	h.HandleGetPSASubjects(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
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
		body       string
		getErr     error
		approveErr error
		signed     bool
		wantStatus int
	}{
		{
			name:       "pending row approved",
			body:       `{"pushId":"push-123"}`,
			signed:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "not pending",
			body:       `{"pushId":"push-123"}`,
			signed:     true,
			approveErr: psacampaign.ErrPushNotPending,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "row disappeared",
			body:       `{"pushId":"push-123"}`,
			signed:     true,
			getErr:     psacampaign.ErrPushNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			// The approver reviewed one payload and the queued row now holds
			// another. Approving here would sign changes nobody looked at.
			name:       "payload changed since review",
			body:       `{"pushId":"push-123","payloadDigest":"stale-digest"}`,
			signed:     true,
			wantStatus: http.StatusConflict,
		},
		{
			// Fail closed: an unsigned approval is one the harvester would
			// refuse an hour later, so refuse to write it now.
			name:       "no signing key configured",
			body:       `{"pushId":"push-123"}`,
			signed:     false,
			wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotID string
			var gotApproval psacampaign.Approval
			approveCalled := false
			queue := &mocks.PushQueueStoreMock{
				GetFn: func(ctx context.Context, id string) (psacampaign.PushRow, error) {
					if tt.getErr != nil {
						return psacampaign.PushRow{}, tt.getErr
					}
					return pendingPushRow(id), nil
				},
				ApproveFn: func(ctx context.Context, id string, a psacampaign.Approval) error {
					approveCalled = true
					gotID = id
					gotApproval = a
					return tt.approveErr
				},
			}
			var extra []CampaignsHandlerOption
			if tt.signed {
				extra = append(extra, WithPSAApprovalSigner(testApprovalSigner(t)))
			}
			h := newTestPSAHandler(nil, queue, extra...)

			req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/psa-publish", strings.NewReader(tt.body))
			req.SetPathValue("id", "c1")
			ctx := context.WithValue(req.Context(), middleware.UserContextKey, &auth.User{Username: "alice"})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			h.HandlePSAPublish(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				if tt.approveErr == nil && approveCalled {
					t.Fatal("Approve must not be called when the request is rejected")
				}
				return
			}
			if gotID != "push-123" || gotApproval.ApprovedBy != "alice" {
				t.Fatalf("Approve called with wrong args: id=%s approvedBy=%s", gotID, gotApproval.ApprovedBy)
			}
			// The stored approval must be verifiable on its own — that is the
			// whole point of writing it.
			if gotApproval.ApprovalSignature == "" || gotApproval.PayloadDigest == "" {
				t.Fatalf("approval missing signature/digest: %+v", gotApproval)
			}
			if gotApproval.SignatureKeyID != "test-key" {
				t.Fatalf("expected key id test-key, got %q", gotApproval.SignatureKeyID)
			}
			row := pendingPushRow("push-123")
			row.ApprovedBy = gotApproval.ApprovedBy
			row.ApprovedAt = gotApproval.ApprovedAt
			row.PayloadDigest = gotApproval.PayloadDigest
			row.ApprovalSignature = gotApproval.ApprovalSignature
			row.SignatureKeyID = gotApproval.SignatureKeyID
			if err := psacampaign.VerifyApproval(row, testApprovalSigner(t), time.Now()); err != nil {
				t.Fatalf("approval written by the handler does not verify: %v", err)
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
		GetFn: func(ctx context.Context, id string) (psacampaign.PushRow, error) {
			t.Fatal("Get should not be called without an authenticated user")
			return psacampaign.PushRow{}, nil
		},
		ApproveFn: func(ctx context.Context, id string, a psacampaign.Approval) error {
			t.Fatal("Approve should not be called without an authenticated user")
			return nil
		},
	}
	h := newTestPSAHandler(nil, queue, WithPSAApprovalSigner(testApprovalSigner(t)))
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
		name            string
		queue           *mocks.PushQueueStoreMock
		wantStatus      int
		wantBody        []string // substrings that must appear in the response body
		wantBodyExclude []string // substrings that must NOT appear in the response body
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
			wantBodyExclude: []string{`"formData":null`, `"diff":null`},
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
			for _, exclude := range tt.wantBodyExclude {
				if strings.Contains(rec.Body.String(), exclude) {
					t.Fatalf("body must not contain %q: %s", exclude, rec.Body.String())
				}
			}
		})
	}
}
