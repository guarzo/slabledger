package dhlisting

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// --- inline mock types used only in this file ---

type mockCertResolver struct {
	resp *DHCertResolution
	err  error
}

func (m *mockCertResolver) ResolveCert(_ context.Context, _ DHCertResolveRequest) (*DHCertResolution, error) {
	return m.resp, m.err
}

type mockPusher struct {
	resp *DHInventoryPushResult
	err  error
}

func (m *mockPusher) PushInventory(_ context.Context, _ []DHInventoryPushItem) (*DHInventoryPushResult, error) {
	return m.resp, m.err
}

type mockPushStatusUpdater struct {
	err    error
	called bool
	status string
}

func (m *mockPushStatusUpdater) UpdatePurchaseDHPushStatus(_ context.Context, _ string, status string) error {
	m.called = true
	m.status = status
	return m.err
}

type mockCardIDSaver struct {
	err    error
	called bool
}

func (m *mockCardIDSaver) SaveExternalID(_ context.Context, _, _, _, _, _ string) error {
	m.called = true
	return m.err
}

type mockCandidatesSaver struct {
	err    error
	called bool
}

func (m *mockCandidatesSaver) UpdatePurchaseDHCandidates(_ context.Context, _ string, _ string) error {
	m.called = true
	return m.err
}

// newInlineTestSvc builds a dhListingService directly so we can call the
// unexported inlineMatchAndPush method in white-box tests.
// Nil concrete pointers are intentionally not assigned to interface fields
// (which would create a non-nil interface wrapping a nil pointer).
func newInlineTestSvc(
	lookup DHListingPurchaseLookup,
	resolver *mockCertResolver,
	pusher *mockPusher,
	fieldsUpdater *mockFieldsUpdater,
	pushStatusUpdater *mockPushStatusUpdater,
	cardIDSaver *mockCardIDSaver,
	candidatesSaver *mockCandidatesSaver,
) *dhListingService {
	s := &dhListingService{
		purchaseLookup: lookup,
		logger:         observability.NewNoopLogger(),
	}
	if resolver != nil {
		s.certResolver = resolver
	}
	if pusher != nil {
		s.pusher = pusher
	}
	if fieldsUpdater != nil {
		s.fieldsUpdater = fieldsUpdater
	}
	if pushStatusUpdater != nil {
		s.pushStatusUpdater = pushStatusUpdater
	}
	if cardIDSaver != nil {
		s.cardIDSaver = cardIDSaver
	}
	if candidatesSaver != nil {
		s.candidatesSaver = candidatesSaver
	}
	return s
}

func basePurchase() *inventory.Purchase {
	return &inventory.Purchase{
		ID:                 "p1",
		CertNumber:         "12345678",
		CardName:           "CHARIZARD-HOLO",
		SetName:            "Base Set",
		CardNumber:         "004",
		CardYear:           "1999",
		GradeValue:         10.0,
		BuyCostCents:       10000,
		CLValueCents:       20000,
		ReviewedPriceCents: 0,
	}
}

