package scheduler

import (
	"context"
	"net/http"
	"strconv"

	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"
	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// SyncStateStore reads and writes sync state key-value pairs.
type SyncStateStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// CardIDMappingLookup resolves CardHedger external IDs back to local card identities.
type CardIDMappingLookup interface {
	// GetLocalCard returns (cardName, setName) for a CardHedger card_id, or ("","") if not mapped.
	GetLocalCard(ctx context.Context, provider, externalID string) (cardName, setName string, err error)
}

// CardHedgerRefreshClient defines the CardHedger API surface used by the delta poll scheduler.
type CardHedgerRefreshClient interface {
	Available() bool
	DailyCallsUsed() int
	GetPriceUpdates(ctx context.Context, since string) (*cardhedger.PriceUpdatesResponse, int, http.Header, error)
}

// CardHedgerRefreshConfig controls the CardHedger delta poll scheduler.
type CardHedgerRefreshConfig struct {
	Enabled      bool
	PollInterval time.Duration // How often to poll for updates (default: 1h)
	InitialDelay time.Duration // Delay before first poll to let batch populate mappings (default: 3m)
}

// CardHedgerRefreshScheduler polls CardHedger's price-updates endpoint
// for delta price changes and stores them in the local price repository.
type CardHedgerRefreshScheduler struct {
	StopHandle
	client     CardHedgerRefreshClient
	priceRepo  pricing.PriceRepository
	apiTracker pricing.APITracker
	syncState  SyncStateStore
	idLookup   CardIDMappingLookup
	logger     observability.Logger
	config     CardHedgerRefreshConfig
}

const syncStateKeyLastPoll = "cardhedger_last_poll"

// RefreshOption configures optional dependencies on CardHedgerRefreshScheduler.
type RefreshOption func(*CardHedgerRefreshScheduler)

// WithRefreshAPITracker injects an APITracker for recording API calls.
func WithRefreshAPITracker(t pricing.APITracker) RefreshOption {
	return func(s *CardHedgerRefreshScheduler) { s.apiTracker = t }
}

// NewCardHedgerRefreshScheduler creates a new delta poll scheduler.
func NewCardHedgerRefreshScheduler(
	client CardHedgerRefreshClient,
	priceRepo pricing.PriceRepository,
	syncState SyncStateStore,
	idLookup CardIDMappingLookup,
	logger observability.Logger,
	config CardHedgerRefreshConfig,
	opts ...RefreshOption,
) *CardHedgerRefreshScheduler {
	s := &CardHedgerRefreshScheduler{
		StopHandle: NewStopHandle(),
		client:     client,
		priceRepo:  priceRepo,
		syncState:  syncState,
		idLookup:   idLookup,
		logger:     logger.With(context.Background(), observability.String("component", "cardhedger-refresh")),
		config:     config,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start begins the delta poll loop.
func (s *CardHedgerRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.WG().Add(1)
		defer s.WG().Done()
		s.logger.Info(ctx, "cardhedger refresh scheduler disabled")
		return
	}

	if s.client == nil || !s.client.Available() {
		s.WG().Add(1)
		defer s.WG().Done()
		s.logger.Info(ctx, "cardhedger refresh scheduler skipped: client not configured")
		return
	}

	interval := s.config.PollInterval
	if interval <= 0 {
		interval = 1 * time.Hour
	}

	initialDelay := s.config.InitialDelay
	if initialDelay <= 0 {
		initialDelay = 3 * time.Minute
	}

	RunLoop(ctx, LoopConfig{
		Name:         "cardhedger-refresh",
		Interval:     interval,
		InitialDelay: initialDelay,
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
	}, s.pollUpdates)
}

// pollUpdates calls GetPriceUpdates and processes results.
func (s *CardHedgerRefreshScheduler) pollUpdates(ctx context.Context) {
	start := time.Now()

	// Read last poll timestamp from sync_state
	since, err := s.syncState.Get(ctx, syncStateKeyLastPoll)
	if err != nil {
		s.logger.Warn(ctx, "failed to read sync state, defaulting to 24h ago",
			observability.Err(err))
	}
	if since == "" {
		// Default to 24 hours ago on first run
		since = time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	}

	// Call delta poll endpoint (1 API call)
	fetchStart := time.Now()
	resp, statusCode, _, err := s.client.GetPriceUpdates(ctx, since)
	latency := time.Since(fetchStart)

	// Record API call for dashboard visibility using a detached context
	// so tracking writes complete even when the poll context is cancelled.
	if s.apiTracker != nil {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		if statusCode == 0 && err != nil {
			statusCode = 500
		}
		if statusCode == 0 && err == nil {
			statusCode = 200
		}
		ctxTrack, cancelTrack := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelTrack()
		//nolint:errcheck // best-effort tracking
		s.apiTracker.RecordAPICall(ctxTrack, &pricing.APICallRecord{
			Provider:   "cardhedger",
			Endpoint:   "delta/price-updates",
			StatusCode: statusCode,
			Error:      errMsg,
			LatencyMS:  latency.Milliseconds(),
			Timestamp:  time.Now(),
		})
	}

	if err != nil {
		s.logger.Warn(ctx, "cardhedger delta poll failed",
			observability.String("since", since),
			observability.Err(err))
		return
	}

	if resp.Count == 0 {
		s.logger.Debug(ctx, "cardhedger delta poll: no updates",
			observability.String("since", since))
		// Update timestamp even with no results to avoid re-polling the same window
		s.updateSyncTimestamp(ctx, time.Now().UTC().Format(time.RFC3339))
		return
	}

	s.logger.Info(ctx, "cardhedger delta poll received updates",
		observability.Int("count", resp.Count),
		observability.String("since", since))

	// Process updates: match against known card ID mappings
	stored := 0
	skipped := 0
	latestTimestamp := since

	for _, update := range resp.Updates {
		// Look up local card identity from the CardHedger card_id
		cardName, setName, err := s.idLookup.GetLocalCard(ctx, "cardhedger", update.CardID)
		if err != nil {
			s.logger.Warn(ctx, "failed to look up card mapping",
				observability.String("card_id", update.CardID),
				observability.Err(err))
			skipped++
			continue
		}
		if cardName == "" {
			// Card not in our mapping cache - we haven't looked it up before, skip
			skipped++
			continue
		}

		// Validate grade is a recognized CardHedger grade
		if !isKnownCardHedgerGrade(update.Grade) {
			skipped++
			continue
		}

		// Parse price
		price, err := strconv.ParseFloat(update.Price, 64)
		if err != nil || price <= 0 {
			skipped++
			continue
		}

		priceCents := mathutil.ToCents(price)
		now := time.Now()

		// Normalize card name to match batch and on-demand storage paths.
		normalizedName := cardutil.NormalizePurchaseName(cardName)
		if normalizedName == "" {
			normalizedName = cardName
		}

		entry := &pricing.PriceEntry{
			CardName:          normalizedName,
			SetName:           setName,
			CardNumber:        update.CardNumber,
			Grade:             update.Grade,
			PriceCents:        priceCents,
			Confidence:        0.85,
			Source:            "cardhedger",
			FusionSourceCount: 1,
			FusionMethod:      "delta_poll",
			PriceDate:         now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		if err := s.priceRepo.StorePrice(ctx, entry); err != nil {
			s.logger.Warn(ctx, "failed to store delta poll price",
				observability.String("card", cardName),
				observability.String("grade", update.Grade),
				observability.Err(err))
			continue
		}
		stored++

		// Only advance cursor after successful persistence
		if update.UpdateTimestamp > latestTimestamp {
			latestTimestamp = update.UpdateTimestamp
		}
	}

	// Update sync state with latest timestamp
	s.updateSyncTimestamp(ctx, latestTimestamp)

	duration := time.Since(start)
	s.logger.Info(ctx, "cardhedger delta poll completed",
		observability.Int("received", resp.Count),
		observability.Int("stored", stored),
		observability.Int("skipped", skipped),
		observability.Duration("duration", duration))
}

func (s *CardHedgerRefreshScheduler) updateSyncTimestamp(ctx context.Context, timestamp string) {
	if err := s.syncState.Set(ctx, syncStateKeyLastPoll, timestamp); err != nil {
		s.logger.Warn(ctx, "failed to update sync state timestamp",
			observability.String("timestamp", timestamp),
			observability.Err(err))
	}
}

// isKnownCardHedgerGrade delegates to the centralized grade validation.
func isKnownCardHedgerGrade(grade string) bool {
	return pricing.IsCardHedgerGrade(grade)
}
