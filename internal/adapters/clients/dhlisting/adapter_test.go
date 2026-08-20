package dhlisting_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	adapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// errPSARateLimit matches dh.IsPSARateLimitMessage, which drives rotation in
// dh.UpdateInventoryWithRotation.
var errPSARateLimit = errors.New("PSA API rate limit exceeded")

// --- stubs ---
//
// Both adapter constructors infer their rotator by type-asserting the client to
// dh.PSAKeyRotator. Each stub therefore comes in two shapes: a plain one that
// does NOT satisfy the interface (exercising the nil-rotator branches) and a
// rotating one that does.

type stubPSAImportClient struct {
	resp  *dh.PSAImportResponse
	err   error
	calls [][]dh.PSAImportItem
}

func (s *stubPSAImportClient) PSAImport(_ context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
	s.calls = append(s.calls, items)
	return s.resp, s.err
}

type rotatingPSAImportClient struct {
	stubPSAImportClient
	rotateResult bool
	rotateCalls  int
	resetCalls   int
}

func (r *rotatingPSAImportClient) RotatePSAKey() bool { r.rotateCalls++; return r.rotateResult }
func (r *rotatingPSAImportClient) ResetPSAKeyRotation() {
	r.resetCalls++
}

type stubInventoryClient struct {
	updateFn func(ctx context.Context, id int, update dh.InventoryUpdate) (*dh.InventoryResult, error)
	syncFn   func(ctx context.Context, id int, channels []string) (*dh.ChannelSyncResponse, error)

	updateIDs []int
	updates   []dh.InventoryUpdate
	syncIDs   []int
	syncArgs  [][]string

	recordSaleFn func(ctx context.Context, inventoryID int, idempotencyKey string, req dh.InventorySaleRequest) (*dh.InventorySaleResponse, error)
	voidSaleFn   func(ctx context.Context, dhSaleID string, req dh.VoidSaleRequest) (*dh.VoidSaleResponse, error)

	recordSaleCalls []recordSaleCall
	voidSaleCalls   []voidSaleCall
}

func (s *stubInventoryClient) UpdateInventory(ctx context.Context, id int, update dh.InventoryUpdate) (*dh.InventoryResult, error) {
	s.updateIDs = append(s.updateIDs, id)
	s.updates = append(s.updates, update)
	if s.updateFn != nil {
		return s.updateFn(ctx, id, update)
	}
	return &dh.InventoryResult{}, nil
}

func (s *stubInventoryClient) SyncChannels(ctx context.Context, id int, channels []string) (*dh.ChannelSyncResponse, error) {
	s.syncIDs = append(s.syncIDs, id)
	s.syncArgs = append(s.syncArgs, channels)
	if s.syncFn != nil {
		return s.syncFn(ctx, id, channels)
	}
	return &dh.ChannelSyncResponse{}, nil
}

func (s *stubInventoryClient) RecordInventorySale(ctx context.Context, inventoryID int, idempotencyKey string, req dh.InventorySaleRequest) (*dh.InventorySaleResponse, error) {
	s.recordSaleCalls = append(s.recordSaleCalls, recordSaleCall{inventoryID, idempotencyKey, req})
	if s.recordSaleFn != nil {
		return s.recordSaleFn(ctx, inventoryID, idempotencyKey, req)
	}
	return &dh.InventorySaleResponse{}, nil
}

func (s *stubInventoryClient) VoidInventorySale(ctx context.Context, dhSaleID string, req dh.VoidSaleRequest) (*dh.VoidSaleResponse, error) {
	s.voidSaleCalls = append(s.voidSaleCalls, voidSaleCall{dhSaleID, req})
	if s.voidSaleFn != nil {
		return s.voidSaleFn(ctx, dhSaleID, req)
	}
	return &dh.VoidSaleResponse{}, nil
}

type recordSaleCall struct {
	inventoryID    int
	idempotencyKey string
	req            dh.InventorySaleRequest
}

type voidSaleCall struct {
	dhSaleID string
	req      dh.VoidSaleRequest
}

type rotatingInventoryClient struct {
	stubInventoryClient
	// rotateBudget bounds the number of successful rotations. dh's rotation loop
	// retries for as long as RotatePSAKey keeps returning true, so an unbounded
	// stub would turn a regression that drops the "listed only" guard into a hung
	// test instead of a failing one.
	rotateBudget int
	rotateCalls  int
	resetCalls   int
}

