package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Local mocks for DHPush-specific interfaces
// ---------------------------------------------------------------------------

type mockDHPushPendingLister struct {
	ListFn func(ctx context.Context, status string, limit int) ([]campaigns.Purchase, error)
}

func (m *mockDHPushPendingLister) GetPurchasesByDHPushStatus(ctx context.Context, status string, limit int) ([]campaigns.Purchase, error) {
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
		ListFn: func(_ context.Context, _ string, _ int) ([]campaigns.Purchase, error) {
			return []campaigns.Purchase{}, nil
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
		ListFn: func(_ context.Context, _ string, _ int) ([]campaigns.Purchase, error) {
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
	purchase := campaigns.Purchase{
		ID:         "pur-1",
		CertNumber: "", // no cert
		CardName:   "Charizard",
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]campaigns.Purchase, error) {
			return []campaigns.Purchase{purchase}, nil
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
	assert.Equal(t, campaigns.DHPushStatusUnmatched, statusUpdater.Calls[0].Status)
	assert.Empty(t, fieldsUpdater.Calls)
}

func TestDHPush_NoCLValue_LeavesAsPending(t *testing.T) {
	purchase := campaigns.Purchase{
		ID:           "pur-nocl",
		CertNumber:   "12345678",
		CardName:     "Pikachu",
		SetName:      "Base Set",
		CLValueCents: 0, // no CL value yet
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]campaigns.Purchase, error) {
			return []campaigns.Purchase{purchase}, nil
		},
	}
	resolverCalled := false
	certResolver := &mockDHPushCertResolver{
		ResolveFn: func(_ context.Context, _ dh.CertResolveRequest) (*dh.CertResolution, error) {
			resolverCalled = true
			return &dh.CertResolution{Status: dh.CertStatusMatched, DHCardID: 42}, nil
		},
	}
	statusUpdater := &mockDHPushStatusUpdater{}
	pusher := &mockDHPushInventoryPusher{}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}

	s := newTestDHPushScheduler(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver)
	s.push(context.Background())

	assert.False(t, resolverCalled, "cert resolver should not be called when CL value is 0")
	assert.Empty(t, statusUpdater.Calls, "purchase should stay pending when CL value is 0")
	assert.Empty(t, fieldsUpdater.Calls)
}

func TestDHPush_CertResolveError_LeavesAsPending(t *testing.T) {
	purchase := campaigns.Purchase{
		ID:           "pur-2",
		CertNumber:   "12345678",
		CardName:     "Pikachu",
		SetName:      "Base Set",
		CLValueCents: 5000,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]campaigns.Purchase, error) {
			return []campaigns.Purchase{purchase}, nil
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
	purchase := campaigns.Purchase{
		ID:           "pur-3",
		CertNumber:   "99999999",
		CardName:     "Machamp",
		SetName:      "Base Set",
		CLValueCents: 5000,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]campaigns.Purchase, error) {
			return []campaigns.Purchase{purchase}, nil
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
	assert.Equal(t, campaigns.DHPushStatusUnmatched, statusUpdater.Calls[0].Status)
}

func TestDHPush_InventoryPushError_LeavesAsPending(t *testing.T) {
	purchase := campaigns.Purchase{
		ID:           "pur-4",
		CertNumber:   "11111111",
		CardName:     "Blastoise",
		CLValueCents: 5000,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]campaigns.Purchase, error) {
			return []campaigns.Purchase{purchase}, nil
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
	purchase := campaigns.Purchase{
		ID:           "pur-5",
		CertNumber:   "22222222",
		CardName:     "Umbreon ex",
		SetName:      "SV Promo",
		CardNumber:   "176",
		GradeValue:   10,
		BuyCostCents: 6000,
		CLValueCents: 8000,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]campaigns.Purchase, error) {
			return []campaigns.Purchase{purchase}, nil
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
			require.NotNil(t, items[0].MarketValueCents)
			assert.Equal(t, 8000, *items[0].MarketValueCents)
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
	assert.Equal(t, campaigns.DHPushStatusMatched, statusUpdater.Calls[0].Status)
}

func TestDHPush_AlreadyMapped_SkipsCertResolve(t *testing.T) {
	purchase := campaigns.Purchase{
		ID:           "pur-6",
		CertNumber:   "33333333",
		CardName:     "Charizard",
		SetName:      "Base Set",
		CardNumber:   "4",
		CLValueCents: 5000,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]campaigns.Purchase, error) {
			return []campaigns.Purchase{purchase}, nil
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
	key := campaigns.DHCardKey(purchase.CardName, purchase.SetName, purchase.CardNumber)
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
	purchase := campaigns.Purchase{
		ID:           "pur-7",
		CertNumber:   "44444444",
		CardName:     "Venusaur",
		CLValueCents: 5000,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]campaigns.Purchase, error) {
			return []campaigns.Purchase{purchase}, nil
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
	purchase := campaigns.Purchase{
		ID:           "pur-8",
		CertNumber:   "55555555",
		CardName:     "Gyarados",
		CLValueCents: 5000,
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]campaigns.Purchase, error) {
			return []campaigns.Purchase{purchase}, nil
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
	purchases := []campaigns.Purchase{
		{ID: "pur-a", CertNumber: "11111111", CardName: "Pikachu", CLValueCents: 5000},
		{ID: "pur-b", CertNumber: "22222222", CardName: "Raichu", CLValueCents: 5000},
		{ID: "pur-c", CertNumber: "", CardName: "Gengar", CLValueCents: 5000}, // no cert → unmatched
	}
	lister := &mockDHPushPendingLister{
		ListFn: func(_ context.Context, _ string, _ int) ([]campaigns.Purchase, error) {
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
		case campaigns.DHPushStatusUnmatched:
			unmatchedCalls = append(unmatchedCalls, c.ID)
		case campaigns.DHPushStatusMatched:
			matchedCalls = append(matchedCalls, c.ID)
		}
	}
	assert.ElementsMatch(t, []string{"pur-c"}, unmatchedCalls)
	assert.ElementsMatch(t, []string{"pur-a", "pur-b"}, matchedCalls)
}

// Compile-time interface checks
var _ DHPushPendingLister = (*mockDHPushPendingLister)(nil)
var _ DHPushStatusUpdater = (*mockDHPushStatusUpdater)(nil)
var _ DHPushCertResolver = (*mockDHPushCertResolver)(nil)
var _ DHPushInventoryPusher = (*mockDHPushInventoryPusher)(nil)
var _ DHPushCardIDSaver = (*mockDHPushCardIDSaver)(nil)
var _ DHFieldsUpdater = (*mocks.MockDHFieldsUpdater)(nil)
