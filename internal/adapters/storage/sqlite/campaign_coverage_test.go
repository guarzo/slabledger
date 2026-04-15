package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedCampaign inserts a campaign with arbitrary inclusion/grade/phase params.
func seedCampaign(t *testing.T, db *DB, id, name, gradeRange, inclusionList, phase string, exclusionMode bool) {
	t.Helper()
	now := time.Now().Truncate(time.Second)
	_, err := db.Exec(
		`INSERT INTO campaigns (id, name, grade_range, inclusion_list, exclusion_mode, phase, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, gradeRange, inclusionList, exclusionMode, phase, now, now,
	)
	require.NoError(t, err)
}

// seedPurchaseWithPlayer inserts a campaign_purchases row with a given
// card_player + grade_value. Minimal field set; uses non-null defaults for
// required columns.
func seedPurchaseWithPlayer(t *testing.T, db *DB, id, campaignID, cardPlayer string, grade float64) {
	t.Helper()
	now := time.Now().Truncate(time.Second)
	_, err := db.Exec(
		`INSERT INTO campaign_purchases (
			id, campaign_id, card_name, cert_number, grader, grade_value,
			buy_cost_cents, psa_sourcing_fee_cents, purchase_date,
			created_at, updated_at, card_player
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, campaignID, "Test Card", "cert-"+id, "PSA", grade,
		10000, 300, "2026-01-15", now, now, cardPlayer,
	)
	require.NoError(t, err)
}

func seedSale(t *testing.T, db *DB, purchaseID string) {
	t.Helper()
	now := time.Now().Truncate(time.Second)
	_, err := db.Exec(
		`INSERT INTO campaign_sales (
			id, purchase_id, sale_channel, sale_price_cents, sale_fee_cents,
			sale_date, days_to_sell, net_profit_cents, created_at, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"sale-"+purchaseID, purchaseID, "ebay", 15000, 1500,
		"2026-02-01", 17, 3200, now, now,
	)
	require.NoError(t, err)
}

func TestCampaignCoverageLookup_CampaignsCovering(t *testing.T) {
	db := setupTestDB(t)
	lookup := NewCampaignCoverageLookup(db.DB)
	ctx := context.Background()

	// id=1 active, grade 9-10, includes Charizard
	seedCampaign(t, db, "1", "Vintage Pokemon", "9-10", "Charizard,Umbreon", "active", false)
	// id=2 active, grade 7-8, includes Charizard
	seedCampaign(t, db, "2", "Lower Grade", "7-8", "Charizard", "active", false)
	// id=3 active, no inclusion list (wildcard), grade 9-10
	seedCampaign(t, db, "3", "Wildcard", "9-10", "", "active", false)
	// id=4 pending — should be ignored
	seedCampaign(t, db, "4", "Pending", "9-10", "Charizard", "pending", false)
	// id=5 active, exclusion mode — Charizard is excluded
	seedCampaign(t, db, "5", "Excluded", "9-10", "Charizard", "active", true)
	// id=external — non-numeric, should be skipped even when matching
	seedCampaign(t, db, "external", "External", "9-10", "Charizard", "active", false)

	t.Run("matches by character and grade", func(t *testing.T) {
		ids, err := lookup.CampaignsCovering(ctx, "Charizard", "", 10)
		require.NoError(t, err)
		// Expect id=1 (Charizard 9-10) and id=3 (wildcard 9-10).
		// id=2 is grade 7-8 (no match for 10). id=5 is exclusion mode.
		// id=external is non-numeric. id=4 is pending.
		assert.ElementsMatch(t, []int64{1, 3}, ids)
	})

	t.Run("exclusion-mode campaigns accept non-matching characters", func(t *testing.T) {
		ids, err := lookup.CampaignsCovering(ctx, "Pikachu", "", 10)
		require.NoError(t, err)
		// id=3 (wildcard, accepts any character) matches.
		// id=5 is exclusion mode excluding Charizard — Pikachu is not excluded, so id=5 accepts it.
		// id=1 requires inclusion of Charizard/Umbreon — Pikachu isn't in the list.
		assert.ElementsMatch(t, []int64{3, 5}, ids)
	})

	t.Run("no campaigns at grade 5", func(t *testing.T) {
		ids, err := lookup.CampaignsCovering(ctx, "Charizard", "", 5)
		require.NoError(t, err)
		// All active grade ranges are 7-8 or 9-10; 5 matches none.
		assert.Empty(t, ids)
	})

	t.Run("empty character returns empty", func(t *testing.T) {
		ids, err := lookup.CampaignsCovering(ctx, "", "", 10)
		require.NoError(t, err)
		assert.Empty(t, ids)
	})

	t.Run("case-insensitive match", func(t *testing.T) {
		ids, err := lookup.CampaignsCovering(ctx, "charizard", "", 10)
		require.NoError(t, err)
		assert.ElementsMatch(t, []int64{1, 3}, ids)
	})
}

func TestCampaignCoverageLookup_UnsoldCountFor(t *testing.T) {
	db := setupTestDB(t)
	lookup := NewCampaignCoverageLookup(db.DB)
	ctx := context.Background()

	seedCampaign(t, db, "1", "C", "9-10", "", "active", false)

	// Charizard grade 10 — unsold
	seedPurchaseWithPlayer(t, db, "p1", "1", "Charizard", 10.0)
	// Charizard grade 10 — unsold
	seedPurchaseWithPlayer(t, db, "p2", "1", "Charizard", 10.0)
	// Charizard grade 9 — unsold
	seedPurchaseWithPlayer(t, db, "p3", "1", "Charizard", 9.0)
	// Charizard grade 10 — SOLD (should not count)
	seedPurchaseWithPlayer(t, db, "p4", "1", "Charizard", 10.0)
	seedSale(t, db, "p4")
	// Pikachu grade 10 — unsold (different character)
	seedPurchaseWithPlayer(t, db, "p5", "1", "Pikachu", 10.0)

	t.Run("count unsold Charizard grade 10 excludes sold", func(t *testing.T) {
		count, err := lookup.UnsoldCountFor(ctx, "Charizard", "", 10)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("count unsold Charizard grade 9", func(t *testing.T) {
		count, err := lookup.UnsoldCountFor(ctx, "Charizard", "", 9)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("count unsold Charizard any grade (grade=0)", func(t *testing.T) {
		count, err := lookup.UnsoldCountFor(ctx, "Charizard", "", 0)
		require.NoError(t, err)
		// p1, p2, p3 unsold; p4 sold; total 3.
		assert.Equal(t, 3, count)
	})

	t.Run("count unsold Pikachu grade 10", func(t *testing.T) {
		count, err := lookup.UnsoldCountFor(ctx, "Pikachu", "", 10)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("case-insensitive", func(t *testing.T) {
		count, err := lookup.UnsoldCountFor(ctx, "charizard", "", 10)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("unknown character returns zero", func(t *testing.T) {
		count, err := lookup.UnsoldCountFor(ctx, "Mewtwo", "", 10)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("empty character returns zero", func(t *testing.T) {
		count, err := lookup.UnsoldCountFor(ctx, "", "", 10)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestCampaignCoverageLookup_ActiveCampaigns(t *testing.T) {
	db := setupTestDB(t)
	lookup := NewCampaignCoverageLookup(db.DB)
	ctx := context.Background()

	// Numeric active with inclusion list + grade range
	seedCampaign(t, db, "1", "Vintage Core", "9-10", "Charizard,Pikachu", "active", false)
	// Non-numeric active (should be skipped)
	seedCampaign(t, db, "external", "External", "9-10", "Charizard", "active", false)
	// Numeric paused (should be skipped)
	seedCampaign(t, db, "2", "Paused", "8-9", "", "paused", false)

	got, err := lookup.ActiveCampaigns(ctx)
	require.NoError(t, err)

	if len(got) != 1 {
		t.Fatalf("want 1 campaign, got %d: %+v", len(got), got)
	}
	if got[0].ID != 1 || got[0].Name != "Vintage Core" {
		t.Errorf("want ID=1 Name=Vintage Core, got %+v", got[0])
	}
	if got[0].InclusionList != "Charizard,Pikachu" || got[0].GradeRange != "9-10" {
		t.Errorf("want inclusion+grade preserved, got %+v", got[0])
	}
}