func (r *rotatingInventoryClient) RotatePSAKey() bool {
	r.rotateCalls++
	return r.rotateCalls <= r.rotateBudget
}

func (r *rotatingInventoryClient) ResetPSAKeyRotation() {
	r.resetCalls++
}

type stubSnapshotClient struct {
	pages   []*dh.InventoryListResponse
	err     error
	errPage int // 1-based page that returns err; 0 means never
	filters []dh.InventoryFilters
}

func (s *stubSnapshotClient) ListInventory(_ context.Context, f dh.InventoryFilters) (*dh.InventoryListResponse, error) {
	s.filters = append(s.filters, f)
	if s.err != nil && f.Page == s.errPage {
		return nil, s.err
	}
	if f.Page-1 < len(s.pages) {
		return s.pages[f.Page-1], nil
	}
	return &dh.InventoryListResponse{}, nil
}

// fullPage builds a page of exactly snapshotPageSize items so FetchAllInventory
// keeps paginating; ids are offset so pages don't collide.
func fullPage(startID int) *dh.InventoryListResponse {
	items := make([]dh.InventoryListItem, 100)
	for i := range items {
		items[i] = dh.InventoryListItem{DHInventoryID: startID + i, Status: "listed"}
	}
	return &dh.InventoryListResponse{Items: items}
}

// --- PSAImporterAdapter.PSAImport ---

func TestPSAImporterAdapter_PSAImport_TranslatesItemsAndResults(t *testing.T) {
	client := &stubPSAImportClient{resp: &dh.PSAImportResponse{
		Success: true,
		Results: []dh.PSAImportResult{
			{
				CertNumber:    "12345678",
				Resolution:    dhlisting.PSAImportStatusMatched,
				DHCardID:      42,
				DHInventoryID: 99,
				Status:        inventory.DHStatusInStock,
			},
			{
				CertNumber:  "87654321",
				Resolution:  dhlisting.PSAImportStatusPSAError,
				Error:       "Daily PSA API limit reached",
				RateLimited: true,
			},
		},
	}}

	results, err := adapter.NewPSAImporterAdapter(client).PSAImport(context.Background(), []dhlisting.DHPSAImportItem{{
		CertNumber:     "12345678",
		CostBasisCents: 25_00,
		CardName:       "Charizard",
		SetName:        "Base Set",
		CardNumber:     "4",
		Year:           "1999",
		Language:       "en",
	}})
	require.NoError(t, err)

	// Request translation: overrides carry the catalog hints, and every item is
	// forced to in_stock regardless of what the domain passed.
	require.Len(t, client.calls, 1)
	require.Equal(t, []dh.PSAImportItem{{
		CertNumber:     "12345678",
		CostBasisCents: 25_00,
		Status:         dh.InventoryStatusInStock,
		Overrides: &dh.PSAImportOverrides{
			Name:       "Charizard",
			SetName:    "Base Set",
			CardNumber: "4",
			Year:       "1999",
			Language:   "en",
		},
	}}, client.calls[0])

	// Response translation, including the RateLimited flag the scheduler
	// rotates PSA keys on.
	require.Equal(t, []dhlisting.DHPSAImportResult{
		{
			CertNumber:    "12345678",
			Resolution:    dhlisting.PSAImportStatusMatched,
			DHCardID:      42,
			DHInventoryID: 99,
			DHStatus:      inventory.DHStatusInStock,
		},
		{
			CertNumber:  "87654321",
			Resolution:  dhlisting.PSAImportStatusPSAError,
			Error:       "Daily PSA API limit reached",
			RateLimited: true,
		},
	}, results)
}

func TestPSAImporterAdapter_PSAImport_Errors(t *testing.T) {
	transportErr := errors.New("connection reset")

	tests := []struct {
		name    string
		resp    *dh.PSAImportResponse
		err     error
		wantErr string
	}{
		{
			name:    "batch rejected with reason",
			resp:    &dh.PSAImportResponse{Success: false, Error: "vendor profile missing"},
			wantErr: "psa_import batch rejected: vendor profile missing",
		},
		{
			name:    "batch rejected without reason",
			resp:    &dh.PSAImportResponse{Success: false},
			wantErr: "psa_import batch rejected (no reason given)",
		},
		{
			name:    "transport error propagated",
			err:     transportErr,
			wantErr: transportErr.Error(),
		},
		{
			// A rejected batch may still carry per-cert results; the adapter must
			// surface the batch error rather than the (meaningless) results.
			name: "rejection wins over populated results",
			resp: &dh.PSAImportResponse{
				Success: false,
				Error:   "batch exceeds 50 items",
				Results: []dh.PSAImportResult{{CertNumber: "1"}},
			},
			wantErr: "psa_import batch rejected: batch exceeds 50 items",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := adapter.NewPSAImporterAdapter(&stubPSAImportClient{resp: tt.resp, err: tt.err})

			results, err := a.PSAImport(context.Background(), []dhlisting.DHPSAImportItem{{CertNumber: "1"}})

			require.Error(t, err)
			require.EqualError(t, err, tt.wantErr)
			require.Nil(t, results)
		})
	}
}

