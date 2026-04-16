package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Local mocks for DHPush-specific interfaces
// ---------------------------------------------------------------------------

type mockDHPushPendingLister struct {
	ListFn func(ctx context.Context, status string, limit int) ([]inventory.Purchase, error)
}

func (m *mockDHPushPendingLister) GetPurchasesByDHPushStatus(ctx context.Context, status string, limit int) ([]inventory.Purchase, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, status, limit)
	}
	return nil, nil
}

type mockDHPushStatusUpdater struct {
	UpdateFn func(ctx context.Context, id string, status string) error
	Calls    []struct{ ID, Status string }
}

func (m *mockDHPushStatusUpdater) UpdatePurchaseDHPushStatus(ctx context.Context, id string, status string) error {
	m.Calls = append(m.Calls, struct{ ID, Status string }{id, status})
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, id, status)
	}
	return nil
}

type mockDHPushCertResolver struct {
	ResolveFn func(ctx context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error)
}

func (m *mockDHPushCertResolver) ResolveCert(ctx context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error) {
	if m.ResolveFn != nil {
		return m.ResolveFn(ctx, req)
	}
	return &dh.CertResolution{Status: dh.CertStatusMatched, DHCardID: 42}, nil
}

type mockDHPushInventoryPusher struct {
	PushFn func(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error)
}

func (m *mockDHPushInventoryPusher) PushInventory(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
	if m.PushFn != nil {
		return m.PushFn(ctx, items)
	}
	return &dh.InventoryPushResponse{
		Results: []dh.InventoryResult{
			{DHInventoryID: 99, CertNumber: "", Status: "in_stock", AssignedPriceCents: 5000},
		},
	}, nil
}

type mockDHPushCardIDSaver struct {
	SaveFn      func(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
	GetMappedFn func(ctx context.Context, provider string) (map[string]string, error)
}

func (m *mockDHPushCardIDSaver) SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error {
	if m.SaveFn != nil {
		return m.SaveFn(ctx, cardName, setName, collectorNumber, provider, externalID)
	}
	return nil
}

func (m *mockDHPushCardIDSaver) GetMappedSet(ctx context.Context, provider string) (map[string]string, error) {
	if m.GetMappedFn != nil {
		return m.GetMappedFn(ctx, provider)
	}
	return make(map[string]string), nil
}

// newTestDHPushScheduler creates a DHPushScheduler with all defaults wired.
func newTestDHPushScheduler(
	lister *mockDHPushPendingLister,
	statusUpdater *mockDHPushStatusUpdater,
	certResolver *mockDHPushCertResolver,
	pusher *mockDHPushInventoryPusher,
	fieldsUpdater *mocks.MockDHFieldsUpdater,
	cardIDSaver *mockDHPushCardIDSaver,
) *DHPushScheduler {
	return NewDHPushScheduler(
		lister,
		statusUpdater,
		certResolver,
		pusher,
		fieldsUpdater,
		cardIDSaver,
		mocks.NewMockLogger(),
		DHPushConfig{Enabled: true, Interval: 1 * time.Hour},
	)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestDHPush_Disabled(t *testing.T) {
	s := NewDHPushScheduler(
		&mockDHPushPendingLister{},
		&mockDHPushStatusUpdater{},
		&mockDHPushCertResolver{},
		&mockDHPushInventoryPusher{},
		&mocks.MockDHFieldsUpdater{},
		&mockDHPushCardIDSaver{},
		mocks.NewMockLogger(),
		DHPushConfig{Enabled: false},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() { s.Start(ctx); close(done) }()

	select {
	case <-done:
		// expected — disabled scheduler returns immediately
	case <-time.After(1 * time.Second):
		t.Fatal("disabled scheduler should return immediately from Start")
	}
}

func TestDHPush_EmptyBatch(t *testing.T) {
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return []inventory.Purchase{}, nil
		},
	}
	statusUpdater := &mockDHPushStatusUpdater{}
	certResolver := &mockDHPushCertResolver{}
	pusher := &mockDHPushInventoryPusher{}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}

	s := newTestDHPushScheduler(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver)
	s.push(context.Background())

	assert.Empty(t, statusUpdater.Calls, "no status updates should occur for empty batch")
	assert.Empty(t, fieldsUpdater.Calls, "no field updates should occur for empty batch")
}

