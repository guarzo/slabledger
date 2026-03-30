package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/justtcg"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// JustTCGRefreshConfig controls the JustTCG NM price refresh scheduler.
type JustTCGRefreshConfig struct {
	Enabled      bool
	RunInterval  time.Duration // How often to run (default: 24h)
	DailyBudget  int           // Max API calls per day (default: 2000)
	RateInterval time.Duration // Pause between calls (default: 600ms)
}

// JustTCGRefreshScheduler refreshes JustTCG NM prices for unsold campaign cards.
// Phase 1: resolve unmapped cards via SearchCards + save mapping.
// Phase 2: batch fetch already-mapped cards via BatchLookup.
type JustTCGRefreshScheduler struct {
	StopHandle
	client        *justtcg.Client
	priceRepo     pricing.PriceRepository
	apiTracker    pricing.APITracker
	mappingLister CardIDMappingLister
	mappingSaver  CardIDMappingSaver
	campLister    CampaignCardLister
	logger        observability.Logger
	config        JustTCGRefreshConfig
}

// JustTCGRefreshOption configures optional dependencies on JustTCGRefreshScheduler.
type JustTCGRefreshOption func(*JustTCGRefreshScheduler)

// WithJustTCGAPITracker injects an APITracker for recording API calls.
func WithJustTCGAPITracker(t pricing.APITracker) JustTCGRefreshOption {
	return func(s *JustTCGRefreshScheduler) { s.apiTracker = t }
}

// NewJustTCGRefreshScheduler creates a new JustTCG NM price refresh scheduler.
func NewJustTCGRefreshScheduler(
	client *justtcg.Client,
	priceRepo pricing.PriceRepository,
	mappingLister CardIDMappingLister,
	mappingSaver CardIDMappingSaver,
	campLister CampaignCardLister,
	logger observability.Logger,
	config JustTCGRefreshConfig,
	opts ...JustTCGRefreshOption,
) *JustTCGRefreshScheduler {
	if config.RunInterval <= 0 {
		config.RunInterval = 24 * time.Hour
	}
	if config.DailyBudget <= 0 {
		config.DailyBudget = 2000
	}
	if config.RateInterval <= 0 {
		config.RateInterval = 600 * time.Millisecond
	}
	s := &JustTCGRefreshScheduler{
		StopHandle:    NewStopHandle(),
		client:        client,
		priceRepo:     priceRepo,
		mappingLister: mappingLister,
		mappingSaver:  mappingSaver,
		campLister:    campLister,
		logger:        logger.With(context.Background(), observability.String("component", "justtcg-refresh")),
		config:        config,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start begins the refresh loop.
func (s *JustTCGRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "justtcg refresh scheduler disabled")
		return
	}
	if s.client == nil || !s.client.Available() {
		s.logger.Info(ctx, "justtcg refresh scheduler skipped: client not configured")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:         "justtcg-refresh",
		Interval:     s.config.RunInterval,
		InitialDelay: 2 * time.Minute,
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
		LogFields: []observability.Field{
			observability.Int("daily_budget", s.config.DailyBudget),
		},
	}, func(ctx context.Context) {
		s.runBatch(ctx)
	})
}

