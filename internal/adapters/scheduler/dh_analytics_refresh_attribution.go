package scheduler

import (
	"context"
	"strconv"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// resolveCardCharacters attributes cached cards to their DH character and
// persists the mapping, then returns the character-level data quality rolled up
// across every attributed card in the window.
//
// The attribution comes from /character_demand seeded with a single card id:
// DH answers with that card's character. Seeding it with many ids at once
// aggregates them into one row per character and loses which card came from
// where, so this costs one call per card against a 1 rps client — hence the
// cache. A card's character does not change, so each card is paid for once and
// maxCharacterLookupsPerRun bounds how much of the backlog a single run works
// through. Rows already attributed are skipped by the query.
func (s *DHAnalyticsRefreshScheduler) resolveCardCharacters(ctx context.Context) (
	qualityByChar map[string]string,
	resolved int,
	apiCalls int,
) {
	pending, err := s.repo.ListCardsMissingCharacter(ctx, s.config.Window, maxCharacterLookupsPerRun)
	if err != nil {
		s.logger.Warn(ctx, "list cards missing character failed", observability.Err(err))
	}

	for _, cardID := range pending {
		if ctx.Err() != nil {
			break
		}
		id, convErr := strconv.Atoi(cardID)
		if convErr != nil {
			s.logger.Debug(ctx, "skipping non-numeric card id",
				observability.String("card_id", cardID))
			continue
		}

		apiCalls++
		resp, demandErr := s.dhClient.CharacterDemand(ctx, []int{id}, s.config.Window, false)
		if demandErr != nil {
			s.logger.Debug(ctx, "character_demand lookup failed",
				observability.Int("card_id", id),
				observability.Err(demandErr))
			continue
		}
		name := soleCharacterName(resp)
		if name == "" {
			continue
		}
		if s.persistCardCharacter(ctx, cardID, name) {
			resolved++
		}
	}

	rows, err := s.repo.ListCardCharacterQuality(ctx, s.config.Window)
	if err != nil {
		s.logger.Warn(ctx, "list card character quality failed", observability.Err(err))
		return map[string]string{}, resolved, apiCalls
	}
	return demand.AggregateCharacterQuality(rows), resolved, apiCalls
}

// persistCardCharacter writes the attribution onto the card's existing cache
// row. It reads the row first because UpsertCardCache writes every column, so
// building a minimal row here would blank the analytics and demand payloads
// the card step just stored. Reports whether the row was written.
func (s *DHAnalyticsRefreshScheduler) persistCardCharacter(ctx context.Context, cardID, character string) bool {
	cache, err := s.repo.GetCardCache(ctx, cardID, s.config.Window)
	if err != nil {
		s.logger.Debug(ctx, "get card cache failed (pre-attribution)",
			observability.String("card_id", cardID),
			observability.Err(err))
		return false
	}
	if cache == nil {
		// The row was listed as missing a character moments ago; if it is gone
		// now there is nothing to attribute and re-creating it would invent a
		// row with no demand or analytics data.
		return false
	}
	cache.CharacterName = &character
	if err := s.repo.UpsertCardCache(ctx, *cache); err != nil {
		s.logger.Warn(ctx, "upsert card cache (character) failed",
			observability.String("card_id", cardID),
			observability.Err(err))
		return false
	}
	return true
}

// soleCharacterName returns the character a single-card /character_demand
// response attributes the card to. A one-card seed normally yields exactly one
// entry; if DH returns several, the one covering the most cards wins, with the
// name as a tie-break so repeated runs agree. Returns "" when the response
// attributes nothing.
func soleCharacterName(resp *dh.CharacterDemandResponse) string {
	if resp == nil {
		return ""
	}
	best := ""
	bestCount := -1
	for _, e := range resp.CharacterDemand {
		if e.CharacterName == "" {
			continue
		}
		if e.CardCount > bestCount || (e.CardCount == bestCount && e.CharacterName < best) {
			best = e.CharacterName
			bestCount = e.CardCount
		}
	}
	return best
}
