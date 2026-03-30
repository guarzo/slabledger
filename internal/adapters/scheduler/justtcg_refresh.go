package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/justtcg"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
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

// JustTCGClient defines the JustTCG API surface used by the refresh scheduler.
type JustTCGClient interface {
	Available() bool
	SearchCards(ctx context.Context, query, set string) ([]justtcg.Card, error)
	BatchLookup(ctx context.Context, cardIDs []string) ([]justtcg.Card, error)
}

// JustTCGRefreshScheduler refreshes JustTCG NM prices for unsold campaign cards.
// Phase 1: resolve unmapped cards via SearchCards + save mapping.
// Phase 2: batch fetch already-mapped cards via BatchLookup.
type JustTCGRefreshScheduler struct {
	StopHandle
	client        JustTCGClient
	priceRepo     pricing.PriceRepository
	apiTracker    pricing.APITracker
	mappingLister CardIDMappingLister
	mappingSaver  CardIDMappingSaver
	campLister    CampaignCardLister
	logger        observability.Logger
	config        JustTCGRefreshConfig

	dayCallsMu   sync.Mutex
	dayCallsDate string // "2006-01-02" UTC date
	dayCallsUsed int    // calls made on dayCallsDate
}

// JustTCGRefreshOption configures optional dependencies on JustTCGRefreshScheduler.
type JustTCGRefreshOption func(*JustTCGRefreshScheduler)

// WithJustTCGAPITracker injects an APITracker for recording API calls.
func WithJustTCGAPITracker(t pricing.APITracker) JustTCGRefreshOption {
	return func(s *JustTCGRefreshScheduler) { s.apiTracker = t }
}

// NewJustTCGRefreshScheduler creates a new JustTCG NM price refresh scheduler.
func NewJustTCGRefreshScheduler(
	client JustTCGClient,
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
	mappings, err := s.mappingLister.ListByProvider(ctx, pricing.SourceJustTCG)
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

	stored := 0
	errCount := 0

	// Phase 1: resolve unmapped cards via SearchCards.
	for _, c := range unmapped {
		if !s.claimBudget(ctx) {
			break
		}

		callStart := time.Now()
		results, err := s.client.SearchCards(ctx, c.CardName, c.SetName)
		s.recordAPICall("search", 0, err, time.Since(callStart).Milliseconds())
		if err != nil {
			s.logger.Debug(ctx, "justtcg refresh: search failed",
				observability.String("card", c.CardName),
				observability.Err(err))
			errCount++
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
			if err := s.mappingSaver.SaveExternalID(ctx, c.CardName, c.SetName, c.CardNumber, pricing.SourceJustTCG, matched.CardID); err != nil {
				s.logger.Warn(ctx, "justtcg refresh: failed to save mapping",
					observability.String("card", c.CardName),
					observability.Err(err))
			}
		}

		// Store price immediately.
		nmPrice := matched.BestNMPrice()
		if nmPrice > 0 {
			if err := s.storeNMPrice(ctx, c.CardName, c.SetName, c.CardNumber, nmPrice); err != nil {
				errCount++
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
		end := min(i+batchSize, len(alreadyMapped))
		batch := alreadyMapped[i:end]

		if !s.claimBudget(ctx) {
			break
		}

		ids := make([]string, len(batch))
		for j, mc := range batch {
			ids[j] = mc.externalID
		}

		callStart := time.Now()
		results, err := s.client.BatchLookup(ctx, ids)
		s.recordAPICall("batch-lookup", 0, err, time.Since(callStart).Milliseconds())
		if err != nil {
			s.logger.Warn(ctx, "justtcg refresh: batch lookup failed", observability.Err(err))
			errCount++
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
				errCount++
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
		observability.Int("errors", errCount),
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
		Confidence:        0.85,
		Source:            pricing.SourceJustTCG,
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

// claimBudget atomically checks and increments the day-scoped API call counter.
// Returns true if a call is allowed, false if the daily budget is exhausted.
// The counter resets automatically when the UTC date changes.
func (s *JustTCGRefreshScheduler) claimBudget(ctx context.Context) bool {
	s.dayCallsMu.Lock()
	defer s.dayCallsMu.Unlock()

	today := time.Now().UTC().Format("2006-01-02")
	if s.dayCallsDate != today {
		s.dayCallsUsed = 0
		s.dayCallsDate = today
	}
	if s.dayCallsUsed >= s.config.DailyBudget {
		s.logger.Info(ctx, "justtcg refresh: daily budget exhausted",
			observability.Int("used", s.dayCallsUsed),
			observability.Int("budget", s.config.DailyBudget))
		return false
	}
	s.dayCallsUsed++
	return true
}

// recordAPICall asynchronously records an API call for dashboard visibility.
func (s *JustTCGRefreshScheduler) recordAPICall(endpoint string, statusCode int, err error, latencyMS int64) {
	if s.apiTracker == nil {
		return
	}
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	sc := statusCode
	if sc == 0 && err != nil {
		sc = statusFromError(err)
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
			Provider:   pricing.SourceJustTCG,
			Endpoint:   endpoint,
			StatusCode: sc,
			Error:      errMsg,
			LatencyMS:  latencyMS,
			Timestamp:  ts,
		})
	}()
}

// statusFromError extracts an HTTP status code from an error.
// It checks for *apperrors.AppError and maps the error code to an HTTP status.
func statusFromError(err error) int {
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		return 500
	}

	// First, check if the AppError has an explicit HTTP status.
	if s := appErr.HTTPStatus(); s != 0 {
		return s
	}

	// Fall back to mapping error codes to HTTP statuses.
	switch appErr.Code {
	case apperrors.ErrCodeProviderRateLimit:
		return 429
	case apperrors.ErrCodeProviderTimeout:
		return 408
	case apperrors.ErrCodeProviderUnavailable, apperrors.ErrCodeProviderCircuitOpen:
		return 503
	case apperrors.ErrCodeProviderNotFound:
		return 404
	case apperrors.ErrCodeProviderAuth:
		return 401
	case apperrors.ErrCodeProviderInvalidReq, apperrors.ErrCodeValidation:
		return 400
	case apperrors.ErrCodeProviderInvalidResp:
		return 502
	default:
		return 500
	}
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
