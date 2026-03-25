package scheduler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"
	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/constants"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// discoverUnmappedCards uses CardHedger's card-match endpoint to find priority
// cards that don't have a mapping yet. Each successful match creates a mapping
// so the card will be included in future batch runs even if discovery budget
// is exhausted. Deducts API calls from *budget. Returns newly discovered mappings.
func (s *CardHedgerBatchScheduler) discoverUnmappedCards(
	ctx context.Context,
	existingMappings []CardIDMapping,
	priorityCards map[string]bool,
	priorityCardNumbers map[string]string,
	budget *int,
) []CardIDMapping {
	if s.mappingSaver == nil || len(priorityCards) == 0 {
		return nil
	}

	// Build set of already-mapped card keys
	mapped := make(map[string]bool, len(existingMappings))
	for _, m := range existingMappings {
		mapped[cardKey(m.CardName, m.SetName, m.CollectorNumber)] = true
	}

	// Collect unmapped priority cards (skip generic set names)
	type unmappedCard struct {
		name       string
		setName    string
		cardNumber string
	}
	var unmapped []unmappedCard
	for key := range priorityCards {
		// Parse "cardName|setName" (2-part) or "cardName|setName|cardNumber" (3-part)
		parts := strings.SplitN(key, "|", 3)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		setName := parts[1]
		var cardNumber string
		if len(parts) == 3 {
			cardNumber = parts[2]
		}
		if name == "" || isGenericSetForDiscovery(setName) {
			continue
		}
		if cardNumber == "" {
			cardNumber = priorityCardNumbers[key]
		}
		if mapped[cardKey(name, setName, cardNumber)] {
			continue
		}
		unmapped = append(unmapped, unmappedCard{name: name, setName: setName, cardNumber: cardNumber})
	}

	if len(unmapped) == 0 {
		return nil
	}

	// Cap discovery at 20% of budget or 50 searches, whichever is smaller
	maxDiscovery := min(*budget/5, 50)
	if maxDiscovery <= 0 {
		return nil
	}
	if len(unmapped) > maxDiscovery {
		unmapped = unmapped[:maxDiscovery]
	}

	s.logger.Info(ctx, "cardhedger batch: discovering unmapped cards",
		observability.Int("unmapped_count", len(unmapped)))

	category := s.config.Category
	if category == "" {
		category = "Pokemon"
	}

	var discovered []CardIDMapping
	for _, card := range unmapped {
		select {
		case <-ctx.Done():
			return discovered
		case <-s.stopChan:
			return discovered
		default:
		}

		*budget-- // Each card-match costs 1 API call
		externalID, ok := s.discoverCard(ctx, card.name, card.setName, card.cardNumber, category)
		if !ok {
			if !s.rateLimitedSleep(ctx) {
				return discovered
			}
			continue
		}

		discovered = append(discovered, CardIDMapping{
			CardName:        card.name,
			SetName:         card.setName,
			CollectorNumber: card.cardNumber,
			ExternalID:      externalID,
		})

		if !s.rateLimitedSleep(ctx) {
			return discovered
		}
	}

	return discovered
}

// isGenericSetForDiscovery delegates to the centralized generic set check.
func isGenericSetForDiscovery(set string) bool {
	return constants.IsGenericSetName(set)
}

// cardKey returns a composite lookup key for 3-part card deduplication.
func cardKey(name, set, number string) string {
	return name + "|" + set + "|" + number
}

// cardBaseKey returns a composite lookup key for 2-part card deduplication.
func cardBaseKey(name, set string) string {
	return name + "|" + set
}

// rateLimitedSleep pauses for cardHedgerRateInterval, returning false if
// ctx is cancelled or stopChan is closed.
func (s *CardHedgerBatchScheduler) rateLimitedSleep(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-s.stopChan:
		return false
	case <-time.After(cardHedgerRateInterval):
		return true
	}
}