func TestPSAImporterAdapter_PSAImport_EmptyItems(t *testing.T) {
	client := &stubPSAImportClient{resp: &dh.PSAImportResponse{Success: true}}

	results, err := adapter.NewPSAImporterAdapter(client).PSAImport(context.Background(), nil)

	require.NoError(t, err)
	require.Empty(t, results)
	require.Len(t, client.calls, 1)
	require.Empty(t, client.calls[0])
}

// --- rotation delegation ---

func TestPSAImporterAdapter_RotationDelegation(t *testing.T) {
	t.Run("delegates when client is a rotator", func(t *testing.T) {
		client := &rotatingPSAImportClient{rotateResult: true}
		a := adapter.NewPSAImporterAdapter(client)

		require.True(t, a.RotatePSAKey())
		a.ResetPSAKeyRotation()

		require.Equal(t, 1, client.rotateCalls)
		require.Equal(t, 1, client.resetCalls)
	})

	t.Run("reports exhausted when client is not a rotator", func(t *testing.T) {
		a := adapter.NewPSAImporterAdapter(&stubPSAImportClient{})

		require.False(t, a.RotatePSAKey())
		require.NotPanics(t, a.ResetPSAKeyRotation)
	})

	t.Run("propagates a false rotation result", func(t *testing.T) {
		client := &rotatingPSAImportClient{rotateResult: false}

		require.False(t, adapter.NewPSAImporterAdapter(client).RotatePSAKey())
		require.Equal(t, 1, client.rotateCalls)
	})
}

func TestInventoryAdapter_RotationDelegation(t *testing.T) {
	t.Run("delegates when client is a rotator", func(t *testing.T) {
		client := &rotatingInventoryClient{rotateBudget: 1}
		a := adapter.NewInventoryAdapter(client)

		require.True(t, a.RotatePSAKey())
		a.ResetPSAKeyRotation()

		require.Equal(t, 1, client.rotateCalls)
		require.Equal(t, 1, client.resetCalls)
	})

	t.Run("reports exhausted when client is not a rotator", func(t *testing.T) {
		a := adapter.NewInventoryAdapter(&stubInventoryClient{})

		require.False(t, a.RotatePSAKey())
		require.NotPanics(t, a.ResetPSAKeyRotation)
	})
}

// --- InventoryAdapter.UpdateInventoryStatus ---

func TestInventoryAdapter_UpdateInventoryStatus_BuildsUpdate(t *testing.T) {
	tests := []struct {
		name   string
		update inventory.DHInventoryStatusUpdate
		want   dh.InventoryUpdate
	}{
		{
			name:   "status only omits optional fields",
			update: inventory.DHInventoryStatusUpdate{Status: inventory.DHStatusInStock},
			want:   dh.InventoryUpdate{Status: inventory.DHStatusInStock},
		},
		{
			name: "positive price is sent as a pointer",
			update: inventory.DHInventoryStatusUpdate{
				Status:            inventory.DHStatusInStock,
				ListingPriceCents: 150_00,
			},
			want: dh.InventoryUpdate{
				Status:            inventory.DHStatusInStock,
				ListingPriceCents: dh.IntPtr(150_00),
			},
		},
		{
			name: "zero price is omitted entirely",
			update: inventory.DHInventoryStatusUpdate{
				Status:            inventory.DHStatusInStock,
				ListingPriceCents: 0,
			},
			want: dh.InventoryUpdate{Status: inventory.DHStatusInStock},
		},
		{
			name: "cert images pass through when set",
			update: inventory.DHInventoryStatusUpdate{
				Status:            inventory.DHStatusInStock,
				CertImageURLFront: "https://psa.example/front.jpg",
				CertImageURLBack:  "https://psa.example/back.jpg",
			},
			want: dh.InventoryUpdate{
				Status:            inventory.DHStatusInStock,
				CertImageURLFront: "https://psa.example/front.jpg",
				CertImageURLBack:  "https://psa.example/back.jpg",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &stubInventoryClient{}

			_, err := adapter.NewInventoryAdapter(client).UpdateInventoryStatus(context.Background(), 77, tt.update)

			require.NoError(t, err)
			require.Equal(t, []int{77}, client.updateIDs)
			require.Equal(t, tt.want, client.updates[0])
		})
	}
}