func TestDHPush_ListerError(t *testing.T) {
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return nil, fmt.Errorf("database unavailable")
		},
	}
	statusUpdater := &mockDHPushStatusUpdater{}
	certResolver := &mockDHPushCertResolver{}
	pusher := &mockDHPushInventoryPusher{}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}

	s := newTestDHPushScheduler(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver)
	// Should not panic; errors are logged and the method returns
	s.push(context.Background())

	assert.Empty(t, statusUpdater.Calls)
	assert.Empty(t, fieldsUpdater.Calls)
}

func TestDHPush_NoCertNumber_MarksUnmatched(t *testing.T) {
	purchase := inventory.Purchase{
		ID:         "pur-1",
		CertNumber: "", // no cert
		CardName:   "Charizard",
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return []inventory.Purchase{purchase}, nil
		},
	}
	statusUpdater := &mockDHPushStatusUpdater{}
	certResolver := &mockDHPushCertResolver{}
	pusher := &mockDHPushInventoryPusher{}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}

	s := newTestDHPushScheduler(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver)
	s.push(context.Background())

	require.Len(t, statusUpdater.Calls, 1)
	assert.Equal(t, "pur-1", statusUpdater.Calls[0].ID)
	assert.Equal(t, inventory.DHPushStatusUnmatched, statusUpdater.Calls[0].Status)
	assert.Empty(t, fieldsUpdater.Calls)
}

// TestDHPush_NoReviewedPrice_PushesWithoutPreset verifies that the scheduler
// no longer blocks the push on having a price. With DH's listing_price_cents
// API change, push happens without a preset (DH catalog fallback) so the item
// gets matched + assigned an inventory ID. The actual list-time price gate
// lives on the manual List-on-DH path.
func TestDHPush_NoReviewedPrice_PushesWithoutPreset(t *testing.T) {
	purchase := inventory.Purchase{
		ID:                 "pur-noreview",
		CertNumber:         "12345678",
		CardName:           "Pikachu",
		SetName:            "Base Set",
		BuyCostCents:       5000,
		CLValueCents:       9000, // CL is no longer used by ResolveListingPriceCents
		ReviewedPriceCents: 0,    // no reviewed price → no preset sent to DH
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return []inventory.Purchase{purchase}, nil
		},
	}
	var capturedItem dh.InventoryItem
	certResolver := &mockDHPushCertResolver{
		ResolveFn: func(_ context.Context, _ dh.CertResolveRequest) (*dh.CertResolution, error) {
			return &dh.CertResolution{Status: dh.CertStatusMatched, DHCardID: 42}, nil
		},
	}
	statusUpdater := &mockDHPushStatusUpdater{}
	pusher := &mockDHPushInventoryPusher{
		PushFn: func(_ context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
			capturedItem = items[0]
			return &dh.InventoryPushResponse{
				Results: []dh.InventoryResult{{DHInventoryID: 99, Status: "in_stock"}},
			}, nil
		},
	}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}

	s := newTestDHPushScheduler(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver)
	s.push(context.Background())

	assert.Nil(t, capturedItem.ListingPriceCents, "listing_price_cents should be omitted when no reviewed price")
	require.Len(t, statusUpdater.Calls, 1, "purchase should still transition to matched")
	assert.Equal(t, inventory.DHPushStatusMatched, statusUpdater.Calls[0].Status)
}

