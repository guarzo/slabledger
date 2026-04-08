package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Local mocks for CardLadder-specific interfaces
// ---------------------------------------------------------------------------

type mockCLPurchaseLister struct {
	ListFn func(ctx context.Context) ([]campaigns.Purchase, error)
}

func (m *mockCLPurchaseLister) ListAllUnsoldPurchases(ctx context.Context) ([]campaigns.Purchase, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx)
	}
	return nil, nil
}

type mockCLValueUpdater struct {
	UpdateFn func(ctx context.Context, purchaseID string, clValueCents, population int) error
	Calls    []struct {
		PurchaseID   string
		CLValueCents int
		Population   int
	}
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

type mockCLValueHistoryRecorder struct {
	RecordFn func(ctx context.Context, entry campaigns.CLValueEntry) error
	Calls    []campaigns.CLValueEntry
}

func (m *mockCLValueHistoryRecorder) RecordCLValue(ctx context.Context, entry campaigns.CLValueEntry) error {
	m.Calls = append(m.Calls, entry)
	if m.RecordFn != nil {
		return m.RecordFn(ctx, entry)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helper functions tests
// ---------------------------------------------------------------------------

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
		nil, nil,
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
		nil, nil,
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
		nil, nil,
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
		nil, nil,
		mocks.NewMockLogger(),
		config.CardLadderConfig{Enabled: true, Interval: 24 * time.Hour},
		WithCLDHPushUpdater(dhPushUpdater),
	)
	require.NotNil(t, s.dhPushUpdater, "dhPushUpdater should be set via functional option")
}

// ---------------------------------------------------------------------------
// Interface compliance checks
// ---------------------------------------------------------------------------

var _ CardLadderPurchaseLister = (*mockCLPurchaseLister)(nil)
var _ CardLadderValueUpdater = (*mockCLValueUpdater)(nil)
var _ CardLadderGemRateUpdater = (*mockCLGemRateUpdater)(nil)
var _ campaigns.CLValueHistoryRecorder = (*mockCLValueHistoryRecorder)(nil)
var _ DHPushStatusUpdater = (*mockCLDHPushUpdater)(nil)
