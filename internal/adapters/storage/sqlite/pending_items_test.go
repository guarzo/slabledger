package sqlite

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupPendingItemsRepo(t *testing.T) *PendingItemsRepository {
	t.Helper()
	db := setupTestDB(t)
	return NewPendingItemsRepository(db.DB)
}

func TestPendingItems(t *testing.T) {
	repo := setupPendingItemsRepo(t)
	ctx := context.Background()

	t.Run("save and list pending items", func(t *testing.T) {
		items := []inventory.PendingItem{
			{
				ID:           "pi-1",
				CertNumber:   "CERT-001",
				CardName:     "Charizard",
				SetName:      "Base Set",
				CardNumber:   "4",
				Grade:        9.0,
				BuyCostCents: 50000,
				PurchaseDate: "2026-01-15",
				Status:       "ambiguous",
				Candidates:   []string{"camp-a", "camp-b"},
				Source:       "scheduler",
			},
			{
				ID:           "pi-2",
				CertNumber:   "CERT-002",
				CardName:     "Pikachu",
				SetName:      "Jungle",
				CardNumber:   "60",
				Grade:        10.0,
				BuyCostCents: 30000,
				PurchaseDate: "2026-02-01",
				Status:       "unmatched",
				Candidates:   nil,
				Source:       "manual",
			},
		}

		err := repo.SavePendingItems(ctx, items)
		require.NoError(t, err)

		got, err := repo.ListPendingItems(ctx)
		require.NoError(t, err)
		require.Len(t, got, 2)

		// Ordered by created_at DESC — both inserted in same tx so order may vary,
		// but we can verify both items are present.
		ids := map[string]bool{got[0].ID: true, got[1].ID: true}
		assert.True(t, ids["pi-1"])
		assert.True(t, ids["pi-2"])

		// Find pi-1 and verify candidates deserialized correctly.
		for _, item := range got {
			if item.ID == "pi-1" {
				assert.Equal(t, "Charizard", item.CardName)
				assert.Equal(t, "Base Set", item.SetName)
				assert.Equal(t, "4", item.CardNumber)
				assert.Equal(t, 9.0, item.Grade)
				assert.Equal(t, 50000, item.BuyCostCents)
				assert.Equal(t, "ambiguous", item.Status)
				assert.Equal(t, []string{"camp-a", "camp-b"}, item.Candidates)
				assert.Equal(t, "scheduler", item.Source)
			}
			if item.ID == "pi-2" {
				assert.Equal(t, "Pikachu", item.CardName)
				assert.Equal(t, "unmatched", item.Status)
			}
		}
	})

	t.Run("upsert updates unresolved items", func(t *testing.T) {
		updated := []inventory.PendingItem{
			{
				ID:           "pi-1-updated",
				CertNumber:   "CERT-001", // same cert as pi-1
				CardName:     "Charizard VMAX",
				SetName:      "Vivid Voltage",
				CardNumber:   "20",
				Grade:        8.5,
				BuyCostCents: 60000,
				PurchaseDate: "2026-03-01",
				Status:       "unmatched",
				Candidates:   []string{"camp-c"},
				Source:       "manual",
			},
		}

		err := repo.SavePendingItems(ctx, updated)
		require.NoError(t, err)

		got, err := repo.ListPendingItems(ctx)
		require.NoError(t, err)

		// Find the item with CERT-001 — should have updated fields.
		var found bool
		for _, item := range got {
			if item.CertNumber == "CERT-001" {
				found = true
				assert.Equal(t, "Charizard VMAX", item.CardName)
				assert.Equal(t, "Vivid Voltage", item.SetName)
				assert.Equal(t, "20", item.CardNumber)
				assert.Equal(t, 8.5, item.Grade)
				assert.Equal(t, 60000, item.BuyCostCents)
				assert.Equal(t, "unmatched", item.Status)
				assert.Equal(t, []string{"camp-c"}, item.Candidates)
				assert.Equal(t, "manual", item.Source)
			}
		}
		assert.True(t, found, "CERT-001 should still be in list")
	})

	t.Run("get pending item by ID", func(t *testing.T) {
		// Save a new item for this test.
		items := []inventory.PendingItem{
			{
				ID:           "pi-get",
				CertNumber:   "CERT-GET",
				CardName:     "Venusaur",
				SetName:      "Base Set",
				CardNumber:   "15",
				Grade:        7.0,
				BuyCostCents: 15000,
				PurchaseDate: "2026-03-10",
				Status:       "ambiguous",
				Candidates:   []string{"camp-a", "camp-b"},
				Source:       "scheduler",
			},
		}
		err := repo.SavePendingItems(ctx, items)
		require.NoError(t, err)

		got, err := repo.GetPendingItemByID(ctx, "pi-get")
		require.NoError(t, err)
		assert.Equal(t, "pi-get", got.ID)
		assert.Equal(t, "Venusaur", got.CardName)
		assert.Equal(t, []string{"camp-a", "camp-b"}, got.Candidates)
	})

	t.Run("get pending item by ID not found", func(t *testing.T) {
		_, err := repo.GetPendingItemByID(ctx, "nonexistent-id")
		require.Error(t, err)
		assert.True(t, inventory.IsPendingItemNotFound(err))
	})

	t.Run("resolve pending item", func(t *testing.T) {
		// pi-2 is still unresolved — resolve it.
		err := repo.ResolvePendingItem(ctx, "pi-2", "camp-x")
		require.NoError(t, err)

		got, err := repo.ListPendingItems(ctx)
		require.NoError(t, err)

		for _, item := range got {
			assert.NotEqual(t, "pi-2", item.ID, "resolved item should not appear in unresolved list")
		}
	})

	t.Run("upsert skips resolved items", func(t *testing.T) {
		// pi-2 was resolved above. Re-save with same cert should not update it.
		reSave := []inventory.PendingItem{
			{
				ID:           "pi-2-new",
				CertNumber:   "CERT-002", // same cert as resolved pi-2
				CardName:     "Pikachu Updated",
				SetName:      "Jungle Updated",
				CardNumber:   "61",
				Grade:        9.5,
				BuyCostCents: 35000,
				PurchaseDate: "2026-04-01",
				Status:       "ambiguous",
				Candidates:   []string{"camp-y"},
				Source:       "scheduler",
			},
		}

		err := repo.SavePendingItems(ctx, reSave)
		require.NoError(t, err)

		got, err := repo.ListPendingItems(ctx)
		require.NoError(t, err)

		// With partial unique index, a resolved cert CAN reappear as a new
		// pending row. The old resolved row is untouched; a fresh unresolved
		// row is inserted.
		var found bool
		for _, item := range got {
			if item.CertNumber == "CERT-002" {
				found = true
				assert.Equal(t, "pi-2-new", item.ID, "should be the new row, not the resolved one")
				assert.Equal(t, "Pikachu Updated", item.CardName)
			}
		}
		assert.True(t, found, "resolved cert should reappear as a new pending row")
	})

	t.Run("resolve not found returns error", func(t *testing.T) {
		err := repo.ResolvePendingItem(ctx, "nonexistent-id", "camp-z")
		require.Error(t, err)
		assert.True(t, inventory.IsPendingItemNotFound(err))
	})

	t.Run("dismiss pending item", func(t *testing.T) {
		// Save a new item to dismiss.
		items := []inventory.PendingItem{
			{
				ID:           "pi-dismiss",
				CertNumber:   "CERT-DISMISS",
				CardName:     "Blastoise",
				SetName:      "Base Set",
				CardNumber:   "2",
				Grade:        8.0,
				BuyCostCents: 20000,
				PurchaseDate: "2026-01-20",
				Status:       "unmatched",
				Candidates:   nil,
				Source:       "scheduler",
			},
		}
		err := repo.SavePendingItems(ctx, items)
		require.NoError(t, err)

		err = repo.DismissPendingItem(ctx, "pi-dismiss")
		require.NoError(t, err)

		got, err := repo.ListPendingItems(ctx)
		require.NoError(t, err)
		for _, item := range got {
			assert.NotEqual(t, "pi-dismiss", item.ID, "dismissed item should not appear")
		}
	})

	t.Run("dismiss not found returns error", func(t *testing.T) {
		err := repo.DismissPendingItem(ctx, "nonexistent-id")
		require.Error(t, err)
		assert.True(t, inventory.IsPendingItemNotFound(err))
	})

	t.Run("count pending items", func(t *testing.T) {
		count, err := repo.CountPendingItems(ctx)
		require.NoError(t, err)
		// At this point we have at least CERT-001 still unresolved.
		assert.GreaterOrEqual(t, count, 1, "should have at least 1 unresolved item")
	})
}
