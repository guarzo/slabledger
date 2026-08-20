package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// The ordering IS the thing under test (design §7): void on DH, then reset the
// purchase, then delete the sale row LAST. Deleting last is what makes the
// sequence retry-safe — the sale row holds dh_sale_id, the only handle that
// can void.
func TestDeleteSaleByPurchaseID_VoidThenResetThenDelete(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	var calls []string

	recorder := &mocks.DHSaleRecorderMock{
		VoidInventorySaleFn: func(_ context.Context, dhSaleID, _ string) error {
			calls = append(calls, "void:"+dhSaleID)
			return nil
		},
	}
	repo.ResetDHFieldsForRelistAfterVoidFn = func(_ context.Context, purchaseID string) error {
		calls = append(calls, "reset:"+purchaseID)
		return nil
	}
	repo.DeleteSaleByPurchaseIDFn = func(_ context.Context, purchaseID string) error {
		calls = append(calls, "delete:"+purchaseID)
		return nil
	}

	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), inventory.WithDHSaleRecorder(recorder))
	ctx := context.Background()

	_, p := seedSaleFixture(t, svc, repo)
	sale := &inventory.Sale{
		ID: "s1", PurchaseID: p.ID, SalePriceCents: 5000, SaleDate: "2026-07-01",
		DHIdempotencyKey: "slabledger-sale-abc", DHSaleID: "dh-sale-77",
	}
	if err := repo.CreateSale(ctx, sale); err != nil {
		t.Fatalf("seed sale: %v", err)
	}

	if err := svc.DeleteSaleByPurchaseID(ctx, p.ID); err != nil {
		t.Fatalf("DeleteSaleByPurchaseID: %v", err)
	}

	want := []string{"void:dh-sale-77", "reset:" + p.ID, "delete:" + p.ID}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
}

// A purchase whose sale never reached DH (no dh_sale_id, no
// dh_idempotency_key) has nothing to void, so un-sell skips straight to the
// local reset+delete.
func TestDeleteSaleByPurchaseID_NoDHHistorySkipsVoid(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	recorder := &mocks.DHSaleRecorderMock{
		VoidInventorySaleFn: func(context.Context, string, string) error {
			t.Fatal("VoidInventorySale must not be called for a purchase with no DH history")
			return nil
		},
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), inventory.WithDHSaleRecorder(recorder))
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup campaign: %v", err)
	}
	p := &inventory.Purchase{CampaignID: c.ID, PurchaseDate: "2026-06-01"} // no DHInventoryID
	if err := repo.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup purchase: %v", err)
	}
	if err := repo.CreateSale(ctx, &inventory.Sale{ID: "s1", PurchaseID: p.ID, SalePriceCents: 5000, SaleDate: "2026-07-01"}); err != nil {
		t.Fatalf("seed sale: %v", err)
	}

	if err := svc.DeleteSaleByPurchaseID(ctx, p.ID); err != nil {
		t.Fatalf("DeleteSaleByPurchaseID: %v", err)
	}
	if _, err := repo.GetSaleByPurchaseID(ctx, p.ID); !errors.Is(err, inventory.ErrSaleNotFound) {
		t.Fatalf("sale still present after un-sell: err=%v", err)
	}
}

// A failure at the reset step leaves the sale row intact — its dh_sale_id
// handle survives — so a retry of the whole operation completes cleanly.
func TestDeleteSaleByPurchaseID_ResetFailureLeavesSaleRowIntact(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	recorder := &mocks.DHSaleRecorderMock{}
	repo.ResetDHFieldsForRelistAfterVoidFn = func(context.Context, string) error {
		return errors.New("db timeout")
	}

	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), inventory.WithDHSaleRecorder(recorder))
	ctx := context.Background()

	_, p := seedSaleFixture(t, svc, repo)
	if err := repo.CreateSale(ctx, &inventory.Sale{
		ID: "s1", PurchaseID: p.ID, SalePriceCents: 5000, SaleDate: "2026-07-01", DHSaleID: "dh-77",
	}); err != nil {
		t.Fatalf("seed sale: %v", err)
	}

	if err := svc.DeleteSaleByPurchaseID(ctx, p.ID); err == nil {
		t.Fatal("expected an error from the failed reset")
	}

	got, err := repo.GetSaleByPurchaseID(ctx, p.ID)
	if err != nil {
		t.Fatalf("sale row was deleted despite the reset failure: %v", err)
	}
	if got.DHSaleID != "dh-77" {
		t.Fatalf("sale row corrupted: DHSaleID = %q, want dh-77", got.DHSaleID)
	}

	// Retry: reset now succeeds and the delete completes.
	repo.ResetDHFieldsForRelistAfterVoidFn = nil
	if err := svc.DeleteSaleByPurchaseID(ctx, p.ID); err != nil {
		t.Fatalf("retry DeleteSaleByPurchaseID: %v", err)
	}
	if _, err := repo.GetSaleByPurchaseID(ctx, p.ID); !errors.Is(err, inventory.ErrSaleNotFound) {
		t.Fatal("sale row should be gone after the successful retry")
	}
}