// runBatch collects unsold campaign cards, resolves JustTCG IDs, and fetches NM prices.
func (s *JustTCGRefreshScheduler) runBatch(ctx context.Context) {
	start := time.Now()

	// Collect unsold campaign cards and deduplicate by identity key.
	cards, err := s.campLister.ListUnsoldCards(ctx)
	if err != nil {
		s.logger.Warn(ctx, "justtcg refresh: failed to list campaign cards", observability.Err(err))
		return
	}
	if len(cards) == 0 {
		s.logger.Debug(ctx, "justtcg refresh: no campaign cards to process")
		return
	}

	// Deduplicate by cardName|setName|cardNumber
	seen := make(map[string]bool, len(cards))
	deduped := cards[:0]
	for _, c := range cards {
		k := cardKey(c.CardName, c.SetName, c.CardNumber)
		if seen[k] {
			continue
		}
		seen[k] = true
		deduped = append(deduped, c)
	}
	cards = deduped

	// Load existing JustTCG mappings.
	mappings, err := s.mappingLister.ListByProvider(ctx, "justtcg")
	if err != nil {
		s.logger.Warn(ctx, "justtcg refresh: failed to list mappings", observability.Err(err))
		return
	}

	// Build lookup: cardKey → externalID
	mapped := make(map[string]string, len(mappings))
	for _, m := range mappings {
		mapped[cardKey(m.CardName, m.SetName, m.CollectorNumber)] = m.ExternalID
	}

	// Split cards into unmapped vs mapped.
	type mappedCard struct {
		card       UnsoldCard
		externalID string
	}
	var unmapped []UnsoldCard
	var alreadyMapped []mappedCard

	for _, c := range cards {
		k := cardKey(c.CardName, c.SetName, c.CardNumber)
		if id, ok := mapped[k]; ok {
			alreadyMapped = append(alreadyMapped, mappedCard{card: c, externalID: id})
		} else {
			unmapped = append(unmapped, c)
		}
	}

	s.logger.Info(ctx, "justtcg refresh starting",
		observability.Int("total_cards", len(cards)),
		observability.Int("unmapped", len(unmapped)),
		observability.Int("mapped", len(alreadyMapped)))

	budget := s.config.DailyBudget
	stored := 0
	errors := 0

	// Phase 1: resolve unmapped cards via SearchCards.
	for _, c := range unmapped {
		if !s.checkBudget(ctx, &budget) {
			break
		}

		results, err := s.client.SearchCards(ctx, c.CardName, "")
		budget--
		if err != nil {
			s.logger.Debug(ctx, "justtcg refresh: search failed",
				observability.String("card", c.CardName),
				observability.Err(err))
			errors++
			if !s.rateLimitSleep(ctx) {
				return
			}
			continue
		}

		// Match by card number.
		var matched *justtcg.Card
		for i := range results {
			if results[i].Number == c.CardNumber {
				matched = &results[i]
				break
			}
		}

		if matched == nil {
			s.logger.Debug(ctx, "justtcg refresh: no match by card number",
				observability.String("card", c.CardName),
				observability.String("number", c.CardNumber))
			if !s.rateLimitSleep(ctx) {
				return
			}
			continue
		}

		// Save mapping for future runs.
		if s.mappingSaver != nil {
			if err := s.mappingSaver.SaveExternalID(ctx, c.CardName, c.SetName, c.CardNumber, "justtcg", matched.CardID); err != nil {
				s.logger.Warn(ctx, "justtcg refresh: failed to save mapping",
					observability.String("card", c.CardName),
					observability.Err(err))
			}
		}

		// Store price immediately.
		nmPrice := matched.BestNMPrice()
		if nmPrice > 0 {
			if err := s.storeNMPrice(ctx, c.CardName, c.SetName, c.CardNumber, nmPrice); err != nil {
				errors++
			} else {
				stored++
			}
		}

		if !s.rateLimitSleep(ctx) {
			return
		}
	}

	// Phase 2: batch fetch already-mapped cards (groups of 100).
	const batchSize = 100
	for i := 0; i < len(alreadyMapped); i += batchSize {
		end := i + batchSize
		if end > len(alreadyMapped) {
			end = len(alreadyMapped)
		}
		batch := alreadyMapped[i:end]

		if !s.checkBudget(ctx, &budget) {
			break
		}

		ids := make([]string, len(batch))
		for j, mc := range batch {
			ids[j] = mc.externalID
		}

		results, err := s.client.BatchLookup(ctx, ids)
		budget--
		if err != nil {
			s.logger.Warn(ctx, "justtcg refresh: batch lookup failed", observability.Err(err))
			errors++
			if !s.rateLimitSleep(ctx) {
				return
			}
			continue
		}

		// Build result map by cardId for fast lookup.
		byID := make(map[string]justtcg.Card, len(results))
		for _, r := range results {
			byID[r.CardID] = r
		}

		for _, mc := range batch {
			r, ok := byID[mc.externalID]
			if !ok {
				continue
			}
			nmPrice := r.BestNMPrice()
			if nmPrice <= 0 {
				continue
			}
			if err := s.storeNMPrice(ctx, mc.card.CardName, mc.card.SetName, mc.card.CardNumber, nmPrice); err != nil {
				errors++
			} else {
				stored++
			}
		}

		if !s.rateLimitSleep(ctx) {
			return
		}
	}

	s.logger.Info(ctx, "justtcg refresh completed",
		observability.Int("prices_stored", stored),
		observability.Int("errors", errors),
		observability.Duration("duration", time.Since(start)))
}

// storeNMPrice writes a JustTCG NM price entry to the price repository.
func (s *JustTCGRefreshScheduler) storeNMPrice(ctx context.Context, cardName, setName, cardNumber string, nmPrice float64) error {
	now := time.Now()
	entry := &pricing.PriceEntry{
		CardName:          cardName,
		SetName:           setName,
		CardNumber:        cardNumber,
		Grade:             pricing.GradeRawNM.DisplayLabel(),
		PriceCents:        mathutil.ToCents(nmPrice),
		Confidence:        0.90,
		Source:            "justtcg",
		FusionSourceCount: 1,
		FusionMethod:      "justtcg-refresh",
		PriceDate:         now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.priceRepo.StorePrice(ctx, entry); err != nil {
		s.logger.Debug(ctx, "justtcg refresh: failed to store price",
			observability.String("card", cardName),
			observability.Err(err))
		return err
	}
	return nil
}

// checkBudget returns true if there is remaining API budget.
func (s *JustTCGRefreshScheduler) checkBudget(ctx context.Context, budget *int) bool {
	if *budget <= 0 {
		s.logger.Info(ctx, "justtcg refresh: daily budget exhausted")
		return false
	}
	if s.client.DailyCalls() >= int64(s.config.DailyBudget) {
		s.logger.Info(ctx, "justtcg refresh: client daily call limit reached",
			observability.Int("daily_calls", int(s.client.DailyCalls())))
		return false
	}
	return true
}

// rateLimitSleep pauses for the configured rate interval.
// Returns false if ctx is cancelled or stop is requested.
func (s *JustTCGRefreshScheduler) rateLimitSleep(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-s.Done():
		return false
	case <-time.After(s.config.RateInterval):
		return true
	}
}
