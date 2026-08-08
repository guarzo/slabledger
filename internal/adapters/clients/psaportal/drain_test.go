package psaportal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/platform/crypto"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// testSigner is the key the drain tests approve and verify under. Any test that
// wants the drain to reach the portal must sign its row with this.
func testSigner(t *testing.T) psacampaign.ApprovalSigner {
	t.Helper()
	s, err := crypto.NewHMACSigner(strings.Repeat("k", 32), "test-key")
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	return s
}

// signRow stamps row with a valid approval envelope, as HandlePSAPublish would.
func signRow(t *testing.T, row psacampaign.PushRow, signer psacampaign.ApprovalSigner) psacampaign.PushRow {
	t.Helper()
	a, err := psacampaign.SignApproval(row, "tester@example.com", time.Now(), signer)
	if err != nil {
		t.Fatalf("sign approval: %v", err)
	}
	row.ApprovedBy = a.ApprovedBy
	row.ApprovedAt = a.ApprovedAt
	row.PayloadDigest = a.PayloadDigest
	row.ApprovalSignature = a.ApprovalSignature
	row.SignatureKeyID = a.SignatureKeyID
	return row
}

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

	signer := testSigner(t)
	row := signRow(t, psacampaign.PushRow{
		ID:            "row-1",
		PSACampaignID: "660a980d-bf1c-4988-9958-1eb2d1853c66",
		Status:        psacampaign.PushApproved,
		Diff: psacampaign.ProposedDiff{
			Changes: []psacampaign.FieldChange{
				{Field: "bidPercentage", Old: "70", New: "80"},
			},
		},
	}, signer)

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

	pushed, failed := DrainPushQueue(context.Background(), c, q, nil, signer, observability.NewNoopLogger())

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

