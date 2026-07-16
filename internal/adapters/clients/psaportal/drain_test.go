package psaportal

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestDrainPushQueue_PushesApprovedRow(t *testing.T) {
	edit, err := os.ReadFile("../../../../docs/psa-campaign-edit-raw.json")
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	ff := &fakeFetcher{routes: map[string]string{
		"/edit/__data.json?x-sveltekit-invalidated=0001":          string(edit),
		"/buyercampaignmanager/_app/remote/abc123/updateCampaign": `{"type":"result","result":"[{}]"}`,
		"/buyercampaignmanager":                                   `<html><script src="/buyercampaignmanager/_app/immutable/entry/app.HASH123.js"></script></html>`,
		"immutable/entry/app.HASH123.js":                          `const __vite__mapDeps=(d=["../chunks/REMOTE.js"]);`,
		"immutable/chunks/REMOTE.js":                              `x=_t("abc123/createCampaign"),y=_t("abc123/updateCampaign")`,
	}}

	c := New(ff, Config{})

	row := psacampaign.PushRow{
		ID:            "row-1",
		PSACampaignID: "660a980d-bf1c-4988-9958-1eb2d1853c66",
		Status:        psacampaign.PushApproved,
		Diff: psacampaign.ProposedDiff{
			Changes: []psacampaign.FieldChange{
				{Field: "bidPercentage", Old: "70", New: "80"},
			},
		},
	}

	var gotStatus psacampaign.PushStatus
	var gotErrMsg string
	q := &mocks.PushQueueStoreMock{
		ListByStatusFn: func(ctx context.Context, status psacampaign.PushStatus) ([]psacampaign.PushRow, error) {
			if status != psacampaign.PushApproved {
				t.Fatalf("expected ListByStatus(PushApproved), got %v", status)
			}
			return []psacampaign.PushRow{row}, nil
		},
		MarkResultFn: func(ctx context.Context, id string, status psacampaign.PushStatus, resultJSON, errMsg string) error {
			if id != "row-1" {
				t.Fatalf("expected row id row-1, got %s", id)
			}
			gotStatus = status
			gotErrMsg = errMsg
			return nil
		},
	}

	pushed, failed := DrainPushQueue(context.Background(), c, q, nil, observability.NewNoopLogger())

	if pushed != 1 || failed != 0 {
		t.Fatalf("expected pushed=1 failed=0, got pushed=%d failed=%d", pushed, failed)
	}
	if gotStatus != psacampaign.PushPushed {
		t.Fatalf("expected MarkResult status PushPushed, got %v", gotStatus)
	}
	if gotErrMsg != "" {
		t.Fatalf("expected empty errMsg, got %q", gotErrMsg)
	}
}

func TestDrainPushQueue_SkipsUnclaimableRow(t *testing.T) {
	c := New(&fakeFetcher{}, Config{PSABaseURL: "http://example.invalid"})

	row := psacampaign.PushRow{
		ID:            "row-1",
		PSACampaignID: "660a980d-bf1c-4988-9958-1eb2d1853c66",
		Status:        psacampaign.PushApproved,
	}

	markResultCalled := false
	q := &mocks.PushQueueStoreMock{
		ListByStatusFn: func(ctx context.Context, status psacampaign.PushStatus) ([]psacampaign.PushRow, error) {
			return []psacampaign.PushRow{row}, nil
		},
		ClaimFn: func(ctx context.Context, id string) (bool, error) {
			return false, nil
		},
		MarkResultFn: func(ctx context.Context, id string, status psacampaign.PushStatus, resultJSON, errMsg string) error {
			markResultCalled = true
			return nil
		},
	}

	pushed, failed := DrainPushQueue(context.Background(), c, q, nil, observability.NewNoopLogger())

	if pushed != 0 || failed != 0 {
		t.Fatalf("expected pushed=0 failed=0, got pushed=%d failed=%d", pushed, failed)
	}
	if markResultCalled {
		t.Fatal("expected MarkResult not to be called for unclaimable row")
	}
}

