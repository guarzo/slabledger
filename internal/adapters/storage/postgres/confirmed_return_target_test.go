package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/require"
)

func TestConfirmedReturnNewTargetAndOwnedRetry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target *inventory.ReturnTargetIdentity
		code   string
	}{
		{"missing target", nil, "client_update_required"},
		{"invalid inventory ID", &inventory.ReturnTargetIdentity{DHInventoryID: 0, CertNumber: "cert", Grader: "PSA"}, "client_update_required"},
		{"blank cert", &inventory.ReturnTargetIdentity{DHInventoryID: 42, CertNumber: " ", Grader: "PSA"}, "client_update_required"},
		{"blank grader", &inventory.ReturnTargetIdentity{DHInventoryID: 42, CertNumber: "cert", Grader: " "}, "client_update_required"},
		{"wrong target", &inventory.ReturnTargetIdentity{DHInventoryID: 43, CertNumber: "cert", Grader: "PSA"}, "identity_conflict"},
		{"same target", &inventory.ReturnTargetIdentity{DHInventoryID: 42, CertNumber: "cert", Grader: "PSA"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _, svc, fake, id := setupReturnCore(t, "cert", 42, "ext-848")
			ctx := context.Background()
			calls := 0
			fake.ReturnInventoryToStockFn = func(_ context.Context, target int, key string) (*inventory.DHReturnResult, error) {
				calls++
				require.Equal(t, 42, target)
				require.NotEmpty(t, key)
				return &inventory.DHReturnResult{DHInventoryID: target, ExternalSaleID: 848, ItemStatus: "in_stock"}, nil
			}
			sale := "old-sale"
			state, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &sale, ExpectedTarget: tc.target})
			if tc.code == "" {
				require.NoError(t, err)
				require.Equal(t, "completed", state.Operation.State)
				require.Equal(t, 1, calls)
				return
			}
			var conflict *inventory.ReturnConflict
			require.ErrorAs(t, err, &conflict)
			require.Equal(t, tc.code, conflict.Code)
			if tc.code == "client_update_required" {
				require.Contains(t, conflict.Message, "reload")
			}
			require.Zero(t, calls)
			var operations, attempts int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM confirmed_dh_returns),(SELECT count(*) FROM dh_mutation_attempts)`).Scan(&operations, &attempts))
			require.Zero(t, operations)
			require.Zero(t, attempts)
		})
	}
}

func TestConfirmedReturnOwnedOperationRetryUsesCapturedTarget(t *testing.T) {
	_, _, svc, fake, id := setupReturnCore(t, "cert", 42, "ext-848")
	ctx := context.Background()
	sale := "old-sale"
	calls := 0
	var capturedKey string
	fake.ReturnInventoryToStockFn = func(_ context.Context, target int, key string) (*inventory.DHReturnResult, error) {
		calls++
		require.Equal(t, 42, target)
		if capturedKey == "" {
			capturedKey = key
			return nil, context.DeadlineExceeded
		}
		require.Equal(t, capturedKey, key)
		return &inventory.DHReturnResult{DHInventoryID: target, ExternalSaleID: 848, ItemStatus: "in_stock"}, nil
	}
	pending, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &sale, ExpectedTarget: &inventory.ReturnTargetIdentity{DHInventoryID: 42, CertNumber: "cert", Grader: "PSA"}})
	require.Error(t, err)
	require.Equal(t, "pending", pending.Operation.State)
	completed, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &sale, OperationID: pending.Operation.ID, ExpectedTarget: &inventory.ReturnTargetIdentity{DHInventoryID: 999, CertNumber: "spoof", Grader: "BGS"}})
	require.NoError(t, err)
	require.Equal(t, pending.Operation.ID, completed.Operation.ID)
	require.Equal(t, "completed", completed.Operation.State)
	require.Equal(t, 2, calls)
	// An older client can replay the same completed operation without expectedTarget.
	again, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &sale, OperationID: pending.Operation.ID})
	require.NoError(t, err)
	require.Equal(t, "completed_replay", again.Outcome)
	require.Equal(t, 2, calls)
}

func TestConfirmedReturnOperationIDCannotCrossPurchaseOwnership(t *testing.T) {
	db, _, svc, fake, id := setupReturnCore(t, "shared-cert", 42, "ext-848")
	ctx := context.Background()
	// Same target and sale ID are not sufficient authority to replay another purchase's episode.
	sale := "old-sale"
	fake.ReturnInventoryToStockFn = func(_ context.Context, target int, _ string) (*inventory.DHReturnResult, error) {
		return &inventory.DHReturnResult{DHInventoryID: target, ExternalSaleID: 848, ItemStatus: "in_stock"}, nil
	}
	state, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &sale, ExpectedTarget: &inventory.ReturnTargetIdentity{DHInventoryID: 42, CertNumber: "shared-cert", Grader: "PSA"}})
	require.NoError(t, err)
	// Remove the old purchase and recreate the same target under a distinct purchase.
	_, err = db.ExecContext(ctx, `DELETE FROM campaign_purchases WHERE id=$1`, id)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,dh_inventory_id,received_at,purchase_date,created_at,updated_at) VALUES('other-p','return-c','Card','shared-cert','PSA',42,now(),'2026-01-01',now(),now())`)
	require.NoError(t, err)
	_, err = svc.ConfirmReturn(ctx, "other-p", inventory.ConfirmReturnRequest{ReturnConfirmed: true, OperationID: state.Operation.ID})
	var conflict *inventory.ReturnConflict
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, "identity_conflict", conflict.Code)
}

func TestConfirmedReturnFreshCompletionIdentity(t *testing.T) {
	for _, tt := range []struct{ name, change string }{{"changed sale", `UPDATE campaign_sales SET id='replacement-sale' WHERE id='old-sale'`}, {"changed inventory linkage", `UPDATE campaign_purchases SET dh_inventory_id=99 WHERE id='return-p'`}} {
		t.Run(tt.name, func(t *testing.T) {
			db, _, svc, fake, id := setupReturnCore(t, "target-race-cert", 364577, "ext-848")
			ctx := context.Background()
			fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
				_, err := db.ExecContext(ctx, tt.change)
				require.NoError(t, err)
				return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
			}
			sale := "old-sale"
			state, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &sale, ExpectedTarget: observedReturnTarget("target-race-cert", 364577)})
			require.ErrorIs(t, err, inventory.ErrReturnConflict)
			require.Equal(t, "conflicted", state.Operation.State)
			require.NotNil(t, state.Sale)
			require.Empty(t, state.Operation.ReturnedOrderID)
			require.NotNil(t, state.PrecedingAttempt)
		})
	}
}
