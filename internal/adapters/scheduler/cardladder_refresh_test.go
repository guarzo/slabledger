package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Local mocks for CardLadder-specific interfaces
// ---------------------------------------------------------------------------

type mockCLPurchaseLister struct {
	ListFn func(ctx context.Context) ([]inventory.Purchase, error)
}

func (m *mockCLPurchaseLister) ListAllUnsoldPurchases(ctx context.Context) ([]inventory.Purchase, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx)
	}
	return nil, nil
}

type clErrorCall struct {
	PurchaseID string
	Reason     string
	ReasonAt   string
}

type mockCLValueUpdater struct {
	UpdateFn      func(ctx context.Context, purchaseID string, clValueCents, population int) error
	UpdateErrorFn func(ctx context.Context, purchaseID, reason, reasonAt string) error
	Calls         []struct {
		PurchaseID   string
		CLValueCents int
		Population   int
	}
	ErrorCalls []clErrorCall
}

func (m *mockCLValueUpdater) UpdatePurchaseCLValue(ctx context.Context, purchaseID string, clValueCents, population int) error {
	m.Calls = append(m.Calls, struct {
		PurchaseID   string
		CLValueCents int
		Population   int
	}{purchaseID, clValueCents, population})
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, purchaseID, clValueCents, population)
	}
	return nil
}

func (m *mockCLValueUpdater) UpdatePurchaseCLError(ctx context.Context, purchaseID, reason, reasonAt string) error {
	m.ErrorCalls = append(m.ErrorCalls, clErrorCall{PurchaseID: purchaseID, Reason: reason, ReasonAt: reasonAt})
	if m.UpdateErrorFn != nil {
		return m.UpdateErrorFn(ctx, purchaseID, reason, reasonAt)
	}
	return nil
}

type mockCLGemRateUpdater struct {
	UpdateGemRateFn                func(ctx context.Context, purchaseID, gemRateID string) error
	UpdatePSASpecFn                func(ctx context.Context, purchaseID string, psaSpecID int) error
	UpdatePurchaseCLCardMetadataFn func(ctx context.Context, id, player, variation, category string) error
	GemRateCalls                   []struct{ PurchaseID, GemRateID string }
	PSASpecCalls                   []struct {
		PurchaseID string
		PSASpecID  int
	}
}

func (m *mockCLGemRateUpdater) UpdatePurchaseGemRateID(ctx context.Context, purchaseID, gemRateID string) error {
	m.GemRateCalls = append(m.GemRateCalls, struct{ PurchaseID, GemRateID string }{purchaseID, gemRateID})
	if m.UpdateGemRateFn != nil {
		return m.UpdateGemRateFn(ctx, purchaseID, gemRateID)
	}
	return nil
}

func (m *mockCLGemRateUpdater) UpdatePurchasePSASpecID(ctx context.Context, purchaseID string, psaSpecID int) error {
	m.PSASpecCalls = append(m.PSASpecCalls, struct {
		PurchaseID string
		PSASpecID  int
	}{purchaseID, psaSpecID})
	if m.UpdatePSASpecFn != nil {
		return m.UpdatePSASpecFn(ctx, purchaseID, psaSpecID)
	}
	return nil
}