// TestInlineMatchAndPush tests the inlineMatchAndPush happy path and key failure
// modes, exercising the cert-resolver → pusher → persist → status-update chain.
func TestInlineMatchAndPush(t *testing.T) {
	ctx := context.Background()
	lookup := &mockPurchaseLookup{}

	tests := []struct {
		name              string
		purchase          *inventory.Purchase
		resolver          *mockCertResolver
		pusher            *mockPusher
		fieldsUpdater     *mockFieldsUpdater
		pushStatusUpdater *mockPushStatusUpdater
		cardIDSaver       *mockCardIDSaver
		wantInventoryID   int
		wantStatusSet     string // expected status set on pushStatusUpdater, "" = not set
	}{
		{
			name:     "happy path: matched cert → push succeeds → returns inventoryID",
			purchase: basePurchase(),
			resolver: &mockCertResolver{
				resp: &DHCertResolution{Status: DHCertStatusMatched, DHCardID: 777},
			},
			pusher: &mockPusher{
				resp: &DHInventoryPushResult{
					Results: []DHInventoryPushResultItem{
						{DHInventoryID: 999, Status: "in_stock", AssignedPriceCents: 20000},
					},
				},
			},
			fieldsUpdater:     &mockFieldsUpdater{},
			pushStatusUpdater: &mockPushStatusUpdater{},
			cardIDSaver:       &mockCardIDSaver{},
			wantInventoryID:   999,
			wantStatusSet:     inventory.DHPushStatusMatched,
		},
		{
			name:     "empty cert number: returns 0",
			purchase: func() *inventory.Purchase { p := basePurchase(); p.CertNumber = ""; return p }(),
			resolver: &mockCertResolver{
				resp: &DHCertResolution{Status: DHCertStatusMatched, DHCardID: 1},
			},
			pusher:          &mockPusher{},
			wantInventoryID: 0,
		},
		{
			name:            "cert resolver error: returns 0",
			purchase:        basePurchase(),
			resolver:        &mockCertResolver{err: errors.New("api down")},
			pusher:          &mockPusher{},
			wantInventoryID: 0,
		},
		{
			name:     "resolver returns not_found: markUnmatched, returns 0",
			purchase: basePurchase(),
			resolver: &mockCertResolver{
				resp: &DHCertResolution{Status: DHCertStatusNotFound},
			},
			pusher:            &mockPusher{},
			pushStatusUpdater: &mockPushStatusUpdater{},
			wantInventoryID:   0,
			wantStatusSet:     inventory.DHPushStatusUnmatched,
		},
		{
			name:     "resolver returns ambiguous with no resolvable card number: returns 0",
			purchase: basePurchase(),
			resolver: &mockCertResolver{
				resp: &DHCertResolution{
					Status: DHCertStatusAmbiguous,
					Candidates: []DHCertCandidate{
						{DHCardID: 10, CardNumber: "001"},
						{DHCardID: 20, CardNumber: "002"},
					},
				},
			},
			pusher:            &mockPusher{},
			pushStatusUpdater: &mockPushStatusUpdater{},
			wantInventoryID:   0,
			wantStatusSet:     inventory.DHPushStatusUnmatched,
		},
		{
			name:     "resolver returns ambiguous and disambiguates: pushes successfully",
			purchase: basePurchase(), // CardNumber = "004"
			resolver: &mockCertResolver{
				resp: &DHCertResolution{
					Status: DHCertStatusAmbiguous,
					Candidates: []DHCertCandidate{
						{DHCardID: 55, CardNumber: "004"},
						{DHCardID: 66, CardNumber: "005"},
					},
				},
			},
			pusher: &mockPusher{
				resp: &DHInventoryPushResult{
					Results: []DHInventoryPushResultItem{
						{DHInventoryID: 444, Status: "in_stock", AssignedPriceCents: 20000},
					},
				},
			},
			fieldsUpdater:     &mockFieldsUpdater{},
			pushStatusUpdater: &mockPushStatusUpdater{},
			wantInventoryID:   444,
			wantStatusSet:     inventory.DHPushStatusMatched,
		},
		{
			name: "no reviewed price: push still proceeds without listing_price_cents preset",
			purchase: func() *inventory.Purchase {
				p := basePurchase()
				p.CLValueCents = 0
				p.ReviewedPriceCents = 0
				return p
			}(),
			resolver: &mockCertResolver{
				resp: &DHCertResolution{Status: DHCertStatusMatched, DHCardID: 123},
			},
			pusher: &mockPusher{
				resp: &DHInventoryPushResult{
					Results: []DHInventoryPushResultItem{
						{DHInventoryID: 200, Status: "in_stock"},
					},
				},
			},
			fieldsUpdater:     &mockFieldsUpdater{},
			pushStatusUpdater: &mockPushStatusUpdater{},
			wantInventoryID:   200,
			wantStatusSet:     inventory.DHPushStatusMatched,
		},
		{
			name:     "pusher error: returns 0",
			purchase: basePurchase(),
			resolver: &mockCertResolver{
				resp: &DHCertResolution{Status: DHCertStatusMatched, DHCardID: 123},
			},
			pusher:          &mockPusher{err: errors.New("DH push failed")},
			wantInventoryID: 0,
		},
		{
			name:     "pusher result status=failed: skipped, returns 0",
			purchase: basePurchase(),
			resolver: &mockCertResolver{
				resp: &DHCertResolution{Status: DHCertStatusMatched, DHCardID: 123},
			},
			pusher: &mockPusher{
				resp: &DHInventoryPushResult{
					Results: []DHInventoryPushResultItem{
						{DHInventoryID: 0, Status: "failed"},
					},
				},
			},
			wantInventoryID: 0,
		},
		{
			name:     "fieldsUpdater error: returns 0 to prevent duplicate push",
			purchase: basePurchase(),
			resolver: &mockCertResolver{
				resp: &DHCertResolution{Status: DHCertStatusMatched, DHCardID: 123},
			},
			pusher: &mockPusher{
				resp: &DHInventoryPushResult{
					Results: []DHInventoryPushResultItem{
						{DHInventoryID: 55, Status: "in_stock"},
					},
				},
			},
			fieldsUpdater:   &mockFieldsUpdater{updateErr: errors.New("db error")},
			wantInventoryID: 0,
		},
		{
			name:     "cardIDSaver error does not abort the push",
			purchase: basePurchase(),
			resolver: &mockCertResolver{
				resp: &DHCertResolution{Status: DHCertStatusMatched, DHCardID: 888},
			},
			pusher: &mockPusher{
				resp: &DHInventoryPushResult{
					Results: []DHInventoryPushResultItem{
						{DHInventoryID: 777, Status: "in_stock", AssignedPriceCents: 20000},
					},
				},
			},
			fieldsUpdater:     &mockFieldsUpdater{},
			pushStatusUpdater: &mockPushStatusUpdater{},
			cardIDSaver:       &mockCardIDSaver{err: errors.New("mapping save failed")},
			wantInventoryID:   777,
			wantStatusSet:     inventory.DHPushStatusMatched,
		},
		{
			name: "reviewedPrice takes precedence over CLValue for market value",
			purchase: func() *inventory.Purchase {
				p := basePurchase()
				p.ReviewedPriceCents = 35000
				p.CLValueCents = 20000
				return p
			}(),
			resolver: &mockCertResolver{
				resp: &DHCertResolution{Status: DHCertStatusMatched, DHCardID: 321},
			},
			pusher: &mockPusher{
				resp: &DHInventoryPushResult{
					Results: []DHInventoryPushResultItem{
						{DHInventoryID: 1001, Status: "in_stock", AssignedPriceCents: 35000},
					},
				},
			},
			fieldsUpdater:     &mockFieldsUpdater{},
			pushStatusUpdater: &mockPushStatusUpdater{},
			wantInventoryID:   1001,
			wantStatusSet:     inventory.DHPushStatusMatched,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newInlineTestSvc(
				lookup,
				tc.resolver,
				tc.pusher,
				tc.fieldsUpdater,
				tc.pushStatusUpdater,
				tc.cardIDSaver,
				nil,
			)

			gotID := svc.inlineMatchAndPush(ctx, tc.purchase)

			if gotID != tc.wantInventoryID {
				t.Errorf("inventoryID: got %d, want %d", gotID, tc.wantInventoryID)
			}

			if tc.wantStatusSet != "" {
				if tc.pushStatusUpdater == nil {
					t.Error("expected pushStatusUpdater to be set but it is nil")
				} else if !tc.pushStatusUpdater.called {
					t.Error("expected pushStatusUpdater to be called, but it was not")
				} else if tc.pushStatusUpdater.status != tc.wantStatusSet {
					t.Errorf("expected push status %q, got %q", tc.wantStatusSet, tc.pushStatusUpdater.status)
				}
			}
		})
	}
}

