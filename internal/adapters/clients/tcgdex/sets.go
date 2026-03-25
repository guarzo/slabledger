package tcgdex

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// tcgdexSet is the API response for a single set from TCGdex.
type tcgdexSet struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ReleaseDate string `json:"releaseDate"`
	Serie       struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"serie"`
	CardCount struct {
		Total    int `json:"total"`
		Official int `json:"official"`
	} `json:"cardCount"`
}

// GetSet implements domainCards.CardProvider interface.
func (t *TCGdex) GetSet(ctx context.Context, setID string) (*domainCards.Set, error) {
	// 1. Try in-memory cache first
	cacheKey := fmt.Sprintf("tcgdex:set:%s", setID)
	if t.cache != nil {
		var domainSet domainCards.Set
		found, err := t.cache.Get(ctx, cacheKey, &domainSet)
		if err != nil {
			t.logger.Debug(ctx, "cache get error for set",
				observability.String("cacheKey", cacheKey),
				observability.Err(err))
		}
		if err == nil && found {
			return &domainSet, nil
		}
	}

	// 2. Try persistent storage
	if t.enablePersist && t.setStore != nil && t.setStore.SetExists(setID) {
		persistedSet, err := t.setStore.LoadSet(ctx, setID)
		if err == nil && persistedSet != nil {
			if t.cache != nil {
				if err := t.cache.Set(ctx, cacheKey, &persistedSet.Set, 24*time.Hour); err != nil {
					t.logger.Warn(ctx, "Failed to cache set in memory", observability.Err(err), observability.String("setID", setID))
				}
			}
			return &persistedSet.Set, nil
		}
	}

	// 3. Fetch from API — use the registered source language for this set
	allNotFound := true
	for _, lang := range t.languagesForSet(ctx, setID) {
		u := t.buildURL(lang, fmt.Sprintf("sets/%s", escapePathSegment(setID)))
		var resp tcgdexSet

		if err := t.get(ctx, u, &resp); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			var appErr *apperrors.AppError
			if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeProviderNotFound {
				allNotFound = false
			}
			continue
		}

		domainSet := &domainCards.Set{
			ID:          resp.ID,
			Name:        resp.Name,
			Series:      resp.Serie.Name,
			TotalCards:  resp.CardCount.Total,
			ReleaseDate: resp.ReleaseDate,
		}

		if t.cache != nil {
			if err := t.cache.Set(ctx, cacheKey, domainSet, 24*time.Hour); err != nil {
				t.logger.Warn(ctx, "Failed to cache set", observability.Err(err), observability.String("setID", setID))
			}
		}

		return domainSet, nil
	}

	if allNotFound {
		return nil, apperrors.ProviderNotFound("TCGdex", setID)
	}
	return nil, apperrors.ProviderUnavailable("TCGdex", fmt.Errorf("failed to fetch set %s from any language", setID))
}

// ListAllSets implements domainCards.CardProvider interface.
// Fetches sets from all configured languages and merges them.
func (t *TCGdex) ListAllSets(ctx context.Context) ([]domainCards.Set, error) {
	// 1. Try in-memory cache first
	cacheKey := "tcgdex:all_sets"
	if t.cache != nil {
		var sets []domainCards.Set
		found, err := t.cache.Get(ctx, cacheKey, &sets)
		if err != nil {
			t.logger.Debug(ctx, "cache get error for all sets",
				observability.String("cacheKey", cacheKey),
				observability.Err(err))
		}
		if err == nil && found {
			return sets, nil
		}
	}

	// 2. Try persistent registry (if not stale)
	if t.enablePersist && t.registryMgr != nil {
		registry, err := t.registryMgr.LoadRegistry(ctx)
		if err == nil && len(registry.Sets) > 0 {
			discoveryInterval := t.newSetDiscoveryInterval
			if discoveryInterval == 0 {
				discoveryInterval = DefaultNewSetDiscoveryInterval
			}
			if time.Since(registry.LastUpdated) < discoveryInterval {
				sets := make([]domainCards.Set, 0, len(registry.Sets))
				for _, entry := range registry.Sets {
					sets = append(sets, domainCards.Set{
						ID:          entry.ID,
						Name:        entry.Name,
						Series:      entry.Series,
						TotalCards:  entry.TotalCards,
						ReleaseDate: entry.ReleaseDate,
					})
				}

				t.logger.Info(ctx, "loaded set list from persistent registry", observability.Int("setCount", len(sets)))

				if t.cache != nil {
					if err := t.cache.Set(ctx, cacheKey, sets, 24*time.Hour); err != nil {
						t.logger.Warn(ctx, "Failed to cache sets in memory", observability.Err(err))
					}
				}

				return sets, nil
			}
			t.logger.Info(ctx, "registry stale, re-fetching set list from API",
				observability.Duration("age", time.Since(registry.LastUpdated)),
				observability.Duration("threshold", discoveryInterval))
		}
	}

	// 3. Fetch from API for each language, merge by set ID.
	// Track which language each set came from so GetCards can use the correct endpoint.
	t.logger.Info(ctx, "fetching set list from API (cache miss)")

	type setWithLang struct {
		set  domainCards.Set
		lang string
	}
	setMap := make(map[string]setWithLang)
	var lastErr error

	for _, lang := range t.languages {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		u := t.buildURL(lang, "sets")
		var apiSets []tcgdexSet

		if err := t.get(ctx, u, &apiSets); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			t.logger.Warn(ctx, "failed to fetch sets for language",
				observability.String("language", lang), observability.Err(err))
			lastErr = err
			continue
		}

		for _, s := range apiSets {
			// English takes precedence for naming; add language-exclusive sets
			if _, exists := setMap[s.ID]; !exists || lang == "en" {
				setMap[s.ID] = setWithLang{
					set: domainCards.Set{
						ID:          s.ID,
						Name:        s.Name,
						Series:      s.Serie.Name,
						TotalCards:  s.CardCount.Total,
						ReleaseDate: s.ReleaseDate,
					},
					lang: lang,
				}
			}
		}

		t.logger.Info(ctx, "fetched sets for language",
			observability.String("language", lang),
			observability.Int("count", len(apiSets)))
	}

	if len(setMap) == 0 && lastErr != nil {
		return nil, apperrors.ProviderUnavailable("TCGdex", lastErr)
	}

	sets := make([]domainCards.Set, 0, len(setMap))
	for _, sw := range setMap {
		sets = append(sets, sw.set)
	}

	t.logger.Info(ctx, "fetched set list from API", observability.Int("setCount", len(sets)))

	// 4. Update registry with newly discovered sets (including source language)
	if t.enablePersist && t.registryMgr != nil {
		registry, err := t.registryMgr.LoadRegistry(ctx)
		if err != nil {
			t.logger.Warn(ctx, "Failed to load registry, skipping registry update", observability.Err(err))
		} else {
			newSetsFound := 0
			for _, sw := range setMap {
				if existing, exists := registry.Sets[sw.set.ID]; !exists {
					registry.Sets[sw.set.ID] = SetEntry{
						ID:          sw.set.ID,
						Name:        sw.set.Name,
						Series:      sw.set.Series,
						Language:    sw.lang,
						ReleaseDate: sw.set.ReleaseDate,
						TotalCards:  sw.set.TotalCards,
						Status:      "discovered",
					}
					newSetsFound++
				} else {
					// Update metadata on existing entries so changes propagate
					existing.Name = sw.set.Name
					existing.Series = sw.set.Series
					existing.Language = sw.lang
					existing.ReleaseDate = sw.set.ReleaseDate
					existing.TotalCards = sw.set.TotalCards
					registry.Sets[sw.set.ID] = existing
				}
			}

			if err := t.registryMgr.SaveRegistry(ctx, registry); err != nil {
				t.logger.Warn(ctx, "Failed to save updated registry", observability.Err(err))
			} else if newSetsFound > 0 {
				t.logger.Info(ctx, "Discovered new sets", observability.Int("newSets", newSetsFound))
			}
		}
	}

	// 5. Cache results in memory
	if t.cache != nil {
		if err := t.cache.Set(ctx, cacheKey, sets, 24*time.Hour); err != nil {
			t.logger.Warn(ctx, "Failed to cache all sets", observability.Err(err))
		}
	}

	return sets, nil
}
