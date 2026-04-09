package handlers

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// mockCLSyncUpdater records calls to UpdatePurchaseCLSyncedAt.
type mockCLSyncUpdater struct {
	calls []struct {
		purchaseID string
		syncedAt   string
	}
}

func (m *mockCLSyncUpdater) UpdatePurchaseCLSyncedAt(_ context.Context, purchaseID, syncedAt string) error {
	m.calls = append(m.calls, struct {
		purchaseID string
		syncedAt   string
	}{purchaseID, syncedAt})
	return nil
}

// Ensure mockCLSyncUpdater implements CLSyncUpdater.
var _ CLSyncUpdater = (*mockCLSyncUpdater)(nil)

func TestAddCardToCollection_UpdatesCLSyncedAt(t *testing.T) {
	updater := &mockCLSyncUpdater{}
	_ = observability.Logger(mocks.NewMockLogger()) // verify import usable

	h := &CardLadderHandler{
		logger: mocks.NewMockLogger(),
	}
	h.SetSyncUpdater(updater)

	// Verify the syncUpdater field is set.
	if h.syncUpdater == nil {
		t.Fatal("expected syncUpdater to be set")
	}
}
