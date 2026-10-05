//go:build integration

package confirmedreturns_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestWholeLinkWriterFailureRetainsJournal(t *testing.T) {
	for _, tc := range []struct{ kind, writer string }{{"unmatch", ""}, {"fix", ""}, {"retry", ""}, {"select", ""}, {"unmatch", "candidates"}, {"unmatch", "mapping"}, {"fix", "status"}, {"fix", "candidates"}, {"retry", "mapping"}, {"retry", "candidates"}, {"select", "status"}, {"select", "candidates"}} {
		t.Run(tc.kind+"/"+tc.writer, func(t *testing.T) {
			db, store, _, _, id := setupIntegrationReturn(t, "link-cert")
			ctx := context.Background()
			target := 42
			status := "matched"
			if tc.kind == "select" {
				target = 0
			}
			if tc.kind == "retry" {
				status = "unmatched"
			}
			_, e := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=$1,dh_push_status=$2,buy_cost_cents=1000 WHERE id=$3`, target, status, id)
			require.NoError(t, e)
			repo := postgres.NewPurchaseStore(db.DB, mocks.NewMockLogger())
			failure := errors.New("writer rejected")
			candidates := &mocks.PurchaseRepositoryMock{UpdatePurchaseDHCandidatesFn: func(c context.Context, id, j string) error {
				if tc.writer == "candidates" {
					return failure
				}
				return repo.UpdatePurchaseDHCandidates(c, id, j)
			}, UpdatePurchaseDHPushStatusFn: func(c context.Context, id, s string) error {
				if tc.writer == "status" {
					return failure
				}
				return repo.UpdatePurchaseDHPushStatus(c, id, s)
			}}
			saver := &mocks.DHCardIDSaverMock{SaveExternalIDFn: func(context.Context, string, string, string, string, string) error {
				if tc.writer == "mapping" {
					return failure
				}
				return nil
			}}
			delMap := &mocks.DHMappingDeleterMock{DeleteAutoMappingFn: func(context.Context, string, string, string, string) (int64, error) {
				if tc.writer == "mapping" {
					return 0, failure
				}
				return 0, nil
			}}
			push := &mocks.DHInventoryPusherMock{PushInventoryFn: func(context.Context, []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
				return &dh.InventoryPushResponse{Results: []dh.InventoryResult{{DHInventoryID: 99, CertNumber: "link-cert", Status: "in_stock"}}}, nil
			}}
			psa := &mocks.DHPSAImportClientMock{PSAImportFn: func(context.Context, []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
				return &dh.PSAImportResponse{Success: true, Results: []dh.PSAImportResult{{CertNumber: "link-cert", DHInventoryID: 99, DHCardID: 123, Resolution: dh.PSAImportStatusMatched, Status: "in_stock"}}}, nil
			}}
			channels := &mocks.DHChannelDelisterMock{DelistChannelsFn: func(context.Context, int, []string) (*dh.ChannelSyncResponse, error) {
				return &dh.ChannelSyncResponse{DHInventoryID: 42, Status: "in_stock"}, nil
			}}
			h := handlers.NewDHHandler(handlers.DHHandlerDeps{Logger: mocks.NewMockLogger(), PurchaseLister: repo, CardIDSaver: saver, MappingDeleter: delMap, InventoryDeleter: &mocks.DHInventoryDeleterMock{}, InventoryPusher: push, PSAImporter: psa, ChannelDelister: channels, DHFieldsUpdater: repo, DHUnmatcher: repo, CandidatesSaver: candidates, PushStatusUpdater: candidates, MutationRequired: true, MutationCoordinator: inventory.NewDHMutationCoordinator(store, store), MutationGuards: store})
			want := 500
			if tc.writer == "" {
				want = 200
			}
			callLinkTest(t, h, tc.kind, id, want)
			state, e := store.GetReturnState(ctx, id)
			require.NoError(t, e)
			if tc.writer == "" {
				require.Nil(t, state.PrecedingAttempt)
				wantTarget := 99
				if tc.kind == "unmatch" {
					wantTarget = 0
				}
				require.Equal(t, wantTarget, state.Purchase.DHInventoryID)
				return
			}
			require.NotNil(t, state.PrecedingAttempt)
			require.Equal(t, "open", state.PrecedingAttempt.Outcome)
			require.Equal(t, target, state.Purchase.DHInventoryID)
		})
	}
}
func callLinkTest(t *testing.T, h *handlers.DHHandler, kind, id string, want int) {
	t.Helper()
	fn := map[string]http.HandlerFunc{"unmatch": h.HandleUnmatchDH, "fix": h.HandleFixMatch, "retry": h.HandleRetryMatch, "select": h.HandleSelectMatch}[kind]
	r := httptest.NewRequest("POST", "/", strings.NewReader(linkTestBody(kind, id)))
	r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &auth.User{ID: 1}))
	w := httptest.NewRecorder()
	fn(w, r)
	h.Wait()
	require.Equal(t, want, w.Code, w.Body.String())
}
func linkTestBody(kind, id string) string {
	switch kind {
	case "fix":
		return fmt.Sprintf(`{"purchaseId":%q,"dhUrl":"https://doubleholo.com/card/123"}`, id)
	case "select":
		return fmt.Sprintf(`{"purchaseId":%q,"dhCardId":123}`, id)
	default:
		return fmt.Sprintf(`{"purchaseId":%q}`, id)
	}
}
func TestLinkInvalidInputCreatesNoMarker(t *testing.T) {
	for _, kind := range []string{"fix", "select", "retry", "unmatch"} {
		t.Run(kind, func(t *testing.T) {
			db, store, _, _, id := setupIntegrationReturn(t, "invalid-cert")
			repo := postgres.NewPurchaseStore(db.DB, mocks.NewMockLogger())
			h := handlers.NewDHHandler(handlers.DHHandlerDeps{Logger: mocks.NewMockLogger(), PurchaseLister: repo, CardIDSaver: &mocks.DHCardIDSaverMock{}, MappingDeleter: &mocks.DHMappingDeleterMock{}, InventoryDeleter: &mocks.DHInventoryDeleterMock{}, InventoryPusher: &mocks.DHInventoryPusherMock{}, PSAImporter: &mocks.DHPSAImportClientMock{}, ChannelDelister: &mocks.DHChannelDelisterMock{}, DHFieldsUpdater: repo, DHUnmatcher: repo, CandidatesSaver: repo, PushStatusUpdater: repo, MutationRequired: true, MutationCoordinator: inventory.NewDHMutationCoordinator(store, store), MutationGuards: store})
			_, e := db.ExecContext(context.Background(), `UPDATE campaign_purchases SET dh_push_status='pending',dh_inventory_id=0 WHERE id=$1`, id)
			require.NoError(t, e)
			fn := map[string]http.HandlerFunc{"unmatch": h.HandleUnmatchDH, "fix": h.HandleFixMatch, "retry": h.HandleRetryMatch, "select": h.HandleSelectMatch}[kind]
			r := httptest.NewRequest("POST", "/", strings.NewReader(fmt.Sprintf(`{"purchaseId":%q,"dhCardId":-1,"dhUrl":"invalid"}`, id)))
			r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &auth.User{ID: 1}))
			w := httptest.NewRecorder()
			fn(w, r)
			require.GreaterOrEqual(t, w.Code, 400)
			state, e := store.GetReturnState(context.Background(), id)
			require.NoError(t, e)
			require.Nil(t, state.PrecedingAttempt)
		})
	}
}
