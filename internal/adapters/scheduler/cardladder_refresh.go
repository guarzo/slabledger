package scheduler

import (
	"context"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// CardLadderPurchaseLister lists unsold purchases with their image URLs.
type CardLadderPurchaseLister interface {
	ListAllUnsoldPurchases(ctx context.Context) ([]campaigns.Purchase, error)
}

// CardLadderValueUpdater updates CL values on purchases.
type CardLadderValueUpdater interface {
	UpdatePurchaseCLValue(ctx context.Context, purchaseID string, clValueCents, population int) error
}

// CardLadderGemRateUpdater persists gemRateID, psaSpecID, and CL card metadata on purchases.
type CardLadderGemRateUpdater interface {
	UpdatePurchaseGemRateID(ctx context.Context, purchaseID, gemRateID string) error
	UpdatePurchasePSASpecID(ctx context.Context, purchaseID string, psaSpecID int) error
	UpdatePurchaseCLCardMetadata(ctx context.Context, id, player, variation, category string) error
}

// CardLadderRefreshOption configures optional dependencies on a CardLadderRefreshScheduler.
type CardLadderRefreshOption func(*CardLadderRefreshScheduler)

// WithCLDHPushUpdater enables DH re-push when CL values change.
func WithCLDHPushUpdater(u DHPushStatusUpdater) CardLadderRefreshOption {
	return func(s *CardLadderRefreshScheduler) { s.dhPushUpdater = u }
}

// CardLadderSyncUpdater sets cl_synced_at on purchases after CL push/sync.
type CardLadderSyncUpdater interface {
	UpdatePurchaseCLSyncedAt(ctx context.Context, purchaseID string, syncedAt string) error
}

// WithCLSyncUpdater enables cl_synced_at tracking on push.
func WithCLSyncUpdater(u CardLadderSyncUpdater) CardLadderRefreshOption {
	return func(s *CardLadderRefreshScheduler) { s.syncUpdater = u }
}

// CLRunStats holds the counters from the most recent Card Ladder refresh run.
type CLRunStats struct {
	LastRunAt    time.Time `json:"lastRunAt"`
	DurationMs   int64     `json:"durationMs"`
	Updated      int       `json:"updated"`
	Mapped       int       `json:"mapped"`
	Skipped      int       `json:"skipped"`
	TotalCLCards int       `json:"totalCLCards"`
	CardsPushed  int       `json:"cardsPushed"`
	CardsRemoved int       `json:"cardsRemoved"`
}

// CardLadderRefreshScheduler refreshes CL values from the Card Ladder API daily.
type CardLadderRefreshScheduler struct {
	StopHandle
	statsMu        sync.RWMutex
	clientMu       sync.RWMutex
	client         *cardladder.Client
	store          *sqlite.CardLadderStore
	purchaseLister CardLadderPurchaseLister
	valueUpdater   CardLadderValueUpdater
	gemRateUpdater CardLadderGemRateUpdater
	syncUpdater    CardLadderSyncUpdater // optional: sets cl_synced_at on push
	clRecorder     campaigns.CLValueHistoryRecorder
	salesStore     *sqlite.CLSalesStore
	dhPushUpdater  DHPushStatusUpdater // optional: re-enrolls changed items for DH push
	logger         observability.Logger
	config         config.CardLadderConfig
	lastRunStats   *CLRunStats
}

// GetLastRunStats returns a copy of the stats from the most recent refresh run,
// or nil if no run has completed yet.
func (s *CardLadderRefreshScheduler) GetLastRunStats() *CLRunStats {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()
	if s.lastRunStats == nil {
		return nil
	}
	cp := *s.lastRunStats
	return &cp
}

// SetClient replaces the API client used by the scheduler. This is called when
// credentials are saved for the first time after startup (no client at boot).
func (s *CardLadderRefreshScheduler) SetClient(client *cardladder.Client) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	s.client = client
}

// getClient returns the current API client under a read lock.
func (s *CardLadderRefreshScheduler) getClient() *cardladder.Client {
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	return s.client
}

// NewCardLadderRefreshScheduler creates a new CL refresh scheduler.
func NewCardLadderRefreshScheduler(
	client *cardladder.Client,
	store *sqlite.CardLadderStore,
	purchaseLister CardLadderPurchaseLister,
	valueUpdater CardLadderValueUpdater,
	gemRateUpdater CardLadderGemRateUpdater,
	clRecorder campaigns.CLValueHistoryRecorder,
	salesStore *sqlite.CLSalesStore,
	logger observability.Logger,
	cfg config.CardLadderConfig,
	opts ...CardLadderRefreshOption,
) *CardLadderRefreshScheduler {
	cfg.ApplyDefaults()
	s := &CardLadderRefreshScheduler{
		StopHandle:     NewStopHandle(),
		client:         client,
		store:          store,
		purchaseLister: purchaseLister,
		valueUpdater:   valueUpdater,
		gemRateUpdater: gemRateUpdater,
		clRecorder:     clRecorder,
		salesStore:     salesStore,
		logger:         logger,
		config:         cfg,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Start begins the scheduler loop.
func (s *CardLadderRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "Card Ladder refresh scheduler disabled")
		return
	}

	// Check for disabled scheduler: RefreshHour < 0 means do not run
	if s.config.RefreshHour < 0 {
		s.logger.Info(ctx, "Card Ladder refresh scheduler disabled (RefreshHour < 0)")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:         "card-ladder-refresh",
		Interval:     s.config.Interval,
		InitialDelay: timeUntilHour(time.Now(), s.config.RefreshHour),
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
		LogFields:    []observability.Field{observability.Int("refreshHour", s.config.RefreshHour)},
	}, func(ctx context.Context) {
		s.runOnce(ctx) //nolint:errcheck
	})
}

