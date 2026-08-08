package scheduler

import (
	"context"
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

	// 1c. Velocity + saturation — walk pages until we've seen total_count
	// rows, an empty page, or we hit characterListMaxPages. Without this,
	// only the top per_page=50 entries from each leaderboard get analytics
	// hydrated; characters ranked further down were silently missed and
	// caused our coverage diff with DH (~9% on our cache vs ~44% system-wide).
	velocityEntries, velCalls := s.fetchAllVelocity(ctx)
	apiCalls += velCalls
	for _, e := range velocityEntries {
		addCharacter(e.CharacterName)
	}

	saturationEntries, satCalls := s.fetchAllSaturation(ctx)
	apiCalls += satCalls
	for _, e := range saturationEntries {
		addCharacter(e.CharacterName)
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
			row.Demand = mapCharacterDemand(entry)
		}
		if entry, ok := velocityByChar[name]; ok {
			row.Velocity = mapCharacterVelocity(entry.Velocity)
			if t, tErr := parseDHTimestamp(entry.ComputedAt); tErr == nil {
				row.AnalyticsComputedAt = &t
			}
		}
		if entry, ok := saturationByChar[name]; ok {
			row.Saturation = mapCharacterSaturation(entry)
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

// mapCharacterDemand converts a DH character-demand entry into the domain
// payload persisted on CharacterCache.Demand. dh.CharacterDemandEntry has no
// computed_at or data_quality field, so both are left "" on the mapped
// struct — identical to what today's json.Marshal(entry) blob persists.
func mapCharacterDemand(entry dh.CharacterDemandEntry) *demand.CharacterDemand {
	out := &demand.CharacterDemand{
		CharacterName:     entry.CharacterName,
		CardCount:         entry.CardCount,
		AvgDemandScore:    entry.AvgDemandScore,
		TotalViews:        entry.TotalViews,
		TotalSearchClicks: entry.TotalSearchClicks,
		TotalWishlistAdds: entry.TotalWishlistAdds,
	}
	if len(entry.ByEra) > 0 {
		out.ByEra = make(map[string]demand.ByEraDemand, len(entry.ByEra))
		for era, e := range entry.ByEra {
			out.ByEra[era] = demand.ByEraDemand{
				CardCount:         e.CardCount,
				AvgDemandScore:    e.AvgDemandScore,
				TotalViews:        e.TotalViews,
				TotalSearchClicks: e.TotalSearchClicks,
				TotalWishlistAdds: e.TotalWishlistAdds,
			}
		}
	}
	return out
}

// mapCharacterVelocity converts DH's flat velocity block into the domain
// payload. AvgDaysToSell and SellThrough are the two accepted losses (see
// the SLA-41 design doc, "Accepted losses") — nothing in the domain reads
// them, so they are not carried into demand.CharacterVelocity.
func mapCharacterVelocity(v dh.CharacterVelocityFields) *demand.CharacterVelocity {
	out := &demand.CharacterVelocity{
		MedianDaysToSell:   v.MedianDaysToSell,
		SampleSize:         v.SampleSize,
		VelocityChangePct:  v.VelocityChangePct,
		AvgDailySales:      v.AvgDailySales,
		SellThroughRate30d: v.SellThroughRate30d,
		SalesVolume7d:      v.SalesVolume7d,
		SalesVolume30d:     v.SalesVolume30d,
		SupplyCount:        v.SupplyCount,
	}
	if len(v.ByGrade) > 0 {
		out.ByGrade = make(map[string]demand.VelocityTierStat, len(v.ByGrade))
		for tier, stat := range v.ByGrade {
			out.ByGrade[tier] = demand.VelocityTierStat{MedianDays: stat.MedianDays, SampleSize: stat.SampleSize}
		}
	}
	if len(v.ByPriceTier) > 0 {
		out.ByPriceTier = make(map[string]demand.VelocityTierStat, len(v.ByPriceTier))
		for tier, stat := range v.ByPriceTier {
			out.ByPriceTier[tier] = demand.VelocityTierStat{MedianDays: stat.MedianDays, SampleSize: stat.SampleSize}
		}
	}
	return out
}

// mapCharacterSaturation converts DH's nested saturation entry into the flat
// domain payload. This is where Defect 1 dies: reading
// entry.Saturation.ActiveListingCount directly is an explicit assignment the
// compiler checks, and there is no marshal depth left to get wrong.
func mapCharacterSaturation(entry dh.CharacterSaturationEntry) *demand.CharacterSaturation {
	return &demand.CharacterSaturation{
		ActiveListingCount: entry.Saturation.ActiveListingCount,
		ComputedAt:         entry.ComputedAt,
	}
}

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

// fetchAllVelocity walks /characters/velocity pages until total_count is
// reached, an empty page is returned, or characterListMaxPages is hit. Returns
// the accumulated entries and the number of API calls made (always ≥1 even on
// error so the caller's call counter stays accurate).
func (s *DHAnalyticsRefreshScheduler) fetchAllVelocity(ctx context.Context) ([]dh.CharacterVelocityEntry, int) {
	var out []dh.CharacterVelocityEntry
	calls := 0
	for page := 1; page <= characterListMaxPages; page++ {
		calls++
		resp, err := s.dhClient.CharacterVelocity(ctx, dh.CharacterListOpts{
			Page:    page,
			PerPage: characterListPerPage,
		})
		if err != nil {
			if !errors.Is(err, dh.ErrAnalyticsNotComputed) {
				s.logger.Warn(ctx, "character_velocity failed",
					observability.Int("page", page),
					observability.Err(err))
			}
			break
		}
		if resp == nil || len(resp.Characters) == 0 {
			break
		}
		out = append(out, resp.Characters...)
		if resp.Pagination.TotalCount > 0 && len(out) >= resp.Pagination.TotalCount {
			break
		}
	}
	if len(out) > 0 {
		s.logger.Info(ctx, "character_velocity fetched",
			observability.Int("entries", len(out)),
			observability.Int("pages", calls))
	}
	return out, calls
}

// fetchAllSaturation walks /characters/saturation the same way as
// fetchAllVelocity. See that function's docstring for stop conditions.
func (s *DHAnalyticsRefreshScheduler) fetchAllSaturation(ctx context.Context) ([]dh.CharacterSaturationEntry, int) {
	var out []dh.CharacterSaturationEntry
	calls := 0
	for page := 1; page <= characterListMaxPages; page++ {
		calls++
		resp, err := s.dhClient.CharacterSaturation(ctx, dh.CharacterListOpts{
			Page:    page,
			PerPage: characterListPerPage,
		})
		if err != nil {
			if !errors.Is(err, dh.ErrAnalyticsNotComputed) {
				s.logger.Warn(ctx, "character_saturation failed",
					observability.Int("page", page),
					observability.Err(err))
			}
			break
		}
		if resp == nil || len(resp.Characters) == 0 {
			break
		}
		out = append(out, resp.Characters...)
		if resp.Pagination.TotalCount > 0 && len(out) >= resp.Pagination.TotalCount {
			break
		}
	}
	if len(out) > 0 {
		s.logger.Info(ctx, "character_saturation fetched",
			observability.Int("entries", len(out)),
			observability.Int("pages", calls))
	}
	return out, calls
}
