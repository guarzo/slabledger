package psaportal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newTestResolver(t *testing.T, portal []psacampaign.PortalCampaign, internal []inventory.Campaign, fetchedAt time.Time) *CampaignResolver {
	t.Helper()
	snap := &mocks.SnapshotStoreMock{
		GetSnapshotFn: func(_ context.Context) ([]psacampaign.PortalCampaign, time.Time, error) {
			return portal, fetchedAt, nil
		},
	}
	campaigns := &mocks.CampaignRepositoryMock{
		ListCampaignsFn: func(_ context.Context, _ bool) ([]inventory.Campaign, error) {
			return internal, nil
		},
	}
	return NewCampaignResolver(snap, campaigns, func() time.Time { return time.Now() })
}

func TestCampaignResolver_ResolveCampaignID(t *testing.T) {
	portal := []psacampaign.PortalCampaign{
		{CampaignRequestID: "req-modern", Name: "Modern"},
		{CampaignRequestID: "req-crystal", Name: "Crystal"},
	}
	internal := []inventory.Campaign{
		{ID: "camp-1", Name: "Modern", PSACampaignRequestID: "req-modern"},
		{ID: "camp-2", Name: "Crystal Pokemon", PSACampaignRequestID: "req-crystal"},
	}

	tests := []struct {
		name    string
		psaName string
		wantID  string
		wantOK  bool
	}{
		{"exact match", "Modern", "camp-1", true},
		{"case drift", "modern", "camp-1", true},
		{"whitespace drift", "  Modern  ", "camp-1", true},
		{"name drift via request id", "Crystal", "camp-2", true},
		{"dead campaign name", "Brady modern", "", false},
		{"empty name", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestResolver(t, portal, internal, time.Now())
			gotID, gotOK, err := r.ResolveCampaignID(context.Background(), tt.psaName)
			if err != nil {
				t.Fatalf("ResolveCampaignID: %v", err)
			}
			if gotID != tt.wantID || gotOK != tt.wantOK {
				t.Errorf("= (%q, %v), want (%q, %v)", gotID, gotOK, tt.wantID, tt.wantOK)
			}
		})
	}
}

func TestCampaignResolver_RefusesStaleSnapshot(t *testing.T) {
	stale := time.Now().Add(-27 * time.Hour)
	r := newTestResolver(t, []psacampaign.PortalCampaign{{CampaignRequestID: "req-modern", Name: "Modern"}},
		[]inventory.Campaign{{ID: "camp-1", PSACampaignRequestID: "req-modern"}}, stale)
	_, _, err := r.ResolveCampaignID(context.Background(), "Modern")
	if !errors.Is(err, ErrStaleCampaignSnapshot) {
		t.Fatalf("err = %v, want ErrStaleCampaignSnapshot", err)
	}
}

func TestCampaignResolver_UnlinkedCampaignNeverMatches(t *testing.T) {
	portal := []psacampaign.PortalCampaign{{CampaignRequestID: "req-modern", Name: "Modern"}}
	internal := []inventory.Campaign{{ID: "camp-1", Name: "Other", PSACampaignRequestID: ""}}
	r := newTestResolver(t, portal, internal, time.Now())
	gotID, gotOK, err := r.ResolveCampaignID(context.Background(), "Modern")
	if err != nil {
		t.Fatalf("ResolveCampaignID: %v", err)
	}
	if gotOK || gotID != "" {
		t.Fatalf("= (%q, %v), want (\"\", false) — unlinked campaign must never match", gotID, gotOK)
	}
}
