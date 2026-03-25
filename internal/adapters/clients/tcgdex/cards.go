package tcgdex

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/cache"
)

// tcgdexSetDetail is the full set response from TCGdex, including card stubs.
type tcgdexSetDetail struct {
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
	Cards []tcgdexCardStub `json:"cards"`
}

// tcgdexCardStub is the abbreviated card data returned inside a set response.
type tcgdexCardStub struct {
	ID      string `json:"id"`
	LocalID string `json:"localId"`
	Name    string `json:"name"`
	Image   string `json:"image"`
}

// GetCards implements domainCards.CardProvider interface.
// Fetches all cards for a set from TCGdex using the set detail endpoint.
func (t *TCGdex) GetCards(ctx context.Context, setID string) ([]domainCards.Card, error) {
	// 1. Try in-memory cache first
	cacheKey := "tcgdex:" + cache.CardsKey(setID)
	if t.cache != nil {
		var domainCardList []domainCards.Card
		found, err := t.cache.Get(ctx, cacheKey, &domainCardList)
		if err != nil {
			t.logger.Warn(ctx, "cache get failed, falling back to primary source",
				observability.Err(err), observability.String("setID", setID))
			// Treat as cache miss — fall through to persistent storage / API
		} else if found {
			return domainCardList, nil
		}
	}

	// 2. Try persistent storage
	if t.enablePersist && t.setStore != nil && t.setStore.SetExists(setID) {
		persistedSet, err := t.setStore.LoadSet(ctx, setID)
		if err == nil && persistedSet != nil {
			t.logger.Info(ctx, "loaded cards from persistent cache",
				observability.String("setID", setID),
				observability.Int("cardCount", len(persistedSet.Cards)))

			if t.cache != nil {
				if err := t.cache.Set(ctx, cacheKey, persistedSet.Cards, 24*time.Hour); err != nil {
					t.logger.Warn(ctx, "Failed to cache cards in memory", observability.Err(err), observability.String("setID", setID))
				}
			}

			return persistedSet.Cards, nil
		}
		t.logger.Warn(ctx, "Failed to load from persistent storage, fetching from API",
			observability.Err(err), observability.String("setID", setID))
	}

	// 3. Fetch from API — use the registered source language for this set
	t.logger.Info(ctx, "fetching cards from API (cache miss)", observability.String("setID", setID))

	var domainCardList []domainCards.Card
	var setMeta domainCards.Set
	var fetchedLang string
	allNotFound := true

	for _, lang := range t.languagesForSet(ctx, setID) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		u := t.buildURL(lang, fmt.Sprintf("sets/%s", escapePathSegment(setID)))
		var resp tcgdexSetDetail

		if err := t.get(ctx, u, &resp); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			// Track whether all failures are 404s vs real errors
			var appErr *apperrors.AppError
			if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeProviderNotFound {
				allNotFound = false
			}
			continue
		}

		displayLang := langToDisplay(lang)
		setMeta = domainCards.Set{
			ID:          resp.ID,
			Name:        resp.Name,
			Series:      resp.Serie.Name,
			TotalCards:  resp.CardCount.Total,
			ReleaseDate: resp.ReleaseDate,
		}

		domainCardList = make([]domainCards.Card, 0, len(resp.Cards))
		for _, c := range resp.Cards {
			imageURL := ""
			if c.Image != "" {
				imageURL = c.Image + "/low.webp"
			}

			domainCardList = append(domainCardList, domainCards.Card{
				ID:       c.ID,
				Name:     c.Name,
				Number:   c.LocalID,
				Set:      resp.ID,
				SetName:  resp.Name,
				Language: displayLang,
				ImageURL: imageURL,
			})
		}

		fetchedLang = lang
		break
	}

	if domainCardList == nil {
		if allNotFound {
			return nil, apperrors.ProviderNotFound("TCGdex", setID)
		}
		return nil, apperrors.ProviderUnavailable("TCGdex", fmt.Errorf("failed to fetch set %s from any language", setID))
	}

	t.logger.Info(ctx, "fetched cards from API",
		observability.String("setID", setID),
		observability.String("language", fetchedLang),
		observability.Int("cardCount", len(domainCardList)))

	// 4. Persist to disk
	if t.enablePersist && t.setStore != nil && t.registryMgr != nil {
		if err := t.setStore.SaveSet(ctx, setID, setMeta, domainCardList); err != nil {
			t.logger.Warn(ctx, "Failed to persist set to disk", observability.Err(err), observability.String("setID", setID))
		} else {
			t.logger.Info(ctx, "persisted set to disk",
				observability.String("setID", setID),
				observability.Int("cardCount", len(domainCardList)))

			if err := t.registryMgr.MarkSetDiscovered(ctx, setID, setMeta.Name, fetchedLang, setMeta.ReleaseDate, setMeta.TotalCards); err != nil {
				t.logger.Warn(ctx, "Failed to mark set as discovered", observability.Err(err), observability.String("setID", setID))
			}
			if err := t.registryMgr.MarkSetFinalized(ctx, setID); err != nil {
				t.logger.Warn(ctx, "Failed to update registry", observability.Err(err), observability.String("setID", setID))
			}
		}
	}

	// 5. Cache in memory
	if t.cache != nil {
		if err := t.cache.Set(ctx, cacheKey, domainCardList, 24*time.Hour); err != nil {
			t.logger.Warn(ctx, "Failed to cache cards", observability.Err(err), observability.String("setID", setID))
		}
	}

	return domainCardList, nil
}

