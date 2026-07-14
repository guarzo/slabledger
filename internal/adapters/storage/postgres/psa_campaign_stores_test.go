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

	// Saving again replaces the singleton (ON CONFLICT DO UPDATE): the old data
	// must be gone, not merged.
	replacement := []psacampaign.PortalCampaign{
		{CampaignRequestID: "b", Name: "Modern 10", BuyPercentClv: 72},
		{CampaignRequestID: "c", Name: "Vintage", BuyPercentClv: 65},
	}
	require.NoError(t, s.SaveSnapshot(context.Background(), replacement))

	got, _, err = s.GetSnapshot(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "Modern 10", got[0].Name)
	require.Equal(t, "Vintage", got[1].Name)
	for _, c := range got {
		require.NotEqual(t, "Crystal", c.Name, "old snapshot data must not survive replace")
	}
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

func TestPushQueueStore_CreateOperation_RoundTrip(t *testing.T) {
	db := setupTestDB(t)
	truncatePSACampaignTables(t, db)
	s := NewPSACampaignPushQueueStore(db.DB)
	ctx := context.Background()

	fd := &psacampaign.CampaignFormData{
		CampaignName: "Modern 10s", CampaignType: "CATEGORY", Category: "POKEMON",
		PrepackagedSpecListIDs: []string{}, IsActive: false,
		BidPercentage: 72, FlatFee: 3, DailyBudget: 3000, DailySpecLimit: 2,
		GradeMinimum: "10", GradeMaximum: "10", YearMinimum: 2024, YearMaximum: 2026,
		PriceMinimum: 500, PriceMaximum: 3000, CardLadderConfidenceMinimum: 3,
		PublisherFilterType: "Target", SelectedPublishers: []psacampaign.SubjectRef{},
		SubjectFilterType: "Target", SelectedSubjects: []psacampaign.SubjectRef{},
		DeniedSpecs: []psacampaign.SubjectRef{},
	}
	row := psacampaign.PushRow{
		ID: "row-create-1", Operation: psacampaign.OpCreate,
		InternalCampaignID: "c1", RequestedBy: "test",
		Diff: psacampaign.ProposedDiff{Create: fd}, Status: psacampaign.PushPending,
	}
	require.NoError(t, s.Enqueue(ctx, row))

	rows, err := s.ListByStatus(ctx, psacampaign.PushPending)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	got := rows[0]
	require.Equal(t, psacampaign.OpCreate, got.Operation)
	require.Empty(t, got.PSACampaignID, "create rows have no portal id yet")
	require.Equal(t, "c1", got.InternalCampaignID)
	require.NotNil(t, got.Diff.Create)
	require.Equal(t, "Modern 10s", got.Diff.Create.CampaignName)
	require.False(t, got.Diff.Create.IsActive, "creates must be born paused")
	require.Equal(t, "10", got.Diff.Create.GradeMinimum)
	require.Equal(t, 3000, got.Diff.Create.PriceMaximum)

	// Rows enqueued without an Operation (the existing update path) default to update.
	legacy := psacampaign.PushRow{
		ID: "row-legacy-1", PSACampaignID: "portal-1",
		Diff:   psacampaign.ProposedDiff{Changes: []psacampaign.FieldChange{{Field: "bidPercentage", Old: "70", New: "72"}}},
		Status: psacampaign.PushPending,
	}
	require.NoError(t, s.Enqueue(ctx, legacy))
	rows, err = s.ListByStatus(ctx, psacampaign.PushPending)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, r := range rows {
		if r.ID == "row-legacy-1" {
			require.Equal(t, psacampaign.OpUpdate, r.Operation)
		}
	}
}

func TestPushQueueStore_CreateUnique_RejectsConcurrentUnresolved(t *testing.T) {
	db := setupTestDB(t)
	truncatePSACampaignTables(t, db)
	s := NewPSACampaignPushQueueStore(db.DB)
	ctx := context.Background()

	mk := func(id string) psacampaign.PushRow {
		return psacampaign.PushRow{
			ID: id, Operation: psacampaign.OpCreate, InternalCampaignID: "camp-x",
			Diff:   psacampaign.ProposedDiff{Create: &psacampaign.CampaignFormData{CampaignName: "X"}},
			Status: psacampaign.PushPending,
		}
	}

	// First unresolved create for camp-x succeeds.
	require.NoError(t, s.Enqueue(ctx, mk("row-1")))

	// A second unresolved create for the same campaign is rejected atomically.
	err := s.Enqueue(ctx, mk("row-2"))
	require.ErrorIs(t, err, psacampaign.ErrDuplicateCreate)

	// Once the first leaves the unresolved set (e.g. failed), a retry is allowed.
	require.NoError(t, s.MarkResult(ctx, "row-1", psacampaign.PushFailed, "", "portal down"))
	require.NoError(t, s.Enqueue(ctx, mk("row-3")))

	// A create for a different campaign is never blocked.
	other := mk("row-4")
	other.InternalCampaignID = "camp-y"
	require.NoError(t, s.Enqueue(ctx, other))
}