// discoverCard performs a single CardMatch + confidence check + optional cache
// for one card. Returns the external ID and true if a usable match was found.
func (s *CardHedgerBatchScheduler) discoverCard(ctx context.Context, name, setName, cardNumber, category string) (externalID string, ok bool) {
	query := cardutil.BuildCardMatchQuery(setName, name, cardNumber)

	resp, _, _, err := s.client.CardMatch(ctx, query, category, 10)
	if err != nil || resp == nil || resp.Match == nil {
		s.logger.Debug(ctx, "cardhedger discovery: no match",
			observability.String("card", name),
			observability.String("set", setName))
		reason := "no_match"
		if err != nil {
			reason = "api_error"
		}
		s.recordDiscoveryFailure(ctx, name, setName, cardNumber, reason, query)
		return "", false
	}

	confidence := resp.Match.Confidence

	s.logger.Info(ctx, "cardhedger discovery: card-match result",
		observability.String("card", name),
		observability.String("set", setName),
		observability.String("card_id", resp.Match.CardID),
		observability.Float64("confidence", confidence))

	if cardhedger.ShouldRejectMatch(confidence) {
		s.logger.Debug(ctx, "cardhedger discovery: confidence too low",
			observability.String("card", name),
			observability.String("set", setName),
			observability.Float64("confidence", confidence))
		s.recordDiscoveryFailure(ctx, name, setName, cardNumber, "low_confidence", query)
		return "", false
	}

	externalID = resp.Match.CardID

	// Cache only high-confidence matches; moderate-confidence matches
	// still get priced in this batch but aren't persisted for future runs.
	if cardhedger.ShouldCacheMatch(confidence) && s.mappingSaver != nil {
		if err := s.mappingSaver.SaveExternalID(ctx, name, setName, cardNumber, "cardhedger", externalID); err != nil {
			s.logger.Warn(ctx, "cardhedger discovery: failed to save mapping",
				observability.String("card", name),
				observability.Err(err))
		} else {
			s.logger.Info(ctx, "cardhedger discovery: mapped new card",
				observability.String("card", name),
				observability.String("set", setName),
				observability.String("external_id", externalID))
			s.clearDiscoveryFailure(ctx, name, setName, cardNumber)
		}
	} else if !cardhedger.ShouldCacheMatch(confidence) {
		s.logger.Debug(ctx, "cardhedger discovery: moderate confidence, skipping cache",
			observability.String("card", name),
			observability.String("set", setName),
			observability.Float64("confidence", confidence))
	}

	return externalID, true
}

// recordAPICall asynchronously records an API call for dashboard visibility.
// Runs in a background goroutine so it cannot stall the batch loop.
func (s *CardHedgerBatchScheduler) recordAPICall(endpoint string, statusCode int, err error, latencyMS int64) {
	if s.apiTracker == nil {
		return
	}
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	sc := statusCode
	if sc == 0 && err != nil {
		sc = 500
	}
	if sc == 0 && err == nil {
		sc = 200
	}
	ts := time.Now()
	go func() {
		ctxTrack, cancelTrack := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancelTrack()
		//nolint:errcheck // best-effort tracking
		s.apiTracker.RecordAPICall(ctxTrack, &pricing.APICallRecord{
			Provider:   "cardhedger",
			Endpoint:   endpoint,
			StatusCode: sc,
			Error:      errMsg,
			LatencyMS:  latencyMS,
			Timestamp:  ts,
		})
	}()
}

