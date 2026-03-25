package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newMappingRepo(t *testing.T) (*CardIDMappingRepository, func()) {
	t.Helper()
	db := setupTestDB(t)
	repo := NewCardIDMappingRepository(db.DB)
	return repo, func() { db.Close() }
}

func TestCardIDMapping_SaveAndGetExternalID(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	err := repo.SaveExternalID(ctx, "Charizard", "Base Set", "4", "pricecharting", "ext-123")
	require.NoError(t, err)

	id, err := repo.GetExternalID(ctx, "Charizard", "Base Set", "4", "pricecharting")
	require.NoError(t, err)
	require.Equal(t, "ext-123", id)
}

func TestCardIDMapping_GetExternalID_NotFound(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	id, err := repo.GetExternalID(ctx, "Nonexistent", "Set", "1", "provider")
	require.NoError(t, err)
	require.Equal(t, "", id)
}

func TestCardIDMapping_GetLocalCard(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	err := repo.SaveExternalID(ctx, "Charizard", "Base Set", "4", "pricecharting", "ext-123")
	require.NoError(t, err)

	cardName, setName, err := repo.GetLocalCard(ctx, "pricecharting", "ext-123")
	require.NoError(t, err)
	require.Equal(t, "Charizard", cardName)
	require.Equal(t, "Base Set", setName)
}

func TestCardIDMapping_GetLocalCard_NotFound(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	cardName, setName, err := repo.GetLocalCard(ctx, "provider", "nonexistent")
	require.NoError(t, err)
	require.Equal(t, "", cardName)
	require.Equal(t, "", setName)
}

func TestCardIDMapping_ListByProvider(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, repo.SaveExternalID(ctx, "Card1", "Set1", "1", "providerA", "a1"))
	require.NoError(t, repo.SaveExternalID(ctx, "Card2", "Set2", "2", "providerA", "a2"))
	require.NoError(t, repo.SaveExternalID(ctx, "Card3", "Set3", "3", "providerB", "b1"))

	mappings, err := repo.ListByProvider(ctx, "providerA")
	require.NoError(t, err)
	require.Len(t, mappings, 2)

	mappingsB, err := repo.ListByProvider(ctx, "providerB")
	require.NoError(t, err)
	require.Len(t, mappingsB, 1)
}

func TestCardIDMapping_GetExternalIDFresh_Fresh(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, repo.SaveExternalID(ctx, "Card", "Set", "1", "provider", "ext-1"))

	id, err := repo.GetExternalIDFresh(ctx, "Card", "Set", "1", "provider", 1*time.Hour)
	require.NoError(t, err)
	require.Equal(t, "ext-1", id)
}

func TestCardIDMapping_GetExternalIDFresh_Stale(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, repo.SaveExternalID(ctx, "Card", "Set", "1", "provider", "ext-1"))

	// Query with a very short maxAge — the record was just created so it may or may not
	// be "stale" depending on timing. Use a maxAge of 0 to force staleness.
	id, err := repo.GetExternalIDFresh(ctx, "Card", "Set", "1", "provider", 0)
	require.NoError(t, err)
	require.Equal(t, "", id, "expected stale result with maxAge=0")
}

func TestCardIDMapping_DeleteByCard_WithCollectorNumber(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, repo.SaveExternalID(ctx, "Card", "Set", "1", "provider", "ext-1"))
	require.NoError(t, repo.SaveExternalID(ctx, "Card", "Set", "2", "provider", "ext-2"))

	deleted, err := repo.DeleteByCard(ctx, "Card", "Set", "1")
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	// Variant "1" should be gone
	id, err := repo.GetExternalID(ctx, "Card", "Set", "1", "provider")
	require.NoError(t, err)
	require.Equal(t, "", id)

	// Variant "2" should still exist
	id, err = repo.GetExternalID(ctx, "Card", "Set", "2", "provider")
	require.NoError(t, err)
	require.Equal(t, "ext-2", id)
}

