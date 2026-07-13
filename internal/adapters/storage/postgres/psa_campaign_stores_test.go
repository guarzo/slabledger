package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/stretchr/testify/require"
)

func truncatePSACampaignTables(t *testing.T, db *DB) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`TRUNCATE TABLE psa_campaign_snapshot, psa_campaign_push_queue RESTART IDENTITY CASCADE`)
	require.NoError(t, err, "truncate psa_campaign tables")
}

func TestSnapshotStore_SaveGet(t *testing.T) {
	db := setupTestDB(t)
	truncatePSACampaignTables(t, db)
	s := NewPSACampaignSnapshotStore(db.DB)

	in := []psacampaign.PortalCampaign{{CampaignRequestID: "a", Name: "Crystal", BuyPercentClv: 70}}
	require.NoError(t, s.SaveSnapshot(context.Background(), in))

	got, fetchedAt, err := s.GetSnapshot(context.Background())
	require.NoError(t, err)
	require.False(t, fetchedAt.IsZero())
	require.Len(t, got, 1)
	require.Equal(t, "Crystal", got[0].Name)
	require.Equal(t, "a", got[0].CampaignRequestID)
	require.Equal(t, 70, got[0].BuyPercentClv)
}

func TestSnapshotStore_SaveSnapshot_EmptyRejected(t *testing.T) {
	db := setupTestDB(t)
	truncatePSACampaignTables(t, db)
	s := NewPSACampaignSnapshotStore(db.DB)

	err := s.SaveSnapshot(context.Background(), nil)
	require.Error(t, err)
}

func TestSnapshotStore_GetSnapshot_Empty(t *testing.T) {
	db := setupTestDB(t)
	truncatePSACampaignTables(t, db)
	s := NewPSACampaignSnapshotStore(db.DB)

	got, fetchedAt, err := s.GetSnapshot(context.Background())
	require.NoError(t, err)
	require.True(t, fetchedAt.IsZero())
	require.Empty(t, got)
}

func TestPushQueueStore_Lifecycle(t *testing.T) {
	db := setupTestDB(t)
	truncatePSACampaignTables(t, db)
	s := NewPSACampaignPushQueueStore(db.DB)
	ctx := context.Background()

	row := psacampaign.PushRow{
		ID:                 "push-1",
		PSACampaignID:      "psa-1",
		InternalCampaignID: "internal-1",
		RequestedBy:        "alice",
		Diff: psacampaign.ProposedDiff{
			Changes: []psacampaign.FieldChange{{Field: "buyBox.priceMinCents", Old: "100", New: "200"}},
		},
		Status: psacampaign.PushPending,
	}
	require.NoError(t, s.Enqueue(ctx, row))

	pending, err := s.ListByStatus(ctx, psacampaign.PushPending)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "push-1", pending[0].ID)
	require.Equal(t, "psa-1", pending[0].PSACampaignID)
	require.Equal(t, "internal-1", pending[0].InternalCampaignID)
	require.Equal(t, "alice", pending[0].RequestedBy)
	require.Equal(t, row.Diff, pending[0].Diff)

	require.NoError(t, s.Approve(ctx, "push-1", "bob"))

	approved, err := s.ListByStatus(ctx, psacampaign.PushApproved)
	require.NoError(t, err)
	require.Len(t, approved, 1)
	require.Equal(t, "bob", approved[0].ApprovedBy)
	require.Equal(t, psacampaign.PushApproved, approved[0].Status)

	stillPending, err := s.ListByStatus(ctx, psacampaign.PushPending)
	require.NoError(t, err)
	require.Empty(t, stillPending)

	// Approving an already-approved row should fail.
	err = s.Approve(ctx, "push-1", "carol")
	require.ErrorIs(t, err, psacampaign.ErrPushNotPending)

	require.NoError(t, s.MarkResult(ctx, "push-1", psacampaign.PushPushed, `{"ok":true}`, ""))

	pushed, err := s.ListByStatus(ctx, psacampaign.PushPushed)
	require.NoError(t, err)
	require.Len(t, pushed, 1)
	require.Equal(t, psacampaign.PushPushed, pushed[0].Status)
}

func TestPushQueueStore_Approve_NotPending(t *testing.T) {
	db := setupTestDB(t)
	truncatePSACampaignTables(t, db)
	s := NewPSACampaignPushQueueStore(db.DB)
	ctx := context.Background()

	err := s.Approve(ctx, "does-not-exist", "bob")
	require.ErrorIs(t, err, psacampaign.ErrPushNotPending)
}