// RunOnce runs a single refresh cycle. Exported for manual trigger.
func (s *CardLadderRefreshScheduler) RunOnce(ctx context.Context) error {
	return s.runOnce(ctx)
}

var certFromImageRe = regexp.MustCompile(`/cert/(\d+)/`)
var gradeDigitsRe = regexp.MustCompile(`(\d+(?:\.\d+)?)`)

func (s *CardLadderRefreshScheduler) runOnce(ctx context.Context) error {
	start := time.Now()

	client := s.getClient()
	if client == nil {
		s.logger.Debug(ctx, "CL refresh: no client configured, skipping")
		return nil
	}

	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		s.logger.Error(ctx, "CL refresh: failed to load config", observability.Err(err))
		return err
	}
	if cfg == nil {
		s.logger.Debug(ctx, "CL refresh: not configured, skipping")
		return nil
	}

	// Fetch all collection cards
	cards, err := client.FetchAllCollection(ctx, cfg.CollectionID)
	if err != nil {
		s.logger.Error(ctx, "CL refresh: failed to fetch collection", observability.Err(err))
		return err
	}
	s.logger.Info(ctx, "CL refresh: fetched collection",
		observability.Int("cardCount", len(cards)))

	// Fetch gemRateId data from Firestore
	var firestoreData map[string]cardladder.FirestoreCardData
	if cfg.FirebaseUID != "" {
		firestoreData, err = client.FetchFirestoreCards(ctx, cfg.FirebaseUID, cfg.CollectionID)
		if err != nil {
			s.logger.Warn(ctx, "CL refresh: failed to fetch Firestore card data", observability.Err(err))
			// Continue without gemRateId data — values still sync, just no sales comps
		} else {
			s.logger.Info(ctx, "CL refresh: fetched Firestore data",
				observability.Int("cardsWithGemRate", len(firestoreData)))
		}
	}

	// Load all unsold purchases for image URL matching
	purchases, err := s.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		s.logger.Error(ctx, "CL refresh: failed to list purchases", observability.Err(err))
		return err
	}

	// Build image URL → purchase map for matching
	imageToPurchase := make(map[string]*campaigns.Purchase, len(purchases))
	certToPurchase := make(map[string]*campaigns.Purchase, len(purchases))
	for i := range purchases {
		p := &purchases[i]
		if p.FrontImageURL != "" {
			imageToPurchase[p.FrontImageURL] = p
		}
		if p.CertNumber != "" {
			certToPurchase[p.CertNumber] = p
		}
	}

	// Load existing mappings
	existingMappings, err := s.store.ListMappings(ctx)
	if err != nil {
		s.logger.Warn(ctx, "CL refresh: failed to list mappings", observability.Err(err))
	}
	mappingByCLCardID := make(map[string]*sqlite.CLCardMapping, len(existingMappings))
	for i := range existingMappings {
		mappingByCLCardID[existingMappings[i].CLCollectionCardID] = &existingMappings[i]
	}

	updated, mapped, skipped := 0, 0, 0
	today := time.Now().UTC().Format("2006-01-02")

	for _, card := range cards {
		// Try to find the matching purchase
		var purchase *campaigns.Purchase

		// First check if we have a cached mapping
		if m, ok := mappingByCLCardID[card.CollectionCardID]; ok {
			purchase = certToPurchase[m.SlabSerial]
		}

		// Primary match: image URL
		if purchase == nil && card.Image != "" {
			purchase = imageToPurchase[card.Image]
		}

		// Fallback: extract cert from image URL path
		if purchase == nil && card.Image != "" {
			if matches := certFromImageRe.FindStringSubmatch(card.Image); len(matches) > 1 {
				purchase = certToPurchase[matches[1]]
			}
		}

		if purchase == nil {
			skipped++
			continue
		}

		// Save/update mapping
		// Use Firestore gemRate data if available, otherwise preserve existing mapping values
		condition := card.Condition
		gemRateID := ""
		if fd, ok := firestoreData[card.CollectionCardID]; ok {
			gemRateID = fd.GemRateID
			if fd.GemRateCondition != "" {
				condition = fd.GemRateCondition
			}
		} else if existing, ok := mappingByCLCardID[card.CollectionCardID]; ok {
			// No Firestore entry — preserve previously stored gemRateID
			gemRateID = existing.CLGemRateID
		}

		if err := s.store.SaveMapping(ctx, purchase.CertNumber, card.CollectionCardID, gemRateID, condition); err != nil {
			s.logger.Warn(ctx, "CL refresh: failed to save mapping",
				observability.String("cert", purchase.CertNumber),
				observability.Err(err))
		} else {
			mapped++
		}

		// Persist gemRateID on purchase if resolved
		if gemRateID != "" && s.gemRateUpdater != nil && purchase.GemRateID == "" {
			if err := s.gemRateUpdater.UpdatePurchaseGemRateID(ctx, purchase.ID, gemRateID); err != nil {
				s.logger.Warn(ctx, "CL refresh: failed to persist gemRateID",
					observability.String("cert", purchase.CertNumber),
					observability.Err(err))
			} else {
				purchase.GemRateID = gemRateID // keep in-memory slice in sync for gap-fill phase
			}
		}

		// Update CL value
		newCLCents := mathutil.ToCentsInt(card.CurrentValue)
		if newCLCents <= 0 {
			continue
		}

		oldCLCents := purchase.CLValueCents
		if err := s.valueUpdater.UpdatePurchaseCLValue(ctx, purchase.ID, newCLCents, purchase.Population); err != nil {
			s.logger.Warn(ctx, "CL refresh: failed to update CL value",
				observability.String("cert", purchase.CertNumber),
				observability.Err(err))
			continue
		}

		// Re-enroll already-pushed items for DH re-push when market value changes.
		if s.dhPushUpdater != nil && purchase.DHInventoryID != 0 && newCLCents != oldCLCents {
			if err := s.dhPushUpdater.UpdatePurchaseDHPushStatus(ctx, purchase.ID, campaigns.DHPushStatusPending); err != nil {
				s.logger.Warn(ctx, "CL refresh: failed to re-enroll for DH push",
					observability.String("cert", purchase.CertNumber),
					observability.Err(err))
			}
		}

		// Record history
		if s.clRecorder != nil {
			gradeValue := extractGradeValue(card.Condition)
			if err := s.clRecorder.RecordCLValue(ctx, campaigns.CLValueEntry{
				CertNumber:      purchase.CertNumber,
				CardName:        purchase.CardName,
				SetName:         purchase.SetName,
				CardNumber:      purchase.CardNumber,
				GradeValue:      gradeValue,
				CLValueCents:    newCLCents,
				ObservationDate: today,
				Source:          "api_sync",
			}); err != nil {
				s.logger.Debug(ctx, "CL refresh: failed to record CL value history",
					observability.String("cert", purchase.CertNumber),
					observability.Err(err))
			}
		}
		updated++
	}

	// Phase 2: fetch sales comps for mapped cards with gemRateIDs.
	// Note: newly created mappings from this run are intentionally deferred
	// to the next refresh cycle to avoid extra API calls during initial sync.
	if s.salesStore != nil {
		s.refreshSalesComps(ctx, client, existingMappings)
	}

	// Phase 3: gap-fill gemRateIDs for purchases not matched via collection/Firestore.
	if s.gemRateUpdater != nil {
		s.gapFillGemRateIDs(ctx, client, purchases)
	}

	// Phase 4: push new unsold purchases (with certs) that aren't yet in the CL collection.
	cardsPushed := 0
	if cfg.FirebaseUID != "" {
		cardsPushed = s.pushNewCards(ctx, client, cfg.FirebaseUID, cfg.CollectionID, purchases, existingMappings)
	}

	// Phase 5: remove sold cards from the CL collection.
	cardsRemoved := 0
	if cfg.FirebaseUID != "" {
		cardsRemoved = s.removeSoldCards(ctx, client, purchases, existingMappings)
	}

	s.logger.Info(ctx, "CL refresh: complete",
		observability.Int("updated", updated),
		observability.Int("mapped", mapped),
		observability.Int("skipped", skipped),
		observability.Int("totalCLCards", len(cards)),
		observability.Int("pushed", cardsPushed),
		observability.Int("removed", cardsRemoved))

	s.statsMu.Lock()
	s.lastRunStats = &CLRunStats{
		LastRunAt:    start,
		DurationMs:   time.Since(start).Milliseconds(),
		Updated:      updated,
		Mapped:       mapped,
		Skipped:      skipped,
		TotalCLCards: len(cards),
		CardsPushed:  cardsPushed,
		CardsRemoved: cardsRemoved,
	}
	s.statsMu.Unlock()

	return nil
}

// extractGradeValue parses "PSA 9", "PSA 9.5", or "g9" → numeric grade value.
func extractGradeValue(condition string) float64 {
	if m := gradeDigitsRe.FindString(condition); m != "" {
		v, _ := strconv.ParseFloat(m, 64)
		return v
	}
	return 0
}
