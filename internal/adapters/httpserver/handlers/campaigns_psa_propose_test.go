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

// englishSpecList is the ENABLED "Pokemon - English Language Only" curated
// list, matched by
// diffCampaign()/noDiffPortal()'s TargetLanguages/SpecListIDs so their scalar
// fixtures still produce zero diff by default.
var englishSpecList = []psacampaign.SpecListRef{{ID: "sl-1", Name: "Pokemon - English Language Only", Status: "ENABLED"}}

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
		SpecListIDs: []string{"sl-1"},
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
		TargetLanguages:      []string{"english"},
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
		{
			// Guards the 4xx-vs-500 classification: an operator-entered subject
			// name that hasn't been reconciled with the portal catalog yet is an
			// expected, actionable state (naming the subject), not a server fault.
			name: "unresolved subject name maps to 400 not 500",
			campaign: func() *inventory.Campaign {
				c := diffCampaign()
				c.Subjects = []inventory.TargetSubject{{Name: "Mystery Card"}} // ID 0, unresolved
				return &c
			}(),
			portalRows: []psacampaign.PortalCampaign{noDiffPortal()},
			wantStatus: http.StatusBadRequest,
		},
		{
			// Same classification guard, via the spec-list axis: a campaign
			// targeting a language the portal offers no curated list for.
			name: "unresolved target language maps to 400 not 500",
			campaign: func() *inventory.Campaign {
				c := diffCampaign()
				c.TargetLanguages = []string{"korean"}
				return &c
			}(),
			portalRows: []psacampaign.PortalCampaign{noDiffPortal()},
			wantStatus: http.StatusBadRequest,
		},
		{
			// Migration 000023's legacy backfill is unreconciled until the
			// baseline pull runs. Proposing in that window must refuse with an
			// actionable 400 rather than silently re-resolving legacy subjects
			// to current-generation portal ids.
			name: "legacy unreconciled subject maps to 400 not 500",
			campaign: func() *inventory.Campaign {
				c := diffCampaign()
				c.Subjects = []inventory.TargetSubject{{ID: inventory.LegacyUnreconciledSubjectID, Name: "Charizard"}}
				return &c
			}(),
			portalRows: []psacampaign.PortalCampaign{noDiffPortal()},
			wantStatus: http.StatusBadRequest,
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
			opts = append(opts, WithPSACatalogStore(freshCatalog(englishSpecList, nil)))
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

// TestHandlePSAPropose_CatalogStale guards the ErrCatalogStale branch in
// HandlePSAPropose: a catalog whose fetchedAt exceeds psacampaign.CatalogMaxAge
// must surface as 503 (the operator's signal to run the harvester), not the
// generic 500 that any other buildResolver error gets.
func TestHandlePSAPropose_CatalogStale(t *testing.T) {
	c := diffCampaign()
	portal := noDiffPortal()
	svc := &mocks.MockInventoryService{
		GetCampaignFn: func(ctx context.Context, id string) (*inventory.Campaign, error) { return &c, nil },
	}
	snap := &mocks.SnapshotStoreMock{
		GetSnapshotFn: func(ctx context.Context) ([]psacampaign.PortalCampaign, time.Time, error) {
			return []psacampaign.PortalCampaign{portal}, time.Now(), nil
		},
	}
	h := NewCampaignsHandler(svc, nil, nil, nil, observability.NewNoopLogger(), context.Background(),
		WithPSASnapshotStore(snap),
		WithPSAPushQueue(&mocks.PushQueueStoreMock{}),
		WithPSACatalogStore(staleCatalog(englishSpecList, nil)),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/psa-propose", strings.NewReader(`{}`))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandlePSAPropose(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (stale catalog), got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "stale") {
		t.Fatalf("expected a staleness message, got: %s", rec.Body.String())
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

func TestHandlePSAProposeCreate(t *testing.T) {
	valid := inventory.Campaign{
		ID: "c1", Name: "Modern 10s", BuyTermsCLPct: 0.72, DailySpendCapCents: 300000,
		GradeRange: "10", YearRange: "2024-2026", PriceRange: "500-3000",
		CLConfidence: "3-4", PSASourcingFeeCents: 300, TargetLanguages: []string{"english"},
	}
	englishCatalog := freshCatalog([]psacampaign.SpecListRef{
		{ID: "sl-1", Name: "Pokemon - English Language Only", Status: "ENABLED"},
	}, nil)
	tests := []struct {
		name        string
		noQueue     bool
		campaign    inventory.Campaign
		queueRows   []psacampaign.PushRow
		wantStatus  int
		wantEnqueue bool
	}{
		{name: "queue disabled", noQueue: true, campaign: valid, wantStatus: http.StatusServiceUnavailable},
		{
			name: "already linked",
			campaign: func() inventory.Campaign {
				c := valid
				c.PSACampaignRequestID = "portal-1"
				return c
			}(),
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "unmappable ranges",
			campaign: func() inventory.Campaign {
				c := valid
				c.GradeRange = ""
				return c
			}(),
			wantStatus: http.StatusBadRequest,
		},
		{name: "valid create proposal", campaign: valid, wantStatus: http.StatusOK, wantEnqueue: true},
		{
			name:     "duplicate create already queued",
			campaign: valid,
			queueRows: []psacampaign.PushRow{
				{ID: "existing", Operation: psacampaign.OpCreate, InternalCampaignID: "c1", Status: psacampaign.PushPending},
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:     "pushed-but-unlinked create rejects re-create",
			campaign: valid,
			queueRows: []psacampaign.PushRow{
				{ID: "done", Operation: psacampaign.OpCreate, InternalCampaignID: "c1", Status: psacampaign.PushPushed},
			},
			wantStatus: http.StatusConflict,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var enqueuedRow psacampaign.PushRow
			enqueued := false
			svc := &mocks.MockInventoryService{
				GetCampaignFn: func(ctx context.Context, id string) (*inventory.Campaign, error) {
					c := tt.campaign
					return &c, nil
				},
			}
			queue := &mocks.PushQueueStoreMock{
				EnqueueFn: func(ctx context.Context, p psacampaign.PushRow) error {
					enqueued = true
					enqueuedRow = p
					return nil
				},
				ListByStatusFn: func(ctx context.Context, status psacampaign.PushStatus) ([]psacampaign.PushRow, error) {
					var out []psacampaign.PushRow
					for _, r := range tt.queueRows {
						if r.Status == status {
							out = append(out, r)
						}
					}
					return out, nil
				},
			}
			var opts []CampaignsHandlerOption
			if !tt.noQueue {
				opts = append(opts, WithPSAPushQueue(queue))
			}
			opts = append(opts, WithPSACatalogStore(englishCatalog))
			h := NewCampaignsHandler(svc, nil, nil, nil, observability.NewNoopLogger(), context.Background(), opts...)

			req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/psa-propose-create", strings.NewReader(`{}`))
			req.SetPathValue("id", "c1")
			rec := httptest.NewRecorder()
			h.HandlePSAProposeCreate(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if enqueued != tt.wantEnqueue {
				t.Fatalf("Enqueue called = %v, want %v", enqueued, tt.wantEnqueue)
			}
			if !tt.wantEnqueue {
				return
			}
			if enqueuedRow.Operation != psacampaign.OpCreate {
				t.Fatalf("Operation = %q, want create", enqueuedRow.Operation)
			}
			if enqueuedRow.PSACampaignID != "" || enqueuedRow.InternalCampaignID != "c1" {
				t.Fatalf("ids wrong: %+v", enqueuedRow)
			}
			if enqueuedRow.Diff.Create == nil || enqueuedRow.Diff.Create.IsActive {
				t.Fatalf("Diff.Create = %+v, want formData with IsActive=false", enqueuedRow.Diff.Create)
			}
			var resp struct {
				PushID   string                       `json:"pushId"`
				FormData psacampaign.CampaignFormData `json:"formData"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.PushID == "" || resp.FormData.CampaignName != "Modern 10s" || resp.FormData.BidPercentage != 72 {
				t.Fatalf("response = %+v", resp)
			}
		})
	}
}

// TestHandlePSAProposeCreate_CatalogStale guards the ErrCatalogStale branch in
// HandlePSAProposeCreate: a catalog whose fetchedAt exceeds
// psacampaign.CatalogMaxAge must surface as 503 (the operator's signal to run
// the harvester), not the wholesale 400 that any other buildResolver error
// gets on this path.
func TestHandlePSAProposeCreate_CatalogStale(t *testing.T) {
	c := inventory.Campaign{
		ID: "c1", Name: "Modern 10s", BuyTermsCLPct: 0.72, DailySpendCapCents: 300000,
		GradeRange: "10", YearRange: "2024-2026", PriceRange: "500-3000",
		CLConfidence: "3-4", PSASourcingFeeCents: 300, TargetLanguages: []string{"english"},
	}
	svc := &mocks.MockInventoryService{
		GetCampaignFn: func(ctx context.Context, id string) (*inventory.Campaign, error) {
			cc := c
			return &cc, nil
		},
	}
	h := NewCampaignsHandler(svc, nil, nil, nil, observability.NewNoopLogger(), context.Background(),
		WithPSAPushQueue(&mocks.PushQueueStoreMock{}),
		WithPSACatalogStore(staleCatalog(nil, nil)),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/psa-propose-create", strings.NewReader(`{}`))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandlePSAProposeCreate(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (stale catalog), got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "stale") {
		t.Fatalf("expected a staleness message, got: %s", rec.Body.String())
	}
}