// TestDrainPushQueue_RefusesUnverifiedRow covers the case the whole feature
// exists for: a row that reached "approved" without a matching signature. Every
// variant must fail terminally (PushFailed, never requeued as PushApproved) and
// must not reach the portal.
func TestDrainPushQueue_RefusesUnverifiedRow(t *testing.T) {
	signer := testSigner(t)
	base := psacampaign.PushRow{
		ID:            "row-1",
		PSACampaignID: "660a980d-bf1c-4988-9958-1eb2d1853c66",
		Status:        psacampaign.PushApproved,
		Diff: psacampaign.ProposedDiff{
			Changes: []psacampaign.FieldChange{{Field: "bidPercentage", Old: "70", New: "80"}},
		},
	}

	tamper := func(f func(*psacampaign.PushRow)) psacampaign.PushRow {
		row := signRow(t, base, signer)
		f(&row)
		return row
	}

	otherKey, err := crypto.NewHMACSigner(strings.Repeat("z", 32), "other-key")
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	tests := []struct {
		name string
		row  psacampaign.PushRow
	}{
		{
			name: "unsigned row (approved by a direct database writer)",
			row:  base,
		},
		{
			name: "diff swapped under a valid envelope",
			row: tamper(func(r *psacampaign.PushRow) {
				r.Diff.Changes[0].New = "100"
			}),
		},
		{
			name: "target campaign repointed",
			row: tamper(func(r *psacampaign.PushRow) {
				r.PSACampaignID = "00000000-0000-0000-0000-000000000000"
			}),
		},
		{
			name: "approver rewritten",
			row: tamper(func(r *psacampaign.PushRow) {
				r.ApprovedBy = "someone-else@example.com"
			}),
		},
		{
			name: "signature minted under a different key",
			row: func() psacampaign.PushRow {
				return signRow(t, base, otherKey)
			}(),
		},
		{
			name: "approval older than the freshness window",
			row: tamper(func(r *psacampaign.PushRow) {
				r.ApprovedAt = r.ApprovedAt.Add(-2 * psacampaign.ApprovalFreshness)
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ff := &fakeFetcher{}
			c := New(ff, Config{PSABaseURL: "http://example.invalid"})

			var gotStatus psacampaign.PushStatus
			var gotErrMsg string
			q := &mocks.PushQueueStoreMock{
				ListByStatusFn: func(ctx context.Context, status psacampaign.PushStatus) ([]psacampaign.PushRow, error) {
					return []psacampaign.PushRow{tt.row}, nil
				},
				MarkResultFn: func(ctx context.Context, id string, status psacampaign.PushStatus, resultJSON, errMsg string) error {
					gotStatus, gotErrMsg = status, errMsg
					return nil
				},
			}

			pushed, failed := DrainPushQueue(context.Background(), c, q, nil, signer, observability.NewNoopLogger())

			if pushed != 0 || failed != 1 {
				t.Fatalf("pushed/failed = %d/%d, want 0/1", pushed, failed)
			}
			if gotStatus != psacampaign.PushFailed {
				t.Fatalf("status = %s, want %s (verification failures are terminal)", gotStatus, psacampaign.PushFailed)
			}
			if gotErrMsg == "" {
				t.Fatal("expected the verification failure to be recorded on the row")
			}
			if len(ff.captured) != 0 {
				t.Fatalf("portal must not be called for an unverified row, got %v", ff.captured)
			}
		})
	}
}

// TestDrainPushQueue_NilSignerFailsEveryRow pins the fail-closed contract: a
// deployment with no signing key configured must refuse to push, not fall back
// to trusting the status column.
func TestDrainPushQueue_NilSignerFailsEveryRow(t *testing.T) {
	signer := testSigner(t)
	row := signRow(t, psacampaign.PushRow{
		ID:            "row-1",
		PSACampaignID: "660a980d-bf1c-4988-9958-1eb2d1853c66",
		Status:        psacampaign.PushApproved,
		Diff:          psacampaign.ProposedDiff{Changes: []psacampaign.FieldChange{{Field: "bidPercentage", Old: "70", New: "80"}}},
	}, signer)

	ff := &fakeFetcher{}
	c := New(ff, Config{PSABaseURL: "http://example.invalid"})
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

	pushed, failed := DrainPushQueue(context.Background(), c, q, nil, nil, observability.NewNoopLogger())

	if pushed != 0 || failed != 1 {
		t.Fatalf("pushed/failed = %d/%d, want 0/1", pushed, failed)
	}
	if gotStatus != psacampaign.PushFailed {
		t.Fatalf("status = %s, want %s", gotStatus, psacampaign.PushFailed)
	}
	if len(ff.captured) != 0 {
		t.Fatalf("portal must not be called without a signer, got %v", ff.captured)
	}
}

func TestTransientPushError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "cloudflare 403", err: errors.New("psaportal: remote-hash landing page: status 403"), want: true},
		{name: "rate limit 429", err: errors.New("psaportal: update campaign status 429"), want: true},
		{name: "outage 503", err: errors.New("psaportal: create campaign status 503"), want: true},
		{name: "app rejection 400 is terminal", err: errors.New("psaportal: update campaign status 400"), want: false},
		{name: "server error 500 is terminal", err: errors.New("psaportal: create campaign status 500"), want: false},
		{name: "proxy tunnel drop is transient", err: errors.New("psaportal: campaign fetch: psaportal: browser fetch error: page.goto: net::ERR_TUNNEL_CONNECTION_FAILED"), want: true},
		{name: "proxy connection failure is transient", err: errors.New("psaportal: browser fetch error: net::ERR_PROXY_CONNECTION_FAILED"), want: true},
		{name: "connection reset is transient", err: errors.New("psaportal: browser fetch error: net::ERR_CONNECTION_RESET"), want: true},
		{name: "connection closed is transient", err: errors.New("psaportal: browser fetch error: net::ERR_CONNECTION_CLOSED"), want: true},
		{name: "connection timed out is transient", err: errors.New("psaportal: browser fetch error: page.goto: net::ERR_CONNECTION_TIMED_OUT"), want: true},
		{name: "timed out is transient", err: errors.New("psaportal: browser fetch error: net::ERR_TIMED_OUT"), want: true},
		{name: "network changed is transient", err: errors.New("psaportal: browser fetch error: net::ERR_NETWORK_CHANGED"), want: true},
		{name: "empty response is transient", err: errors.New("psaportal: browser fetch error: net::ERR_EMPTY_RESPONSE"), want: true},
		{name: "generic net::ERR_FAILED stays terminal", err: errors.New("psaportal: browser fetch error: net::ERR_FAILED"), want: false},
		// A stale push refused again on retry is just noise, and the message
		// quotes operator-controlled names that could otherwise smuggle a
		// transient signature into the substring scan.
		{
			name: "staleness is terminal even quoting a transient signature",
			err:  fmt.Errorf("%w: field %q was %q at approval, portal now holds %q", psacampaign.ErrFieldStale, "campaignName", "status 403", "Crystal"),
			want: false,
		},
		{name: "unrenderable field is terminal", err: fmt.Errorf("%w: %q", psacampaign.ErrNoRenderer, "isActive"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := transientPushError(tt.err); got != tt.want {
				t.Fatalf("transientPushError(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDrainPushQueue_UpdateTransient403RequeuesApproved(t *testing.T) {
	edit, err := os.ReadFile("../../../../docs/psa-campaign-edit-raw.json")
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	ff := &fakeFetcher{
		routes: map[string]string{
			"/edit/__data.json?x-sveltekit-invalidated=0001":          string(edit),
			"/buyercampaignmanager/_app/remote/abc123/updateCampaign": `{}`,
			"/buyercampaignmanager":                                   `<html><script src="/buyercampaignmanager/_app/immutable/entry/app.HASH123.js"></script></html>`,
			"immutable/entry/app.HASH123.js":                          `const __vite__mapDeps=(d=["../chunks/REMOTE.js"]);`,
			"immutable/chunks/REMOTE.js":                              `x=_t("abc123/createCampaign"),y=_t("abc123/updateCampaign")`,
		},
		statusFor: map[string]int{"/buyercampaignmanager/_app/remote/abc123/updateCampaign": 403},
	}
	c := New(ff, Config{})

	signer := testSigner(t)
	row := signRow(t, psacampaign.PushRow{
		ID:            "row-1",
		PSACampaignID: "660a980d-bf1c-4988-9958-1eb2d1853c66",
		Status:        psacampaign.PushApproved,
		Diff: psacampaign.ProposedDiff{
			Changes: []psacampaign.FieldChange{{Field: "bidPercentage", Old: "70", New: "80"}},
		},
	}, signer)

	var gotStatus psacampaign.PushStatus
	var gotErrMsg string
	q := &mocks.PushQueueStoreMock{
		ListByStatusFn: func(ctx context.Context, status psacampaign.PushStatus) ([]psacampaign.PushRow, error) {
			return []psacampaign.PushRow{row}, nil
		},
		MarkResultFn: func(ctx context.Context, id string, status psacampaign.PushStatus, resultJSON, errMsg string) error {
			gotStatus = status
			gotErrMsg = errMsg
			return nil
		},
	}

	pushed, failed := DrainPushQueue(context.Background(), c, q, nil, signer, observability.NewNoopLogger())

	if pushed != 0 || failed != 1 {
		t.Fatalf("expected pushed=0 failed=1, got pushed=%d failed=%d", pushed, failed)
	}
	if gotStatus != psacampaign.PushApproved {
		t.Fatalf("expected transient 403 to requeue row as PushApproved, got %v", gotStatus)
	}
	if gotErrMsg == "" {
		t.Fatal("expected the transport error to be recorded on the requeued row")
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

	pushed, failed := DrainPushQueue(context.Background(), c, q, nil, testSigner(t), observability.NewNoopLogger())

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
		// portalNames are the campaign names the portal list endpoint reports.
		// The create-path guard consults it before creating anything.
		portalNames  []string
		listStatus   int
		wantPushed   int
		wantFailed   int
		wantStatus   psacampaign.PushStatus
		wantLinked   bool
		wantLinkedID string
		wantNoPortal bool
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
			name: "cloudflare 403 requeues as approved for next drain", createStatus: 403, createBody: `{}`,
			wantFailed: 1, wantStatus: psacampaign.PushApproved,
		},
		{
			name: "portal 503 requeues as approved for next drain", createStatus: 503, createBody: `{}`,
			wantFailed: 1, wantStatus: psacampaign.PushApproved,
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
		{
			// The replay case the signature alone cannot stop: a database writer
			// clears the link and re-queues an already-drained approved row. The
			// portal itself still holds the campaign, so no duplicate is created.
			name: "campaign already on the portal skips create", portalNames: []string{"Modern 10s"},
			wantPushed: 1, wantStatus: psacampaign.PushPushed, wantNoPortal: true,
			wantLinked: true, wantLinkedID: "uuid-portal-0",
		},
		{
			// Two portal campaigns share the name, so "does it already exist"
			// has no answer. Creating anyway risks a duplicate live buy box.
			name: "ambiguous portal name refuses create", portalNames: []string{"Modern 10s", "modern 10s"},
			wantFailed: 1, wantStatus: psacampaign.PushFailed, wantNoPortal: true,
		},
		{
			// A guard that cannot run has not passed.
			name: "unreadable portal list refuses create", listStatus: 500,
			wantFailed: 1, wantStatus: psacampaign.PushFailed, wantNoPortal: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := make([]any, 0, len(tt.portalNames))
			for i, n := range tt.portalNames {
				items = append(items, campaignItem(fmt.Sprintf("uuid-portal-%d", i), n))
			}
			statusFor := map[string]int{"/buyercampaignmanager/_app/remote/abc123/createCampaign": tt.createStatus}
			if tt.listStatus != 0 {
				statusFor["/buyercampaignmanager/__data.json"] = tt.listStatus
			}
			ff := &fakeFetcher{
				routes: map[string]string{
					"/buyercampaignmanager/_app/remote/abc123/createCampaign": tt.createBody,
					"/buyercampaignmanager":                                   `<html><script src="/buyercampaignmanager/_app/immutable/entry/app.HASH123.js"></script></html>`,
					"/buyercampaignmanager/__data.json":                       string(buildListEnvelope(t, items, 50, len(items))),
					"immutable/entry/app.HASH123.js":                          `const __vite__mapDeps=(d=["../chunks/REMOTE.js"]);`,
					"immutable/chunks/REMOTE.js":                              `x=_t("abc123/createCampaign"),y=_t("abc123/updateCampaign")`,
				},
				statusFor: statusFor,
			}

			fd := &psacampaign.CampaignFormData{CampaignName: "Modern 10s", CampaignType: "CATEGORY", Category: "POKEMON", GradeMinimum: "10", GradeMaximum: "10"}
			if tt.missingFD {
				fd = nil
			}
			signer := testSigner(t)
			row := signRow(t, psacampaign.PushRow{
				ID: "row-1", Operation: psacampaign.OpCreate, InternalCampaignID: "c1",
				Diff: psacampaign.ProposedDiff{Create: fd}, Status: psacampaign.PushApproved,
			}, signer)

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
			pushed, failed := DrainPushQueue(context.Background(), c, q, linker, signer, observability.NewNoopLogger())

			if pushed != tt.wantPushed || failed != tt.wantFailed {
				t.Fatalf("pushed/failed = %d/%d, want %d/%d", pushed, failed, tt.wantPushed, tt.wantFailed)
			}
			if gotStatus != tt.wantStatus {
				t.Fatalf("row status = %s, want %s", gotStatus, tt.wantStatus)
			}
			if tt.wantLinked {
				wantID := tt.wantLinkedID
				if wantID == "" {
					wantID = "uuid-new-1"
				}
				if linkedInternal != "c1" || linkedPSA != wantID {
					t.Fatalf("linked %q/%q, want c1/%s", linkedInternal, linkedPSA, wantID)
				}
			}
			portalCalled := ff.captured["/buyercampaignmanager/_app/remote/abc123/createCampaign"] != ""
			if (tt.missingFD || tt.wantNoPortal) && portalCalled {
				t.Fatal("portal must not be called (missing formData or already-linked idempotent row)")
			}
		})
	}
}