func TestInventoryAdapter_UpdateInventoryStatus_ReturnsListingPrice(t *testing.T) {
	client := &stubInventoryClient{
		updateFn: func(context.Context, int, dh.InventoryUpdate) (*dh.InventoryResult, error) {
			return &dh.InventoryResult{ListingPriceCents: 320_00}, nil
		},
	}

	price, err := adapter.NewInventoryAdapter(client).UpdateInventoryStatus(
		context.Background(), 1, inventory.DHInventoryStatusUpdate{Status: inventory.DHStatusListed})

	require.NoError(t, err)
	require.Equal(t, 320_00, price)
}

func TestInventoryAdapter_UpdateInventoryStatus_NilResponse(t *testing.T) {
	// A nil result with no error must not panic; DH returns 204 on some PATCHes.
	client := &stubInventoryClient{
		updateFn: func(context.Context, int, dh.InventoryUpdate) (*dh.InventoryResult, error) {
			return nil, nil
		},
	}

	price, err := adapter.NewInventoryAdapter(client).UpdateInventoryStatus(
		context.Background(), 1, inventory.DHInventoryStatusUpdate{Status: inventory.DHStatusListed})

	require.NoError(t, err)
	require.Zero(t, price)
}

func TestInventoryAdapter_UpdateInventoryStatus_Rotation(t *testing.T) {
	t.Run("listed status rotates and retries", func(t *testing.T) {
		client := &rotatingInventoryClient{rotateBudget: 1}
		client.updateFn = func(_ context.Context, _ int, _ dh.InventoryUpdate) (*dh.InventoryResult, error) {
			if len(client.updates) == 1 { // first attempt
				return nil, errPSARateLimit
			}
			return &dh.InventoryResult{ListingPriceCents: 500_00}, nil
		}

		price, err := adapter.NewInventoryAdapter(client).UpdateInventoryStatus(
			context.Background(), 5, inventory.DHInventoryStatusUpdate{Status: inventory.DHStatusListed})

		require.NoError(t, err)
		require.Equal(t, 500_00, price)
		require.Len(t, client.updates, 2, "should retry after rotating")
		require.Equal(t, 1, client.rotateCalls)
	})

	t.Run("exhaustion wraps both sentinels", func(t *testing.T) {
		client := &rotatingInventoryClient{rotateBudget: 0}
		client.updateFn = func(context.Context, int, dh.InventoryUpdate) (*dh.InventoryResult, error) {
			return nil, errPSARateLimit
		}

		_, err := adapter.NewInventoryAdapter(client).UpdateInventoryStatus(
			context.Background(), 5, inventory.DHInventoryStatusUpdate{Status: inventory.DHStatusListed})

		require.Error(t, err)
		// The domain sentinel is the contract (types.go:14) — callers must not
		// need to import the dh package to detect exhaustion.
		require.ErrorIs(t, err, dhlisting.ErrPSAKeysExhausted)
		require.ErrorIs(t, err, dh.ErrPSAKeysExhausted)
		require.ErrorIs(t, err, errPSARateLimit, "underlying cause should stay inspectable")
	})

	t.Run("non-listed status never rotates", func(t *testing.T) {
		client := &rotatingInventoryClient{rotateBudget: 1}
		client.updateFn = func(context.Context, int, dh.InventoryUpdate) (*dh.InventoryResult, error) {
			return nil, errPSARateLimit
		}

		_, err := adapter.NewInventoryAdapter(client).UpdateInventoryStatus(
			context.Background(), 5, inventory.DHInventoryStatusUpdate{Status: inventory.DHStatusInStock})

		require.ErrorIs(t, err, errPSARateLimit)
		require.NotErrorIs(t, err, dhlisting.ErrPSAKeysExhausted)
		require.Len(t, client.updates, 1)
		require.Zero(t, client.rotateCalls)
	})

	t.Run("listed status without a rotator takes the direct path", func(t *testing.T) {
		client := &stubInventoryClient{
			updateFn: func(context.Context, int, dh.InventoryUpdate) (*dh.InventoryResult, error) {
				return nil, errPSARateLimit
			},
		}

		_, err := adapter.NewInventoryAdapter(client).UpdateInventoryStatus(
			context.Background(), 5, inventory.DHInventoryStatusUpdate{Status: inventory.DHStatusListed})

		require.ErrorIs(t, err, errPSARateLimit)
		require.NotErrorIs(t, err, dhlisting.ErrPSAKeysExhausted)
		require.Len(t, client.updates, 1)
	})
}

