package psaportal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestDrainPushQueue_PushesApprovedRow(t *testing.T) {
	edit, err := os.ReadFile("../../../../docs/psa-campaign-edit-raw.json")
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/edit/"):
			_, _ = w.Write(edit)
		case strings.Contains(r.URL.Path, "/updateCampaign"):
			_, _ = w.Write([]byte(`{"type":"result","result":"[{}]"}`))
		default:
			_, _ = w.Write([]byte(`<html>build/app/immutable/entry/app.HASH123.js</html>`))
		}
	}))
	defer srv.Close()

	c := New(stubTokens{tok: "tok"}, Config{PSABaseURL: srv.URL})

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

	pushed, failed := DrainPushQueue(context.Background(), c, q, observability.NewNoopLogger())

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
