package dhlisting

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
)

// --- Local mocks (inline interface fields, not central mocks) ---

type mockCertResolver struct {
	ResolveCertFn func(ctx context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error)
}

func (m *mockCertResolver) ResolveCert(ctx context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error) {
	return m.ResolveCertFn(ctx, req)
}

type mockInventoryPusher struct {
	PushInventoryFn func(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error)
}

func (m *mockInventoryPusher) PushInventory(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
	return m.PushInventoryFn(ctx, items)
}

type mockInventoryLister struct {
	UpdateInventoryFn func(ctx context.Context, inventoryID int, update dh.InventoryUpdate) (*dh.InventoryResult, error)
	SyncChannelsFn    func(ctx context.Context, inventoryID int, channels []string) (*dh.ChannelSyncResponse, error)
}

func (m *mockInventoryLister) UpdateInventory(ctx context.Context, inventoryID int, update dh.InventoryUpdate) (*dh.InventoryResult, error) {
	return m.UpdateInventoryFn(ctx, inventoryID, update)
}

func (m *mockInventoryLister) SyncChannels(ctx context.Context, inventoryID int, channels []string) (*dh.ChannelSyncResponse, error) {
	return m.SyncChannelsFn(ctx, inventoryID, channels)
}

// --- CertResolverAdapter tests ---