// recordingLogger reuses the shared discard mock and records only what this
// package needs to prove: that WithLogger's logger is the one rotation uses.
type recordingLogger struct {
	mocks.MockLogger
	infos []string
}

func (r *recordingLogger) Info(_ context.Context, msg string, _ ...observability.Field) {
	r.infos = append(r.infos, msg)
}

func TestInventoryAdapter_WithLogger(t *testing.T) {
	newRotatingClient := func() *rotatingInventoryClient {
		client := &rotatingInventoryClient{rotateBudget: 1}
		client.updateFn = func(_ context.Context, _ int, _ dh.InventoryUpdate) (*dh.InventoryResult, error) {
			if len(client.updates) == 1 {
				return nil, errPSARateLimit
			}
			return &dh.InventoryResult{}, nil
		}
		return client
	}

	t.Run("injected logger receives rotation diagnostics", func(t *testing.T) {
		logger := &recordingLogger{}
		client := newRotatingClient()

		_, err := adapter.NewInventoryAdapter(client).WithLogger(logger).UpdateInventoryStatus(
			context.Background(), 1, inventory.DHInventoryStatusUpdate{Status: inventory.DHStatusListed})

		require.NoError(t, err)
		require.NotEmpty(t, logger.infos, "rotation should log through the injected logger")
	})

	t.Run("nil logger is ignored and leaves the noop default in place", func(t *testing.T) {
		client := newRotatingClient()
		a := adapter.NewInventoryAdapter(client)

		require.Same(t, a, a.WithLogger(nil), "WithLogger should stay chainable")

		// The rotation path logs unconditionally, so a cleared logger would panic here.
		_, err := a.UpdateInventoryStatus(
			context.Background(), 1, inventory.DHInventoryStatusUpdate{Status: inventory.DHStatusListed})
		require.NoError(t, err)
	})
}

// --- InventoryAdapter.SyncChannels ---
//
// MarkInventorySold and its coverage were removed in Task 12: it asserted
// against a permissive stub that accepted any payload, which is exactly why
// the DH-rejected status PATCH shipped undetected. Sale recording is now
// covered by the contract-enforcing fakeDHSaleServer in dh_sale_fake_test.go.

func TestInventoryAdapter_SyncChannels(t *testing.T) {
	t.Run("forwards id and channels", func(t *testing.T) {
		client := &stubInventoryClient{}

		require.NoError(t, adapter.NewInventoryAdapter(client).SyncChannels(
			context.Background(), 8, []string{"ebay", "tcgplayer"}))

		require.Equal(t, []int{8}, client.syncIDs)
		require.Equal(t, [][]string{{"ebay", "tcgplayer"}}, client.syncArgs)
	})

	t.Run("propagates the client error", func(t *testing.T) {
		wantErr := errors.New("sync failed")
		client := &stubInventoryClient{
			syncFn: func(context.Context, int, []string) (*dh.ChannelSyncResponse, error) {
				return nil, wantErr
			},
		}

		err := adapter.NewInventoryAdapter(client).SyncChannels(context.Background(), 8, nil)

		require.ErrorIs(t, err, wantErr)
	})
}

// --- InventorySnapshotAdapter.FetchAllInventory ---

