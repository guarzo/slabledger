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

func TestPushQueueStore_CreateUnique(t *testing.T) {
	db := setupTestDB(t)
	s := NewPSACampaignPushQueueStore(db.DB)
	ctx := context.Background()

	mk := func(id, campaignID string, status psacampaign.PushStatus) psacampaign.PushRow {
		return psacampaign.PushRow{
			ID: id, Operation: psacampaign.OpCreate, InternalCampaignID: campaignID,
			Diff:   psacampaign.ProposedDiff{Create: &psacampaign.CampaignFormData{CampaignName: "X"}},
			Status: status,
		}
	}

	// Each case runs against a freshly-truncated table and, before the assertion,
	// applies its `setup` (prior enqueues / state transitions) so the sequential
	// dependencies stay isolated per subtest.
	tests := []struct {
		name    string
		setup   func(t *testing.T)
		enqueue psacampaign.PushRow
		wantErr error
	}{
		{
			name:    "first unresolved create succeeds",
			setup:   func(t *testing.T) {},
			enqueue: mk("row-1", "camp-x", psacampaign.PushPending),
		},
		{
			name: "second unresolved create for same campaign rejected",
			setup: func(t *testing.T) {
				require.NoError(t, s.Enqueue(ctx, mk("row-1", "camp-x", psacampaign.PushPending)))
			},
			enqueue: mk("row-2", "camp-x", psacampaign.PushPending),
			wantErr: psacampaign.ErrDuplicateCreate,
		},
		{
			name: "retry allowed after first create fails",
			setup: func(t *testing.T) {
				require.NoError(t, s.Enqueue(ctx, mk("row-1", "camp-x", psacampaign.PushPending)))
				require.NoError(t, s.MarkResult(ctx, "row-1", psacampaign.PushFailed, "", "portal down"))
			},
			enqueue: mk("row-3", "camp-x", psacampaign.PushPending),
		},
		{
			name: "different campaign never blocked",
			setup: func(t *testing.T) {
				require.NoError(t, s.Enqueue(ctx, mk("row-1", "camp-x", psacampaign.PushPending)))
			},
			enqueue: mk("row-4", "camp-y", psacampaign.PushPending),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncatePSACampaignTables(t, db)
			tt.setup(t)
			err := s.Enqueue(ctx, tt.enqueue)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPushQueueStore_LatestPerCampaign(t *testing.T) {
	db := setupTestDB(t)
	truncatePSACampaignTables(t, db)
	s := NewPSACampaignPushQueueStore(db.DB)
	ctx := context.Background()

	fd := psacampaign.CampaignFormData{CampaignName: "Modern 10s", BidPercentage: 72}

	// Campaign A: two update rows — the newer one must win. Enqueue runs as
	// separate autocommit statements, so each row gets its own now() timestamp
	// and insert order == created_at order (matches existing test style).
	require.NoError(t, s.Enqueue(ctx, psacampaign.PushRow{
		ID: "a-old", PSACampaignID: "psa-a", InternalCampaignID: "camp-a",
		RequestedBy: "alice",
		Diff:        psacampaign.ProposedDiff{Changes: []psacampaign.FieldChange{{Field: "bidPercentage", Old: "70", New: "72"}}},
	}))
	require.NoError(t, s.MarkResult(ctx, "a-old", psacampaign.PushFailed, "", "portal 500"))
	require.NoError(t, s.Enqueue(ctx, psacampaign.PushRow{
		ID: "a-newer", PSACampaignID: "psa-a", InternalCampaignID: "camp-a",
		RequestedBy: "alice",
		Diff:        psacampaign.ProposedDiff{Changes: []psacampaign.FieldChange{{Field: "bidPercentage", Old: "72", New: "75"}}},
	}))

	// Campaign B: a pending create — formData must round-trip.
	require.NoError(t, s.Enqueue(ctx, psacampaign.PushRow{
		ID: "b-1", Operation: psacampaign.OpCreate, InternalCampaignID: "camp-b",
		RequestedBy: "alice",
		Diff:        psacampaign.ProposedDiff{Create: &fd},
	}))

	// Campaign C: a single failed row — Error and UpdatedAt must round-trip.
	require.NoError(t, s.Enqueue(ctx, psacampaign.PushRow{
		ID: "c-1", PSACampaignID: "psa-c", InternalCampaignID: "camp-c",
		RequestedBy: "bob",
		Diff:        psacampaign.ProposedDiff{Changes: []psacampaign.FieldChange{{Field: "flatFee", Old: "3", New: "4"}}},
	}))
	require.NoError(t, s.MarkResult(ctx, "c-1", psacampaign.PushFailed, "", "boom"))

	got, err := s.LatestPerCampaign(ctx)
	require.NoError(t, err)

	byCampaign := map[string]psacampaign.PushRow{}
	for _, r := range got {
		byCampaign[r.InternalCampaignID] = r
	}

	tests := []struct {
		name       string
		campaignID string
		wantID     string
		wantOp     psacampaign.Operation
		wantStatus psacampaign.PushStatus
		wantError  string
	}{
		{"latest update row wins", "camp-a", "a-newer", psacampaign.OpUpdate, psacampaign.PushPending, ""},
		{"pending create row carries formData", "camp-b", "b-1", psacampaign.OpCreate, psacampaign.PushPending, ""},
		{"failed row round-trips error", "camp-c", "c-1", psacampaign.OpUpdate, psacampaign.PushFailed, "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, ok := byCampaign[tt.campaignID]
			require.True(t, ok, "row for %s", tt.campaignID)
			require.Equal(t, tt.wantID, r.ID)
			require.Equal(t, tt.wantOp, r.Operation)
			require.Equal(t, tt.wantStatus, r.Status)
			require.Equal(t, tt.wantError, r.Error)
			require.False(t, r.UpdatedAt.IsZero(), "UpdatedAt must be scanned")
		})
	}

	// The create row must carry its formData back.
	require.NotNil(t, byCampaign["camp-b"].Diff.Create)
	require.Equal(t, "Modern 10s", byCampaign["camp-b"].Diff.Create.CampaignName)
	require.Len(t, got, 3)
}

func TestPushQueueStore_LatestPerCampaign_Empty(t *testing.T) {
	db := setupTestDB(t)
	truncatePSACampaignTables(t, db)
	s := NewPSACampaignPushQueueStore(db.DB)

	got, err := s.LatestPerCampaign(context.Background())
	require.NoError(t, err)
	require.Empty(t, got)
}
