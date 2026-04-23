package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// refreshCharacters runs Step 1: union characters from top-characters (overall +
// per era), velocity, saturation, then upserts character cache rows. Returns
// the (ranked) character-name set, top_cards IDs for step 2 seeding, and API
// call count. Ranking is order-of-first-appearance across the four sources, so
// overall-top entries outrank per-era entries, which outrank velocity and
// saturation entries — matters when the result is capped at
// maxCharactersPerRun.
func (s *DHAnalyticsRefreshScheduler) refreshCharacters(ctx context.Context) (
	characters map[string]struct{},
	topCardIDs []int,
	apiCalls int,
) {
	characters = make(map[string]struct{})
	orderedCharacters := make([]string, 0, maxCharactersPerRun)
	addCharacter := func(name string) {
		if name == "" {
			return
		}
		if _, seen := characters[name]; seen {
			return
		}
		characters[name] = struct{}{}
		orderedCharacters = append(orderedCharacters, name)
	}
	cardIDSet := make(map[int]struct{})

	// 1a. Top characters overall.
	apiCalls++
	overallTop, err := s.dhClient.TopCharacters(ctx, topCharactersOverallLimit, "")
	if err != nil && !errors.Is(err, dh.ErrAnalyticsNotComputed) {
		s.logger.Warn(ctx, "top_characters overall failed", observability.Err(err))
	}
	var overallEntries []dh.CharacterDemandEntry
	if overallTop != nil {
		overallEntries = overallTop.CharacterDemand
		for _, e := range overallEntries {
			addCharacter(e.CharacterName)
		}
	}

	// 1b. Top characters per era.
	//
	// NOTE: TopCharactersResponse.CharacterDemand entries do not include a
	// top_cards field in our wire types today (see types_analytics.go) — the
	// `top_cards` attribute lives on the DH response but hasn't been typed on
	// CharacterDemandEntry. When T2 exposes that field the loop below will
	// start producing card IDs. Until then, step 2 is seeded by unsold
	// inventory only, which is the intended Phase-1 behavior anyway.
	eraEntries := make([]dh.CharacterDemandEntry, 0, len(defaultAnalyticsEras)*topCharactersPerEraLimit)
	for _, era := range defaultAnalyticsEras {
		apiCalls++
		resp, eraErr := s.dhClient.TopCharacters(ctx, topCharactersPerEraLimit, era)
		if eraErr != nil {
			if !errors.Is(eraErr, dh.ErrAnalyticsNotComputed) {
				s.logger.Warn(ctx, "top_characters per-era failed",
					observability.String("era", era),
					observability.Err(eraErr))
			}
			continue
		}
		if resp == nil {
			continue
		}
		for _, e := range resp.CharacterDemand {
			addCharacter(e.CharacterName)
			eraEntries = append(eraEntries, e)
		}
	}

	// 1c. Velocity + saturation page 1.
	apiCalls++
	velResp, err := s.dhClient.CharacterVelocity(ctx, dh.CharacterListOpts{
		Page:    1,
		PerPage: characterListPerPage,
	})
	if err != nil && !errors.Is(err, dh.ErrAnalyticsNotComputed) {
		s.logger.Warn(ctx, "character_velocity failed", observability.Err(err))
	}
	var velocityEntries []dh.CharacterVelocityEntry
	if velResp != nil {
		velocityEntries = velResp.Characters
		for _, e := range velocityEntries {
			addCharacter(e.CharacterName)
		}
	}

	apiCalls++
	satResp, err := s.dhClient.CharacterSaturation(ctx, dh.CharacterListOpts{
		Page:    1,
		PerPage: characterListPerPage,
	})
	if err != nil && !errors.Is(err, dh.ErrAnalyticsNotComputed) {
		s.logger.Warn(ctx, "character_saturation failed", observability.Err(err))
	}
	var saturationEntries []dh.CharacterSaturationEntry
	if satResp != nil {
		saturationEntries = satResp.Characters
		for _, e := range saturationEntries {
			addCharacter(e.CharacterName)
		}
	}

	// Cap character set size. orderedCharacters preserves order-of-first-
	// appearance across the four sources (overall → era → velocity →
	// saturation), so a prefix trim keeps DH's highest-ranked entries.
	if len(orderedCharacters) > maxCharactersPerRun {
		total := len(orderedCharacters)
		orderedCharacters = orderedCharacters[:maxCharactersPerRun]
		trimmed := make(map[string]struct{}, maxCharactersPerRun)
		for _, name := range orderedCharacters {
			trimmed[name] = struct{}{}
		}
		s.logger.Info(ctx, "character set capped",
			observability.Int("total", total),
			observability.Int("kept", maxCharactersPerRun))
		characters = trimmed
	}

	for id := range cardIDSet {
		topCardIDs = append(topCardIDs, id)
	}

	now := time.Now()
	demandByChar := indexDemand(append(overallEntries, eraEntries...))
	velocityByChar := indexVelocity(velocityEntries)
	saturationByChar := indexSaturation(saturationEntries)

	for _, name := range orderedCharacters {
		row := demand.CharacterCache{
			Character: name,
			Window:    s.config.Window,
			FetchedAt: now,
		}
		if entry, ok := demandByChar[name]; ok {
			if blob, encErr := json.Marshal(entry); encErr == nil {
				str := string(blob)
				row.DemandJSON = &str
			}
		}
		if entry, ok := velocityByChar[name]; ok {
			if blob, encErr := json.Marshal(entry.Velocity); encErr == nil {
				str := string(blob)
				row.VelocityJSON = &str
			}
			if t, tErr := parseDHTimestamp(entry.ComputedAt); tErr == nil {
				row.AnalyticsComputedAt = &t
			}
		}
		if entry, ok := saturationByChar[name]; ok {
			if blob, encErr := json.Marshal(entry); encErr == nil {
				str := string(blob)
				row.SaturationJSON = &str
			}
			if t, tErr := parseDHTimestamp(entry.ComputedAt); tErr == nil {
				row.AnalyticsComputedAt = &t
			}
		}
		if err := s.repo.UpsertCharacterCache(ctx, row); err != nil {
			s.logger.Warn(ctx, "upsert character cache failed",
				observability.String("character", name),
				observability.Err(err))
		}
	}

	return characters, topCardIDs, apiCalls
}

