package scheduler

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

const (
	// defaultMaxCardsPerBatch is the default maximum number of cards to process
	// in a single CardHedger batch run.
	defaultMaxCardsPerBatch = 200

	// cardHedgerRateInterval is the pause between CardHedger API calls
	// to avoid hitting rate limits. The CardHedger client uses a token-bucket
	// limiter (100 req/min, burst 5); this application-level pause adds safety.
	cardHedgerRateInterval = 700 * time.Millisecond
)

// CertSweeper resolves unmapped purchase certs to CardHedger card_ids.
// Called periodically by the batch scheduler to fill gaps in card_id_mappings.
type CertSweeper interface {
	SweepUnmappedCerts(ctx context.Context) (resolved int, err error)
}

// CardDiscoverer discovers and prices cards via CardHedger on demand.
// Used after imports to immediately populate CardHedger data for new cards.
type CardDiscoverer interface {
	DiscoverAndPrice(ctx context.Context, cards []campaigns.CardIdentity) (discovered, priced int)
}

// CardIDMapping represents a cached external ID mapping for a card.
type CardIDMapping struct {
	CardName        string
	SetName         string
	CollectorNumber string
	ExternalID      string
}

// CardIDMappingLister lists all mapped cards for a given provider.
type CardIDMappingLister interface {
	ListByProvider(ctx context.Context, provider string) ([]CardIDMapping, error)
}

// FavoriteCard represents a card identity from the favorites table.
type FavoriteCard struct {
	CardName   string
	SetName    string
	CardNumber string
}

// FavoritesLister lists all distinct favorited cards across all users.
type FavoritesLister interface {
	ListAllDistinctCards(ctx context.Context) ([]FavoriteCard, error)
}

// UnsoldCard represents a card identity from unsold purchases.
type UnsoldCard struct {
	CardName   string
	SetName    string
	CardNumber string
}

// CampaignCardLister lists cards from unsold purchases in active campaigns.
type CampaignCardLister interface {
	ListUnsoldCards(ctx context.Context) ([]UnsoldCard, error)
}

// CardHedgerBatchClient defines the CardHedger API surface used by the batch scheduler.
type CardHedgerBatchClient interface {
	Available() bool
	DailyCallsUsed() int
	GetAllPrices(ctx context.Context, cardID string) (*cardhedger.AllPricesByCardResponse, int, http.Header, error)
	CardMatch(ctx context.Context, query, category string, maxCandidates int) (*cardhedger.CardMatchResponse, int, http.Header, error)
}

// CardIDMappingSaver persists card ID mappings discovered during batch discovery.
type CardIDMappingSaver interface {
	SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
}

// CardHedgerBatchConfig controls the daily batch scheduler.
type CardHedgerBatchConfig struct {
	Enabled        bool
	RunInterval    time.Duration // How often to run (default: 24h)
	MaxCardsPerRun int           // Max cards to process per run (default: 200)
	Category       string        // Card category for discovery searches (default: "Pokemon")
}

// CardHedgerBatchScheduler runs a daily batch that refreshes CardHedger prices
// for tracked cards (favorites, campaign purchases, previously mapped cards).
// Also discovers and maps unmapped priority cards via CardMatch.
type CardHedgerBatchScheduler struct {
	client         CardHedgerBatchClient
	priceRepo      pricing.PriceRepository
	apiTracker     pricing.APITracker
	mappingLister  CardIDMappingLister
	mappingSaver   CardIDMappingSaver
	certSweeper    CertSweeper
	failureTracker pricing.DiscoveryFailureTracker
	favLister      FavoritesLister
	campLister     CampaignCardLister
	logger         observability.Logger
	config         CardHedgerBatchConfig
	stopChan       chan struct{}
	stopOnce       sync.Once
	wg             sync.WaitGroup
}

// BatchOption configures optional dependencies on CardHedgerBatchScheduler.
type BatchOption func(*CardHedgerBatchScheduler)

// WithBatchAPITracker injects an APITracker for recording API calls.
func WithBatchAPITracker(t pricing.APITracker) BatchOption {
	return func(s *CardHedgerBatchScheduler) { s.apiTracker = t }
}

// WithBatchMappingSaver injects a CardIDMappingSaver for persisting discovered card mappings.
func WithBatchMappingSaver(saver CardIDMappingSaver) BatchOption {
	return func(s *CardHedgerBatchScheduler) { s.mappingSaver = saver }
}

// WithCertSweeper injects a CertSweeper for periodic cert resolution.
func WithCertSweeper(sweeper CertSweeper) BatchOption {
	return func(s *CardHedgerBatchScheduler) { s.certSweeper = sweeper }
}