func TestCardIDMapping_DeleteByCard_WithoutCollectorNumber(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, repo.SaveExternalID(ctx, "Card", "Set", "1", "provider", "ext-1"))
	require.NoError(t, repo.SaveExternalID(ctx, "Card", "Set", "2", "provider", "ext-2"))

	deleted, err := repo.DeleteByCard(ctx, "Card", "Set", "")
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)

	// Both variants should be gone
	id, err := repo.GetExternalID(ctx, "Card", "Set", "1", "provider")
	require.NoError(t, err)
	require.Equal(t, "", id)

	id, err = repo.GetExternalID(ctx, "Card", "Set", "2", "provider")
	require.NoError(t, err)
	require.Equal(t, "", id)
}

func TestCardIDMapping_SaveExternalID_NoOverwriteManual(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Save a manual hint first
	require.NoError(t, repo.SaveHint(ctx, "Card", "Set", "1", "provider", "manual-id"))

	// Try to overwrite with auto-discovery
	require.NoError(t, repo.SaveExternalID(ctx, "Card", "Set", "1", "provider", "auto-id"))

	// Manual hint should be preserved
	id, err := repo.GetExternalID(ctx, "Card", "Set", "1", "provider")
	require.NoError(t, err)
	require.Equal(t, "manual-id", id, "manual hint should not be overwritten by auto")
}

func TestCardIDMapping_SaveHint_OverwritesAuto(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Save auto-discovery first
	require.NoError(t, repo.SaveExternalID(ctx, "Card", "Set", "1", "provider", "auto-id"))

	// Overwrite with manual hint
	require.NoError(t, repo.SaveHint(ctx, "Card", "Set", "1", "provider", "manual-id"))

	// Should now return the manual hint
	id, err := repo.GetExternalID(ctx, "Card", "Set", "1", "provider")
	require.NoError(t, err)
	require.Equal(t, "manual-id", id)
}

func TestCardIDMapping_GetHint_IgnoresAuto(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Save auto-discovery
	require.NoError(t, repo.SaveExternalID(ctx, "Card", "Set", "1", "provider", "auto-id"))

	// GetHint should return "" because it's not a manual hint
	id, err := repo.GetHint(ctx, "Card", "Set", "1", "provider")
	require.NoError(t, err)
	require.Equal(t, "", id)
}

func TestCardIDMapping_GetHint_ReturnsManual(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, repo.SaveHint(ctx, "Card", "Set", "1", "provider", "manual-id"))

	id, err := repo.GetHint(ctx, "Card", "Set", "1", "provider")
	require.NoError(t, err)
	require.Equal(t, "manual-id", id)
}

func TestCardIDMapping_DeleteHint_OnlyManual(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Save both auto and manual for different providers
	require.NoError(t, repo.SaveExternalID(ctx, "Card", "Set", "1", "providerA", "auto-id"))
	require.NoError(t, repo.SaveHint(ctx, "Card", "Set", "1", "providerB", "manual-id"))

	// Delete hint for providerB
	require.NoError(t, repo.DeleteHint(ctx, "Card", "Set", "1", "providerB"))

	// Auto mapping for providerA should still exist
	id, err := repo.GetExternalID(ctx, "Card", "Set", "1", "providerA")
	require.NoError(t, err)
	require.Equal(t, "auto-id", id)

	// Manual hint for providerB should be gone
	id, err = repo.GetHint(ctx, "Card", "Set", "1", "providerB")
	require.NoError(t, err)
	require.Equal(t, "", id)
}

func TestCardIDMapping_ListHints_OnlyManual(t *testing.T) {
	repo, cleanup := newMappingRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Save auto and manual
	require.NoError(t, repo.SaveExternalID(ctx, "AutoCard", "Set", "1", "provider", "auto-id"))
	require.NoError(t, repo.SaveHint(ctx, "ManualCard", "Set", "2", "provider", "manual-id"))

	hints, err := repo.ListHints(ctx)
	require.NoError(t, err)
	require.Len(t, hints, 1, "only manual hints should appear")
	require.Equal(t, "ManualCard", hints[0].CardName)
	require.Equal(t, "manual-id", hints[0].ExternalID)
}
