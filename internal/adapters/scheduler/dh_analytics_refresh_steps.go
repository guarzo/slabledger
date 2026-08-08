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

// refreshCharacters runs the character step: union characters from
// top-characters (overall + per era), velocity, saturation, then upserts
// character cache rows. Returns the (ranked) character-name set and API call
// count. Ranking is order-of-first-appearance across the four sources, so
// overall-top entries outrank per-era entries, which outrank velocity and
// saturation entries — matters when the result is capped at
// maxCharactersPerRun.
//
// qualityByChar carries the character-level data quality aggregated from the
// card step, keyed by character name. DH never reports data_quality on a
// character, so this is the only source for it; characters absent from the map
// get no quality rather than a guessed one.
func (s *DHAnalyticsRefreshScheduler) refreshCharacters(ctx context.Context, qualityByChar map[string]string) (
	characters map[string]struct{},
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

	// 1a. Top characters overall.
	apiCalls++
	overallTop, err := s.dhClient.TopCharacters(ctx, topCharactersOverallLimit, "")
	if err != nil && !errors.Is(err, dh.ErrAnalyticsNotComputed) {
		s.logger.Warn(ctx, "top_characters overall failed", observability.Err(err))
	}
	var overallEntries []stampedDemandEntry
	if overallTop != nil {
		for _, e := range overallTop.CharacterDemand {
			addCharacter(e.CharacterName)
			overallEntries = append(overallEntries, stampedDemandEntry{Entry: e, ComputedAt: overallTop.ComputedAt})
		}
	}

	// 1b. Top characters per era.
	eraEntries := make([]stampedDemandEntry, 0, len(defaultAnalyticsEras)*topCharactersPerEraLimit)
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
			eraEntries = append(eraEntries, stampedDemandEntry{Entry: e, ComputedAt: resp.ComputedAt})
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
			row.Demand = mapCharacterDemand(entry, qualityByChar[name])
			if t, tErr := parseDHTimestamp(entry.ComputedAt); tErr == nil {
				row.DemandComputedAt = &t
			}
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

	return characters, apiCalls
}

// buildCardSeed is the card step's seed: our unsold dh_card_ids, capped at
// maxSeedCardIDs.
//
// It used to also union the "top cards" of each hot character. That path was
// dead: DH's top_characters response carries no top_cards field on any variant
// (verified against the live API on 2026-08-08 for the overall, per-era, and
// by_era forms), so the set it contributed was always empty. Removing it also
// removes the card step's dependency on the character step, which is what lets
// cards run first and feed character data quality in the same run (SLA-63).
func (s *DHAnalyticsRefreshScheduler) buildCardSeed(ctx context.Context) []int {
	if s.cardLister == nil {
		return nil
	}
	invIDs, err := s.cardLister.ListUnsoldDHCardIDs(ctx)
	if err != nil {
		s.logger.Warn(ctx, "list unsold dh card ids failed", observability.Err(err))
		return nil
	}
	seen := make(map[int]struct{}, len(invIDs))
	ids := make([]int, 0, len(invIDs))
	for _, id := range invIDs {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
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

// stampedDemandEntry pairs a character demand entry with the computed_at of
// the TopCharacters response it arrived in. DH reports computed_at once per
// response rather than per entry, so the timestamp has to be carried alongside
// the entry to survive the overall/per-era merge in indexDemand.
type stampedDemandEntry struct {
	Entry      dh.CharacterDemandEntry
	ComputedAt string
}

// mapCharacterDemand converts a DH character-demand entry into the domain
// payload persisted on CharacterCache.Demand.
//
// dataQuality does not come from DH: data_quality is reported per card
// (dh.DemandSignal) and never per character. It is rolled up by
// demand.AggregateCharacterQuality over the cards we have attributed, and is
// left "" when we have attributed none of a character's cards — an empty
// value reads as "unknown" downstream, which is correct, whereas defaulting
// to "proxy" or "full" would assert something we did not measure.
//
// ComputedAt is the response-level timestamp carried on the stamped entry, so
// the stored payload always describes the response it came from.
func mapCharacterDemand(stamped stampedDemandEntry, dataQuality string) *demand.CharacterDemand {
	entry := stamped.Entry
	out := &demand.CharacterDemand{
		CharacterName:     entry.CharacterName,
		CardCount:         entry.CardCount,
		AvgDemandScore:    entry.AvgDemandScore,
		TotalViews:        entry.TotalViews,
		TotalSearchClicks: entry.TotalSearchClicks,
		TotalWishlistAdds: entry.TotalWishlistAdds,
		DataQuality:       dataQuality,
		ComputedAt:        stamped.ComputedAt,
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
// domain payload. active_listing_count is written flat here and parsed flat
// by the reader; reading entry.Saturation.ActiveListingCount directly is an
// explicit assignment the compiler checks, so the nested wire shape never
// reaches storage.
func mapCharacterSaturation(entry dh.CharacterSaturationEntry) *demand.CharacterSaturation {
	return &demand.CharacterSaturation{
		ActiveListingCount: entry.Saturation.ActiveListingCount,
		ComputedAt:         entry.ComputedAt,
	}
}

func indexDemand(entries []stampedDemandEntry) map[string]stampedDemandEntry {
	m := make(map[string]stampedDemandEntry, len(entries))
	for _, e := range entries {
		if e.Entry.CharacterName == "" {
			continue
		}
		// Later entries (per-era) overwrite earlier (overall) so the persisted
		// CharacterDemand carries ByEra when available. The entry's own
		// computed_at travels with it, so the stored timestamp always
		// describes the stored payload.
		m[e.Entry.CharacterName] = e
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