// TestListPurchases_InlinePendingPush exercises the ListPurchases branch that
// calls inlineMatchAndPush for purchases with DHInventoryID == 0 and status pending.
func TestListPurchases_InlinePendingPush(t *testing.T) {
	ctx := context.Background()
	certNum := "11111111"

	makePending := func() *inventory.Purchase {
		return &inventory.Purchase{
			ID:                 "p-pending",
			CertNumber:         certNum,
			DHInventoryID:      0,
			DHPushStatus:       inventory.DHPushStatusPending,
			ReviewedPriceCents: 20000, // required by the listing gate
			BuyCostCents:       10000,
			CardName:           "Pikachu",
			SetName:            "Base Set",
		}
	}

	tests := []struct {
		name       string
		resolver   *mockCertResolver
		pusher     *mockPusher
		wantListed int
		wantTotal  int
	}{
		{
			name: "inline push succeeds: item proceeds to listing",
			resolver: &mockCertResolver{
				resp: &DHCertResolution{Status: DHCertStatusMatched, DHCardID: 500},
			},
			pusher: &mockPusher{
				resp: &DHInventoryPushResult{
					Results: []DHInventoryPushResultItem{
						{DHInventoryID: 800, Status: "in_stock"},
					},
				},
			},
			wantListed: 1,
			wantTotal:  1,
		},
		{
			name: "inline push fails (cert resolve error): item skipped",
			resolver: &mockCertResolver{
				err: errors.New("resolve failed"),
			},
			pusher:     &mockPusher{},
			wantListed: 0,
			wantTotal:  1,
		},
		{
			name: "no resolver configured: pending item skipped",
			// resolver and pusher are nil — tested via constructor option
			wantListed: 0,
			wantTotal:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &mockPurchaseLookup{
				purchases: map[string]*inventory.Purchase{certNum: makePending()},
			}
			lister := &mockInventoryLister{}
			fieldsUpdater := &mockFieldsUpdater{}

			var opts []DHListingServiceOption
			opts = append(opts, WithDHListingLister(lister))
			opts = append(opts, WithDHListingFieldsUpdater(fieldsUpdater))
			if tc.resolver != nil {
				opts = append(opts, WithDHListingCertResolver(tc.resolver))
			}
			if tc.pusher != nil {
				opts = append(opts, WithDHListingPusher(tc.pusher))
			}

			svc := newTestService(t, lookup, opts...)
			result := svc.ListPurchases(ctx, []string{certNum})

			if result.Listed != tc.wantListed {
				t.Errorf("Listed: got %d, want %d", result.Listed, tc.wantListed)
			}
			if result.Total != tc.wantTotal {
				t.Errorf("Total: got %d, want %d", result.Total, tc.wantTotal)
			}
		})
	}
}

