package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// bulkMatchCardIDSaver is a configurable DHCardIDSaver test double for the
// bulk-match entry-path tests. The package already has a zero-value
// mockDHCardIDSaver; this variant exposes Fn fields so tests can drive
// GetMappedSet errors.
type bulkMatchCardIDSaver struct {
	GetMappedSetFn func(ctx context.Context, provider string) (map[string]string, error)
}

func (m *bulkMatchCardIDSaver) SaveExternalID(_ context.Context, _, _, _, _, _ string) error {
	return nil
}

func (m *bulkMatchCardIDSaver) GetExternalID(_ context.Context, _, _, _, _ string) (string, error) {
	return "", nil
}

func (m *bulkMatchCardIDSaver) GetMappedSet(ctx context.Context, provider string) (map[string]string, error) {
	if m.GetMappedSetFn != nil {
		return m.GetMappedSetFn(ctx, provider)
	}
	return map[string]string{}, nil
}

// bulkMatchHandler builds a DHHandler with the deps needed for the bulk-match
// entry-path tests. The async match goroutine itself runs with empty inputs in
// these tests (no purchases / empty mapped set), so it returns quickly and is
// drained via h.Wait() before each test exits.
func bulkMatchHandler(lister DHPurchaseLister, saver DHCardIDSaver) *DHHandler {
	return NewDHHandler(DHHandlerDeps{
		PurchaseLister: lister,
		CardIDSaver:    saver,
		Logger:         mocks.NewMockLogger(),
		BaseCtx:        context.Background(),
	})
}

// TestHandleBulkMatch covers the synchronous entry-path branches. The
// concurrency case (TestHandleBulkMatch_AcceptedAndConcurrentRejected) lives
// separately because forcing channel/wg coordination into a table struct hurts
// readability.
func TestHandleBulkMatch(t *testing.T) {
	tests := []struct {
		name string
		// lister/saver factories let each case configure only what it needs.
		lister   func() *mockDHPurchaseLister
		saver    func() *bulkMatchCardIDSaver
		withAuth bool
		wantCode int
		// wantMutexReleased asserts that a follow-up call does NOT receive
		// 409 — useful for verifying the failure paths unlock the mutex.
		wantMutexReleased bool
	}{
		{
			name:     "no user → 401",
			lister:   func() *mockDHPurchaseLister { return &mockDHPurchaseLister{} },
			saver:    func() *bulkMatchCardIDSaver { return &bulkMatchCardIDSaver{} },
			withAuth: false,
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "list purchases fails → 500 with mutex released",
			lister: func() *mockDHPurchaseLister {
				return &mockDHPurchaseLister{
					ListAllUnsoldPurchasesFn: func(_ context.Context) ([]inventory.Purchase, error) {
						return nil, errors.New("db down")
					},
				}
			},
			saver:             func() *bulkMatchCardIDSaver { return &bulkMatchCardIDSaver{} },
			withAuth:          true,
			wantCode:          http.StatusInternalServerError,
			wantMutexReleased: true,
		},
		{
			name:   "get mapped set fails → 500 with mutex released",
			lister: func() *mockDHPurchaseLister { return &mockDHPurchaseLister{} },
			saver: func() *bulkMatchCardIDSaver {
				return &bulkMatchCardIDSaver{
					GetMappedSetFn: func(_ context.Context, _ string) (map[string]string, error) {
						return nil, errors.New("db down")
					},
				}
			},
			withAuth:          true,
			wantCode:          http.StatusInternalServerError,
			wantMutexReleased: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := bulkMatchHandler(tt.lister(), tt.saver())

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/dh/bulk-match", nil)
			if tt.withAuth {
				req = withUser(req)
			}
			h.HandleBulkMatch(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantMutexReleased {
				rec2 := httptest.NewRecorder()
				req2 := httptest.NewRequest(http.MethodPost, "/api/dh/bulk-match", nil)
				req2 = withUser(req2)
				h.HandleBulkMatch(rec2, req2)
				if rec2.Code == http.StatusConflict {
					t.Errorf("follow-up call returned 409 — mutex was not released after the failure")
				}
			}
			h.Wait()
		})
	}
}

func TestHandleBulkMatch_AcceptedAndConcurrentRejected(t *testing.T) {
	// Block the first bulk-match's lister call so we can fire the second
	// while the first still holds the mutex. The empty-purchase return makes
	// runBulkMatch a fast no-op once unblocked.
	started := make(chan struct{})
	release := make(chan struct{})
	lister := &mockDHPurchaseLister{
		ListAllUnsoldPurchasesFn: func(_ context.Context) ([]inventory.Purchase, error) {
			close(started)
			<-release
			return nil, nil
		},
	}
	h := bulkMatchHandler(lister, &bulkMatchCardIDSaver{})

	var wg sync.WaitGroup
	first := httptest.NewRecorder()

	wg.Add(1)
	go func() {
		defer wg.Done()
		req := httptest.NewRequest(http.MethodPost, "/api/dh/bulk-match", nil)
		req = withUser(req)
		h.HandleBulkMatch(first, req)
	}()

	<-started // first caller has the mutex inside the lister call

	second := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dh/bulk-match", nil)
	req = withUser(req)
	h.HandleBulkMatch(second, req)

	if second.Code != http.StatusConflict {
		t.Fatalf("second call: got %d, want 409", second.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(second.Body).Decode(&body); err != nil {
		t.Fatalf("decode second body: %v", err)
	}
	if body["status"] != "already_running" {
		t.Errorf("second body: got %v, want already_running", body)
	}

	close(release)
	wg.Wait()
	h.Wait()

	if first.Code != http.StatusAccepted {
		t.Errorf("first call: got %d, want 202", first.Code)
	}
	var firstBody map[string]string
	if err := json.NewDecoder(first.Body).Decode(&firstBody); err != nil {
		t.Fatalf("decode first body: %v", err)
	}
	if firstBody["status"] != "started" {
		t.Errorf("first body: got %v, want started", firstBody)
	}
}