func TestDHPush_CertResolveError_LeavesAsPending(t *testing.T) {
	purchase := inventory.Purchase{
		ID:           "pur-2",
		CertNumber:   "12345678",
		CardName:     "Pikachu",
		SetName:      "Base Set",
		CLValueCents: 5000,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return []inventory.Purchase{purchase}, nil
		},
	}
	certResolver := &mockDHPushCertResolver{
		ResolveFn: func(_ context.Context, _ dh.CertResolveRequest) (*dh.CertResolution, error) {
			return nil, fmt.Errorf("cert API error")
		},
	}
	statusUpdater := &mockDHPushStatusUpdater{}
	pusher := &mockDHPushInventoryPusher{}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}

	s := newTestDHPushScheduler(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver)
	s.push(context.Background())

	// Status should NOT be updated when cert resolve errors (stays pending)
	assert.Empty(t, statusUpdater.Calls, "purchase should stay pending when cert resolve fails")
	assert.Empty(t, fieldsUpdater.Calls)
}

func TestDHPush_CertNotMatched_MarksUnmatched(t *testing.T) {
	purchase := inventory.Purchase{
		ID:           "pur-3",
		CertNumber:   "99999999",
		CardName:     "Machamp",
		SetName:      "Base Set",
		CLValueCents: 5000,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return []inventory.Purchase{purchase}, nil
		},
	}
	certResolver := &mockDHPushCertResolver{
		ResolveFn: func(_ context.Context, _ dh.CertResolveRequest) (*dh.CertResolution, error) {
			return &dh.CertResolution{Status: dh.CertStatusNotFound}, nil
		},
	}
	statusUpdater := &mockDHPushStatusUpdater{}
	pusher := &mockDHPushInventoryPusher{}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}

	s := newTestDHPushScheduler(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver)
	s.push(context.Background())

	require.Len(t, statusUpdater.Calls, 1)
	assert.Equal(t, "pur-3", statusUpdater.Calls[0].ID)
	assert.Equal(t, inventory.DHPushStatusUnmatched, statusUpdater.Calls[0].Status)
}

func TestDHPush_InventoryPushError_LeavesAsPending(t *testing.T) {
	purchase := inventory.Purchase{
		ID:           "pur-4",
		CertNumber:   "11111111",
		CardName:     "Blastoise",
		CLValueCents: 5000,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return []inventory.Purchase{purchase}, nil
		},
	}
	certResolver := &mockDHPushCertResolver{
		ResolveFn: func(_ context.Context, _ dh.CertResolveRequest) (*dh.CertResolution, error) {
			return &dh.CertResolution{Status: dh.CertStatusMatched, DHCardID: 10}, nil
		},
	}
	pusher := &mockDHPushInventoryPusher{
		PushFn: func(_ context.Context, _ []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
			return nil, fmt.Errorf("push API error")
		},
	}
	statusUpdater := &mockDHPushStatusUpdater{}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}

	s := newTestDHPushScheduler(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver)
	s.push(context.Background())

	// Status should NOT be updated on push error — stays pending for retry
	assert.Empty(t, statusUpdater.Calls)
	assert.Empty(t, fieldsUpdater.Calls)
}