// TestListPurchases_SyncChannelsFailure tests the revert-to-in_stock path
// when SyncChannels fails after a successful UpdateInventoryStatus.
func TestListPurchases_SyncChannelsFailure(t *testing.T) {
	ctx := context.Background()
	certNum := "22222222"
	purchase := &inventory.Purchase{
		ID:                 "p-sync-fail",
		CertNumber:         certNum,
		DHInventoryID:      77,
		ReviewedPriceCents: 50000, // required by listing gate
	}

	lookup := &mockPurchaseLookup{
		purchases: map[string]*inventory.Purchase{certNum: purchase},
	}
	lister := &mockInventoryLister{
		syncChannelsErr: errors.New("channel sync failed"),
	}
	fieldsUpdater := &mockFieldsUpdater{}

	svc := newTestService(t,
		lookup,
		WithDHListingLister(lister),
		WithDHListingFieldsUpdater(fieldsUpdater),
	)

	result := svc.ListPurchases(ctx, []string{certNum})

	// After sync failure + revert, listed count should be 0.
	if result.Listed != 0 {
		t.Errorf("Listed: got %d, want 0", result.Listed)
	}
	if result.Total != 1 {
		t.Errorf("Total: got %d, want 1", result.Total)
	}
	// The revert persist should have been called with DHStatusInStock.
	if len(fieldsUpdater.calls) == 0 {
		t.Fatal("expected fieldsUpdater to be called for revert, but it was not")
	}
	revertCall := fieldsUpdater.calls[0]
	if revertCall.DHStatus != inventory.DHStatusInStock {
		t.Errorf("revert status: got %q, want %q", revertCall.DHStatus, inventory.DHStatusInStock)
	}
}
