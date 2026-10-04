package handlers

import (
	"context"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDHBulkMatchMissingCoordination(t *testing.T) {
	writes := 0
	push := &mocks.DHInventoryPusherMock{}
	repo := &mocks.PurchaseRepositoryMock{UpdatePurchaseDHPushStatusFn: func(context.Context, string, string) error { writes++; return nil }}
	saver := &mocks.DHCardIDSaverMock{SaveExternalIDFn: func(context.Context, string, string, string, string, string) error { writes++; return nil }}
	h := NewDHHandler(DHHandlerDeps{PurchaseLister: repo, PushStatusUpdater: repo, CardIDSaver: saver, InventoryPusher: push, Logger: mocks.NewMockLogger(), MutationRequired: true})
	received := "2026-01-01"
	p := inventory.Purchase{ID: "p1", CertNumber: "cert", CardName: "Card", ReceivedAt: &received, ReviewedPriceCents: 25000, BuyCostCents: 1000}
	h.runBulkMatch(context.Background(), []inventory.Purchase{p}, map[string]string{p.DHCardKey(): "23"})
	if writes != 0 || push.CallCount != 0 {
		t.Fatalf("missing coordination writes=%d pushes=%d", writes, push.CallCount)
	}
}
func TestDHWholeHandlersMissingCoordination(t *testing.T) {
	for _, kind := range []string{"unmatch", "fix", "retry", "select"} {
		t.Run(kind, func(t *testing.T) {
			writes := 0
			deletion := &mocks.DHInventoryDeleterMock{}
			push := &mocks.DHInventoryPusherMock{}
			mappings := &mocks.DHCardIDSaverMock{SaveExternalIDFn: func(context.Context, string, string, string, string, string) error { writes++; return nil }}
			repo := &mocks.PurchaseRepositoryMock{GetPurchaseFn: func(context.Context, string) (*inventory.Purchase, error) {
				return &inventory.Purchase{ID: "p1", DHPushStatus: "matched", DHInventoryID: 42, BuyCostCents: 1000}, nil
			}}
			h := NewDHHandler(DHHandlerDeps{PurchaseLister: repo, InventoryDeleter: deletion, InventoryPusher: push, CardIDSaver: mappings, DHFieldsUpdater: repo, Logger: mocks.NewMockLogger(), MutationRequired: true})
			methods := map[string]http.HandlerFunc{"unmatch": h.HandleUnmatchDH, "fix": h.HandleFixMatch, "retry": h.HandleRetryMatch, "select": h.HandleSelectMatch}
			body := map[string]string{"unmatch": `{"purchaseId":"p1"}`, "fix": `{"purchaseId":"p1","dhUrl":"https://doubleholo.com/card/123"}`, "retry": `{"purchaseId":"p1"}`, "select": `{"purchaseId":"p1","dhCardId":123}`}[kind]
			r := withUser(httptest.NewRequest("POST", "/", strings.NewReader(body)))
			w := httptest.NewRecorder()
			methods[kind](w, r)
			h.Wait()
			if w.Code != 503 || writes != 0 || deletion.Called || push.CallCount != 0 {
				t.Fatalf("status=%d mapping=%d delete=%v push=%d", w.Code, writes, deletion.Called, push.CallCount)
			}
		})
	}
}
