package tcgdex

import (
	"context"
	"errors"
	"sort"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// propagateOrWrap returns the error as-is if it's already an *AppError,
// otherwise wraps it with StorageError.
func propagateOrWrap(err error, operation string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return apperrors.StorageError(operation, err)
}

// InvalidateSet removes a set from persistent storage and forces re-fetch.
func (t *TCGdex) InvalidateSet(ctx context.Context, setID string) error {
	if !t.enablePersist {
		return apperrors.NewAppError(apperrors.ErrCodeConfigInvalid, "persistent storage is not enabled")
	}
	if t.registryMgr == nil || t.setStore == nil {
		return apperrors.NewAppError(apperrors.ErrCodeConfigInvalid, "persistent storage enabled but collaborators not initialized")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := t.setStore.DeleteSet(ctx, setID); err != nil {
		return propagateOrWrap(err, "delete set from storage")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := t.registryMgr.RemoveSet(ctx, setID); err != nil {
		return propagateOrWrap(err, "remove set from registry")
	}

	t.logger.Info(ctx, "Invalidated set cache", observability.String("setID", setID))
	return nil
}

// InvalidateAllSets removes all sets from persistent storage.
func (t *TCGdex) InvalidateAllSets(ctx context.Context) error {
	if !t.enablePersist {
		return apperrors.NewAppError(apperrors.ErrCodeConfigInvalid, "persistent storage is not enabled")
	}
	if t.registryMgr == nil || t.setStore == nil {
		return apperrors.NewAppError(apperrors.ErrCodeConfigInvalid, "persistent storage enabled but collaborators not initialized")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	registry, err := t.registryMgr.LoadRegistry(ctx)
	if err != nil {
		return propagateOrWrap(err, "load registry")
	}

	deletedCount := 0
	var deletedSetIDs []string
	for setID := range registry.Sets {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := t.setStore.DeleteSet(ctx, setID); err != nil {
			t.logger.Warn(ctx, "Failed to delete set", observability.Err(err), observability.String("setID", setID))
		} else {
			deletedCount++
			deletedSetIDs = append(deletedSetIDs, setID)
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if deletedCount == len(registry.Sets) {
		if err := t.registryMgr.Clear(ctx); err != nil {
			return propagateOrWrap(err, "clear registry")
		}
	} else {
		for _, setID := range deletedSetIDs {
			if err := t.registryMgr.RemoveSet(ctx, setID); err != nil {
				t.logger.Warn(ctx, "Failed to remove set from registry", observability.Err(err), observability.String("setID", setID))
			}
		}
	}

	t.logger.Info(ctx, "Invalidated all set caches", observability.Int("deletedSets", deletedCount))
	return nil
}

// GetCacheStats returns statistics about the persistent cache.
func (t *TCGdex) GetCacheStats(ctx context.Context) (domainCards.CacheStats, error) {
	if !t.enablePersist {
		return domainCards.CacheStats{Enabled: false}, nil
	}
	if t.registryMgr == nil || t.setStore == nil {
		return domainCards.CacheStats{}, apperrors.NewAppError(apperrors.ErrCodeConfigInvalid, "persistent storage enabled but collaborators not initialized")
	}

	select {
	case <-ctx.Done():
		return domainCards.CacheStats{}, ctx.Err()
	default:
	}

	registry, err := t.registryMgr.LoadRegistry(ctx)
	if err != nil {
		return domainCards.CacheStats{}, propagateOrWrap(err, "load registry")
	}

	finalizedCount := 0
	discoveredCount := 0
	for _, entry := range registry.Sets {
		select {
		case <-ctx.Done():
			return domainCards.CacheStats{}, ctx.Err()
		default:
		}

		if entry.Status == "finalized" {
			finalizedCount++
		} else {
			discoveredCount++
		}
	}

	setKeys := make([]string, 0, len(registry.Sets))
	for k := range registry.Sets {
		setKeys = append(setKeys, k)
	}
	sort.Strings(setKeys)

	sets := make([]domainCards.CacheSetInfo, 0, len(registry.Sets))
	for _, k := range setKeys {
		entry := registry.Sets[k]
		sets = append(sets, domainCards.CacheSetInfo{
			ID:          entry.ID,
			Name:        entry.Name,
			Series:      entry.Series,
			ReleaseDate: entry.ReleaseDate,
			TotalCards:  entry.TotalCards,
			Status:      entry.Status,
			FetchedAt:   entry.FetchedAt,
		})
	}

	return domainCards.CacheStats{
		Enabled:         true,
		TotalSets:       len(registry.Sets),
		FinalizedSets:   finalizedCount,
		DiscoveredSets:  discoveredCount,
		LastUpdated:     registry.LastUpdated,
		RegistryVersion: registry.Version,
		Sets:            sets,
	}, nil
}