// SearchCards implements domainCards.CardProvider interface.
// Searches for cards using persistent cache first, API fallback if needed.
func (t *TCGdex) SearchCards(ctx context.Context, criteria domainCards.SearchCriteria) ([]domainCards.Card, int, error) {
	if criteria.CardName == "" && criteria.SetName == "" && criteria.CardNumber == "" && criteria.Query == "" {
		return nil, 0, apperrors.ValidationError("search_criteria", "no search criteria provided")
	}

	// Use persistent cache if available
	if t.enablePersist && t.registryMgr != nil && t.setStore != nil {
		results, total, err := t.searchCardsFromCache(ctx, criteria)
		if err == nil && len(results) > 0 {
			return results, total, nil
		}
		if err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			return nil, 0, err
		}
		if err != nil {
			t.logger.Warn(ctx, "cache search failed, falling back to API", observability.Err(err))
		} else {
			t.logger.Debug(ctx, "cache search returned no results, falling back to API",
				observability.String("cardName", criteria.CardName),
				observability.String("cardNumber", criteria.CardNumber))
		}
	}

	// Fallback to API search
	return t.searchCardsFromAPI(ctx, criteria)
}

// searchCardsFromCache searches through cached cards (no API calls).
func (t *TCGdex) searchCardsFromCache(ctx context.Context, criteria domainCards.SearchCriteria) ([]domainCards.Card, int, error) {
	registry, err := t.registryMgr.LoadRegistry(ctx)
	if err != nil {
		return nil, 0, err
	}

	// Sort set IDs for deterministic iteration order (Go maps are random)
	setIDs := make([]string, 0, len(registry.Sets))
	for setID, entry := range registry.Sets {
		if entry.Status == "finalized" {
			setIDs = append(setIDs, setID)
		}
	}
	sort.Strings(setIDs)

	var allCards []domainCards.Card
	for _, setID := range setIDs {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		default:
		}

		setData, err := t.setStore.LoadSet(ctx, setID)
		if err != nil {
			t.logger.Warn(ctx, "Failed to load set from cache",
				observability.String("setId", setID), observability.Err(err))
			continue
		}

		allCards = append(allCards, setData.Cards...)
	}

	t.logger.Info(ctx, "searching through cached cards",
		observability.Int("totalCards", len(allCards)),
		observability.String("cardName", criteria.CardName),
		observability.String("setName", criteria.SetName))

	if len(allCards) == 0 {
		return nil, 0, apperrors.NotFoundError("cached_cards", "no cached cards available")
	}

	// Perform local search/filtering
	var matches []domainCards.Card
	for _, card := range allCards {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		default:
		}

		if criteria.CardName != "" {
			if !strings.Contains(strings.ToLower(card.Name), strings.ToLower(criteria.CardName)) {
				continue
			}
		} else if criteria.Query != "" {
			if !strings.Contains(strings.ToLower(card.Name), strings.ToLower(criteria.Query)) {
				continue
			}
		}

		if criteria.SetName != "" {
			if !strings.Contains(strings.ToLower(card.SetName), strings.ToLower(criteria.SetName)) {
				continue
			}
		}

		if criteria.CardNumber != "" {
			if !strings.HasPrefix(card.Number, criteria.CardNumber) && card.Number != criteria.CardNumber {
				continue
			}
		}

		matches = append(matches, card)
	}

	limit := criteria.Limit
	if limit <= 0 {
		limit = 10
	}

	totalCount := len(matches)
	matches = matches[:min(totalCount, limit)]

	t.logger.Info(ctx, "search completed from cache",
		observability.Int("matchCount", totalCount),
		observability.Int("returned", len(matches)))

	return matches, totalCount, nil
}