func TestDHPush_SuccessPath_UpdatesFields(t *testing.T) {
	purchase := inventory.Purchase{
		ID:           "pur-5",
		CertNumber:   "22222222",
		CardName:     "Umbreon ex",
		SetName:      "SV Promo",
		CardNumber:   "176",
		GradeValue:         10,
		BuyCostCents:       6000,
		ReviewedPriceCents: 8000, // reviewed price → DH listing_price_cents preset
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return []inventory.Purchase{purchase}, nil
		},
	}
	certResolver := &mockDHPushCertResolver{
		ResolveFn: func(_ context.Context, _ dh.CertResolveRequest) (*dh.CertResolution, error) {
			return &dh.CertResolution{Status: dh.CertStatusMatched, DHCardID: 777}, nil
		},
	}
	pusher := &mockDHPushInventoryPusher{
		PushFn: func(_ context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
			require.Len(t, items, 1)
			assert.Equal(t, 777, items[0].DHCardID)
			assert.Equal(t, "22222222", items[0].CertNumber)
			assert.Equal(t, float64(10), items[0].Grade)
			assert.Equal(t, 6000, items[0].CostBasisCents)
			require.NotNil(t, items[0].ListingPriceCents)
			assert.Equal(t, 8000, *items[0].ListingPriceCents)
			return &dh.InventoryPushResponse{
				Results: []dh.InventoryResult{
					{DHInventoryID: 555, Status: "in_stock", AssignedPriceCents: 9000},
				},
			}, nil
		},
	}
	statusUpdater := &mockDHPushStatusUpdater{}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}

	s := newTestDHPushScheduler(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver)
	s.push(context.Background())

	require.Len(t, fieldsUpdater.Calls, 1)
	assert.Equal(t, "pur-5", fieldsUpdater.IDs[0])
	assert.Equal(t, 777, fieldsUpdater.Calls[0].CardID)
	assert.Equal(t, 555, fieldsUpdater.Calls[0].InventoryID)
	assert.Equal(t, 9000, fieldsUpdater.Calls[0].ListingPriceCents)
	assert.Equal(t, dh.CertStatusMatched, fieldsUpdater.Calls[0].CertStatus)

	require.Len(t, statusUpdater.Calls, 1)
	assert.Equal(t, "pur-5", statusUpdater.Calls[0].ID)
	assert.Equal(t, inventory.DHPushStatusMatched, statusUpdater.Calls[0].Status)
}

func TestDHPush_AlreadyMapped_SkipsCertResolve(t *testing.T) {
	purchase := inventory.Purchase{
		ID:           "pur-6",
		CertNumber:   "33333333",
		CardName:     "Charizard",
		SetName:      "Base Set",
		CardNumber:   "4",
		CLValueCents: 5000,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return []inventory.Purchase{purchase}, nil
		},
	}
	resolverCalled := false
	certResolver := &mockDHPushCertResolver{
		ResolveFn: func(_ context.Context, _ dh.CertResolveRequest) (*dh.CertResolution, error) {
			resolverCalled = true
			return &dh.CertResolution{Status: dh.CertStatusMatched, DHCardID: 999}, nil
		},
	}
	pusher := &mockDHPushInventoryPusher{}
	statusUpdater := &mockDHPushStatusUpdater{}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	key := inventory.DHCardKey(purchase.CardName, purchase.SetName, purchase.CardNumber)
	cardIDSaver := &mockDHPushCardIDSaver{
		GetMappedFn: func(_ context.Context, _ string) (map[string]string, error) {
			// Pre-populate cache so cert resolve is skipped
			return map[string]string{key: "888"}, nil
		},
	}

	s := newTestDHPushScheduler(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver)
	s.push(context.Background())

	assert.False(t, resolverCalled, "cert resolver should not be called when DH card ID is already mapped")
}

func TestDHPush_InventoryPushEmptyResults_LeavesAsPending(t *testing.T) {
	purchase := inventory.Purchase{
		ID:           "pur-7",
		CertNumber:   "44444444",
		CardName:     "Venusaur",
		CLValueCents: 5000,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return []inventory.Purchase{purchase}, nil
		},
	}
	certResolver := &mockDHPushCertResolver{
		ResolveFn: func(_ context.Context, _ dh.CertResolveRequest) (*dh.CertResolution, error) {
			return &dh.CertResolution{Status: dh.CertStatusMatched, DHCardID: 100}, nil
		},
	}
	pusher := &mockDHPushInventoryPusher{
		PushFn: func(_ context.Context, _ []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
			return &dh.InventoryPushResponse{Results: []dh.InventoryResult{}}, nil
		},
	}
	statusUpdater := &mockDHPushStatusUpdater{}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}

	s := newTestDHPushScheduler(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver)
	s.push(context.Background())

	// Empty results = leave as pending for retry
	assert.Empty(t, statusUpdater.Calls, "empty push results should leave purchase as pending")
	assert.Empty(t, fieldsUpdater.Calls)
}