// storePrices converts CardHedger all-prices response to PriceEntry records and stores them.
func (s *CardHedgerBatchScheduler) storePrices(ctx context.Context, cardName, setName, collectorNumber string, resp *cardhedger.AllPricesByCardResponse) int {
	// Normalize card name so batch-stored prices match names used by GetPrice/supplementCardHedgerFromDB
	originalCardName := cardName
	cardName = cardutil.NormalizePurchaseName(cardName)
	if cardName == "" {
		s.logger.Debug(ctx, "cardhedger batch: skipping empty card name after normalization",
			observability.String("original_name", originalCardName),
			observability.String("original_set", setName))
		return 0
	}

	stored := 0
	now := time.Now()

	for _, gp := range resp.Prices {
		if !isKnownCardHedgerGrade(gp.Grade) {
			continue
		}

		price, err := strconv.ParseFloat(gp.Price, 64)
		if err != nil || price <= 0 {
			continue
		}

		entry := &pricing.PriceEntry{
			CardName:          cardName,
			SetName:           setName,
			Grade:             gp.Grade,
			PriceCents:        mathutil.ToCents(price),
			Confidence:        0.85,
			Source:            "cardhedger",
			FusionSourceCount: 1,
			FusionMethod:      "batch",
			PriceDate:         now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		entry.CardNumber = collectorNumber

		if err := s.priceRepo.StorePrice(ctx, entry); err != nil {
			s.logger.Debug(ctx, "failed to store batch price",
				observability.String("card", cardName),
				observability.String("grade", gp.Grade),
				observability.Err(err))
			continue
		}
		stored++
	}

	return stored
}

// DiscoverAndPrice discovers and fetches CardHedger prices for specific cards.
// Used after imports to immediately populate CardHedger data for newly imported cards.
// Caps throughput to 100 cards per invocation.
func (s *CardHedgerBatchScheduler) DiscoverAndPrice(ctx context.Context, cards []campaigns.CardIdentity) (discovered, priced int) {
	if s.client == nil || !s.client.Available() {
		return 0, 0
	}
	if len(cards) == 0 {
		return 0, 0
	}

	// Cap to a reasonable throughput limit per invocation
	maxCards := 100
	if len(cards) > maxCards {
		cards = cards[:maxCards]
	}

	// Load existing mappings to skip already-mapped cards
	existingMappings, err := s.mappingLister.ListByProvider(ctx, "cardhedger")
	if err != nil {
		s.logger.Warn(ctx, "cardhedger post-import: failed to list mappings", observability.Err(err))
		return 0, 0
	}
	mapped := make(map[string]string, len(existingMappings))
	for _, m := range existingMappings {
		mapped[cardKey(m.CardName, m.SetName, m.CollectorNumber)] = m.ExternalID
	}

	category := s.config.Category
	if category == "" {
		category = "Pokemon"
	}

	// Track processed keys to avoid duplicate API calls for identical cards.
	seen := make(map[string]bool)

	for _, card := range cards {
		if ctx.Err() != nil {
			break
		}
		if card.CardName == "" || isGenericSetForDiscovery(card.SetName) {
			continue
		}

		key := cardKey(card.CardName, card.SetName, card.CardNumber)
		if seen[key] {
			continue
		}
		seen[key] = true
		externalID := mapped[key]

		// Discover if not mapped
		if externalID == "" {
			foundID, ok := s.discoverCard(ctx, card.CardName, card.SetName, card.CardNumber, category)
			if !ok {
				if !s.rateLimitedSleep(ctx) {
					return discovered, priced
				}
				continue
			}
			externalID = foundID
			mapped[key] = externalID
			discovered++

			// Brief pause between API calls
			if !s.rateLimitedSleep(ctx) {
				return discovered, priced
			}
		}

		// Fetch prices
		resp, statusCode, _, err := s.client.GetAllPrices(ctx, externalID)
		if err != nil || resp == nil {
			s.recordAPICall("post-import/all-prices", statusCode, err, 0)
			if !s.rateLimitedSleep(ctx) {
				return discovered, priced
			}
			continue
		}

		s.recordAPICall("post-import/all-prices", statusCode, nil, 0)

		count := s.storePrices(ctx, card.CardName, card.SetName, card.CardNumber, resp)
		if count > 0 {
			priced++
		}

		// Brief pause between API calls
		if !s.rateLimitedSleep(ctx) {
			return discovered, priced
		}
	}

	if discovered > 0 || priced > 0 {
		s.logger.Info(ctx, "cardhedger post-import complete",
			observability.Int("cards", len(cards)),
			observability.Int("discovered", discovered),
			observability.Int("priced", priced))
	}

	return discovered, priced
}

// recordDiscoveryFailure persists a discovery failure if a tracker is configured.
func (s *CardHedgerBatchScheduler) recordDiscoveryFailure(ctx context.Context, cardName, setName, cardNumber, reason, query string) {
	if s.failureTracker == nil {
		return
	}
	//nolint:errcheck // best-effort tracking
	s.failureTracker.RecordDiscoveryFailure(ctx, &pricing.DiscoveryFailure{
		CardName:      cardName,
		SetName:       setName,
		CardNumber:    cardNumber,
		Provider:      "cardhedger",
		FailureReason: reason,
		Query:         query,
	})
}

// clearDiscoveryFailure removes a discovery failure record when a card is successfully mapped.
func (s *CardHedgerBatchScheduler) clearDiscoveryFailure(ctx context.Context, cardName, setName, cardNumber string) {
	if s.failureTracker == nil {
		return
	}
	//nolint:errcheck // best-effort tracking
	s.failureTracker.ClearDiscoveryFailure(ctx, cardName, setName, cardNumber, "cardhedger")
}