// searchCardsFromAPI searches using TCGdex API (fallback when cache unavailable).
func (t *TCGdex) searchCardsFromAPI(ctx context.Context, criteria domainCards.SearchCriteria) ([]domainCards.Card, int, error) {
	t.logger.Warn(ctx, "searching cards via API (cache unavailable)")

	// Build search query for TCGdex card list endpoint
	searchName := criteria.CardName
	if searchName == "" {
		searchName = criteria.Query
	}
	if searchName == "" {
		// API search requires a card name or query for the name-based endpoint.
		// Searches using only SetName or CardNumber can only be served from cache.
		return nil, 0, nil
	}

	limit := criteria.Limit
	if limit <= 0 {
		limit = 10
	}
	fetchLimit := limit * 2

	// Load registry once before the language loop to avoid repeated I/O
	type registrySetEntry struct{ Name string }
	var registrySets map[string]registrySetEntry
	if t.enablePersist && t.registryMgr != nil {
		if reg, err := t.registryMgr.LoadRegistry(ctx); err == nil {
			registrySets = make(map[string]registrySetEntry, len(reg.Sets))
			for id, entry := range reg.Sets {
				registrySets[id] = registrySetEntry{Name: entry.Name}
			}
		}
	}

	// Search all configured languages
	var domainCardList []domainCards.Card
	for _, lang := range t.languages {
		u := t.buildURL(lang, fmt.Sprintf("cards?name=like:%s&pagination:itemsPerPage=%d", url.QueryEscape(searchName), fetchLimit))

		var apiCards []tcgdexCardStub
		if err := t.get(ctx, u, &apiCards); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, 0, err
			}
			t.logger.Debug(ctx, "API card search failed for language",
				observability.String("language", lang), observability.Err(err))
			continue
		}

		// Successful response with no cards — try next language.
		if len(apiCards) == 0 {
			continue
		}

		displayLang := langToDisplay(lang)
		for _, c := range apiCards {
			// Extract set ID from card ID (format: "{setId}-{localId}")
			setID := ""
			if idx := strings.LastIndex(c.ID, "-"); idx > 0 {
				setID = c.ID[:idx]
			}

			// Resolve set name from cached registry; fall back to setID
			resolvedSetName := setID
			if entry, ok := registrySets[setID]; ok && entry.Name != "" {
				resolvedSetName = entry.Name
			}

			imageURL := ""
			if c.Image != "" {
				imageURL = c.Image + "/low.webp"
			}

			domainCardList = append(domainCardList, domainCards.Card{
				ID:       c.ID,
				Name:     c.Name,
				Number:   c.LocalID,
				Set:      setID,
				SetName:  resolvedSetName,
				Language: displayLang,
				ImageURL: imageURL,
			})
		}
	}

	if len(domainCardList) == 0 {
		return nil, 0, nil
	}

	// Apply additional filters
	var filtered []domainCards.Card
	for _, card := range domainCardList {
		if criteria.SetName != "" {
			if !strings.Contains(strings.ToLower(card.SetName), strings.ToLower(criteria.SetName)) {
				continue
			}
		}
		if criteria.CardNumber != "" {
			if !strings.HasPrefix(card.Number, criteria.CardNumber) && card.Number != criteria.CardNumber {
				continue
			}
		}
		filtered = append(filtered, card)
	}

	totalCount := len(filtered)
	filtered = filtered[:min(totalCount, limit)]

	return filtered, totalCount, nil
}