func (m *mockCLGemRateUpdater) UpdatePurchaseCLCardMetadata(ctx context.Context, id, player, variation, category string) error {
	if m.UpdatePurchaseCLCardMetadataFn != nil {
		return m.UpdatePurchaseCLCardMetadataFn(ctx, id, player, variation, category)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helper functions tests
// ---------------------------------------------------------------------------

func TestShouldReenrollForCLChange(t *testing.T) {
	received := "2026-04-01"
	cases := []struct {
		name     string
		purchase inventory.Purchase
		want     bool
	}{
		{
			name:     "already-pushed row always re-enrolls",
			purchase: inventory.Purchase{ID: "p1", DHInventoryID: 42, ReceivedAt: &received, DHPushStatus: inventory.DHPushStatusMatched},
			want:     true,
		},
		{
			name:     "received unmatched row re-enrolls (NEW behavior)",
			purchase: inventory.Purchase{ID: "p2", DHInventoryID: 0, ReceivedAt: &received, DHPushStatus: inventory.DHPushStatusUnmatched},
			want:     true,
		},
		{
			name:     "received empty-status row re-enrolls (NEW behavior)",
			purchase: inventory.Purchase{ID: "p3", DHInventoryID: 0, ReceivedAt: &received, DHPushStatus: ""},
			want:     true,
		},
		{
			name:     "unreceived unmatched row does not re-enroll",
			purchase: inventory.Purchase{ID: "p4", DHInventoryID: 0, ReceivedAt: nil, DHPushStatus: inventory.DHPushStatusUnmatched},
			want:     false,
		},
		{
			name:     "held unmatched row does not re-enroll",
			purchase: inventory.Purchase{ID: "p5", DHInventoryID: 0, ReceivedAt: &received, DHPushStatus: inventory.DHPushStatusHeld},
			want:     false,
		},
		{
			name:     "dismissed received row does not re-enroll",
			purchase: inventory.Purchase{ID: "p6", DHInventoryID: 0, ReceivedAt: &received, DHPushStatus: inventory.DHPushStatusDismissed},
			want:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldReenrollForCLChange(&tc.purchase)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestExtractGradeValue(t *testing.T) {
	tests := []struct {
		name      string
		condition string
		want      float64
	}{
		{"PSA 10", "PSA 10", 10},
		{"PSA 9.5", "PSA 9.5", 9.5},
		{"PSA 9", "PSA 9", 9},
		{"g10", "g10", 10},
		{"just digits", "10", 10},
		{"empty", "", 0},
		{"no digits", "graded", 0},
		{"decimal only", "PSA 8.5", 8.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractGradeValue(tt.condition)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Constructor / config tests
// ---------------------------------------------------------------------------

func TestNewCardLadderRefreshScheduler_DefaultInterval(t *testing.T) {
	s := NewCardLadderRefreshScheduler(
		nil, nil,
		&mockCLPurchaseLister{},
		&mockCLValueUpdater{},
		&mockCLGemRateUpdater{},
		nil,
		mocks.NewMockLogger(),
		config.CardLadderConfig{
			Enabled:  true,
			Interval: 0, // zero → should default to 24h
		},
	)
	require.Equal(t, 24*time.Hour, s.config.Interval,
		"zero interval should default to 24h")
}

func TestNewCardLadderRefreshScheduler_ExplicitInterval(t *testing.T) {
	s := NewCardLadderRefreshScheduler(
		nil, nil,
		&mockCLPurchaseLister{},
		&mockCLValueUpdater{},
		&mockCLGemRateUpdater{},
		nil,
		mocks.NewMockLogger(),
		config.CardLadderConfig{
			Enabled:  true,
			Interval: 12 * time.Hour,
		},
	)
	require.Equal(t, 12*time.Hour, s.config.Interval,
		"explicit interval should be preserved")
}

// ---------------------------------------------------------------------------
// Start / disabled tests
// ---------------------------------------------------------------------------

func TestCardLadderRefreshScheduler_Disabled(t *testing.T) {
	s := NewCardLadderRefreshScheduler(
		nil, nil,
		&mockCLPurchaseLister{},
		&mockCLValueUpdater{},
		&mockCLGemRateUpdater{},
		nil,
		mocks.NewMockLogger(),
		config.CardLadderConfig{Enabled: false, Interval: 24 * time.Hour},
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

// ---------------------------------------------------------------------------
// DH re-enrollment tests
// ---------------------------------------------------------------------------

type mockCLDHPushUpdater struct {
	Calls []struct{ ID, Status string }
}

func (m *mockCLDHPushUpdater) UpdatePurchaseDHPushStatus(_ context.Context, id string, status string) error {
	m.Calls = append(m.Calls, struct{ ID, Status string }{id, status})
	return nil
}

func TestWithCLDHPushUpdater_ReEnrollsOnValueChange(t *testing.T) {
	dhPushUpdater := &mockCLDHPushUpdater{}
	s := NewCardLadderRefreshScheduler(
		nil, nil,
		&mockCLPurchaseLister{},
		&mockCLValueUpdater{},
		&mockCLGemRateUpdater{},
		nil,
		mocks.NewMockLogger(),
		config.CardLadderConfig{Enabled: true, Interval: 24 * time.Hour},
		WithCLDHPushUpdater(dhPushUpdater),
	)
	require.NotNil(t, s.dhPushUpdater, "dhPushUpdater should be set via functional option")
}

// ---------------------------------------------------------------------------
// Interface compliance checks
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// SetClient / getClient tests
// ---------------------------------------------------------------------------

func TestCardLadderRefreshScheduler_SetClient(t *testing.T) {
	s := NewCardLadderRefreshScheduler(
		nil, nil,
		&mockCLPurchaseLister{},
		&mockCLValueUpdater{},
		&mockCLGemRateUpdater{},
		nil,
		mocks.NewMockLogger(),
		config.CardLadderConfig{Enabled: true, Interval: 24 * time.Hour},
	)

	// Initially nil
	assert.Nil(t, s.getClient(), "client should be nil initially")

	// Set a client
	client := &cardladder.Client{}
	s.SetClient(client)
	assert.Equal(t, client, s.getClient(), "getClient should return the set client")

	// Replace with another
	client2 := &cardladder.Client{}
	s.SetClient(client2)
	assert.Equal(t, client2, s.getClient(), "getClient should return the new client")
}

var _ CardLadderPurchaseLister = (*mockCLPurchaseLister)(nil)
var _ CardLadderValueUpdater = (*mockCLValueUpdater)(nil)
var _ CardLadderGemRateUpdater = (*mockCLGemRateUpdater)(nil)
var _ DHPushStatusUpdater = (*mockCLDHPushUpdater)(nil)

// ---------------------------------------------------------------------------
// Phase function tests: pushNewCards, removeSoldCards, refreshSalesComps
// ---------------------------------------------------------------------------

// TestPushNewCards_FilterLogic tests the filtering logic of pushNewCards
// via the filterUnmappedCerts helper. We verify it correctly identifies
// which purchases to push based on: has a cert number + not already mapped.
func TestPushNewCards_FilterLogic(t *testing.T) {
	tests := []struct {
		name         string
		purchases    []inventory.Purchase
		existingMaps []sqlite.CLCardMapping
		expectCount  int // count of purchases that should be considered for push
	}{
		{
			name: "pushes unmapped purchases with certs",
			purchases: []inventory.Purchase{
				{CertNumber: "123456"},
				{CertNumber: "789012"},
			},
			existingMaps: []sqlite.CLCardMapping{},
			expectCount:  2,
		},
		{
			name: "skips already-mapped certs",
			purchases: []inventory.Purchase{
				{CertNumber: "123456"},
				{CertNumber: "789012"},
			},
			existingMaps: []sqlite.CLCardMapping{
				{SlabSerial: "123456"},
			},
			expectCount: 1,
		},
		{
			name: "skips purchases without cert numbers",
			purchases: []inventory.Purchase{
				{CertNumber: ""},
				{CertNumber: "789012"},
			},
			existingMaps: []sqlite.CLCardMapping{},
			expectCount:  1,
		},
		{
			name: "skips all when all are mapped",
			purchases: []inventory.Purchase{
				{CertNumber: "123456"},
				{CertNumber: "789012"},
			},
			existingMaps: []sqlite.CLCardMapping{
				{SlabSerial: "123456"},
				{SlabSerial: "789012"},
			},
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterUnmappedCerts(tt.purchases, tt.existingMaps)
			assert.Equal(t, tt.expectCount, len(got), "filter count mismatch")
		})
	}
}

// TestRemoveSoldCards_IdentifySold tests identifying sold cards via the
// identifySoldMappings helper. Sold cards are those in existingMaps but NOT
// in unsoldPurchases.
func TestRemoveSoldCards_IdentifySold(t *testing.T) {
	tests := []struct {
		name         string
		unsoldPurch  []inventory.Purchase
		existingMaps []sqlite.CLCardMapping
		expectSold   int // count of mappings that should be removed
	}{
		{
			name: "identifies sold when cert not in unsold set",
			unsoldPurch: []inventory.Purchase{
				{CertNumber: "111111"},
				{CertNumber: "222222"},
			},
			existingMaps: []sqlite.CLCardMapping{
				{SlabSerial: "111111", CLCollectionCardID: "card_111"},
				{SlabSerial: "222222", CLCollectionCardID: "card_222"},
				{SlabSerial: "333333", CLCollectionCardID: "card_333"},
			},
			expectSold: 1,
		},
		{
			name: "all still unsold",
			unsoldPurch: []inventory.Purchase{
				{CertNumber: "111111"},
				{CertNumber: "222222"},
				{CertNumber: "333333"},
			},
			existingMaps: []sqlite.CLCardMapping{
				{SlabSerial: "111111", CLCollectionCardID: "card_111"},
				{SlabSerial: "222222", CLCollectionCardID: "card_222"},
				{SlabSerial: "333333", CLCollectionCardID: "card_333"},
			},
			expectSold: 0,
		},
		{
			name:        "all sold",
			unsoldPurch: []inventory.Purchase{},
			existingMaps: []sqlite.CLCardMapping{
				{SlabSerial: "111111", CLCollectionCardID: "card_111"},
				{SlabSerial: "222222", CLCollectionCardID: "card_222"},
			},
			expectSold: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sold := identifySoldMappings(tt.unsoldPurch, tt.existingMaps)
			assert.Equal(t, tt.expectSold, len(sold), "sold count mismatch")
		})
	}
}

// TestRefreshSalesComps_Dedup tests deduplication logic via the
// dedupGemRateConditionPairs helper. We should fetch comps only once per
// unique (gemRateID, condition) pair.
func TestRefreshSalesComps_Dedup(t *testing.T) {
	tests := []struct {
		name        string
		mappings    []sqlite.CLCardMapping
		expectFetch int // count of unique (gemRateID, condition) pairs
	}{
		{
			name: "fetches each unique pair once",
			mappings: []sqlite.CLCardMapping{
				{CLGemRateID: "gem_123", CLCondition: "PSA 10"},
				{CLGemRateID: "gem_456", CLCondition: "PSA 9"},
			},
			expectFetch: 2,
		},
		{
			name: "deduplicates identical pairs",
			mappings: []sqlite.CLCardMapping{
				{CLGemRateID: "gem_123", CLCondition: "PSA 10"},
				{CLGemRateID: "gem_123", CLCondition: "PSA 10"},
				{CLGemRateID: "gem_456", CLCondition: "PSA 9"},
			},
			expectFetch: 2,
		},
		{
			name: "skips missing gemRateID",
			mappings: []sqlite.CLCardMapping{
				{CLGemRateID: "", CLCondition: "PSA 10"},
				{CLGemRateID: "gem_456", CLCondition: "PSA 9"},
			},
			expectFetch: 1,
		},
		{
			name: "skips missing condition",
			mappings: []sqlite.CLCardMapping{
				{CLGemRateID: "gem_123", CLCondition: ""},
				{CLGemRateID: "gem_456", CLCondition: "PSA 9"},
			},
			expectFetch: 1,
		},
		{
			name: "all skipped",
			mappings: []sqlite.CLCardMapping{
				{CLGemRateID: "", CLCondition: ""},
				{CLGemRateID: "", CLCondition: "PSA 9"},
			},
			expectFetch: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pairs := dedupGemRateConditionPairs(tt.mappings)
			assert.Equal(t, tt.expectFetch, len(pairs), "fetch count mismatch")
		})
	}
}