func TestInventorySnapshotAdapter_FetchAllInventory(t *testing.T) {
	t.Run("returns a single short page keyed by inventory id", func(t *testing.T) {
		client := &stubSnapshotClient{pages: []*dh.InventoryListResponse{{
			Items: []dh.InventoryListItem{
				{DHInventoryID: 1, Status: inventory.DHStatusListed},
				{DHInventoryID: 2, Status: inventory.DHStatusInStock},
			},
		}}}

		inv, err := adapter.NewInventorySnapshotAdapter(client).FetchAllInventory(context.Background())

		require.NoError(t, err)
		require.Equal(t, map[int]string{
			1: inventory.DHStatusListed,
			2: inventory.DHStatusInStock,
		}, inv)
		require.Len(t, client.filters, 1, "a short page must stop pagination")
		require.Equal(t, dh.InventoryFilters{Page: 1, PerPage: 100}, client.filters[0])
	})

	t.Run("paginates until a short page", func(t *testing.T) {
		client := &stubSnapshotClient{pages: []*dh.InventoryListResponse{
			fullPage(1000),
			fullPage(2000),
			{Items: []dh.InventoryListItem{{DHInventoryID: 3000, Status: inventory.DHStatusListed}}},
		}}

		inv, err := adapter.NewInventorySnapshotAdapter(client).FetchAllInventory(context.Background())

		require.NoError(t, err)
		require.Len(t, inv, 201)
		require.Equal(t, inventory.DHStatusListed, inv[3000])
		require.Len(t, client.filters, 3)
		require.Equal(t, []dh.InventoryFilters{
			{Page: 1, PerPage: 100},
			{Page: 2, PerPage: 100},
			{Page: 3, PerPage: 100},
		}, client.filters)
	})

	t.Run("stops on an empty page after an exactly-full one", func(t *testing.T) {
		client := &stubSnapshotClient{pages: []*dh.InventoryListResponse{
			fullPage(1),
			{},
		}}

		inv, err := adapter.NewInventorySnapshotAdapter(client).FetchAllInventory(context.Background())

		require.NoError(t, err)
		require.Len(t, inv, 100)
		require.Len(t, client.filters, 2)
	})

	t.Run("skips items with a zero inventory id", func(t *testing.T) {
		client := &stubSnapshotClient{pages: []*dh.InventoryListResponse{{
			Items: []dh.InventoryListItem{
				{DHInventoryID: 0, Status: inventory.DHStatusListed},
				{DHInventoryID: 7, Status: inventory.DHStatusListed},
			},
		}}}

		inv, err := adapter.NewInventorySnapshotAdapter(client).FetchAllInventory(context.Background())

		require.NoError(t, err)
		require.Equal(t, map[int]string{7: inventory.DHStatusListed}, inv)
	})

	t.Run("fails the whole call on a mid-pagination page error", func(t *testing.T) {
		// A partial snapshot would make the reconciler reset healthy items, so
		// any page error must discard everything collected so far.
		wantErr := errors.New("dh 503")
		client := &stubSnapshotClient{
			pages:   []*dh.InventoryListResponse{fullPage(1)},
			err:     wantErr,
			errPage: 2,
		}

		inv, err := adapter.NewInventorySnapshotAdapter(client).FetchAllInventory(context.Background())

		require.ErrorIs(t, err, wantErr)
		require.Nil(t, inv)
	})

	t.Run("fails when pagination exceeds the page cap", func(t *testing.T) {
		// Every page comes back full, so the loop only ends at maxSnapshotPages.
		client := &stubSnapshotClient{}
		alwaysFull := &neverEndingSnapshotClient{stubSnapshotClient: client}

		inv, err := adapter.NewInventorySnapshotAdapter(alwaysFull).FetchAllInventory(context.Background())

		require.Error(t, err)
		require.Contains(t, err.Error(), "exceeded max pages")
		require.Nil(t, inv)
		require.Len(t, client.filters, 500)
	})
}

// neverEndingSnapshotClient always returns a full page, forcing FetchAllInventory
// into its maxSnapshotPages guard.
type neverEndingSnapshotClient struct {
	stubSnapshotClient *stubSnapshotClient
}

func (n *neverEndingSnapshotClient) ListInventory(_ context.Context, f dh.InventoryFilters) (*dh.InventoryListResponse, error) {
	n.stubSnapshotClient.filters = append(n.stubSnapshotClient.filters, f)
	return fullPage(f.Page * 1000), nil
}

// Guard against the page size drifting out of step with the stubs above, which
// hard-code 100-item pages to trigger the "keep paginating" branch.
func TestSnapshotStubsMatchPageSize(t *testing.T) {
	client := &stubSnapshotClient{pages: []*dh.InventoryListResponse{fullPage(1), {}}}

	_, err := adapter.NewInventorySnapshotAdapter(client).FetchAllInventory(context.Background())

	require.NoError(t, err)
	require.Len(t, client.filters, 2,
		fmt.Sprintf("fullPage builds %d items; snapshotPageSize must match for pagination tests to mean anything", 100))
}