// WithDiscoveryFailureTracker injects a tracker for persisting discovery failures.
func WithDiscoveryFailureTracker(t pricing.DiscoveryFailureTracker) BatchOption {
	return func(s *CardHedgerBatchScheduler) { s.failureTracker = t }
}

// NewCardHedgerBatchScheduler creates a new daily batch scheduler.
func NewCardHedgerBatchScheduler(
	client CardHedgerBatchClient,
	priceRepo pricing.PriceRepository,
	mappingLister CardIDMappingLister,
	favLister FavoritesLister,
	campLister CampaignCardLister,
	logger observability.Logger,
	config CardHedgerBatchConfig,
	opts ...BatchOption,
) *CardHedgerBatchScheduler {
	s := &CardHedgerBatchScheduler{
		client:        client,
		priceRepo:     priceRepo,
		mappingLister: mappingLister,
		favLister:     favLister,
		campLister:    campLister,
		logger:        logger.With(context.Background(), observability.String("component", "cardhedger-batch")),
		config:        config,
		stopChan:      make(chan struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start begins the daily batch loop.
func (s *CardHedgerBatchScheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	defer s.wg.Done()

	if !s.config.Enabled {
		s.logger.Info(ctx, "cardhedger batch scheduler disabled")
		return
	}

	if s.client == nil || !s.client.Available() {
		s.logger.Info(ctx, "cardhedger batch scheduler skipped: client not configured")
		return
	}

	interval := s.config.RunInterval
	if interval <= 0 {
		interval = 24 * time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.logger.Info(ctx, "cardhedger batch scheduler started",
		observability.Duration("interval", interval),
		observability.Int("max_cards", s.config.MaxCardsPerRun))

	// Run batch early on startup to populate card_id_mappings before the
	// delta poll scheduler needs them (delta poll starts at T=3m).
	select {
	case <-ctx.Done():
		return
	case <-s.stopChan:
		return
	case <-time.After(30 * time.Second):
		s.runBatch(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info(ctx, "cardhedger batch scheduler stopped (context cancelled)")
			return
		case <-s.stopChan:
			s.logger.Info(ctx, "cardhedger batch scheduler stopped (signal)")
			return
		case <-ticker.C:
			s.runBatch(ctx)
		}
	}
}

// Stop gracefully stops the scheduler.
func (s *CardHedgerBatchScheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopChan)
	})
}

// Wait blocks until the scheduler's Start goroutine has returned.
func (s *CardHedgerBatchScheduler) Wait() {
	s.wg.Wait()
}

// runBatch collects target cards, resolves CardHedger IDs, and fetches prices.
func (s *CardHedgerBatchScheduler) runBatch(ctx context.Context) {
	start := time.Now()

	// Determine batch size from configuration
	budget := defaultMaxCardsPerBatch
	if s.config.MaxCardsPerRun > 0 {
		budget = s.config.MaxCardsPerRun
	}

	// Collect target cards: mapped CardHedger IDs
	mappings, err := s.mappingLister.ListByProvider(ctx, pricing.SourceCardHedger)
	if err != nil {
		s.logger.Warn(ctx, "failed to list card ID mappings", observability.Err(err))
		return
	}

	// Sweep unmapped purchase certs via DetailsByCerts (authoritative resolution).
	// Runs before discovery so newly resolved certs don't waste CardMatch budget.
	if s.certSweeper != nil {
		resolved, sweepErr := s.certSweeper.SweepUnmappedCerts(ctx)
		if sweepErr != nil {
			s.logger.Warn(ctx, "cert sweep failed", observability.Err(sweepErr))
		} else if resolved > 0 {
			s.logger.Info(ctx, "cert sweep resolved new mappings",
				observability.Int("resolved", resolved))
			// Reload mappings to include newly resolved certs
			newMappings, reloadErr := s.mappingLister.ListByProvider(ctx, pricing.SourceCardHedger)
			if reloadErr != nil {
				s.logger.Warn(ctx, "failed to reload mappings after cert sweep", observability.Err(reloadErr))
			} else {
				mappings = newMappings
			}
		}
	}

	// Build priority set: favorites and campaign cards first
	priorityCards, priorityCardNumbers := s.collectPriorityCards(ctx)

	// Discover unmapped priority cards via CardMatch and add them to mappings.
	// Each card-match costs 1 API call, so we cap discovery and deduct from budget.
	discovered := s.discoverUnmappedCards(ctx, mappings, priorityCards, priorityCardNumbers, &budget)

	// Sort: priority cards first, then remaining mapped cards
	type batchItem struct {
		cardName        string
		setName         string
		collectorNumber string
		externalID      string
		priority        bool
	}

	seen := make(map[string]bool)
	var items []batchItem

	// Newly discovered cards are highest priority (never had data before)
	for _, d := range discovered {
		key := cardKey(d.CardName, d.SetName, d.CollectorNumber)
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, batchItem{
			cardName:        d.CardName,
			setName:         d.SetName,
			collectorNumber: d.CollectorNumber,
			externalID:      d.ExternalID,
			priority:        true,
		})
	}

	// First pass: mapped cards that are in the priority set
	for _, m := range mappings {
		key := cardKey(m.CardName, m.SetName, m.CollectorNumber)
		if seen[key] {
			continue
		}
		bk := cardBaseKey(m.CardName, m.SetName)
		numKey := cardKey(m.CardName, m.SetName, m.CollectorNumber)
		if priorityCards[bk] || priorityCards[numKey] {
			seen[key] = true
			items = append(items, batchItem{
				cardName:        m.CardName,
				setName:         m.SetName,
				collectorNumber: m.CollectorNumber,
				externalID:      m.ExternalID,
				priority:        true,
			})
		}
	}

	// Second pass: remaining mapped cards
	for _, m := range mappings {
		key := cardKey(m.CardName, m.SetName, m.CollectorNumber)
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, batchItem{
			cardName:        m.CardName,
			setName:         m.SetName,
			collectorNumber: m.CollectorNumber,
			externalID:      m.ExternalID,
		})
	}

	if len(items) == 0 {
		s.logger.Debug(ctx, "cardhedger batch: no cards to process")
		return
	}

	// Limit to budget
	if len(items) > budget {
		items = items[:budget]
	}

	s.logger.Info(ctx, "cardhedger batch starting",
		observability.Int("total_mapped", len(mappings)),
		observability.Int("newly_discovered", len(discovered)),
		observability.Int("priority_cards", len(priorityCards)),
		observability.Int("processing", len(items)),
		observability.Int("budget", budget))

	stored := 0
	errors := 0

	for i, item := range items {
		select {
		case <-ctx.Done():
			s.logger.Info(ctx, "cardhedger batch cancelled")
			return
		case <-s.stopChan:
			return
		default:
		}

		// Rate limit: pause briefly between calls
		if i > 0 && !s.rateLimitedSleep(ctx) {
			return
		}

		fetchStart := time.Now()
		resp, statusCode, _, err := s.client.GetAllPrices(ctx, item.externalID)
		latency := time.Since(fetchStart)

		s.recordAPICall("batch/all-prices", statusCode, err, latency.Milliseconds())

		if err != nil {
			s.logger.Warn(ctx, "cardhedger batch: fetch failed",
				observability.String("card", item.cardName),
				observability.Err(err))
			errors++
			continue
		}

		count := s.storePrices(ctx, item.cardName, item.setName, item.collectorNumber, resp)
		stored += count
	}

	duration := time.Since(start)
	s.logger.Info(ctx, "cardhedger batch completed",
		observability.Int("processed", len(items)),
		observability.Int("prices_stored", stored),
		observability.Int("errors", errors),
		observability.Duration("duration", duration))
}