// buildCardSeed is the Step-2 seed: union of our unsold dh_card_ids and the
// top_cards surfaced by Step 1. Capped at maxSeedCardIDs.
func (s *DHAnalyticsRefreshScheduler) buildCardSeed(ctx context.Context, hotIDs []int) []int {
	seen := make(map[int]struct{}, len(hotIDs))
	for _, id := range hotIDs {
		if id > 0 {
			seen[id] = struct{}{}
		}
	}
	if s.cardLister != nil {
		invIDs, err := s.cardLister.ListUnsoldDHCardIDs(ctx)
		if err != nil {
			s.logger.Warn(ctx, "list unsold dh card ids failed", observability.Err(err))
		} else {
			for _, id := range invIDs {
				if id > 0 {
					seen[id] = struct{}{}
				}
			}
		}
	}
	ids := make([]int, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
		if len(ids) >= maxSeedCardIDs {
			break
		}
	}
	return ids
}

// refreshCards runs Step 2: BatchAnalytics + DemandSignals, upserts per-card
// cache rows, returns 404 analytics_not_computed count and total API call count.
func (s *DHAnalyticsRefreshScheduler) refreshCards(ctx context.Context, cardIDs []int) (notComputed int, apiCalls int) {
	if len(cardIDs) == 0 {
		s.logger.Info(ctx, "no card IDs to refresh")
		return 0, 0
	}

	// --- BatchAnalytics ---
	apiCalls++
	analyticsResp, err := s.dhClient.BatchAnalytics(ctx, cardIDs, []string{"velocity", "trend", "saturation", "price_distribution"})
	if err != nil {
		s.logger.Warn(ctx, "batch_analytics failed", observability.Err(err))
	}

	now := time.Now()
	analyticsByID := make(map[int]dh.CardAnalytics)
	if analyticsResp != nil {
		for _, row := range analyticsResp.Results {
			if row.Error != "" {
				if row.Error == "analytics_not_computed" {
					notComputed++
					continue
				}
				s.logger.Debug(ctx, "per-card analytics error",
					observability.Int("card_id", row.CardID),
					observability.String("dh_error", row.Error))
				continue
			}
			analyticsByID[row.CardID] = row
		}
	}

	for cardID, row := range analyticsByID {
		cache := demand.CardCache{
			CardID:    strconv.Itoa(cardID),
			Window:    s.config.Window,
			FetchedAt: now,
		}
		if row.Velocity != nil {
			if blob, encErr := json.Marshal(row.Velocity); encErr == nil {
				str := string(blob)
				cache.VelocityJSON = &str
			}
		}
		if row.Trend != nil {
			if blob, encErr := json.Marshal(row.Trend); encErr == nil {
				str := string(blob)
				cache.TrendJSON = &str
			}
		}
		if row.Saturation != nil {
			if blob, encErr := json.Marshal(row.Saturation); encErr == nil {
				str := string(blob)
				cache.SaturationJSON = &str
			}
		}
		if row.PriceDistribution != nil {
			if blob, encErr := json.Marshal(row.PriceDistribution); encErr == nil {
				str := string(blob)
				cache.PriceDistributionJSON = &str
			}
		}
		if t, tErr := parseDHTimestamp(row.ComputedAt); tErr == nil {
			cache.AnalyticsComputedAt = &t
		}
		if err := s.repo.UpsertCardCache(ctx, cache); err != nil {
			s.logger.Warn(ctx, "upsert card cache (analytics) failed",
				observability.Int("card_id", cardID),
				observability.Err(err))
		}
	}

	// --- DemandSignals ---
	apiCalls++
	demandResp, err := s.dhClient.DemandSignals(ctx, cardIDs, s.config.Window)
	if err != nil {
		s.logger.Warn(ctx, "demand_signals failed", observability.Err(err))
		return notComputed, apiCalls
	}
	if demandResp == nil {
		return notComputed, apiCalls
	}

	for _, ds := range demandResp.DemandSignals {
		cardID := ds.CardID
		cache, getErr := s.repo.GetCardCache(ctx, strconv.Itoa(cardID), s.config.Window)
		if getErr != nil {
			s.logger.Debug(ctx, "get card cache failed (pre-merge)",
				observability.Int("card_id", cardID),
				observability.Err(getErr))
		}
		if cache == nil {
			cache = &demand.CardCache{
				CardID:    strconv.Itoa(cardID),
				Window:    s.config.Window,
				FetchedAt: now,
			}
		}
		score := ds.DemandScore
		cache.DemandScore = &score
		quality := ds.DataQuality
		cache.DemandDataQuality = &quality
		if blob, encErr := json.Marshal(ds); encErr == nil {
			str := string(blob)
			cache.DemandJSON = &str
		}
		cache.DemandComputedAt = &now
		cache.FetchedAt = now
		if err := s.repo.UpsertCardCache(ctx, *cache); err != nil {
			s.logger.Warn(ctx, "upsert card cache (demand) failed",
				observability.Int("card_id", cardID),
				observability.Err(err))
		}
	}
	return notComputed, apiCalls
}