func TestDHPush_InventoryPushFailedStatus_LeavesAsPending(t *testing.T) {
	purchase := inventory.Purchase{
		ID:           "pur-8",
		CertNumber:   "55555555",
		CardName:     "Gyarados",
		CLValueCents: 5000,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return []inventory.Purchase{purchase}, nil
		},
	}
	certResolver := &mockDHPushCertResolver{
		ResolveFn: func(_ context.Context, _ dh.CertResolveRequest) (*dh.CertResolution, error) {
			return &dh.CertResolution{Status: dh.CertStatusMatched, DHCardID: 200}, nil
		},
	}
	pusher := &mockDHPushInventoryPusher{
		PushFn: func(_ context.Context, _ []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
			return &dh.InventoryPushResponse{
				Results: []dh.InventoryResult{
					{DHInventoryID: 0, Status: "failed"},
				},
			}, nil
		},
	}
	statusUpdater := &mockDHPushStatusUpdater{}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}

	s := newTestDHPushScheduler(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver)
	s.push(context.Background())

	assert.Empty(t, statusUpdater.Calls)
	assert.Empty(t, fieldsUpdater.Calls)
}

func TestDHPush_MultiplePurchases_AllProcessed(t *testing.T) {
	purchases := []inventory.Purchase{
		{ID: "pur-a", CertNumber: "11111111", CardName: "Pikachu", CLValueCents: 5000},
		{ID: "pur-b", CertNumber: "22222222", CardName: "Raichu", CLValueCents: 5000},
		{ID: "pur-c", CertNumber: "", CardName: "Gengar", CLValueCents: 5000}, // no cert → unmatched
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return purchases, nil
		},
	}
	certResolver := &mockDHPushCertResolver{
		ResolveFn: func(_ context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error) {
			return &dh.CertResolution{Status: dh.CertStatusMatched, DHCardID: 1}, nil
		},
	}
	pusher := &mockDHPushInventoryPusher{
		PushFn: func(_ context.Context, _ []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
			return &dh.InventoryPushResponse{
				Results: []dh.InventoryResult{
					{DHInventoryID: 10, Status: "in_stock", AssignedPriceCents: 1000},
				},
			}, nil
		},
	}
	statusUpdater := &mockDHPushStatusUpdater{}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}

	s := newTestDHPushScheduler(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver)
	s.push(context.Background())

	// pur-a and pur-b: matched → both get DHFields update + status=matched
	// pur-c: no cert → status=unmatched
	assert.Len(t, fieldsUpdater.Calls, 2, "two purchases should be matched")

	var unmatchedCalls []string
	var matchedCalls []string
	for _, c := range statusUpdater.Calls {
		switch c.Status {
		case inventory.DHPushStatusUnmatched:
			unmatchedCalls = append(unmatchedCalls, c.ID)
		case inventory.DHPushStatusMatched:
			matchedCalls = append(matchedCalls, c.ID)
		}
	}
	assert.ElementsMatch(t, []string{"pur-c"}, unmatchedCalls)
	assert.ElementsMatch(t, []string{"pur-a", "pur-b"}, matchedCalls)
}

// ---------------------------------------------------------------------------
// Hold setter mock
// ---------------------------------------------------------------------------

type mockDHPushHoldSetter struct {
	Calls []struct{ ID, Reason string }
}

func (m *mockDHPushHoldSetter) SetHeldWithReason(_ context.Context, purchaseID, reason string) error {
	m.Calls = append(m.Calls, struct{ ID, Reason string }{purchaseID, reason})
	return nil
}

