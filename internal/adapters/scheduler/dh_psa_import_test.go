package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

type mockDHPushPSAImporter struct {
	ImportFn func(ctx context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error)
	Calls    [][]dh.PSAImportItem
}

func (m *mockDHPushPSAImporter) PSAImport(ctx context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
	m.Calls = append(m.Calls, items)
	if m.ImportFn != nil {
		return m.ImportFn(ctx, items)
	}
	return nil, nil
}

// newTestDHPushSchedulerWithPSAImport builds a scheduler with a PSA importer wired.
func newTestDHPushSchedulerWithPSAImport(
	lister *mockDHPushPendingLister,
	statusUpdater *mockDHPushStatusUpdater,
	certResolver *mockDHPushCertResolver,
	pusher *mockDHPushInventoryPusher,
	fieldsUpdater *mocks.MockDHFieldsUpdater,
	cardIDSaver *mockDHPushCardIDSaver,
	importer *mockDHPushPSAImporter,
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
		WithDHPushPSAImporter(importer),
	)
}

// TestDHPSAImport_NotFound_FallsBackToPSAImport verifies that when standard cert
// resolve returns not_found and a psa importer is wired, we attempt psa_import
// before giving up.
func TestDHPSAImport_NotFound_FallsBackToPSAImport(t *testing.T) {
	purchase := inventory.Purchase{
		ID:           "pur-jp",
		CertNumber:   "147746751",
		CardName:     "Sceptile McDonald's Promo",
		SetName:      "JAPANESE PROMO",
		CardNumber:   "046",
		CardYear:     "2004",
		BuyCostCents: 5000,
		DHPushStatus: inventory.DHPushStatusPending,
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
	pusher := &mockDHPushInventoryPusher{
		PushFn: func(_ context.Context, _ []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
			t.Fatal("PushInventory should not be called on psa_import success path")
			return nil, nil
		},
	}
	fieldsUpdater := &mocks.MockDHFieldsUpdater{}
	cardIDSaver := &mockDHPushCardIDSaver{}
	importer := &mockDHPushPSAImporter{
		ImportFn: func(_ context.Context, _ []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
			return &dh.PSAImportResponse{
				Results: []dh.PSAImportResult{{
					CertNumber:    "147746751",
					Resolution:    dh.PSAImportStatusUnmatchedCreated,
					DHCardID:      7001,
					DHInventoryID: 8001,
					Status:        dh.InventoryStatusInStock,
				}},
				Summary: dh.PSAImportSummary{UnmatchedCreated: 1},
			}, nil
		},
	}

	s := newTestDHPushSchedulerWithPSAImport(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver, importer)
	s.push(context.Background())

	require.Len(t, importer.Calls, 1, "psa_import should be called once")
	item := importer.Calls[0][0]
	assert.Equal(t, "147746751", item.CertNumber)
	assert.Equal(t, 5000, item.CostBasisCents)
	require.NotNil(t, item.Overrides)
	assert.Equal(t, "Sceptile McDonald's Promo", item.Overrides.Name)
	assert.Equal(t, "JAPANESE PROMO", item.Overrides.SetName)
	assert.Equal(t, "046", item.Overrides.CardNumber)
	assert.Equal(t, "2004", item.Overrides.Year)
	assert.Equal(t, "japanese", item.Overrides.Language, "language should be inferred from set_name")

	require.Len(t, fieldsUpdater.Calls, 1, "DH fields should be persisted after psa_import")
	assert.Equal(t, 7001, fieldsUpdater.Calls[0].CardID)
	assert.Equal(t, 8001, fieldsUpdater.Calls[0].InventoryID)
	assert.Equal(t, dh.CertStatusMatched, fieldsUpdater.Calls[0].CertStatus)
	assert.Equal(t, inventory.DHStatus(dh.InventoryStatusInStock), fieldsUpdater.Calls[0].DHStatus)

	require.Len(t, statusUpdater.Calls, 1)
	assert.Equal(t, inventory.DHPushStatusMatched, statusUpdater.Calls[0].Status)
}

// TestDHPSAImport_PartnerCardError_MarksUnmatched verifies that a partner_card_error
// result still lands the purchase as unmatched (not silently lost).
func TestDHPSAImport_PartnerCardError_MarksUnmatched(t *testing.T) {
	purchase := inventory.Purchase{
		ID:           "pur-err",
		CertNumber:   "12345678",
		CardName:     "Unknown",
		SetName:      "Mystery Set",
		DHPushStatus: inventory.DHPushStatusPending,
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
	importer := &mockDHPushPSAImporter{
		ImportFn: func(_ context.Context, _ []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
			return &dh.PSAImportResponse{
				Results: []dh.PSAImportResult{{
					CertNumber: "12345678",
					Resolution: dh.PSAImportStatusPartnerCardError,
					Error:      "invalid language enum",
				}},
				Summary: dh.PSAImportSummary{PartnerCardError: 1},
			}, nil
		},
	}

	s := newTestDHPushSchedulerWithPSAImport(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver, importer)
	s.push(context.Background())

	require.Len(t, importer.Calls, 1)
	require.Len(t, statusUpdater.Calls, 1)
	assert.Equal(t, inventory.DHPushStatusUnmatched, statusUpdater.Calls[0].Status)
	assert.Empty(t, fieldsUpdater.Calls, "fields should NOT be updated when psa_import fails")
}

// TestDHPSAImport_APIError_LeavesAsPending verifies that a transient error from
// DH's psa_import endpoint leaves the purchase pending for retry.
func TestDHPSAImport_APIError_LeavesAsPending(t *testing.T) {
	purchase := inventory.Purchase{
		ID:           "pur-retry",
		CertNumber:   "99999999",
		DHPushStatus: inventory.DHPushStatusPending,
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
	importer := &mockDHPushPSAImporter{
		ImportFn: func(_ context.Context, _ []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
			return nil, errors.New("dh 503")
		},
	}

	s := newTestDHPushSchedulerWithPSAImport(lister, statusUpdater, certResolver, pusher, fieldsUpdater, cardIDSaver, importer)
	s.push(context.Background())

	require.Len(t, importer.Calls, 1)
	assert.Empty(t, statusUpdater.Calls, "status should stay pending on transient error")
	assert.Empty(t, fieldsUpdater.Calls)
}

// TestDHPSAImport_NoImporter_FallsBackToMarkUnmatched verifies the legacy
// behavior is preserved when no importer is wired.
func TestDHPSAImport_NoImporter_FallsBackToMarkUnmatched(t *testing.T) {
	purchase := inventory.Purchase{
		ID:           "pur-legacy",
		CertNumber:   "11111111",
		DHPushStatus: inventory.DHPushStatusPending,
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
	assert.Equal(t, inventory.DHPushStatusUnmatched, statusUpdater.Calls[0].Status)
}

func TestBuildPSAImportItem(t *testing.T) {
	tests := []struct {
		name         string
		purchase     inventory.Purchase
		wantLanguage string
	}{
		{
			name: "japanese promo",
			purchase: inventory.Purchase{
				CertNumber: "1", CardName: "Sceptile", SetName: "JAPANESE PROMO",
				CardNumber: "046", CardYear: "2004", BuyCostCents: 5000,
			},
			wantLanguage: "japanese",
		},
		{
			name: "english swsh promo",
			purchase: inventory.Purchase{
				CertNumber: "2", CardName: "Gengar", SetName: "SWSH BLACK STAR PROMO",
				CardNumber: "052", CardYear: "2020", BuyCostCents: 3000,
			},
			wantLanguage: "", // omitted for english (DH default)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := buildPSAImportItem(tt.purchase)
			assert.Equal(t, tt.purchase.CertNumber, item.CertNumber)
			assert.Equal(t, tt.purchase.BuyCostCents, item.CostBasisCents)
			assert.Equal(t, dh.InventoryStatusInStock, item.Status)
			require.NotNil(t, item.Overrides)
			assert.Equal(t, tt.purchase.CardName, item.Overrides.Name)
			assert.Equal(t, tt.purchase.SetName, item.Overrides.SetName)
			assert.Equal(t, tt.purchase.CardNumber, item.Overrides.CardNumber)
			assert.Equal(t, tt.purchase.CardYear, item.Overrides.Year)
			assert.Equal(t, tt.wantLanguage, item.Overrides.Language)
		})
	}
}