// collectPriorityCards returns a set of "cardName|setName" keys for cards that
// should be prioritized in the batch (favorites + active campaign purchases).
// Also returns a map of card numbers keyed by "cardName|setName|cardNumber"
// (when cardNumber is non-empty) or "cardName|setName" for discovery.
func (s *CardHedgerBatchScheduler) collectPriorityCards(ctx context.Context) (map[string]bool, map[string]string) {
	priority := make(map[string]bool)
	cardNumbers := make(map[string]string)

	// Favorites across all users
	if s.favLister != nil {
		favCards, err := s.favLister.ListAllDistinctCards(ctx)
		if err != nil {
			s.logger.Warn(ctx, "failed to list favorites for batch", observability.Err(err))
		} else {
			for _, fc := range favCards {
				bk := cardBaseKey(fc.CardName, fc.SetName)
				if fc.CardNumber != "" {
					numKey := cardKey(fc.CardName, fc.SetName, fc.CardNumber)
					priority[numKey] = true
					cardNumbers[numKey] = fc.CardNumber
				} else {
					priority[bk] = true
				}
			}
		}
	}

	// Campaign purchase cards
	if s.campLister != nil {
		cards, err := s.campLister.ListUnsoldCards(ctx)
		if err != nil {
			s.logger.Warn(ctx, "failed to list campaign cards for batch", observability.Err(err))
		} else {
			for _, c := range cards {
				bk := cardBaseKey(c.CardName, c.SetName)
				if c.CardNumber != "" {
					numKey := cardKey(c.CardName, c.SetName, c.CardNumber)
					priority[numKey] = true
					cardNumbers[numKey] = c.CardNumber
				} else {
					priority[bk] = true
				}
			}
		}
	}

	return priority, cardNumbers
}