func TestDHPush_Hold_RecordsHeldEvent(t *testing.T) {
	// Market value (3000) < 50% of buy cost (10000) triggers initial_value_mismatch hold.
	purchase := inventory.Purchase{
		ID:           "pur-hold-1",
		CertNumber:   "11112222",
		CardName:     "Charizard",
		SetName:      "Base Set",
		CardNumber:   "4",
		BuyCostCents: 10000,
		ReviewedPriceCents: 3000, // listing_price_cents = 3000, floor = 50% of 10000 = 5000
		DHCardID:     42,   // already mapped, skip cert resolve
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return []inventory.Purchase{purchase}, nil
		},
	}
	statusUpdater := &mockDHPushStatusUpdater{}
	certResolver := &mockDHPushCertResolver{}
	pusher := &mockDHPushInventoryPusher{}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}
	holdSetter := &mockDHPushHoldSetter{}
	rec := &mocks.MockEventRecorder{}

	s := NewDHPushScheduler(
		lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver,
		mocks.NewMockLogger(),
		DHPushConfig{Enabled: true, Interval: 1 * time.Hour},
		WithDHPushHoldSetter(holdSetter),
		WithDHPushEventRecorder(rec),
	)
	s.push(context.Background())

	// Hold setter should have been called.
	require.Len(t, holdSetter.Calls, 1)
	assert.Equal(t, "pur-hold-1", holdSetter.Calls[0].ID)
	assert.Contains(t, holdSetter.Calls[0].Reason, "initial_value_mismatch")

	// Event recorder should have captured TypeHeld.
	require.Len(t, rec.Events, 1)
	evt := rec.Events[0]
	assert.Equal(t, dhevents.TypeHeld, evt.Type)
	assert.Equal(t, "pur-hold-1", evt.PurchaseID)
	assert.Equal(t, "11112222", evt.CertNumber)
	assert.Equal(t, inventory.DHPushStatusHeld, evt.NewPushStatus)
	assert.Equal(t, 42, evt.DHCardID)
	assert.Contains(t, evt.Notes, "initial_value_mismatch")
	assert.Equal(t, dhevents.SourceDHPush, evt.Source)

	// No push should have occurred.
	assert.Empty(t, fieldsUpdater.Calls)
}

func TestDHPush_NilEventRecorderIsSafe(t *testing.T) {
	// Same hold scenario but without event recorder — should not panic.
	purchase := inventory.Purchase{
		ID:           "pur-hold-nil",
		CertNumber:   "33334444",
		CardName:     "Charizard",
		SetName:      "Base Set",
		BuyCostCents: 10000,
		ReviewedPriceCents: 3000,
		DHCardID:     42,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]inventory.Purchase, error) {
			return []inventory.Purchase{purchase}, nil
		},
	}
	holdSetter := &mockDHPushHoldSetter{}

	s := NewDHPushScheduler(
		lister, &mockDHPushStatusUpdater{}, &mockDHPushCertResolver{},
		&mockDHPushInventoryPusher{}, &mocks.MockDHFieldsUpdater{}, &mockDHPushCardIDSaver{},
		mocks.NewMockLogger(),
		DHPushConfig{Enabled: true, Interval: 1 * time.Hour},
		WithDHPushHoldSetter(holdSetter),
		// no WithDHPushEventRecorder
	)
	s.push(context.Background())

	require.Len(t, holdSetter.Calls, 1, "hold should still be set even without event recorder")
}

// Compile-time interface checks
var _ DHPushPendingLister = (*mockDHPushPendingLister)(nil)
var _ DHPushStatusUpdater = (*mockDHPushStatusUpdater)(nil)
var _ DHPushCertResolver = (*mockDHPushCertResolver)(nil)
var _ DHPushInventoryPusher = (*mockDHPushInventoryPusher)(nil)
var _ DHPushCardIDSaver = (*mockDHPushCardIDSaver)(nil)
var _ DHFieldsUpdater = (*mocks.MockDHFieldsUpdater)(nil)
var _ DHPushHoldSetter = (*mockDHPushHoldSetter)(nil)