func TestCertResolverAdapter_ResolveCert_Success(t *testing.T) {
	tests := []struct {
		name    string
		req     dhlisting.DHCertResolveRequest
		dhResp  *dh.CertResolution
		wantRes *dhlisting.DHCertResolution
	}{
		{
			name: "matched with card ID and no candidates",
			req: dhlisting.DHCertResolveRequest{
				CertNumber: "12345678",
				CardName:   "Charizard",
				SetName:    "Base Set",
				CardNumber: "4",
				Year:       "1999",
				Variant:    "1st Edition",
			},
			dhResp: &dh.CertResolution{
				CertNumber: "12345678",
				Status:     "matched",
				DHCardID:   42,
				CardName:   "Charizard",
				SetName:    "Base Set",
				CardNumber: "4",
				Grade:      9.0,
				ImageURL:   "https://example.com/img.jpg",
			},
			wantRes: &dhlisting.DHCertResolution{
				Status:   "matched",
				DHCardID: 42,
			},
		},
		{
			name: "ambiguous with candidates",
			req: dhlisting.DHCertResolveRequest{
				CertNumber: "99999999",
				CardName:   "Pikachu",
			},
			dhResp: &dh.CertResolution{
				CertNumber: "99999999",
				Status:     "ambiguous",
				Candidates: []dh.CertResolutionCandidate{
					{DHCardID: 10, CardName: "Pikachu", SetName: "Base Set", CardNumber: "58", ImageURL: "https://example.com/a.jpg"},
					{DHCardID: 11, CardName: "Pikachu", SetName: "Jungle", CardNumber: "60", ImageURL: "https://example.com/b.jpg"},
				},
			},
			wantRes: &dhlisting.DHCertResolution{
				Status: "ambiguous",
				Candidates: []dhlisting.DHCertCandidate{
					{DHCardID: 10, CardName: "Pikachu", SetName: "Base Set", CardNumber: "58"},
					{DHCardID: 11, CardName: "Pikachu", SetName: "Jungle", CardNumber: "60"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedReq dh.CertResolveRequest
			mock := &mockCertResolver{
				ResolveCertFn: func(_ context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error) {
					capturedReq = req
					return tc.dhResp, nil
				},
			}

			adapter := NewCertResolverAdapter(mock)
			got, err := adapter.ResolveCert(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify request field mapping (domain → dh)
			if capturedReq.CertNumber != tc.req.CertNumber {
				t.Errorf("CertNumber: got %q, want %q", capturedReq.CertNumber, tc.req.CertNumber)
			}
			if capturedReq.CardName != tc.req.CardName {
				t.Errorf("CardName: got %q, want %q", capturedReq.CardName, tc.req.CardName)
			}
			if capturedReq.SetName != tc.req.SetName {
				t.Errorf("SetName: got %q, want %q", capturedReq.SetName, tc.req.SetName)
			}
			if capturedReq.CardNumber != tc.req.CardNumber {
				t.Errorf("CardNumber: got %q, want %q", capturedReq.CardNumber, tc.req.CardNumber)
			}
			if capturedReq.Year != tc.req.Year {
				t.Errorf("Year: got %q, want %q", capturedReq.Year, tc.req.Year)
			}
			if capturedReq.Variant != tc.req.Variant {
				t.Errorf("Variant: got %q, want %q", capturedReq.Variant, tc.req.Variant)
			}

			// Verify response mapping (dh → domain)
			if got.Status != tc.wantRes.Status {
				t.Errorf("Status: got %q, want %q", got.Status, tc.wantRes.Status)
			}
			if got.DHCardID != tc.wantRes.DHCardID {
				t.Errorf("DHCardID: got %d, want %d", got.DHCardID, tc.wantRes.DHCardID)
			}
			if len(got.Candidates) != len(tc.wantRes.Candidates) {
				t.Fatalf("Candidates length: got %d, want %d", len(got.Candidates), len(tc.wantRes.Candidates))
			}
			for i, c := range got.Candidates {
				want := tc.wantRes.Candidates[i]
				if c.DHCardID != want.DHCardID {
					t.Errorf("Candidate[%d].DHCardID: got %d, want %d", i, c.DHCardID, want.DHCardID)
				}
				if c.CardName != want.CardName {
					t.Errorf("Candidate[%d].CardName: got %q, want %q", i, c.CardName, want.CardName)
				}
				if c.SetName != want.SetName {
					t.Errorf("Candidate[%d].SetName: got %q, want %q", i, c.SetName, want.SetName)
				}
				if c.CardNumber != want.CardNumber {
					t.Errorf("Candidate[%d].CardNumber: got %q, want %q", i, c.CardNumber, want.CardNumber)
				}
			}
		})
	}
}

func TestCertResolverAdapter_ResolveCert_Error(t *testing.T) {
	wantErr := errors.New("connection refused")
	mock := &mockCertResolver{
		ResolveCertFn: func(_ context.Context, _ dh.CertResolveRequest) (*dh.CertResolution, error) {
			return nil, wantErr
		},
	}

	adapter := NewCertResolverAdapter(mock)
	got, err := adapter.ResolveCert(context.Background(), dhlisting.DHCertResolveRequest{
		CertNumber: "11111111",
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("error: got %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Errorf("result: got %+v, want nil", got)
	}
}

// --- InventoryPusherAdapter tests ---

func TestInventoryPusherAdapter_PushInventory_Success(t *testing.T) {
	tests := []struct {
		name        string
		items       []dhlisting.DHInventoryPushItem
		wantDHItems []dh.InventoryItem
		dhResp      *dh.InventoryPushResponse
		wantResults []dhlisting.DHInventoryPushResultItem
	}{
		{
			name: "single item with market value",
			items: []dhlisting.DHInventoryPushItem{
				{DHCardID: 42, CertNumber: "12345678", Grade: 9.0, CostBasisCents: 5000, MarketValueCents: 8000},
			},
			wantDHItems: []dh.InventoryItem{
				{
					DHCardID:         42,
					CertNumber:       "12345678",
					GradingCompany:   dh.GraderPSA,
					Grade:            9.0,
					CostBasisCents:   5000,
					MarketValueCents: intPtr(8000),
					Status:           dh.InventoryStatusInStock,
				},
			},
			dhResp: &dh.InventoryPushResponse{
				Results: []dh.InventoryResult{
					{
						DHInventoryID:      100,
						CertNumber:         "12345678",
						Status:             "in_stock",
						AssignedPriceCents: 7500,
						Channels: []dh.InventoryChannelStatus{
							{Name: "ebay", Status: "pending"},
						},
					},
				},
			},
			wantResults: []dhlisting.DHInventoryPushResultItem{
				{
					DHInventoryID:      100,
					Status:             "in_stock",
					AssignedPriceCents: 7500,
					ChannelsJSON:       `[{"name":"ebay","status":"pending"}]`,
				},
			},
		},
		{
			name: "zero market value maps to nil pointer",
			items: []dhlisting.DHInventoryPushItem{
				{DHCardID: 7, CertNumber: "00000001", Grade: 10.0, CostBasisCents: 2000, MarketValueCents: 0},
			},
			wantDHItems: []dh.InventoryItem{
				{
					DHCardID:         7,
					CertNumber:       "00000001",
					GradingCompany:   dh.GraderPSA,
					Grade:            10.0,
					CostBasisCents:   2000,
					MarketValueCents: nil,
					Status:           dh.InventoryStatusInStock,
				},
			},
			dhResp: &dh.InventoryPushResponse{
				Results: []dh.InventoryResult{
					{DHInventoryID: 200, Status: "in_stock", AssignedPriceCents: 3000},
				},
			},
			wantResults: []dhlisting.DHInventoryPushResultItem{
				{DHInventoryID: 200, Status: "in_stock", AssignedPriceCents: 3000, ChannelsJSON: "[]"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedItems []dh.InventoryItem
			mock := &mockInventoryPusher{
				PushInventoryFn: func(_ context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
					capturedItems = items
					return tc.dhResp, nil
				},
			}

			adapter := NewInventoryPusherAdapter(mock)
			got, err := adapter.PushInventory(context.Background(), tc.items)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify DH items mapping
			if len(capturedItems) != len(tc.wantDHItems) {
				t.Fatalf("DH items length: got %d, want %d", len(capturedItems), len(tc.wantDHItems))
			}
			for i, item := range capturedItems {
				want := tc.wantDHItems[i]
				if item.DHCardID != want.DHCardID {
					t.Errorf("item[%d].DHCardID: got %d, want %d", i, item.DHCardID, want.DHCardID)
				}
				if item.CertNumber != want.CertNumber {
					t.Errorf("item[%d].CertNumber: got %q, want %q", i, item.CertNumber, want.CertNumber)
				}
				if item.GradingCompany != dh.GraderPSA {
					t.Errorf("item[%d].GradingCompany: got %q, want %q", i, item.GradingCompany, dh.GraderPSA)
				}
				if item.Grade != want.Grade {
					t.Errorf("item[%d].Grade: got %f, want %f", i, item.Grade, want.Grade)
				}
				if item.CostBasisCents != want.CostBasisCents {
					t.Errorf("item[%d].CostBasisCents: got %d, want %d", i, item.CostBasisCents, want.CostBasisCents)
				}
				if item.Status != dh.InventoryStatusInStock {
					t.Errorf("item[%d].Status: got %q, want %q", i, item.Status, dh.InventoryStatusInStock)
				}
				// MarketValueCents pointer check
				if want.MarketValueCents == nil {
					if item.MarketValueCents != nil {
						t.Errorf("item[%d].MarketValueCents: got %d, want nil", i, *item.MarketValueCents)
					}
				} else {
					if item.MarketValueCents == nil {
						t.Errorf("item[%d].MarketValueCents: got nil, want %d", i, *want.MarketValueCents)
					} else if *item.MarketValueCents != *want.MarketValueCents {
						t.Errorf("item[%d].MarketValueCents: got %d, want %d", i, *item.MarketValueCents, *want.MarketValueCents)
					}
				}
			}

			// Verify result mapping
			if len(got.Results) != len(tc.wantResults) {
				t.Fatalf("results length: got %d, want %d", len(got.Results), len(tc.wantResults))
			}
			for i, r := range got.Results {
				want := tc.wantResults[i]
				if r.DHInventoryID != want.DHInventoryID {
					t.Errorf("result[%d].DHInventoryID: got %d, want %d", i, r.DHInventoryID, want.DHInventoryID)
				}
				if r.Status != want.Status {
					t.Errorf("result[%d].Status: got %q, want %q", i, r.Status, want.Status)
				}
				if r.AssignedPriceCents != want.AssignedPriceCents {
					t.Errorf("result[%d].AssignedPriceCents: got %d, want %d", i, r.AssignedPriceCents, want.AssignedPriceCents)
				}
				if r.ChannelsJSON != want.ChannelsJSON {
					t.Errorf("result[%d].ChannelsJSON: got %q, want %q", i, r.ChannelsJSON, want.ChannelsJSON)
				}
			}
		})
	}
}

func TestInventoryPusherAdapter_PushInventory_EmptyInput(t *testing.T) {
	tests := []struct {
		name  string
		items []dhlisting.DHInventoryPushItem
	}{
		{name: "nil input", items: nil},
		{name: "empty slice", items: []dhlisting.DHInventoryPushItem{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockInventoryPusher{
				PushInventoryFn: func(_ context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
					return &dh.InventoryPushResponse{}, nil
				},
			}

			adapter := NewInventoryPusherAdapter(mock)
			got, err := adapter.PushInventory(context.Background(), tc.items)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got.Results) != 0 {
				t.Errorf("results length: got %d, want 0", len(got.Results))
			}
		})
	}
}

func TestInventoryPusherAdapter_PushInventory_Error(t *testing.T) {
	wantErr := errors.New("push failed")
	mock := &mockInventoryPusher{
		PushInventoryFn: func(_ context.Context, _ []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
			return nil, wantErr
		},
	}

	adapter := NewInventoryPusherAdapter(mock)
	got, err := adapter.PushInventory(context.Background(), []dhlisting.DHInventoryPushItem{
		{DHCardID: 1, CertNumber: "111"},
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("error: got %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Errorf("result: got %+v, want nil", got)
	}
}

// --- InventoryListerAdapter tests ---

func TestInventoryListerAdapter_UpdateInventoryStatus_Success(t *testing.T) {
	var capturedID int
	var capturedUpdate dh.InventoryUpdate
	mock := &mockInventoryLister{
		UpdateInventoryFn: func(_ context.Context, id int, update dh.InventoryUpdate) (*dh.InventoryResult, error) {
			capturedID = id
			capturedUpdate = update
			return &dh.InventoryResult{DHInventoryID: id, Status: update.Status}, nil
		},
		SyncChannelsFn: func(_ context.Context, _ int, _ []string) (*dh.ChannelSyncResponse, error) {
			t.Fatal("SyncChannels should not be called")
			return nil, nil
		},
	}

	adapter := NewInventoryListerAdapter(mock)
	err := adapter.UpdateInventoryStatus(context.Background(), 42, "listed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedID != 42 {
		t.Errorf("inventoryID: got %d, want 42", capturedID)
	}
	if capturedUpdate.Status != "listed" {
		t.Errorf("status: got %q, want %q", capturedUpdate.Status, "listed")
	}
}

func TestInventoryListerAdapter_SyncChannels_Success(t *testing.T) {
	var capturedID int
	var capturedChannels []string
	mock := &mockInventoryLister{
		UpdateInventoryFn: func(_ context.Context, _ int, _ dh.InventoryUpdate) (*dh.InventoryResult, error) {
			t.Fatal("UpdateInventory should not be called")
			return nil, nil
		},
		SyncChannelsFn: func(_ context.Context, id int, channels []string) (*dh.ChannelSyncResponse, error) {
			capturedID = id
			capturedChannels = channels
			return &dh.ChannelSyncResponse{DHInventoryID: id, Status: "synced"}, nil
		},
	}

	adapter := NewInventoryListerAdapter(mock)
	err := adapter.SyncChannels(context.Background(), 55, []string{"ebay", "shopify"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedID != 55 {
		t.Errorf("inventoryID: got %d, want 55", capturedID)
	}
	if len(capturedChannels) != 2 || capturedChannels[0] != "ebay" || capturedChannels[1] != "shopify" {
		t.Errorf("channels: got %v, want [ebay shopify]", capturedChannels)
	}
}

func TestInventoryListerAdapter_UpdateInventoryStatus_Error(t *testing.T) {
	wantErr := errors.New("update failed")
	mock := &mockInventoryLister{
		UpdateInventoryFn: func(_ context.Context, _ int, _ dh.InventoryUpdate) (*dh.InventoryResult, error) {
			return nil, wantErr
		},
		SyncChannelsFn: func(_ context.Context, _ int, _ []string) (*dh.ChannelSyncResponse, error) {
			return nil, nil
		},
	}

	adapter := NewInventoryListerAdapter(mock)
	err := adapter.UpdateInventoryStatus(context.Background(), 1, "listed")
	if !errors.Is(err, wantErr) {
		t.Errorf("error: got %v, want %v", err, wantErr)
	}
}

func TestInventoryListerAdapter_SyncChannels_Error(t *testing.T) {
	wantErr := errors.New("sync failed")
	mock := &mockInventoryLister{
		UpdateInventoryFn: func(_ context.Context, _ int, _ dh.InventoryUpdate) (*dh.InventoryResult, error) {
			return nil, nil
		},
		SyncChannelsFn: func(_ context.Context, _ int, _ []string) (*dh.ChannelSyncResponse, error) {
			return nil, wantErr
		},
	}

	adapter := NewInventoryListerAdapter(mock)
	err := adapter.SyncChannels(context.Background(), 1, []string{"ebay"})
	if !errors.Is(err, wantErr) {
		t.Errorf("error: got %v, want %v", err, wantErr)
	}
}

// intPtr is a test helper that returns a pointer to v.
func intPtr(v int) *int {
	return &v
}
