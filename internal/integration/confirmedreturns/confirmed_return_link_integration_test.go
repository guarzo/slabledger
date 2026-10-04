//go:build integration

package confirmedreturns_test

import (
	"context"
	"fmt"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfirmedReturnWholeLinkRoutesFence(t *testing.T) {
	for _, state := range []string{"pending", "awaiting", "uncertain"} {
		for _, kind := range []string{"unmatch", "fix", "retry", "select"} {
			t.Run(fmt.Sprintf("%s/%s", state, kind), func(t *testing.T) {
				db, store, returns, fake, id := setupIntegrationReturn(t, "link-cert")
				ctx := context.Background()
				if state == "uncertain" {
					e := inventory.NewDHMutationCoordinator(store, store).Run(ctx, id, func(context.Context) (inventory.DHMutationRequest, error) {
						return inventory.DHMutationRequest{Kind: "list", Phase: "patch", PayloadIdentity: "old"}, nil
					}, func(context.Context, *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
						return nil, context.DeadlineExceeded
					})
					require.Error(t, e)
				} else {
					fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
						if state == "pending" {
							return nil, context.DeadlineExceeded
						}
						return &inventory.DHReturnResult{DHInventoryID: 42, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
					}
					_, e := returns.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true})
					if state == "pending" {
						require.Error(t, e)
					} else {
						require.NoError(t, e)
					}
				}
				repo := postgres.NewPurchaseStore(db.DB, mocks.NewMockLogger())
				del := &mocks.DHInventoryDeleterMock{}
				push := &mocks.DHInventoryPusherMock{}
				channels := &mocks.DHChannelDelisterMock{}
				maps := 0
				saver := &mocks.DHCardIDSaverMock{SaveExternalIDFn: func(context.Context, string, string, string, string, string) error { maps++; return nil }}
				h := handlers.NewDHHandler(handlers.DHHandlerDeps{Logger: mocks.NewMockLogger(), PurchaseLister: repo, InventoryDeleter: del, InventoryPusher: push, ChannelDelister: channels, CardIDSaver: saver, DHFieldsUpdater: repo, DHUnmatcher: repo, MutationRequired: true, MutationCoordinator: inventory.NewDHMutationCoordinator(store, store), MutationGuards: store})
				method := map[string]http.HandlerFunc{"unmatch": h.HandleUnmatchDH, "fix": h.HandleFixMatch, "retry": h.HandleRetryMatch, "select": h.HandleSelectMatch}[kind]
				body := fmt.Sprintf(`{"purchaseId":%q,"dhCardId":123,"dhUrl":"https://doubleholo.com/card/123"}`, id)
				req := httptest.NewRequest("POST", "/", strings.NewReader(body))
				req = req.WithContext(context.WithValue(ctx, middleware.UserContextKey, &auth.User{ID: 1}))
				w := httptest.NewRecorder()
				method(w, req)
				h.Wait()
				require.Equal(t, 409, w.Code, w.Body.String())
				require.False(t, del.Called)
				require.Zero(t, push.CallCount)
				require.False(t, channels.Called)
				require.Zero(t, maps)
			})
		}
	}
}