func TestDrainPushQueue_CreateRow(t *testing.T) {
	tests := []struct {
		name          string
		createStatus  int
		createBody    string
		linkErr       error
		missingFD     bool
		alreadyLinked string
		wantPushed    int
		wantFailed    int
		wantStatus    psacampaign.PushStatus
		wantLinked    bool
		wantNoPortal  bool
	}{
		{
			name: "success creates links and marks pushed", createStatus: 200,
			createBody: `{"type":"result","result":"[{\"campaignRequestId\":1},\"uuid-new-1\"]"}`,
			wantPushed: 1, wantStatus: psacampaign.PushPushed, wantLinked: true,
		},
		{
			name: "portal failure marks failed", createStatus: 500, createBody: `{}`,
			wantFailed: 1, wantStatus: psacampaign.PushFailed,
		},
		{
			name: "link failure still pushed", createStatus: 200,
			createBody: `{"type":"result","result":"[{\"campaignRequestId\":1},\"uuid-new-1\"]"}`,
			linkErr:    errors.New("db down"),
			wantPushed: 1, wantStatus: psacampaign.PushPushed, wantLinked: true,
		},
		{
			name: "missing formData marks failed without portal call", missingFD: true,
			wantFailed: 1, wantStatus: psacampaign.PushFailed,
		},
		{
			name: "already linked skips create (idempotent retry)", alreadyLinked: "uuid-existing-1",
			wantPushed: 1, wantStatus: psacampaign.PushPushed, wantNoPortal: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ff := &fakeFetcher{
				routes: map[string]string{
					"/buyercampaignmanager/_app/remote/abc123/createCampaign": tt.createBody,
					"/buyercampaignmanager":                                   `<html><script src="/buyercampaignmanager/_app/immutable/entry/app.HASH123.js"></script></html>`,
					"immutable/entry/app.HASH123.js":                          `const __vite__mapDeps=(d=["../chunks/REMOTE.js"]);`,
					"immutable/chunks/REMOTE.js":                              `x=_t("abc123/createCampaign"),y=_t("abc123/updateCampaign")`,
				},
				statusFor: map[string]int{"/buyercampaignmanager/_app/remote/abc123/createCampaign": tt.createStatus},
			}

			fd := &psacampaign.CampaignFormData{CampaignName: "Modern 10s", CampaignType: "CATEGORY", Category: "POKEMON", GradeMinimum: "10", GradeMaximum: "10"}
			if tt.missingFD {
				fd = nil
			}
			row := psacampaign.PushRow{
				ID: "row-1", Operation: psacampaign.OpCreate, InternalCampaignID: "c1",
				Diff: psacampaign.ProposedDiff{Create: fd}, Status: psacampaign.PushApproved,
			}

			var gotStatus psacampaign.PushStatus
			q := &mocks.PushQueueStoreMock{
				ListByStatusFn: func(ctx context.Context, status psacampaign.PushStatus) ([]psacampaign.PushRow, error) {
					return []psacampaign.PushRow{row}, nil
				},
				MarkResultFn: func(ctx context.Context, id string, status psacampaign.PushStatus, resultJSON, errMsg string) error {
					gotStatus = status
					return nil
				},
			}

			var linkedInternal, linkedPSA string
			linker := &mocks.CampaignLinkerMock{
				LinkPSACampaignFn: func(ctx context.Context, internalID, psaID string) error {
					linkedInternal, linkedPSA = internalID, psaID
					return tt.linkErr
				},
				LinkedPSACampaignIDFn: func(ctx context.Context, internalID string) (string, error) {
					return tt.alreadyLinked, nil
				},
			}

			c := New(ff, Config{})
			pushed, failed := DrainPushQueue(context.Background(), c, q, linker, observability.NewNoopLogger())

			if pushed != tt.wantPushed || failed != tt.wantFailed {
				t.Fatalf("pushed/failed = %d/%d, want %d/%d", pushed, failed, tt.wantPushed, tt.wantFailed)
			}
			if gotStatus != tt.wantStatus {
				t.Fatalf("row status = %s, want %s", gotStatus, tt.wantStatus)
			}
			if tt.wantLinked && (linkedInternal != "c1" || linkedPSA != "uuid-new-1") {
				t.Fatalf("linked %q/%q, want c1/uuid-new-1", linkedInternal, linkedPSA)
			}
			portalCalled := ff.captured["/buyercampaignmanager/_app/remote/abc123/createCampaign"] != ""
			if (tt.missingFD || tt.wantNoPortal) && portalCalled {
				t.Fatal("portal must not be called (missing formData or already-linked idempotent row)")
			}
		})
	}
}