// Un-selling a purchase with no sale row surfaces ErrSaleNotFound, even
// though the function wraps it with fmt.Errorf("%w") — errors.Is must still
// see through that wrap.
func TestDeleteSaleByPurchaseID_NoSaleReturnsErrSaleNotFound(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	_, p := seedSaleFixture(t, svc, repo)

	err := svc.DeleteSaleByPurchaseID(ctx, p.ID)
	if err == nil {
		t.Fatal("expected an error for a purchase with no sale row")
	}
	if !errors.Is(err, inventory.ErrSaleNotFound) {
		t.Fatalf("err = %v, want errors.Is match against ErrSaleNotFound", err)
	}
}

// A 404 from DH on void (design §7) covers not-found, another account's sale,
// a marketplace-mirror deal, and a UI-created deal — none voidable by us, none
// of which should fail an un-sell the user already performed locally.
func TestDeleteSaleByPurchaseID_VoidNotFoundIsSuccess(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	recorder := &mocks.DHSaleRecorderMock{
		VoidInventorySaleFn: func(context.Context, string, string) error {
			return inventory.ErrDHSaleNotFound
		},
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), inventory.WithDHSaleRecorder(recorder))
	ctx := context.Background()

	_, p := seedSaleFixture(t, svc, repo)
	if err := repo.CreateSale(ctx, &inventory.Sale{
		ID: "s1", PurchaseID: p.ID, SalePriceCents: 5000, SaleDate: "2026-07-01", DHSaleID: "dh-gone",
	}); err != nil {
		t.Fatalf("seed sale: %v", err)
	}

	if err := svc.DeleteSaleByPurchaseID(ctx, p.ID); err != nil {
		t.Fatalf("DeleteSaleByPurchaseID: %v", err)
	}
	if _, err := repo.GetSaleByPurchaseID(ctx, p.ID); !errors.Is(err, inventory.ErrSaleNotFound) {
		t.Fatal("sale row should be gone after a 404-treated-as-success void")
	}
}

// A sale with a key but no persisted dh_sale_id (the crash window design §5b
// names) is replayed to obtain a handle before voiding — never skipped.
func TestDeleteSaleByPurchaseID_ReplaysForMissingHandle(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	var recordedKey, voidedID string
	recorder := &mocks.DHSaleRecorderMock{
		RecordInventorySaleFn: func(_ context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
			recordedKey = req.IdempotencyKey
			return &inventory.DHSaleResult{DHSaleID: "dh-recovered", Replayed: true, Delisted: true}, nil
		},
		VoidInventorySaleFn: func(_ context.Context, dhSaleID, _ string) error {
			voidedID = dhSaleID
			return nil
		},
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), inventory.WithDHSaleRecorder(recorder))
	ctx := context.Background()

	_, p := seedSaleFixture(t, svc, repo)
	// Key present (DH confirmed the call) but dh_sale_id never persisted —
	// the crash window design §5b names.
	if err := repo.CreateSale(ctx, &inventory.Sale{
		ID: "s1", PurchaseID: p.ID, SalePriceCents: 5000, SaleDate: "2026-07-01",
		DHIdempotencyKey: "slabledger-sale-orphan",
	}); err != nil {
		t.Fatalf("seed sale: %v", err)
	}

	if err := svc.DeleteSaleByPurchaseID(ctx, p.ID); err != nil {
		t.Fatalf("DeleteSaleByPurchaseID: %v", err)
	}
	if recordedKey != "slabledger-sale-orphan" {
		t.Fatalf("replay used key %q, want the persisted key", recordedKey)
	}
	if voidedID != "dh-recovered" {
		t.Fatalf("voided id = %q, want the id obtained from the replay", voidedID)
	}
}