// --- helpers ---

func indexDemand(entries []dh.CharacterDemandEntry) map[string]dh.CharacterDemandEntry {
	m := make(map[string]dh.CharacterDemandEntry, len(entries))
	for _, e := range entries {
		if e.CharacterName == "" {
			continue
		}
		// Later entries (per-era) overwrite earlier (overall) so the cached
		// blob carries by_era when available.
		m[e.CharacterName] = e
	}
	return m
}

func indexVelocity(entries []dh.CharacterVelocityEntry) map[string]dh.CharacterVelocityEntry {
	m := make(map[string]dh.CharacterVelocityEntry, len(entries))
	for _, e := range entries {
		if e.CharacterName == "" {
			continue
		}
		m[e.CharacterName] = e
	}
	return m
}

func indexSaturation(entries []dh.CharacterSaturationEntry) map[string]dh.CharacterSaturationEntry {
	m := make(map[string]dh.CharacterSaturationEntry, len(entries))
	for _, e := range entries {
		if e.CharacterName == "" {
			continue
		}
		m[e.CharacterName] = e
	}
	return m
}

// parseDHTimestamp parses the `computed_at` ISO-8601 strings DH returns.
func parseDHTimestamp(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty timestamp")
	}
	return time.Parse(time.RFC3339, s)
}
